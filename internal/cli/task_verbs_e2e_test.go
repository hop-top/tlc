package cli

// The status verbs (submit, approve, reject, block, unblock, cancel,
// release) driven through the real binary under the seven-status
// vocabulary, declared in config so the cases do not depend on the
// built-in default.

import (
	"strings"
	"testing"
)

const sevenStatusHead = `storage:
  db_path: %s
task:
  default_status: todo
  statuses:
    - {name: backlog, label: Backlog, role: backlog, tls_marker: "-"}
    - {name: todo, label: To Do, role: initial, tls_marker: " "}
    - {name: in_progress, label: In Progress, role: active, tls_marker: ">"}
    - {name: in_review, label: In Review, role: review, tls_marker: "?"}
    - {name: blocked, label: Blocked, role: blocked, tls_marker: "!"}
    - {name: done, label: Done, role: completed, is_terminal: true, tls_marker: "x"}
    - {name: cancelled, label: Cancelled, role: skipped, is_terminal: true, tls_marker: "~"}
  state_machine:
    rules:
`

const sevenStatusConfig = sevenStatusHead + `      backlog: [todo, cancelled]
      todo: [backlog, in_progress, blocked, cancelled]
      in_progress: [todo, in_review, blocked, done, cancelled]
      in_review: [in_progress, done, blocked, cancelled]
      blocked: [todo, in_progress, in_review, cancelled]
`

// noBlockedToActiveConfig forbids blocked -> in_progress, so an unblock
// of a task blocked from in_progress cannot return there.
const noBlockedToActiveConfig = sevenStatusHead + `      todo: [in_progress, blocked, cancelled]
      in_progress: [todo, in_review, blocked, done, cancelled]
      in_review: [in_progress, done, blocked, cancelled]
      blocked: [todo, in_review, cancelled]
`

func verbTask(t *testing.T, cfg string, steps ...[]string) (bin, home string, env []string) {
	t.Helper()
	bin, home, env = statusVocabFixture(t, cfg)
	runTLCOK(t, bin, home, env, "task", "create", "verb probe", "--status", "todo")
	for _, s := range steps {
		runTLCOK(t, bin, home, env, s...)
	}
	return
}

func showHas(t *testing.T, bin, home string, env []string, want string) {
	t.Helper()
	shown := runTLCOK(t, bin, home, env, "task", "show", "T-0001")
	if !strings.Contains(shown, want) {
		t.Errorf("task show missing %q:\n%s", want, shown)
	}
}

func TestSubmitApproveFlow(t *testing.T) {
	bin, home, env := verbTask(t, sevenStatusConfig, []string{"task", "claim", "T-0001"})
	runTLCOK(t, bin, home, env, "task", "submit", "T-0001", "--note", "ready")
	showHas(t, bin, home, env, "In Review")
	runTLCOK(t, bin, home, env, "task", "approve", "T-0001")
	showHas(t, bin, home, env, "Done")
}

func TestSubmitRefusedFromTodo(t *testing.T) {
	bin, home, env := verbTask(t, sevenStatusConfig)
	out, code := runTLC(t, bin, home, env, "task", "submit", "T-0001")
	if code == 0 {
		t.Fatalf("todo -> in_review should be refused:\n%s", out)
	}
	showHas(t, bin, home, env, "To Do")
}

func TestSubmitDryRunWritesNothing(t *testing.T) {
	bin, home, env := verbTask(t, sevenStatusConfig, []string{"task", "claim", "T-0001"})
	runTLCOK(t, bin, home, env, "task", "submit", "T-0001", "--dry-run")
	showHas(t, bin, home, env, "In Progress")
}

func TestApproveRefusedOutsideReview(t *testing.T) {
	bin, home, env := verbTask(t, sevenStatusConfig, []string{"task", "claim", "T-0001"})
	out, code := runTLC(t, bin, home, env, "task", "approve", "T-0001")
	if code == 0 {
		t.Fatalf("approve of an in_progress task should be refused:\n%s", out)
	}
	showHas(t, bin, home, env, "In Progress")
}

func TestRejectNeedsReasonAndReturnsToActive(t *testing.T) {
	bin, home, env := verbTask(t, sevenStatusConfig,
		[]string{"task", "claim", "T-0001"}, []string{"task", "submit", "T-0001"})
	out, code := runTLC(t, bin, home, env, "task", "reject", "T-0001")
	if code == 0 || !strings.Contains(out, "--reason") {
		t.Fatalf("reject without --reason should fail naming it (exit %d):\n%s", code, out)
	}
	showHas(t, bin, home, env, "In Review")
	runTLCOK(t, bin, home, env, "task", "reject", "T-0001", "--reason", "tests missing")
	showHas(t, bin, home, env, "In Progress")
}

func TestBlockUnblockReturnsToRecordedStatus(t *testing.T) {
	bin, home, env := verbTask(t, sevenStatusConfig,
		[]string{"task", "claim", "T-0001"}, []string{"task", "submit", "T-0001"})
	runTLCOK(t, bin, home, env, "task", "block", "T-0001", "--reason", "vendor")
	showHas(t, bin, home, env, "Blocked")
	showHas(t, bin, home, env, "vendor")
	runTLCOK(t, bin, home, env, "task", "unblock", "T-0001")
	showHas(t, bin, home, env, "In Review")
}

