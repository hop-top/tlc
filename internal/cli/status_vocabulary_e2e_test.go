package cli

// End-to-end coverage for the config-driven task-status vocabulary.
//
// Driven through a real config FILE and the real binary, deliberately.
// internal/core/workflow_test.go passed for months while `--status
// IN_REVIEW` was rejected in the field, because it built TaskConfig
// structs directly and called the workflow engine — bypassing the CLI's
// own status gate, which was the thing that was broken. A test that does
// not enter through the same door the user does cannot see that class of
// bug, so every case below spawns the binary with a config file.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// customStatusConfig declares TODO/IN_PROGRESS/IN_REVIEW/DONE/SKIPPED with
// rules that only permit the linear walk through review. IN_REVIEW is the
// status no built-in canon knows about.
const customStatusConfig = `storage:
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
    - name: IN_REVIEW
      label: In Review
      role: active
      tls_marker: "?"
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
      TODO: [IN_PROGRESS]
      IN_PROGRESS: [IN_REVIEW, DONE]
      IN_REVIEW: [DONE]
`

// renamedStatusConfig declares a vocabulary with NO IN_PROGRESS at all:
// TODO/IN_REVIEW/DONE, with IN_REVIEW carrying role "active".
//
// This is the shape that broke bare `task list`. The default status
// filter was the literal IN_PROGRESS + TODO, so on this config the tool
// rejected its own default with `unknown status "IN_PROGRESS"` — no
// unusual input required, the user typed nothing. Every other config in
// this file happens to declare IN_PROGRESS and so cannot see it.
const renamedStatusConfig = `storage:
  db_path: %s
task:
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

// twoActiveStatusConfig declares TWO statuses with role "active", DOING
// and IN_REVIEW, to pin the multiple-active-status decision: the default
// filter includes every active-role status, and the FIRST declared one
// leads the sort.
const twoActiveStatusConfig = `storage:
  db_path: %s
task:
  statuses:
    - name: TODO
      label: To Do
      role: initial
    - name: DOING
      label: Doing
      role: active
    - name: IN_REVIEW
      label: In Review
      role: active
    - name: DONE
      label: Done
      is_terminal: true
      role: completed
  state_machine:
    rules:
      TODO: [DOING]
      DOING: [IN_REVIEW, DONE]
      IN_REVIEW: [DONE]
`

// defaultStatusConfig declares no statuses at all, so the built-in
// vocabulary applies. Used to pin "unchanged config behaves as before".
const defaultStatusConfig = `storage:
  db_path: %s
