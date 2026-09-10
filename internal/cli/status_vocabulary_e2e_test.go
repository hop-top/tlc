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

// writeVocabConfig materialises one of the templates above into home and
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

	// The lowercase list filter is the other half of the old behaviour.
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
