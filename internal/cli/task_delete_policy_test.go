package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/kit/go/console/output"
	"hop.top/kit/go/runtime/bus"
	"hop.top/tlc/internal/core"
)

// TestTaskDelete_PolicyDeniesWithoutNote covers the deny path of
// T-1192's `delete-requires-note` default policy: `tlc task delete <id>`
// with no `--note` must surface a PolicyDeniedError that wraps
// domain.ErrConflict and exit-maps to 4.
func TestTaskDelete_PolicyDeniesWithoutNote(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, _ := getStorageRaw()
		defer s.Close()
		_ = s.CreateTask(ctx, &core.Task{
			ID:     "T-0001",
			Title:  "to be denied",
			Status: core.StatusTodo,
		})

		setupPolicyForTest(t)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "delete", "T-0001", "--yes"})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected delete-without-note to be denied; got nil")
		}
		// The CLI handler wraps the policy veto in an *output.Error
		// envelope (CONFLICT, exit 4) so kit cli's RunE middleware
		// round-trips it unchanged; the original PolicyDeniedError
		// text is embedded in Message.
		var oe *output.Error
		if !errors.As(err, &oe) {
			t.Fatalf("err = %v; want *output.Error", err)
		}
		if oe.Code != output.CodeConflict {
			t.Errorf("Code = %q; want %q", oe.Code, output.CodeConflict)
		}
		if oe.ExitCode != 4 {
			t.Errorf("ExitCode = %d; want 4", oe.ExitCode)
		}
		if !strings.Contains(oe.Message, "delete-requires-note") {
			t.Errorf("Message %q must name the denying policy", oe.Message)
		}
		if got := exitCodeFor(err); got != 4 {
			t.Errorf("exitCodeFor = %d; want 4", got)
		}

		// The task must NOT have been removed.
		got := getTaskByAlias(t, ctx, "T-0001")
		if got == nil {
			t.Error("task removed despite policy veto; want task to survive denied delete")
		}
	})
}

// TestTaskDelete_PolicyAllowsWithNote covers the allow path: a delete
// with a non-empty `--note` must succeed and the note must survive in
// task_logs (depends on T-1232's cascade drop).
func TestTaskDelete_PolicyAllowsWithNote(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, _ := getStorageRaw()
		defer s.Close()
		_ = s.CreateTask(ctx, &core.Task{
			ID:     "T-0001",
			Title:  "policy-allowed delete",
			Status: core.StatusTodo,
		})

		setupPolicyForTest(t)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "delete", "T-0001",
			"--yes",
			"--note", "audited cleanup",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("delete with --note: %v", err)
		}

		gone := getTaskByAlias(t, ctx, "T-0001")
		if gone != nil {
			t.Errorf("task survived allowed delete: %+v", gone)
		}

		s2, _ := getStorageRaw()
		defer s2.Close()
		logs, err := s2.GetLogs(ctx, "T-0001", "asc")
		if err != nil {
			t.Fatalf("GetLogs: %v", err)
		}
		var found bool
		for _, le := range logs {
			if le.Action == core.ActionDeleted && le.Note == "audited cleanup" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("DELETED log entry with note not found post-delete (T-1232 round-trip): %+v", logs)
		}
	})
}

// setupPolicyForTest wires a fresh kit/runtime/policy engine with the
// bundled tlc default to the global eventBus, then restores prior
// state on cleanup. Mirrors what root.go's PersistentPreRunE does at
// startup, but for tests that bypass kit cli's lifecycle.
func setupPolicyForTest(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "policies.yaml")
	if err := os.WriteFile(yamlPath, []byte(`policies:
  - name: delete-requires-note
    on: kit.runtime.entity.pre_persisted
    when: 'payload.Op != "delete" || context.note != ""'
    effect: allow
    otherwise: deny
    message: "deleting a task requires --note explaining why"
`), 0o600); err != nil {
		t.Fatalf("write policy yaml: %v", err)
	}
	t.Setenv("TLC_POLICY_FILE", yamlPath)

	prevBus := eventBus
	prevPub := busPublisher
	prevEng := policyEng
	prevUnwire := policyUnwire
	prevErr := policyErr

	eventBus = bus.New()
	policyOnceReset()
	if _, err := initPolicyEngine(eventBus); err != nil {
		t.Fatalf("initPolicyEngine: %v", err)
	}

	t.Cleanup(func() {
		closePolicy()
		_ = eventBus.Close(context.Background())
		eventBus = prevBus
		busPublisher = prevPub
		policyEng = prevEng
		policyUnwire = prevUnwire
		policyErr = prevErr
		policyOnceReset()
	})
}
