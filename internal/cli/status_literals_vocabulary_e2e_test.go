package cli

// End-to-end coverage for the three remaining config-blind status
// literals: `task stale`, `tlc status`, and `tlc tag`'s default filter.
//
// These three fail SILENTLY, unlike the `task list` P0 that failed loudly
// with `unknown status "IN_PROGRESS"`. Each pushes a hardcoded status name
// into a filter that is never normalised, so on a vocabulary declaring no
// IN_PROGRESS the filter simply matches nothing: the command exits 0 and
// reports an empty result the user has no reason to doubt. "No stale
// tasks." and "0 in progress, 0 todo" are both read as facts.
//
// That shape dictates how every case below asserts. An exit-code check
// alone passes against the broken build — the broken build exits 0. So
// every assertion here is on CONTENT: the task must actually appear, or
// the count must actually be non-zero. A test that only checked `code != 0`
// would have been green throughout the defect's entire life.
//
// Driven through the real binary and a real config file for the reason
// status_vocabulary_e2e_test.go spells out: a test that does not enter
// through the same door the user does cannot see this class of bug.

import (
	"fmt"
	"strings"
	"testing"
)

// staleRenamedStatusConfig is renamedStatusConfig (TODO/IN_REVIEW/DONE,
// no IN_PROGRESS at all) plus a stale timeout short enough that a task
// created by the test is immediately stale.
//
// The two template holes are the DB path and nothing else; the timeout is
// fixed, because a nanosecond threshold makes "is it stale yet" a
// question about the vocabulary rather than about the clock.
const staleRenamedStatusConfig = `storage:
  db_path: %s
task:
  stale:
    default_timeout: 1ns
  statuses:
    - name: TODO
      label: To Do
      role: initial
    - name: IN_REVIEW
      label: In Review
      role: active
    - name: DONE
      label: Done
      is_terminal: true
      role: completed
  state_machine:
    rules:
      TODO: [IN_REVIEW]
      IN_REVIEW: [DONE]
`

// staleDefaultStatusConfig is the built-in vocabulary with the same short
// timeout, for the default-unchanged half of each case.
const staleDefaultStatusConfig = `storage:
  db_path: %s
task:
  stale:
    default_timeout: 1ns
`

// vocabFixtureAs builds the same isolated world statusVocabFixture does
// and additionally pins TLC_USER, so `tlc status` — which counts only
// tasks assigned to the caller — has a stable identity to count against.
// Without it the counts depend on the developer's ambient username.
func vocabFixtureAs(t *testing.T, template, user string) (bin, home string, env []string) {
	t.Helper()
	bin, home, env = statusVocabFixture(t, template)
	return bin, home, append(env, "TLC_USER="+user)
}

const vocabProbeUser = "vocab-probe-owner"

// TestStaleFindsActiveRoleTaskWithoutInProgress is the `task stale`
// regression. The literal filter (IN_PROGRESS + TODO) matched no row on a
// vocabulary declaring neither active name, so the command reported "No
// stale tasks." over a store holding a stale one.
//
// The assertion is that the task is PRESENT in the output. Asserting only
// exit 0 would pass against the defect, which exits 0 by construction —
// that is the whole defect.
func TestStaleFindsActiveRoleTaskWithoutInProgress(t *testing.T) {
	bin, home, env := statusVocabFixture(t, staleRenamedStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "under review")
	runTLCOK(t, bin, home, env, "task", "update", "T-0001", "--status", "IN_REVIEW")

	out, code := runTLC(t, bin, home, env, "task", "stale")
	if code != 0 {
		t.Fatalf("`task stale` failed on a vocabulary without IN_PROGRESS (exit %d):\n%s", code, out)
	}
	if strings.Contains(out, "No stale tasks.") {
		t.Fatalf("`task stale` reported no stale tasks while an IN_REVIEW task was stale:\n%s", out)
	}
	if !strings.Contains(out, "T-0001") {
		t.Errorf("stale listing should include the stale IN_REVIEW task T-0001, got:\n%s", out)
	}
}