`

// statusVocabEnv wires a subprocess run to a private HOME, a private DB, and a
// specific config file. TLC_CONFIG is the documented env equivalent of
// -c <path>; e2eEnv strips it from the inherited environment, so appending
// it here is what makes the config file the one under test rather than the
// developer's ambient one.
func statusVocabEnv(t *testing.T, home, dbPath, cfgPath string) []string {
	t.Helper()
	return append(e2eEnv(t, home, dbPath), "TLC_CONFIG="+cfgPath)
}

// writeVocabConfig materializes one of the templates above into home and
// returns the config path. The DB path is baked into the file rather than
// left to the ambient project registry: an unpinned probe resolves through
// the global registry, not the cwd, and silently reads an unrelated
// database.
func writeVocabConfig(t *testing.T, home, dbPath, template string) string {
	t.Helper()
	cfgPath := filepath.Join(home, "config.yaml")
	body := strings.Replace(template, "%s", dbPath, 1)
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return cfgPath
}

// statusVocabFixture builds the whole isolated world for one case and returns
// the binary, cwd, and env ready to run.
func statusVocabFixture(t *testing.T, template string) (bin, home string, env []string) {
	t.Helper()
	bin = buildTLCBinary(t)
	home = t.TempDir()
	dbPath := filepath.Join(home, "tasks.db")
	cfgPath := writeVocabConfig(t, home, dbPath, template)
	return bin, home, statusVocabEnv(t, home, dbPath, cfgPath)
}

// TestConfiguredStatusAccepted is the headline regression: a status that
// exists only in the user's config must be accepted by `task update`.
// Before the CLI's canon became config-driven this failed at
// NormalizeStatus with `unknown status "IN_REVIEW"`, never reaching the
// workflow engine that already understood the value.
func TestConfiguredStatusAccepted(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "review me")
	runTLCOK(t, bin, home, env, "task", "update", "T-0001", "--status", "IN_PROGRESS")

	out, code := runTLC(t, bin, home, env, "task", "update", "T-0001", "--status", "IN_REVIEW")
	if code != 0 {
		t.Fatalf("--status IN_REVIEW rejected (exit %d):\n%s", code, out)
	}

	// Assert the status actually landed, not merely that the command
	// exited 0: a normaliser that silently fuzzy-matched IN_REVIEW onto
	// IN_PROGRESS would also exit 0 here.
	shown := runTLCOK(t, bin, home, env, "task", "show", "T-0001")
	if !strings.Contains(shown, "In Review") && !strings.Contains(shown, "IN_REVIEW") {
		t.Errorf("task did not land in IN_REVIEW:\n%s", shown)
	}
}

// TestUnknownStatusNamesConfiguredSet pins the rejection message to the
// CONFIGURED vocabulary. A message still naming the built-in four would
// tell a user with IN_REVIEW declared that their own status is illegal.
func TestUnknownStatusNamesConfiguredSet(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	runTLCOK(t, bin, home, env, "task", "create", "bogus probe")

	out, code := runTLC(t, bin, home, env, "task", "update", "T-0001", "--status", "NOSUCHSTATUS")
	if code == 0 {
		t.Fatalf("--status NOSUCHSTATUS should have been rejected:\n%s", out)
	}
	if !strings.Contains(out, "IN_REVIEW") {
		t.Errorf("rejection should name the configured vocabulary including IN_REVIEW, got:\n%s", out)
	}
	for _, want := range []string{"TODO", "IN_PROGRESS", "DONE", "SKIPPED"} {
		if !strings.Contains(out, want) {
			t.Errorf("rejection should name %q, got:\n%s", want, out)
		}
	}
}

// TestStateMachineStillEnforced guards against the obvious wrong fix:
// opening the vocabulary must not open the transition rules with it. The
// config forbids TODO -> DONE, and that refusal has to survive.
func TestStateMachineStillEnforced(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	runTLCOK(t, bin, home, env, "task", "create", "rule probe")

	out, code := runTLC(t, bin, home, env, "task", "update", "T-0001", "--status", "DONE")
	if code == 0 {
		t.Fatalf("TODO -> DONE should be rejected by the state machine:\n%s", out)
	}
	if !strings.Contains(out, "transition") {
		t.Errorf("rejection should come from the state machine, got:\n%s", out)
	}
}

// TestDefaultVocabularyUnchanged is the no-regression contract: a config
// declaring no statuses must behave exactly as before, aliases and fuzzy
// matching included. This is what stops the config-driven canon from
// quietly costing existing users their shorthands.
func TestDefaultVocabularyUnchanged(t *testing.T) {
	bin, home, env := statusVocabFixture(t, defaultStatusConfig)

	cases := []struct {
		input string
		want  string
	}{
		{"wip", "IN_PROGRESS"},         // hand-written alias
		{"complete", "DONE"},           // hand-written alias
		{"inprog", "IN_PROGRESS"},      // fuzzy
		{"in-progress", "IN_PROGRESS"}, // derived variant
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			runTLCOK(t, bin, home, env, "task", "create", "alias "+tc.input)
			id := lastVocabTaskID(t, bin, home, env)

			out, code := runTLC(t, bin, home, env, "task", "update", id, "--status", tc.input, "--force")
			if code != 0 {
				t.Fatalf("--status %s rejected (exit %d):\n%s", tc.input, code, out)
			}
			shown := runTLCOK(t, bin, home, env, "task", "show", id)
			if !strings.Contains(shown, tc.want) && !strings.Contains(shown, statusVocabLabel(tc.want)) {
				t.Errorf("--status %s should resolve to %s, got:\n%s", tc.input, tc.want, shown)
			}
		})
	}

	// The lowercase list filter is the other half of the old behavior.
	out, code := runTLC(t, bin, home, env, "task", "list", "--status", "todo")
	if code != 0 {
		t.Errorf("task list --status todo should work (exit %d):\n%s", code, out)
	}

	// And the rejection message must still name exactly the built-in four.
	rej, _ := runTLC(t, bin, home, env, "task", "update", "T-0001", "--status", "BOGUS")
	if !strings.Contains(rej, "TODO, IN_PROGRESS, DONE, SKIPPED") {
		t.Errorf("default rejection should name the built-in four verbatim, got:\n%s", rej)
	}
	if strings.Contains(rej, "IN_REVIEW") {
		t.Errorf("default vocabulary leaked a custom status:\n%s", rej)
	}
}

// TestConfiguredStatusInHelpAndCompletion covers the discoverability half.
// kit stamps flag enums in prepareTree, before argv is parsed and long
// before any config file is read, so the configured vocabulary reaches
// --help and the shell only via the post-config restamp. Without it a user
// is told their own status is not a legal value for the flag.
func TestConfiguredStatusInHelpAndCompletion(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)

	help := runTLCOK(t, bin, home, env, "task", "update", "--help")
	if !strings.Contains(help, "IN_REVIEW") {
		t.Errorf("--help should advertise the configured vocabulary, got:\n%s", help)
	}
	if strings.Contains(help, "one of: TODO, IN_PROGRESS, DONE, SKIPPED") {
		t.Errorf("--help still shows the pre-config built-in set:\n%s", help)
	}

	comp := runTLCOK(t, bin, home, env, "__complete", "task", "update", "--status", "")
	if !strings.Contains(comp, "IN_REVIEW") {
		t.Errorf("completion should offer IN_REVIEW, got:\n%s", comp)
	}

	// Track statuses are a separate flag with a separate vocabulary and
	// are explicitly out of scope; assert the restamp did not reach them.
	trackHelp := runTLCOK(t, bin, home, env, "track", "update", "--help")
	if strings.Contains(trackHelp, "IN_REVIEW") {
		t.Errorf("task vocabulary leaked onto track --status:\n%s", trackHelp)
	}
}

// statusVocabLabel maps a canonical status to the human label the table and
// detail views render, so assertions can accept either spelling.
func statusVocabLabel(status string) string {
	switch status {
	case "TODO":
		return "To Do"
	case "IN_PROGRESS":
		return "In Progress"
	case "DONE":
		return "Done"
	case "SKIPPED":
		return "Skipped"
	default:
		return status
	}
}

// lastVocabTaskID returns the highest-numbered task id present, so a subtest can
// address the task it just created without assuming a fixed sequence.
func lastVocabTaskID(t *testing.T, bin, cwd string, env []string) string {
	t.Helper()
	out := runTLCOK(t, bin, cwd, env, "task", "list", "--status", "TODO")
	last := ""
	for _, line := range strings.Split(out, "\n") {
		for _, field := range strings.Fields(line) {
			if strings.HasPrefix(field, "T-") && field > last {
				last = field
			}
		}
	}
	if last == "" {
		t.Fatalf("no task id found in:\n%s", out)
	}
	return last
}

// TestBareListWorksWithoutInProgress is the P0 regression: bare
// `task list` — no flags at all — must work on a vocabulary that declares
// no IN_PROGRESS.
//
// The literal default filter (IN_PROGRESS + TODO) failed this outright:
// the filter named a status the config did not declare, so the CLI's own
// status gate rejected the CLI's own default with
// `unknown status "IN_PROGRESS"`. Creating the task worked; only listing
// it failed. That is what made it P0 — every other bug in this class
// needed the user to type something unusual, this one needed them to
// type nothing.
func TestBareListWorksWithoutInProgress(t *testing.T) {
	bin, home, env := statusVocabFixture(t, renamedStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "still open")
	runTLCOK(t, bin, home, env, "task", "create", "under review")
	runTLCOK(t, bin, home, env, "task", "update", "T-0002", "--status", "IN_REVIEW")

	out, code := runTLC(t, bin, home, env, "task", "list")
	if code != 0 {
		t.Fatalf("bare `task list` failed on a vocabulary without IN_PROGRESS (exit %d):\n%s", code, out)
	}
	// Assert the unfinished work is actually PRESENT, not merely that the
	// command exited 0: a default filter that resolved to nothing at all
	// would also exit 0, and would be just as broken.
	for _, want := range []string{"T-0001", "T-0002"} {
		if !strings.Contains(out, want) {
			t.Errorf("default listing should include unfinished task %s, got:\n%s", want, out)
		}
	}
}

// TestBareGraphWorksWithoutInProgress is the same regression for
// `task graph`, whose failure mode was quieter and therefore worse:
// graph pushed the literal into the filter WITHOUT normalising it, so it
// exited 0 and simply omitted the user's in-flight work. A user would
// see a plausible graph that was missing rows.
func TestBareGraphWorksWithoutInProgress(t *testing.T) {
	bin, home, env := statusVocabFixture(t, renamedStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "still open")
	runTLCOK(t, bin, home, env, "task", "create", "under review")
	runTLCOK(t, bin, home, env, "task", "update", "T-0002", "--status", "IN_REVIEW")

	out, code := runTLC(t, bin, home, env, "task", "graph")
	if code != 0 {
		t.Fatalf("bare `task graph` failed on a vocabulary without IN_PROGRESS (exit %d):\n%s", code, out)
	}
	if !strings.Contains(out, "T-0002") {
		t.Errorf("graph silently omitted the active-role task T-0002:\n%s", out)
	}
	if !strings.Contains(out, "T-0001") {
		t.Errorf("graph omitted the initial-role task T-0001:\n%s", out)
	}
}

// TestDefaultFilterExcludesFinishedWork pins the other half of the
// derivation. "Unfinished" must still EXCLUDE completed work — a default
// that resolved to the whole vocabulary would pass the two tests above
// while quietly changing what `task list` means.
func TestDefaultFilterExcludesFinishedWork(t *testing.T) {
	bin, home, env := statusVocabFixture(t, renamedStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "open one")
	runTLCOK(t, bin, home, env, "task", "create", "finished one")
	runTLCOK(t, bin, home, env, "task", "update", "T-0002", "--status", "IN_REVIEW")
	runTLCOK(t, bin, home, env, "task", "update", "T-0002", "--status", "DONE")

	out := runTLCOK(t, bin, home, env, "task", "list")
	if strings.Contains(out, "T-0002") {
		t.Errorf("default listing must exclude completed-role work, got:\n%s", out)
	}
	if !strings.Contains(out, "T-0001") {
		t.Errorf("default listing should still include unfinished work, got:\n%s", out)
	}
}

// TestDefaultFilterIncludesEveryActiveStatus pins the multiple-active
// decision: a vocabulary declaring two active-role statuses shows BOTH
// by default.
//
// Deriving from WorkflowManager.StatusForRole would fail this — its
// roleIndex keeps only the FIRST status per role, which is right for
// picking a transition target and wrong for describing a filter set.
// Hiding the second active status would be the same config-blind bug as
// the literal, one config edit further along.
func TestDefaultFilterIncludesEveryActiveStatus(t *testing.T) {
	bin, home, env := statusVocabFixture(t, twoActiveStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "waiting")
	runTLCOK(t, bin, home, env, "task", "create", "in flight")
	runTLCOK(t, bin, home, env, "task", "create", "in review")
	runTLCOK(t, bin, home, env, "task", "create", "shipped")
	runTLCOK(t, bin, home, env, "task", "update", "T-0002", "--status", "DOING")
	runTLCOK(t, bin, home, env, "task", "update", "T-0003", "--status", "DOING")
	runTLCOK(t, bin, home, env, "task", "update", "T-0003", "--status", "IN_REVIEW")
	runTLCOK(t, bin, home, env, "task", "update", "T-0004", "--status", "DOING")
	runTLCOK(t, bin, home, env, "task", "update", "T-0004", "--status", "DONE")

	out := runTLCOK(t, bin, home, env, "task", "list")
	for _, want := range []string{"T-0001", "T-0002", "T-0003"} {
		if !strings.Contains(out, want) {
			t.Errorf("default listing should include unfinished task %s, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "T-0004") {
		t.Errorf("default listing must exclude the completed task T-0004, got:\n%s", out)
	}

	// The FIRST declared active status leads the sort. Declaration order
	// is lifecycle order everywhere else in this schema, so the earliest
	// active status is the one nearest "being worked on right now".
	if idx := statusVocabRowOrder(out, "T-0002", "T-0003"); idx >= 0 {
		t.Errorf("DOING (first declared active) should sort before IN_REVIEW, got:\n%s", out)
	}
}

// TestDefaultSortPutsActiveFirst pins the StatusPriority half of the
// derivation under the BUILT-IN vocabulary, where the replaced literal
// hardcoded IN_PROGRESS. The titles are chosen so alphabetical order and
// status order disagree: a sort that quietly stopped happening would
// otherwise be invisible.
func TestDefaultSortPutsActiveFirst(t *testing.T) {
	bin, home, env := statusVocabFixture(t, defaultStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "aaa waiting")
	runTLCOK(t, bin, home, env, "task", "create", "zzz working")
	runTLCOK(t, bin, home, env, "task", "update", "T-0002", "--status", "IN_PROGRESS")

	out := runTLCOK(t, bin, home, env, "task", "list")
	if idx := statusVocabRowOrder(out, "T-0002", "T-0001"); idx >= 0 {
		t.Errorf("IN_PROGRESS task should sort first by default, got:\n%s", out)
	}
}

// TestGraphRejectsUndeclaredStatus covers the explicit-flag half for
// graph. It previously pushed --status through raw: an undeclared status
// produced "No tasks found." — indistinguishable from a genuinely empty
// result — and aliases and lowercase input silently matched nothing.
func TestGraphRejectsUndeclaredStatus(t *testing.T) {
	bin, home, env := statusVocabFixture(t, renamedStatusConfig)
	runTLCOK(t, bin, home, env, "task", "create", "probe")

	out, code := runTLC(t, bin, home, env, "task", "graph", "--status", "IN_PROGRESS")
	if code == 0 {
		t.Fatalf("graph --status IN_PROGRESS should be rejected on a vocabulary without it:\n%s", out)
	}
	if !strings.Contains(out, "IN_REVIEW") {
		t.Errorf("rejection should name the configured vocabulary, got:\n%s", out)
	}

	// The alias/casing path the raw pass-through dropped.
	if out, code := runTLC(t, bin, home, env, "task", "graph", "--status", "todo"); code != 0 {
		t.Errorf("graph --status todo should resolve via the normaliser (exit %d):\n%s", code, out)
	} else if !strings.Contains(out, "T-0001") {
		t.Errorf("graph --status todo should match the TODO task, got:\n%s", out)
	}
}

// statusVocabRowOrder returns -1 when first appears before second in the
// rendered table, and a non-negative value otherwise, so a caller can
// assert ordering without hand-rolling index arithmetic per test.
func statusVocabRowOrder(out, first, second string) int {
	fi := strings.Index(out, first)
	si := strings.Index(out, second)
	if fi < 0 || si < 0 {
		return 1
	}
	if fi < si {
		return -1
	}
	return 1
}
