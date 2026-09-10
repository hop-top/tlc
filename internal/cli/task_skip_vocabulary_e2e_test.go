package cli

// The role-resolution contract for `tlc task skip`, driven through a real
// config FILE and the real binary.
//
// This is the only place that can catch the bug this command is most
// likely to grow: resolving the skip target by NAME instead of by ROLE.
// A struct-level test constructs its own TaskConfig and so never sees
// the difference — which is exactly how the original config-wiring bug
// shipped green for months. Every case here declares a vocabulary that
// contains no status named SKIPPED at all, so a hardcoded constant has
// nowhere to hide.

import (
	"strings"
	"testing"
)

// Every fully-renamed vocabulary below sets `default_status`, AND every
// create below passes --status explicitly. Both are needed because
// `task create --status` carries a hardcoded "TODO" flag default that
// wins over `task.default_status`, so a create under a renamed
// vocabulary is rejected by the user's own config ("unknown status
// TODO"). That is a pre-existing bug in create, unrelated to skip;
// noted here so these fixtures do not read as superstition.

// wontfixConfig is the spec's own "Full Custom Statuses" shape: the skip
// analog is called WONTFIX. It carries `role: skipped`, which is the
// declaration this command is built to read. Nothing here is named
// SKIPPED, so the name fallback cannot rescue a hardcoded lookup.
const wontfixConfig = `storage:
  db_path: %s
task:
  default_status: OPEN
  statuses:
    - name: OPEN
      label: Open
      role: initial
      tls_marker: " "
    - name: ACTIVE
      label: Active
      role: active
      tls_marker: "~"
    - name: MERGED
      label: Merged
      is_terminal: true
      role: completed
      tls_marker: "x"
    - name: WONTFIX
      label: Won't Fix
      is_terminal: true
      role: skipped
      tls_marker: "-"
  state_machine:
    rules:
      OPEN: [ACTIVE, WONTFIX]
      ACTIVE: [MERGED, OPEN]
`

// noSkipRoleConfig declares a terminal "abandoned" status but gives it no
// role, matching a config written before the role existed. There is no
// status named SKIPPED either, so both resolution rules miss and the
// command must refuse rather than electing ABANDONED on its own.
const noSkipRoleConfig = `storage:
  db_path: %s
task:
  default_status: OPEN
  statuses:
    - name: OPEN
      label: Open
      role: initial
      tls_marker: " "
    - name: ACTIVE
      label: Active
      role: active
      tls_marker: "~"
    - name: MERGED
      label: Merged
      is_terminal: true
      role: completed
      tls_marker: "x"
    - name: ABANDONED
      label: Abandoned
      is_terminal: true
      tls_marker: "-"
  state_machine:
    rules:
      OPEN: [ACTIVE, ABANDONED]
      ACTIVE: [MERGED, OPEN]
`

// legacySkippedNameConfig keeps the pre-role shape of the built-in set:
// SKIPPED declared with role "completed", so the role index hands
// "completed" to DONE and nothing claims "skipped". This is the config
// the documented NAME fallback exists for, and the case that proves the
// fallback announces itself instead of applying silently.
const legacySkippedNameConfig = `storage:
  db_path: %s
task:
  statuses:
    - name: TODO
      label: To Do
      role: initial
      tls_marker: " "
    - name: IN_PROGRESS
      label: In Progress
      role: active
      tls_marker: ">"
    - name: DONE
      label: Done
      is_terminal: true
      role: completed
      tls_marker: "x"
    - name: SKIPPED
      label: Skipped
      is_terminal: true
      role: completed
      tls_marker: "-"
  state_machine:
    rules:
      TODO: [IN_PROGRESS, SKIPPED]
      IN_PROGRESS: [DONE, TODO, SKIPPED]
`

// noSkipTransitionConfig declares the role but forbids the transition, to
// pin that opening the vocabulary did not open the rules with it.
const noSkipTransitionConfig = `storage:
  db_path: %s
task:
  default_status: OPEN
  statuses:
    - name: OPEN
      label: Open
      role: initial
      tls_marker: " "
    - name: ACTIVE
      label: Active
      role: active
      tls_marker: "~"
    - name: MERGED
      label: Merged
      is_terminal: true
      role: completed
      tls_marker: "x"
    - name: WONTFIX
      label: Won't Fix
      is_terminal: true
      role: skipped
      tls_marker: "-"
  state_machine:
    rules:
      OPEN: [ACTIVE]
      ACTIVE: [MERGED, WONTFIX]
`