// TestStaleStillExcludesFinishedWork pins the other half of the
// derivation. A fix that widened the filter to the whole vocabulary would
// pass the test above while quietly making finished work "stale" —
// staleness is a statement about work that has not finished.
func TestStaleStillExcludesFinishedWork(t *testing.T) {
	bin, home, env := statusVocabFixture(t, staleRenamedStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "open one")
	runTLCOK(t, bin, home, env, "task", "create", "finished one")
	runTLCOK(t, bin, home, env, "task", "update", "T-0002", "--status", "IN_REVIEW")
	runTLCOK(t, bin, home, env, "task", "update", "T-0002", "--status", "DONE")

	out := runTLCOK(t, bin, home, env, "task", "stale")
	if strings.Contains(out, "T-0002") {
		t.Errorf("a DONE task must never be stale, got:\n%s", out)
	}
	if !strings.Contains(out, "T-0001") {
		t.Errorf("stale listing should still include the unfinished task, got:\n%s", out)
	}
}

// TestStaleDefaultVocabularyUnchanged is the no-regression contract for
// `task stale` under the built-in vocabulary, where the replaced literal
// named exactly the two statuses the role derivation now yields.
func TestStaleDefaultVocabularyUnchanged(t *testing.T) {
	bin, home, env := statusVocabFixture(t, staleDefaultStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "waiting")
	runTLCOK(t, bin, home, env, "task", "create", "working")
	runTLCOK(t, bin, home, env, "task", "create", "shipped")
	runTLCOK(t, bin, home, env, "task", "update", "T-0002", "--status", "IN_PROGRESS")
	// The built-in state machine forbids TODO -> DONE, so walk it.
	runTLCOK(t, bin, home, env, "task", "update", "T-0003", "--status", "IN_PROGRESS")
	runTLCOK(t, bin, home, env, "task", "update", "T-0003", "--status", "DONE")

	out := runTLCOK(t, bin, home, env, "task", "stale")
	for _, want := range []string{"T-0001", "T-0002"} {
		if !strings.Contains(out, want) {
			t.Errorf("default stale listing should include unfinished task %s, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "T-0003") {
		t.Errorf("default stale listing must exclude the DONE task, got:\n%s", out)
	}
}

// TestStatusCountsActiveRoleWithoutInProgress is the `tlc status`
// regression. mineCounts was keyed by the literal names, so on a
// vocabulary declaring neither the map lookups missed and the snapshot
// printed "0 in progress, 0 todo" over a store full of open work.
//
// Both numbers are asserted non-zero, and the in-progress row is asserted
// present: a fix that counted the task but dropped it from the preview
// list would leave the two halves of the snapshot contradicting.
func TestStatusCountsActiveRoleWithoutInProgress(t *testing.T) {
	bin, home, env := vocabFixtureAs(t, renamedStatusConfig, vocabProbeUser)

	runTLCOK(t, bin, home, env, "task", "create", "still open",
		"--assigned-to", vocabProbeUser)
	runTLCOK(t, bin, home, env, "task", "create", "under review",
		"--assigned-to", vocabProbeUser)
	runTLCOK(t, bin, home, env, "task", "update", "T-0002", "--status", "IN_REVIEW")

	out, code := runTLC(t, bin, home, env, "status")
	if code != 0 {
		t.Fatalf("`tlc status` failed on a vocabulary without IN_PROGRESS (exit %d):\n%s", code, out)
	}

	inProgress, todo := parseStatusTaskCounts(t, out)
	if inProgress != 1 {
		t.Errorf("in progress = %d, want 1 (the IN_REVIEW task carries role active):\n%s",
			inProgress, out)
	}
	if todo != 1 {
		t.Errorf("todo = %d, want 1 (the TODO task carries role initial):\n%s", todo, out)
	}

	// The inline preview must agree with the count it sits under.
	if !strings.Contains(out, "In progress:") || !strings.Contains(out, "T-0002") {
		t.Errorf("the active-role task should appear in the inline preview, got:\n%s", out)
	}
}

// TestStatusCountsEveryActiveStatus pins the set-not-first-match half for
// `tlc status`: a vocabulary declaring two active-role statuses has BOTH
// counted, exactly as UnfinishedTaskStatuses includes both. Counting only
// the first would be the same config-blind bug one config edit further on.
func TestStatusCountsEveryActiveStatus(t *testing.T) {
	bin, home, env := vocabFixtureAs(t, twoActiveStatusConfig, vocabProbeUser)

	for _, title := range []string{"waiting", "in flight", "in review", "shipped"} {
		runTLCOK(t, bin, home, env, "task", "create", title, "--assigned-to", vocabProbeUser)
	}
	runTLCOK(t, bin, home, env, "task", "update", "T-0002", "--status", "DOING")
	runTLCOK(t, bin, home, env, "task", "update", "T-0003", "--status", "DOING")
	runTLCOK(t, bin, home, env, "task", "update", "T-0003", "--status", "IN_REVIEW")
	runTLCOK(t, bin, home, env, "task", "update", "T-0004", "--status", "DOING")
	runTLCOK(t, bin, home, env, "task", "update", "T-0004", "--status", "DONE")

	out := runTLCOK(t, bin, home, env, "status")
	inProgress, todo := parseStatusTaskCounts(t, out)
	if inProgress != 2 {
		t.Errorf("in progress = %d, want 2 (DOING + IN_REVIEW both carry role active):\n%s",
			inProgress, out)
	}
	if todo != 1 {
		t.Errorf("todo = %d, want 1:\n%s", todo, out)
	}
	// The completed task must not be counted into either bucket.
	if strings.Contains(out, "T-0004") {
		t.Errorf("the completed task leaked into the snapshot:\n%s", out)
	}
}

// TestStatusDefaultVocabularyUnchanged is the no-regression contract for
// `tlc status`: under the built-in vocabulary the role buckets must
// resolve to exactly the two names the literals hardcoded.
func TestStatusDefaultVocabularyUnchanged(t *testing.T) {
	bin, home, env := vocabFixtureAs(t, defaultStatusConfig, vocabProbeUser)

	for _, title := range []string{"waiting", "working", "shipped"} {
		runTLCOK(t, bin, home, env, "task", "create", title, "--assigned-to", vocabProbeUser)
	}
	runTLCOK(t, bin, home, env, "task", "update", "T-0002", "--status", "IN_PROGRESS")
	// The built-in state machine forbids TODO -> DONE, so walk it.
	runTLCOK(t, bin, home, env, "task", "update", "T-0003", "--status", "IN_PROGRESS")
	runTLCOK(t, bin, home, env, "task", "update", "T-0003", "--status", "DONE")

	out := runTLCOK(t, bin, home, env, "status")
	inProgress, todo := parseStatusTaskCounts(t, out)
	if inProgress != 1 {
		t.Errorf("in progress = %d, want 1:\n%s", inProgress, out)
	}
	if todo != 1 {
		t.Errorf("todo = %d, want 1 (DONE must not count):\n%s", todo, out)
	}
}

// TestTagFilterFindsActiveRoleTaskWithoutInProgress is the `tlc tag`
// filter regression. Its literals were RAW STRINGS rather than the core
// constants, so they were config-blind twice over, and the failure was
// the same silent empty result.
func TestTagFilterFindsActiveRoleTaskWithoutInProgress(t *testing.T) {
	bin, home, env := statusVocabFixture(t, renamedStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "under review", "--tag", "probe")
	runTLCOK(t, bin, home, env, "task", "update", "T-0001", "--status", "IN_REVIEW")

	out, code := runTLC(t, bin, home, env, "tag", "probe")
	if code != 0 {
		t.Fatalf("`tlc tag probe` failed on a vocabulary without IN_PROGRESS (exit %d):\n%s", code, out)
	}
	if !strings.Contains(out, "T-0001") {
		t.Errorf("tag filter should include the tagged IN_REVIEW task, got:\n%s", out)
	}
}

// TestTagFilterExcludesFinishedWorkByDefault pins the other half: the
// default filter still means "unfinished", and --all-statuses is what
// widens it.
func TestTagFilterExcludesFinishedWorkByDefault(t *testing.T) {
	bin, home, env := statusVocabFixture(t, renamedStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "open one", "--tag", "probe")
	runTLCOK(t, bin, home, env, "task", "create", "finished one", "--tag", "probe")
	runTLCOK(t, bin, home, env, "task", "update", "T-0002", "--status", "IN_REVIEW")
	runTLCOK(t, bin, home, env, "task", "update", "T-0002", "--status", "DONE")

	out := runTLCOK(t, bin, home, env, "tag", "probe")
	if strings.Contains(out, "T-0002") {
		t.Errorf("default tag filter must exclude completed work, got:\n%s", out)
	}
	if !strings.Contains(out, "T-0001") {
		t.Errorf("default tag filter should include unfinished work, got:\n%s", out)
	}

	all := runTLCOK(t, bin, home, env, "tag", "--all-statuses", "probe")
	if !strings.Contains(all, "T-0002") {
		t.Errorf("--all-statuses should include the completed task, got:\n%s", all)
	}
}

// TestTagHelpNamesNoStatuses pins the prose half. A help string listing
// statuses the user's config does not declare is its own small lie, and
// the flag is registered at package init — before any config is read — so
// it can only ever name the statuses this package happens to know.
func TestTagHelpNamesNoStatuses(t *testing.T) {
	bin, home, env := statusVocabFixture(t, renamedStatusConfig)

	help := runTLCOK(t, bin, home, env, "tag", "--help")
	for _, leaked := range []string{"IN_PROGRESS", "TODO"} {
		if strings.Contains(help, leaked) {
			t.Errorf("`tlc tag --help` names %q, a status this config does not declare:\n%s",
				leaked, help)
		}
	}
	if !strings.Contains(help, "--all-statuses") {
		t.Fatalf("`tlc tag --help` did not render the flag at all:\n%s", help)
	}
}

// TestTagFilterDefaultVocabularyUnchanged is the no-regression contract
// for the tag filter under the built-in vocabulary.
func TestTagFilterDefaultVocabularyUnchanged(t *testing.T) {
	bin, home, env := statusVocabFixture(t, defaultStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "waiting", "--tag", "probe")
	runTLCOK(t, bin, home, env, "task", "create", "working", "--tag", "probe")
	runTLCOK(t, bin, home, env, "task", "create", "shipped", "--tag", "probe")
	runTLCOK(t, bin, home, env, "task", "update", "T-0002", "--status", "IN_PROGRESS")
	// The built-in state machine forbids TODO -> DONE, so walk it.
	runTLCOK(t, bin, home, env, "task", "update", "T-0003", "--status", "IN_PROGRESS")
	runTLCOK(t, bin, home, env, "task", "update", "T-0003", "--status", "DONE")

	out := runTLCOK(t, bin, home, env, "tag", "probe")
	for _, want := range []string{"T-0001", "T-0002"} {
		if !strings.Contains(out, want) {
			t.Errorf("default tag filter should include unfinished task %s, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "T-0003") {
		t.Errorf("default tag filter must exclude the DONE task, got:\n%s", out)
	}
}

// parseStatusTaskCounts extracts the two numbers from the `tlc status`
// Tasks line, failing the test when the line is absent or unparseable —
// a snapshot missing its counts is itself the regression.
func parseStatusTaskCounts(t *testing.T, out string) (inProgress, todo int) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "in progress") {
			continue
		}
		if _, err := fmt.Sscanf(strings.TrimSpace(line),
			"Tasks: %d in progress, %d todo (yours)", &inProgress, &todo); err != nil {
			t.Fatalf("parse %q: %v", line, err)
		}
		return inProgress, todo
	}
	t.Fatalf("no Tasks count line in `tlc status` output:\n%s", out)
	return 0, 0
}
