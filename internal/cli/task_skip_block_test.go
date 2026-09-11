package cli

// In-process coverage for skip / block / unblock. The role-resolution
// half of skip is NOT tested here — it cannot be, because these tests
// never load a config file. See task_skip_vocabulary_e2e_test.go.

import (
	"bytes"
	"testing"

	"hop.top/tlc/internal/core"
)

// runTaskCmd executes one task subcommand in-process and returns its
// combined output plus the error, so each case states only what differs.
func runTaskCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(append([]string{"task"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

func TestTaskSkip(t *testing.T) {
	t.Run("SkipsTodoTask", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		if err := s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Skip me", Status: core.StatusTodo,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}

		out, err := runTaskCmd(t, "skip", "T-0001")
		if err != nil {
			t.Fatalf("task skip failed: %v\n%s", err, out)
		}
		if !contains(out, "Skipped task T-0001") {
			t.Errorf("unexpected output: %s", out)
		}

		task := getTaskByAlias(t, ctx, "T-0001")
		if task == nil {
			t.Fatal("task not found after skip")
		}
		if task.Status != core.StatusSkipped {
			t.Errorf("expected SKIPPED, got %s", task.Status)
		}
	})

	t.Run("RecordsAuditEntry", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		if err := s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Audit me", Status: core.StatusTodo,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}

		if out, err := runTaskCmd(t, "skip", "T-0001", "--note", "superseded upstream"); err != nil {
			t.Fatalf("task skip failed: %v\n%s", err, out)
		}

		task := getTaskByAlias(t, ctx, "T-0001")
		if !contains(task.Description, "SKIPPED") {
			t.Errorf("audit entry should record SKIPPED, got description:\n%s", task.Description)
		}
		if !contains(task.Description, "superseded upstream") {
			t.Errorf("audit entry should carry the note, got description:\n%s", task.Description)
		}
		if !contains(task.Description, "TODO → SKIPPED") {
			t.Errorf("audit entry should record the transition, got description:\n%s", task.Description)
		}
	})

	t.Run("RespectsStateMachineOnTerminalTask", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		// DONE is terminal, and terminal states are immutable without a
		// force. Skipping one must be refused, not silently applied.
		if err := s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Already done", Status: core.StatusDone,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}

		out, err := runTaskCmd(t, "skip", "T-0001")
		if err == nil {
			t.Fatalf("skipping a DONE task should be rejected, got:\n%s", out)
		}
		if task := getTaskByAlias(t, ctx, "T-0001"); task.Status != core.StatusDone {
			t.Errorf("rejected skip must not move the task, got %s", task.Status)
		}
	})

	t.Run("NoVerifyIsTheEscapeHatch", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		if err := s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Force me", Status: core.StatusDone,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}

		if out, err := runTaskCmd(t, "skip", "T-0001", "--no-verify"); err != nil {
			t.Fatalf("task skip --no-verify failed: %v\n%s", err, out)
		}
		if task := getTaskByAlias(t, ctx, "T-0001"); task.Status != core.StatusSkipped {
			t.Errorf("expected SKIPPED after --no-verify, got %s", task.Status)
		}
	})

	t.Run("Idempotent", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		if err := s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Twice", Status: core.StatusTodo,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}

		if out, err := runTaskCmd(t, "skip", "T-0001"); err != nil {
			t.Fatalf("first skip failed: %v\n%s", err, out)
		}
		// Same-status transitions are always valid, so a re-run converges
		// rather than tripping the terminal-immutability rule.
		if out, err := runTaskCmd(t, "skip", "T-0001"); err != nil {
			t.Fatalf("second skip should converge, got: %v\n%s", err, out)
		}
		if task := getTaskByAlias(t, ctx, "T-0001"); task.Status != core.StatusSkipped {
			t.Errorf("expected SKIPPED, got %s", task.Status)
		}
	})

	t.Run("RegexPatternBatch", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		for _, id := range []string{"T-0001", "T-0002", "T-0003"} {
			if err := s.CreateTask(ctx, &core.Task{
				ID: id, Title: "batch " + id, Status: core.StatusTodo,
			}); err != nil {
				t.Fatalf("seed %s: %v", id, err)
			}
		}

		out, err := runTaskCmd(t, "skip", `T-000[12]`, "--no-prompt")
		if err != nil {
			t.Fatalf("batch skip failed: %v\n%s", err, out)
		}
		for _, id := range []string{"T-0001", "T-0002"} {
			if task := getTaskByAlias(t, ctx, id); task.Status != core.StatusSkipped {
				t.Errorf("%s should be SKIPPED, got %s", id, task.Status)
			}
		}
		// The pattern must not reach beyond what it matched.
		if task := getTaskByAlias(t, ctx, "T-0003"); task.Status != core.StatusTodo {
			t.Errorf("T-0003 should be untouched, got %s", task.Status)
		}
	})
}

