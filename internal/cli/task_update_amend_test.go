package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

// TestTaskUpdateAmend covers `tlc task update --amend` end-to-end against
// the live SQLite storage layer:
//
//   - amend rewrites the most recent log entry in place rather than
//     emitting a new one (log row count stays constant)
//   - the status-transition prefix is preserved on transition logs;
//     only the trailing user note is replaced
//   - meta["amended_at"] is recorded so the audit row stays self-
//     describing without requiring a schema migration
//   - --amend with --status is rejected (workflow side effects)
//   - --amend without --note is rejected (nothing to amend)
//   - --amend on a terminal task (DONE/SKIPPED) is rejected without
//     --force, and accepted with --force
func TestTaskUpdateAmend(t *testing.T) {
	t.Run("AmendStatusTransitionNote", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		if err := s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Amend test", Status: core.StatusTodo,
		}); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		// Emit two transition logs (TODO -> IN_PROGRESS -> TODO).
		execTaskUpdate(t, []string{"task", "update", "T-0001", "--status", "IN_PROGRESS", "--note", "first try"})
		execTaskUpdate(t, []string{"task", "update", "T-0001", "--status", "TODO", "--note", "wrong direction", "--force"})

		logsBefore, err := s.GetLogs(ctx, "T-0001", "desc")
		if err != nil {
			t.Fatalf("GetLogs: %v", err)
		}
		if len(logsBefore) == 0 {
			t.Fatalf("expected at least one log entry, got 0")
		}
		latestBefore := logsBefore[0]
		latestID := latestBefore.ID
		rowCountBefore := len(logsBefore)

		// Amend the latest log's note in place.
		execTaskUpdate(t, []string{"task", "update", "T-0001", "--amend", "--note", "corrected reason"})

		logsAfter, err := s.GetLogs(ctx, "T-0001", "desc")
		if err != nil {
			t.Fatalf("GetLogs after amend: %v", err)
		}
		if len(logsAfter) != rowCountBefore {
			t.Fatalf("amend must not add a row; before=%d after=%d", rowCountBefore, len(logsAfter))
		}

		latestAfter := logsAfter[0]
		if latestAfter.ID != latestID {
			t.Errorf("amend mutated a different row: latestID=%d amendedID=%d", latestID, latestAfter.ID)
		}
		if !strings.HasPrefix(latestAfter.Note, "Status changed from ") {
			t.Errorf("expected status-transition prefix preserved, got %q", latestAfter.Note)
		}
		if !strings.HasSuffix(latestAfter.Note, ": corrected reason") {
			t.Errorf("expected suffix replaced, got %q", latestAfter.Note)
		}
		if strings.Contains(latestAfter.Note, "wrong direction") {
			t.Errorf("expected old note suffix gone, still found in %q", latestAfter.Note)
		}
		if latestAfter.Meta == nil {
			t.Fatalf("expected meta on amended row, got nil")
		}
		if _, ok := latestAfter.Meta["amended_at"]; !ok {
			t.Errorf("expected meta[\"amended_at\"] on amended row, got %v", latestAfter.Meta)
		}
	})

	t.Run("AmendRejectsStatusFlag", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()
		if err := s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "x", Status: core.StatusTodo}); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		err := tryTaskUpdate([]string{"task", "update", "T-0001", "--amend", "--status", "IN_PROGRESS", "--note", "x"})
		if err == nil {
			t.Fatalf("expected error when combining --amend with --status, got nil")
		}
		if !strings.Contains(err.Error(), "--amend with --status") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("AmendRequiresNote", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()
		if err := s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "x", Status: core.StatusTodo}); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		err := tryTaskUpdate([]string{"task", "update", "T-0001", "--amend"})
		if err == nil {
			t.Fatalf("expected error when --amend used without --note, got nil")
		}
		if !strings.Contains(err.Error(), "--amend requires --note") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("AmendBlockedOnTerminalWithoutForce", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()
		if err := s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "x", Status: core.StatusTodo}); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		execTaskUpdate(t, []string{"task", "update", "T-0001", "--status", "IN_PROGRESS", "--note", "claiming"})
		execTaskUpdate(t, []string{"task", "update", "T-0001", "--status", "DONE", "--note", "completed"})

		err := tryTaskUpdate([]string{"task", "update", "T-0001", "--amend", "--note", "actually skipped"})
		if err == nil {
			t.Fatalf("expected error when amending terminal task without --force, got nil")
		}
		if !strings.Contains(err.Error(), "terminal task") {
			t.Errorf("unexpected error: %v", err)
		}

		// --force unblocks the amend.
		execTaskUpdate(t, []string{"task", "update", "T-0001", "--amend", "--note", "post-mortem note", "--force"})
		logs, err := s.GetLogs(ctx, "T-0001", "desc")
		if err != nil {
			t.Fatalf("GetLogs: %v", err)
		}
		if !strings.HasSuffix(logs[0].Note, ": post-mortem note") {
			t.Errorf("expected forced amend to take effect, got %q", logs[0].Note)
		}
	})
}

// execTaskUpdate runs a tlc task update invocation and fails the test on
// error. The CLI package leaks cobra flag state across invocations
// because the flags are global vars, so reset them first.
func execTaskUpdate(t *testing.T, args []string) {
	t.Helper()
	resetTaskUpdateFlagsAmend()
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v failed: %v\noutput: %s", args, err, buf.String())
	}
}

// tryTaskUpdate runs and returns the error without failing the test.
func tryTaskUpdate(args []string) error {
	resetTaskUpdateFlagsAmend()
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	return cmd.Execute()
}

// resetTaskUpdateFlagsAmend clears the package-level flag vars AND the
// flag's Changed state on the underlying pflag.FlagSet so subsequent
// invocations see a fresh slate. cobra reuses the same FlagSet across
// invocations in the same test binary; without explicit reset, a prior
// --status leaks into the next call's Flags().Changed("status") check
// and the amend short-circuit fires unexpectedly.
func resetTaskUpdateFlagsAmend() {
	taskUpdateAmend = false
	taskUpdateForce = false
	taskUpdateNote = ""
	taskUpdateStatus = ""
	taskUpdateTitle = ""
	taskUpdateDescription = ""
	fs := TaskUpdateCmd.Flags()
	for _, name := range []string{"amend", "force", "note", "status", "title", "description"} {
		if f := fs.Lookup(name); f != nil {
			f.Changed = false
		}
	}
}

// Compile-time check: ensure context import is used; the helpers above
// rely on the resolved context propagated through the cobra command.
var _ = context.Background
