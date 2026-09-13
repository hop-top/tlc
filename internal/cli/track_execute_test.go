// Tests for `tlc track execute`. Sandboxed end-to-end: HOME and storage
// both temp, agents.yaml planted via plantAgentsYAML. Exec-kind tasks let
// the suite drive the real executor without an agent binary.
package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

const execTestTrack = "track_exec_test"

func execTestTask(id string, seq int64, blockedBy ...string) *core.Task {
	track := execTestTrack
	now := time.Now().UTC()
	t := &core.Task{
		ID: id, Seq: seq, Title: "run " + id, Status: core.StatusTodo,
		TrackID:     &track,
		Kind:        core.TaskKindExec,
		Spec:        &core.TaskSpec{Exec: &core.ExecSpec{Argv: []string{"sh", "-c", "echo " + id}}},
		RunID:       "run_1",
		StepID:      id,
		StepOrdinal: int(seq),
		CreatedAt:   now, UpdatedAt: now,
	}
	t.SetBlockedBy(blockedBy)
	return t
}

func seedExecTrack(t *testing.T, ctx context.Context, s *storage.SQLiteStorage, tasks ...*core.Task) {
	t.Helper()
	track := &core.Track{ID: execTestTrack, Slug: "exec-test", Title: "exec test", Status: core.TrackStatusActive}
	if err := s.CreateTrack(ctx, track); err != nil {
		t.Fatalf("CreateTrack: %v", err)
	}
	for _, task := range tasks {
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask %s: %v", task.ID, err)
		}
	}
}

func execTrackCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := trackExecuteCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	err := cmd.RunE(cmd, args)
	return buf.String(), err
}

// TestTrackExecute_CommandShape pins the renamed surface: `execute` with
// `exec` kept as an alias, the executor flags present, and the flags the
// executor made meaningless gone.
func TestTrackExecute_CommandShape(t *testing.T) {
	if !strings.HasPrefix(trackExecuteCmd.Use, "execute ") {
		t.Errorf("Use = %q; want execute <track>", trackExecuteCmd.Use)
	}
	hasAlias := false
	for _, a := range trackExecuteCmd.Aliases {
		if a == "exec" {
			hasAlias = true
		}
	}
	if !hasAlias {
		t.Errorf("Aliases = %v; want exec", trackExecuteCmd.Aliases)
	}
	for _, name := range []string{"agent", "with-pod", "concurrency", "permissive", "reclaim", "wait", "poll", "dry-run", "trust-project", "ctxt", "timeout"} {
		if trackExecuteCmd.Flags().Lookup(name) == nil {
			t.Errorf("flag --%s missing", name)
		}
	}
	for _, name := range []string{"local", "no-state-update"} {
		if trackExecuteCmd.Flags().Lookup(name) != nil {
			t.Errorf("flag --%s should be gone (executor owns mode and state)", name)
		}
	}
	if trackExecuteCmd.Annotations["kit/side-effect"] != "interactive" {
		t.Errorf("kit/side-effect = %q; want interactive", trackExecuteCmd.Annotations["kit/side-effect"])
	}
}

// TestTrackExecute_DryRunShowsPlanAndCtxt keeps the ctxt regression from
// the old command (refs must reach the context builder) and pins that
// the dry run prints the batch plan without dispatching anything.
func TestTrackExecute_DryRunShowsPlanAndCtxt(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		plantAgentsYAML(t, "agents:\n  claude:\n    image: ghcr.io/example/claude:latest\n")
		defer resetTrackExecuteFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		seedExecTrack(t, ctx, s, execTestTask("task_a", 1), execTestTask("task_b", 2, "task_a"))

		resetTrackExecuteFlags()
		trackExecuteAgent = "claude"
		trackExecuteDryRun = true
		trackExecuteCtxtRefs = []string{"engineering?tag=runtime"}

		out, err := execTrackCmd(t, execTestTrack)
		if err != nil {
			t.Fatalf("track execute --dry-run: %v\n%s", err, out)
		}
		for _, want := range []string{"Dry run", "engineering?tag=runtime", "run task_a", "run task_b", "batch"} {
			if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
				t.Errorf("dry-run output lacks %q:\n%s", want, out)
			}
		}
		if got := mustGetTask(t, ctx, s, "task_a"); got.Status != core.StatusTodo {
			t.Errorf("dry run changed task_a to %q", got.Status)
		}
	})
}