func TestBlockRequiresReason(t *testing.T) {
	bin, home, env := verbTask(t, sevenStatusConfig)
	if out, code := runTLC(t, bin, home, env, "task", "block", "T-0001"); code == 0 {
		t.Fatalf("block without reason should fail:\n%s", out)
	}
	showHas(t, bin, home, env, "To Do")
}

func TestUnblockFallsBackToInitial(t *testing.T) {
	bin, home, env := verbTask(t, noBlockedToActiveConfig, []string{"task", "claim", "T-0001"})
	runTLCOK(t, bin, home, env, "task", "block", "T-0001", "--reason", "x")
	runTLCOK(t, bin, home, env, "task", "unblock", "T-0001")
	showHas(t, bin, home, env, "To Do")
}

func TestBlockDryRunWritesNothing(t *testing.T) {
	bin, home, env := verbTask(t, sevenStatusConfig, []string{"task", "claim", "T-0001"})
	runTLCOK(t, bin, home, env, "task", "block", "T-0001", "--reason", "x", "--dry-run")
	showHas(t, bin, home, env, "In Progress")
}

func TestCancelAndSkipAlias(t *testing.T) {
	bin, home, env := verbTask(t, sevenStatusConfig)
	runTLCOK(t, bin, home, env, "task", "cancel", "T-0001", "--note", "obsolete")
	showHas(t, bin, home, env, "Cancelled")

	bin, home, env = verbTask(t, sevenStatusConfig)
	runTLCOK(t, bin, home, env, "task", "skip", "T-0001", "--note", "obsolete")
	showHas(t, bin, home, env, "Cancelled")
}

func TestReleaseAlias(t *testing.T) {
	bin, home, env := verbTask(t, sevenStatusConfig, []string{"task", "claim", "T-0001"})
	runTLCOK(t, bin, home, env, "task", "release", "T-0001")
	showHas(t, bin, home, env, "To Do")
}

// Guard refusals under the built-in vocabulary and guards.

func TestCancelWithoutNoteRefusedByGuard(t *testing.T) {
	bin, home, env := statusVocabFixture(t, defaultStatusConfig)
	runTLCOK(t, bin, home, env, "task", "create", "guarded")
	out, code := runTLC(t, bin, home, env, "task", "cancel", "T-0001")
	if code == 0 || !strings.Contains(out, "note") {
		t.Fatalf("cancel without note should be refused naming the note check (exit %d):\n%s", code, out)
	}
	showHas(t, bin, home, env, "To Do")
	runTLCOK(t, bin, home, env, "task", "skip", "T-0001", "--note", "obsolete")
	showHas(t, bin, home, env, "Cancelled")
}

func TestBlockWithoutReasonRefused(t *testing.T) {
	bin, home, env := statusVocabFixture(t, defaultStatusConfig)
	runTLCOK(t, bin, home, env, "task", "create", "guarded")
	if out, code := runTLC(t, bin, home, env, "task", "block", "T-0001"); code == 0 {
		t.Fatalf("block without reason should be refused:\n%s", out)
	}
	showHas(t, bin, home, env, "To Do")
	runTLCOK(t, bin, home, env, "task", "block", "T-0001", "--reason", "vendor")
	showHas(t, bin, home, env, "Blocked")
	runTLCOK(t, bin, home, env, "task", "unblock", "T-0001")
	showHas(t, bin, home, env, "To Do")
}

func TestSubmitWithoutAssigneeRefusedByGuard(t *testing.T) {
	bin, home, env := statusVocabFixture(t, defaultStatusConfig)
	runTLCOK(t, bin, home, env, "task", "create", "guarded")
	runTLCOK(t, bin, home, env, "task", "claim", "T-0001")
	runTLCOK(t, bin, home, env, "task", "update", "T-0001", "--assigned-to", "-")
	out, code := runTLC(t, bin, home, env, "task", "submit", "T-0001")
	if code == 0 || !strings.Contains(out, "assignee") {
		t.Fatalf("submit without assignee should be refused naming it (exit %d):\n%s", code, out)
	}
	showHas(t, bin, home, env, "In Progress")
}

func TestCompleteWithOpenBlockerRefusedByGuard(t *testing.T) {
	bin, home, env := statusVocabFixture(t, defaultStatusConfig)
	runTLCOK(t, bin, home, env, "task", "create", "blocker")
	runTLCOK(t, bin, home, env, "task", "create", "blocked one")
	runTLCOK(t, bin, home, env, "task", "update", "T-0002", "--add-blocked-by", "T-0001")
	runTLCOK(t, bin, home, env, "task", "claim", "T-0002")
	out, code := runTLC(t, bin, home, env, "task", "complete", "T-0002")
	if code == 0 || !strings.Contains(out, "T-0001") {
		t.Fatalf("complete with an open blocker should be refused naming it (exit %d):\n%s", code, out)
	}
	runTLCOK(t, bin, home, env, "task", "claim", "T-0001")
	runTLCOK(t, bin, home, env, "task", "complete", "T-0001")
	runTLCOK(t, bin, home, env, "task", "complete", "T-0002")
}