// TestSkipResolvesRoleNotName is the headline: with a vocabulary whose
// skip status is called WONTFIX, `task skip` must land the task in
// WONTFIX. A command that reached for core.StatusSkipped would fail here
// with "unknown status: SKIPPED", because SKIPPED is not declared.
func TestSkipResolvesRoleNotName(t *testing.T) {
	bin, home, env := statusVocabFixture(t, wontfixConfig)

	runTLCOK(t, bin, home, env, "task", "create", "abandon me", "--status", "OPEN")

	out, code := runTLC(t, bin, home, env, "task", "skip", "T-0001")
	if code != 0 {
		t.Fatalf("task skip rejected under a custom vocabulary (exit %d):\n%s", code, out)
	}

	// Assert the landing status, not merely exit 0: a command that
	// silently no-opped would also exit 0 here.
	shown := runTLCOK(t, bin, home, env, "task", "show", "T-0001")
	if !strings.Contains(shown, "WONTFIX") && !strings.Contains(shown, "Won't Fix") {
		t.Errorf("task did not land in WONTFIX:\n%s", shown)
	}
	if strings.Contains(shown, "MERGED") {
		t.Errorf("skip landed in the completed status instead of the skipped one:\n%s", shown)
	}
	// No fallback was needed, so nothing should warn about one.
	if strings.Contains(out, "falling back") {
		t.Errorf("a config declaring role skipped must not trigger the name fallback:\n%s", out)
	}
}

// TestSkipRefusesWithoutASkipTarget pins the deliberate non-answer: with
// no role and no SKIPPED name, the command refuses. Electing ABANDONED
// because it happens to be the other terminal status would move a task
// somewhere the user never nominated.
func TestSkipRefusesWithoutASkipTarget(t *testing.T) {
	bin, home, env := statusVocabFixture(t, noSkipRoleConfig)

	runTLCOK(t, bin, home, env, "task", "create", "no target", "--status", "OPEN")

	out, code := runTLC(t, bin, home, env, "task", "skip", "T-0001")
	if code == 0 {
		t.Fatalf("skip should refuse when no status declares role skipped:\n%s", out)
	}
	if !strings.Contains(out, "skipped") {
		t.Errorf("refusal should name the role to declare, got:\n%s", out)
	}
	if !strings.Contains(out, "ABANDONED") {
		t.Errorf("refusal should list the declared statuses, got:\n%s", out)
	}

	// And it must not have guessed.
	shown := runTLCOK(t, bin, home, env, "task", "show", "T-0001")
	if strings.Contains(shown, "ABANDONED") || strings.Contains(shown, "Abandoned") {
		t.Errorf("refused skip must leave the task alone, got:\n%s", shown)
	}
}

// TestSkipNameFallbackWarns covers the documented compatibility path: a
// config predating the role still works, and says so.
func TestSkipNameFallbackWarns(t *testing.T) {
	bin, home, env := statusVocabFixture(t, legacySkippedNameConfig)

	runTLCOK(t, bin, home, env, "task", "create", "legacy shape")

	out, code := runTLC(t, bin, home, env, "task", "skip", "T-0001")
	if code != 0 {
		t.Fatalf("skip should still work via the name fallback (exit %d):\n%s", code, out)
	}
	if !strings.Contains(out, "falling back") {
		t.Errorf("the name fallback must announce itself, got:\n%s", out)
	}
	if !strings.Contains(out, "role") {
		t.Errorf("the warning should name the fix, got:\n%s", out)
	}

	shown := runTLCOK(t, bin, home, env, "task", "show", "T-0001")
	if !strings.Contains(shown, "SKIPPED") && !strings.Contains(shown, "Skipped") {
		t.Errorf("task did not land in SKIPPED:\n%s", shown)
	}
}

// TestSkipRespectsStateMachine guards the other half: the role says WHERE
// skip goes, the state machine still says WHETHER it may.
func TestSkipRespectsStateMachine(t *testing.T) {
	bin, home, env := statusVocabFixture(t, noSkipTransitionConfig)

	runTLCOK(t, bin, home, env, "task", "create", "rule bound", "--status", "OPEN")

	// OPEN -> WONTFIX is not in the rules.
	out, code := runTLC(t, bin, home, env, "task", "skip", "T-0001")
	if code == 0 {
		t.Fatalf("OPEN -> WONTFIX should be rejected by the state machine:\n%s", out)
	}
	if !strings.Contains(out, "transition") {
		t.Errorf("rejection should come from the state machine, got:\n%s", out)
	}

	// The escape hatch every other lifecycle command offers must work here too.
	if out, code := runTLC(t, bin, home, env, "task", "skip", "T-0001", "--no-verify"); code != 0 {
		t.Fatalf("--no-verify should bypass the rule (exit %d):\n%s", code, out)
	}
	shown := runTLCOK(t, bin, home, env, "task", "show", "T-0001")
	if !strings.Contains(shown, "WONTFIX") && !strings.Contains(shown, "Won't Fix") {
		t.Errorf("--no-verify skip did not land in WONTFIX:\n%s", shown)
	}
}