func TestTaskBlockUnblock(t *testing.T) {
	t.Run("BlockRequiresReason", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		if err := s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Needs reason", Status: core.StatusTodo,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}

		out, err := runTaskCmd(t, "block", "T-0001")
		if err == nil {
			t.Fatalf("block without a reason should be rejected, got:\n%s", out)
		}
		if !contains(err.Error(), "reason") {
			t.Errorf("error should ask for a reason, got: %v", err)
		}
		if task := getTaskByAlias(t, ctx, "T-0001"); task.BlockedReason != nil {
			t.Errorf("rejected block must not set a reason, got %q", *task.BlockedReason)
		}
	})

	t.Run("BlockSetsReasonAndLeavesStatus", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		if err := s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Block me", Status: core.StatusInProgress,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}

		out, err := runTaskCmd(t, "block", "T-0001", "--note", "waiting on vendor SLA")
		if err != nil {
			t.Fatalf("task block failed: %v\n%s", err, out)
		}
		if !contains(out, "Blocked task T-0001") {
			t.Errorf("unexpected output: %s", out)
		}

		task := getTaskByAlias(t, ctx, "T-0001")
		if task.BlockedReason == nil || *task.BlockedReason != "waiting on vendor SLA" {
			t.Errorf("blocked reason not set, got %v", task.BlockedReason)
		}
		// Blocking is orthogonal to status: IN_PROGRESS must survive.
		if task.Status != core.StatusInProgress {
			t.Errorf("block must not change status, got %s", task.Status)
		}
	})

	t.Run("ReasonFlagIsAliasForNote", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		if err := s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Alias", Status: core.StatusTodo,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}

		if out, err := runTaskCmd(t, "block", "T-0001", "--reason", "upstream outage"); err != nil {
			t.Fatalf("task block --reason failed: %v\n%s", err, out)
		}
		task := getTaskByAlias(t, ctx, "T-0001")
		if task.BlockedReason == nil || *task.BlockedReason != "upstream outage" {
			t.Errorf("--reason should set the blocked reason, got %v", task.BlockedReason)
		}
	})

	t.Run("UnblockClearsReason", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		reason := "waiting on vendor"
		if err := s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Unblock me", Status: core.StatusInProgress,
			BlockedReason: &reason,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}

		out, err := runTaskCmd(t, "unblock", "T-0001")
		if err != nil {
			t.Fatalf("task unblock failed: %v\n%s", err, out)
		}
		if !contains(out, "Unblocked task T-0001") {
			t.Errorf("unexpected output: %s", out)
		}

		task := getTaskByAlias(t, ctx, "T-0001")
		if task.BlockedReason != nil && *task.BlockedReason != "" {
			t.Errorf("blocked reason should be cleared, got %q", *task.BlockedReason)
		}
		if task.Status != core.StatusInProgress {
			t.Errorf("unblock must not change status, got %s", task.Status)
		}
		// The audit trail must still say what was resolved.
		if !contains(task.Description, "waiting on vendor") {
			t.Errorf("audit entry should preserve the cleared reason, got:\n%s", task.Description)
		}
	})

	t.Run("UnblockNoteIsOptional", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		if err := s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Never blocked", Status: core.StatusTodo,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		// Unblocking an unblocked task converges rather than erroring.
		if out, err := runTaskCmd(t, "unblock", "T-0001"); err != nil {
			t.Fatalf("unblock on an unblocked task should converge: %v\n%s", err, out)
		}
	})

	t.Run("BlockDoesNotTouchDependencyEdges", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		if err := s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Dependency", Status: core.StatusTodo,
		}); err != nil {
			t.Fatalf("seed dep: %v", err)
		}
		dependent := &core.Task{ID: "T-0002", Title: "Dependent", Status: core.StatusTodo}
		dependent.SetBlockedBy([]string{"T-0001"})
		if err := s.CreateTask(ctx, dependent); err != nil {
			t.Fatalf("seed: %v", err)
		}

		if out, err := runTaskCmd(t, "block", "T-0002", "--note", "also waiting on legal"); err != nil {
			t.Fatalf("task block failed: %v\n%s", err, out)
		}

		// blocked-by (graph) and blocked-reason (free text) are separate
		// axes; a task carries both at once and neither overwrites the
		// other.
		task := getTaskByAlias(t, ctx, "T-0002")
		if edges := task.BlockedBy(); len(edges) != 1 || edges[0] != "T-0001" {
			t.Errorf("block must not touch blocked_by edges, got %v", edges)
		}
		if task.BlockedReason == nil || *task.BlockedReason != "also waiting on legal" {
			t.Errorf("blocked reason not set alongside the edge, got %v", task.BlockedReason)
		}

		if out, err := runTaskCmd(t, "unblock", "T-0002"); err != nil {
			t.Fatalf("task unblock failed: %v\n%s", err, out)
		}
		task = getTaskByAlias(t, ctx, "T-0002")
		if edges := task.BlockedBy(); len(edges) != 1 || edges[0] != "T-0001" {
			t.Errorf("unblock must not drop blocked_by edges, got %v", edges)
		}
	})

	t.Run("BatchBlock", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		for _, id := range []string{"T-0001", "T-0002", "T-0003"} {
			if err := s.CreateTask(ctx, &core.Task{
				ID: id, Title: "batch " + id, Status: core.StatusTodo,
			}); err != nil {
				t.Fatalf("seed %s: %v", id, err)
			}
		}

		if out, err := runTaskCmd(t, "block", `T-000[12]`, "--note", "shared blocker", "--no-prompt"); err != nil {
			t.Fatalf("batch block failed: %v\n%s", err, out)
		}
		for _, id := range []string{"T-0001", "T-0002"} {
			if task := getTaskByAlias(t, ctx, id); task.BlockedReason == nil {
				t.Errorf("%s should be blocked", id)
			}
		}
		if task := getTaskByAlias(t, ctx, "T-0003"); task.BlockedReason != nil {
			t.Errorf("T-0003 should be untouched, got %q", *task.BlockedReason)
		}
	})
}
