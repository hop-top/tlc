package cli

import (
	"strings"
	"testing"

	"hop.top/tlc/internal/config"
)

const undeclaredDefaultStatusConfig = `storage:
  db_path: %s
task:
  default_status: TODO
`

const declaredDefaultStatusConfig = `storage:
  db_path: %s
task:
  default_status: backlog
`

// An undeclared default_status is a config error reported like any
// other: it names the key, the bad value and the valid statuses, and
// the command does not run.
func TestUndeclaredDefaultStatusFailsWithClearError(t *testing.T) {
	bin, home, env := statusVocabFixture(t, undeclaredDefaultStatusConfig)

	out, code := runTLC(t, bin, home, env, "task", "create", "probe")
	if code == 0 {
		t.Fatalf("task create should fail on an undeclared default_status:\n%s", out)
	}
	// Reported through the normal config-error path, not the process-
	// terminating DefaultWorkflow backstop.
	if strings.Contains(out, "invalid task workflow configuration") {
		t.Errorf("error came from the DefaultWorkflow backstop:\n%s", out)
	}
	for _, want := range []string{
		"task.default_status", `"TODO"`,
		"backlog, todo, in_progress, in_review, blocked, done, cancelled",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("error lacks %q:\n%s", want, out)
		}
	}
}

// The config commands stay usable so the user can fix the value.
func TestUndeclaredDefaultStatusLeavesConfigCommandsUsable(t *testing.T) {
	bin, home, env := statusVocabFixture(t, undeclaredDefaultStatusConfig)
	if out, code := runTLC(t, bin, home, env, "config", "set", "task.default_status", "todo"); code != 0 {
		t.Fatalf("config set should work to repair the value (exit %d):\n%s", code, out)
	}
}

func TestDeclaredDefaultStatusIsUsed(t *testing.T) {
	bin, home, env := statusVocabFixture(t, declaredDefaultStatusConfig)
	runTLCOK(t, bin, home, env, "task", "create", "parked")
	if shown := runTLCOK(t, bin, home, env, "task", "show", "T-0001"); !strings.Contains(shown, "Backlog") {
		t.Errorf("new task should land in the declared default_status backlog:\n%s", shown)
	}
}

func TestOmittedDefaultStatusUsesInitialRole(t *testing.T) {
	bin, home, env := statusVocabFixture(t, defaultStatusConfig)
	runTLCOK(t, bin, home, env, "task", "create", "fresh")
	if shown := runTLCOK(t, bin, home, env, "task", "show", "T-0001"); !strings.Contains(shown, "To Do") {
		t.Errorf("new task should land in the initial-role status:\n%s", shown)
	}
}

// The suite must never read the developer's real config or data dirs.
func TestSuiteUserDirsAreHermetic(t *testing.T) {
	cfgDir, err := config.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	for name, dir := range map[string]string{
		"config": cfgDir,
		"data":   config.UserDataDir(),
	} {
		if !strings.HasPrefix(dir, hermeticRoot) || hermeticRoot == "" {
			t.Errorf("%s dir %q is outside the suite's temp root %q", name, dir, hermeticRoot)
		}
	}
}