// TestSkipDefaultVocabulary is the no-regression contract for users who
// declare nothing: the built-in SKIPPED is now declared with role
// "skipped", so this must resolve by role and never warn.
func TestSkipDefaultVocabulary(t *testing.T) {
	bin, home, env := statusVocabFixture(t, defaultStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "default shape")

	out, code := runTLC(t, bin, home, env, "task", "skip", "T-0001")
	if code != 0 {
		t.Fatalf("skip failed on the built-in vocabulary (exit %d):\n%s", code, out)
	}
	if strings.Contains(out, "falling back") {
		t.Errorf("the built-in vocabulary declares the role; it must not warn:\n%s", out)
	}

	shown := runTLCOK(t, bin, home, env, "task", "show", "T-0001")
	if !strings.Contains(shown, "SKIPPED") && !strings.Contains(shown, "Skipped") {
		t.Errorf("task did not land in SKIPPED:\n%s", shown)
	}
}

// TestBlockUnblockThroughRealBinary covers the block axis end to end,
// including that `task show` renders both states. Blocking is vocabulary
// independent by construction — it writes a free-text field, not a status
// — and this pins that: it runs under the WONTFIX vocabulary and must
// behave identically.
func TestBlockUnblockThroughRealBinary(t *testing.T) {
	bin, home, env := statusVocabFixture(t, wontfixConfig)

	runTLCOK(t, bin, home, env, "task", "create", "block me", "--status", "OPEN")
	runTLCOK(t, bin, home, env, "task", "update", "T-0001", "--status", "ACTIVE")

	out, code := runTLC(t, bin, home, env, "task", "block", "T-0001", "--note", "waiting on vendor SLA")
	if code != 0 {
		t.Fatalf("task block failed (exit %d):\n%s", code, out)
	}

	shown := runTLCOK(t, bin, home, env, "task", "show", "T-0001")
	if !strings.Contains(shown, "waiting on vendor SLA") {
		t.Errorf("task show should render the blocked reason:\n%s", shown)
	}
	// Blocking is orthogonal to status: ACTIVE must survive.
	if !strings.Contains(shown, "ACTIVE") && !strings.Contains(shown, "Active") {
		t.Errorf("block must not change status:\n%s", shown)
	}

	// unblock is annotated write-local, NOT destructive-local: it clears a
	// single blocked-reason string that the audit log already preserves,
	// and the flag it supersedes (task update --unblock) carries no gate.
	// A guard scripts route around protects nothing, so unblock must run
	// unprompted in a non-TTY. This test is a script: no --confirm.
	if out, code := runTLC(t, bin, home, env, "task", "unblock", "T-0001"); code != 0 {
		t.Fatalf("task unblock failed without --confirm (exit %d):\n%s", code, out)
	}
	cleared := runTLCOK(t, bin, home, env, "task", "show", "T-0001")
	if strings.Contains(cleared, "Blocked Reason:") {
		t.Errorf("task show should no longer render a blocked reason:\n%s", cleared)
	}
}

// TestUpdateBlockedFlagsStillWork is the escape-hatch contract from the
// scope boundary: adding commands must not remove the flags scripts use.
func TestUpdateBlockedFlagsStillWork(t *testing.T) {
	bin, home, env := statusVocabFixture(t, defaultStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "flag path")

	if out, code := runTLC(t, bin, home, env, "task", "update", "T-0001", "--blocked", "legacy flag reason"); code != 0 {
		t.Fatalf("task update --blocked failed (exit %d):\n%s", code, out)
	}
	shown := runTLCOK(t, bin, home, env, "task", "show", "T-0001")
	if !strings.Contains(shown, "legacy flag reason") {
		t.Errorf("--blocked should still set the reason:\n%s", shown)
	}

	if out, code := runTLC(t, bin, home, env, "task", "update", "T-0001", "--unblock"); code != 0 {
		t.Fatalf("task update --unblock failed (exit %d):\n%s", code, out)
	}
	cleared := runTLCOK(t, bin, home, env, "task", "show", "T-0001")
	if strings.Contains(cleared, "legacy flag reason") {
		t.Errorf("--unblock should still clear the reason:\n%s", cleared)
	}

	// And --status skipped, the pre-command way to reach the skip state.
	runTLCOK(t, bin, home, env, "task", "create", "status flag path")
	if out, code := runTLC(t, bin, home, env, "task", "update", "T-0002", "--status", "skipped"); code != 0 {
		t.Fatalf("task update --status skipped failed (exit %d):\n%s", code, out)
	}
	skipped := runTLCOK(t, bin, home, env, "task", "show", "T-0002")
	if !strings.Contains(skipped, "SKIPPED") && !strings.Contains(skipped, "Skipped") {
		t.Errorf("--status skipped should still work:\n%s", skipped)
	}
}