// TestTrackExecute_ExecKindRunsToDone drives two exec-kind tasks through
// the real executor: claim, run the argv, store the result, complete, in
// dependency order.
func TestTrackExecute_ExecKindRunsToDone(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		defer resetTrackExecuteFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		seedExecTrack(t, ctx, s, execTestTask("task_b", 2, "task_a"), execTestTask("task_a", 1))

		resetTrackExecuteFlags()
		out, err := execTrackCmd(t, execTestTrack)
		if err != nil {
			t.Fatalf("track execute: %v\n%s", err, out)
		}

		for _, id := range []string{"task_a", "task_b"} {
			got := mustGetTask(t, ctx, s, id)
			if got.Status != core.StatusDone {
				t.Errorf("%s = %q; want DONE\n%s", id, got.Status, out)
			}
			if got.Result == nil || got.Result["exit_code"] != float64(0) || !strings.Contains(got.Result["stdout"].(string), id) {
				t.Errorf("%s result = %v; want exit_code 0 and stdout containing %s", id, got.Result, id)
			}
			actions := strings.Join(taskActions(t, ctx, s, id), ",")
			if !strings.Contains(actions, core.ActionClaimed) || !strings.Contains(actions, core.ActionDone) {
				t.Errorf("%s actions = %s; want CLAIMED and DONE", id, actions)
			}
		}
		if strings.Index(out, "task_a") > strings.Index(out, "task_b") {
			t.Errorf("task_b ran before its blocker task_a:\n%s", out)
		}
		if !strings.Contains(out, "done") {
			t.Errorf("output lacks completion lines:\n%s", out)
		}
	})
}

func TestTrackExecute_StrictThenPermissive(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		defer resetTrackExecuteFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		plain := execTestTask("task_plain", 1)
		plain.RunID, plain.StepID = "", ""
		seedExecTrack(t, ctx, s, plain)

		resetTrackExecuteFlags()
		if out, err := execTrackCmd(t, execTestTrack); err != nil {
			t.Fatalf("strict track execute: %v\n%s", err, out)
		}
		if got := mustGetTask(t, ctx, s, "task_plain"); got.Status != core.StatusTodo {
			t.Errorf("strict mode dispatched a task without provenance: %q", got.Status)
		}

		resetTrackExecuteFlags()
		trackExecutePermissive = true
		if out, err := execTrackCmd(t, execTestTrack); err != nil {
			t.Fatalf("permissive track execute: %v\n%s", err, out)
		}
		if got := mustGetTask(t, ctx, s, "task_plain"); got.Status != core.StatusDone {
			t.Errorf("permissive mode left task_plain %q; want DONE", got.Status)
		}
	})
}

func TestTrackExecute_HumanWaitsAndReports(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		defer resetTrackExecuteFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		signoff := execTestTask("task_signoff", 2, "task_a")
		signoff.Kind = core.TaskKindHuman
		signoff.Spec = &core.TaskSpec{Human: &core.HumanSpec{Assignee: "@lead"}}
		seedExecTrack(t, ctx, s, execTestTask("task_a", 1), signoff)

		resetTrackExecuteFlags()
		out, err := execTrackCmd(t, execTestTrack)
		if err != nil {
			t.Fatalf("track execute: %v\n%s", err, out)
		}
		if got := mustGetTask(t, ctx, s, "task_a"); got.Status != core.StatusDone {
			t.Errorf("task_a = %q; want DONE before the gate", got.Status)
		}
		if got := mustGetTask(t, ctx, s, "task_signoff"); got.Status != core.StatusTodo {
			t.Errorf("human task changed to %q", got.Status)
		}
		if !strings.Contains(out, "waiting on") || !strings.Contains(out, "human") {
			t.Errorf("output lacks the waiting line:\n%s", out)
		}
	})
}

// TestTrackExecute_LocalModeConcurrencyNotice: --concurrency above 1 with
// local agents announces that agent tasks still run one at a time;
// container mode says nothing of the sort.
func TestTrackExecute_LocalModeConcurrencyNotice(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		plantAgentsYAML(t, "agents:\n  claude:\n    binary: /bin/true\n    image: img\n")
		defer resetTrackExecuteFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		seedExecTrack(t, ctx, s, execTestTask("task_a", 1))

		resetTrackExecuteFlags()
		trackExecuteDryRun = true
		trackExecuteWithPod = "false"
		trackExecuteConcurrency = 3
		out, err := execTrackCmd(t, execTestTrack)
		if err != nil {
			t.Fatalf("track execute --dry-run local: %v\n%s", err, out)
		}
		if !strings.Contains(out, "one at a time") {
			t.Errorf("local mode with --concurrency 3 must announce serialized agents:\n%s", out)
		}

		resetTrackExecuteFlags()
		trackExecuteDryRun = true
		trackExecuteConcurrency = 3
		out, err = execTrackCmd(t, execTestTrack)
		if err != nil {
			t.Fatalf("track execute --dry-run container: %v\n%s", err, out)
		}
		if strings.Contains(out, "one at a time") {
			t.Errorf("container mode must not announce serialized agents:\n%s", out)
		}
	})
}
