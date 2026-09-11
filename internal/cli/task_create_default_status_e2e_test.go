package cli

// Where `tlc task create` gets its status when the user names none.
//
// Driven through a real config FILE and the real binary, deliberately.
// The defect this file pins was invisible to struct-level tests: the
// flag default lived in a cobra registration, so only a run that parses
// argv the way a user does can see that "TODO" was supplied on every
// invocation and outranked task.default_status. A test constructing a
// TaskConfig and calling the workflow engine would have passed against
// the broken binary — which is exactly how the original config-wiring
// bug shipped green for months.
//
// Every custom vocabulary below contains no status named TODO, so a
// hardcoded constant has nowhere to hide.

import (
	"path/filepath"
	"strings"
	"testing"
)

// renamedVocabConfig renames all four statuses and nominates OPEN via
// task.default_status. Nothing is called TODO, so the old hardcoded flag
// default fails here with "unknown status TODO".
const renamedVocabConfig = `storage:
  db_path: %s
task:
  default_status: OPEN
  statuses:
    - name: OPEN
      label: Open
      role: initial
      tls_marker: " "
    - name: WORKING
      label: Working
      role: active
      tls_marker: ">"
    - name: SHIPPED
      label: Shipped
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
      OPEN: [WORKING, WONTFIX]
      WORKING: [SHIPPED, OPEN]
`

// renamedVocabNoDefaultConfig is the same vocabulary with
// task.default_status omitted entirely, which is the common shape: the
// key is optional and most configs never write it. Resolution must fall
// through to the initial-role status rather than failing.
const renamedVocabNoDefaultConfig = `storage:
  db_path: %s
task:
  statuses:
    - name: OPEN
      label: Open
      role: initial
      tls_marker: " "
    - name: WORKING
      label: Working
      role: active
      tls_marker: ">"
    - name: SHIPPED
      label: Shipped
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
      OPEN: [WORKING, WONTFIX]
      WORKING: [SHIPPED, OPEN]
`

// twoInitialConfig declares two statuses with role "initial" and picks
// the second via default_status. This is the case that proves the config
// value is READ rather than merely coinciding with the role fallback:
// role resolution alone would answer TRIAGE, so landing in BACKLOG can
// only come from task.default_status.
const twoInitialConfig = `storage:
  db_path: %s
task:
  default_status: BACKLOG
  statuses:
    - name: TRIAGE
      label: Triage
      role: initial
      tls_marker: "?"
    - name: BACKLOG
      label: Backlog
      role: initial
      tls_marker: " "
    - name: WORKING
      label: Working
      role: active
      tls_marker: ">"
    - name: SHIPPED
      label: Shipped
      is_terminal: true
      role: completed
      tls_marker: "x"
  state_machine:
    rules:
      TRIAGE: [BACKLOG, WORKING]
      BACKLOG: [WORKING]
      WORKING: [SHIPPED]
`

// createdStatus returns the rendered status of T-0001, so each case
// asserts WHERE the task landed rather than merely that create exited 0.
// A create that silently kept a wrong status would also exit 0.
func createdStatus(t *testing.T, bin, home string, env []string) string {
	t.Helper()
	return runTLCOK(t, bin, home, env, "task", "show", "T-0001")
}

// TestCreateHonoursConfiguredDefaultStatus is the headline regression:
// under a fully-renamed vocabulary, a bare create must succeed and land
// in the configured default. Against the hardcoded flag default this
// fails at the normaliser with `unknown status "TODO"`, because the user
// declared no such status — create was rejected by the user's own config.
func TestCreateHonoursConfiguredDefaultStatus(t *testing.T) {
	bin, home, env := statusVocabFixture(t, renamedVocabConfig)

	out, code := runTLC(t, bin, home, env, "task", "create", "bare create")
	if code != 0 {
		t.Fatalf("bare create rejected under a renamed vocabulary (exit %d):\n%s", code, out)
	}

	shown := createdStatus(t, bin, home, env)
	if !strings.Contains(shown, "OPEN") && !strings.Contains(shown, "Open") {
		t.Errorf("task did not land in the configured default_status OPEN:\n%s", shown)
	}
}

// TestCreateFallsBackToInitialRole covers the omitted-key path: with no
// default_status at all, the initial-role status answers. A fix that only
// read task.default_status would fail here.
func TestCreateFallsBackToInitialRole(t *testing.T) {
	bin, home, env := statusVocabFixture(t, renamedVocabNoDefaultConfig)

	out, code := runTLC(t, bin, home, env, "task", "create", "no default key")
	if code != 0 {
		t.Fatalf("bare create failed with default_status omitted (exit %d):\n%s", code, out)
	}

	shown := createdStatus(t, bin, home, env)
	if !strings.Contains(shown, "OPEN") && !strings.Contains(shown, "Open") {
		t.Errorf("task did not land in the initial-role status OPEN:\n%s", shown)
	}
}

// TestCreateDefaultStatusOutranksInitialRole pins that the config value
// is genuinely consulted. With two initial-role statuses, role resolution
// would pick TRIAGE; only reading task.default_status yields BACKLOG.
// This is the case that fails if the default_status consumer is removed
// while the role fallback stays.
func TestCreateDefaultStatusOutranksInitialRole(t *testing.T) {
	bin, home, env := statusVocabFixture(t, twoInitialConfig)

	out, code := runTLC(t, bin, home, env, "task", "create", "two initials")
	if code != 0 {
		t.Fatalf("bare create failed (exit %d):\n%s", code, out)
	}

	shown := createdStatus(t, bin, home, env)
	if !strings.Contains(shown, "BACKLOG") && !strings.Contains(shown, "Backlog") {
		t.Errorf("default_status BACKLOG was ignored in favor of the first initial-role status:\n%s", shown)
	}
	if strings.Contains(shown, "TRIAGE") || strings.Contains(shown, "Triage") {
		t.Errorf("task landed in the role-resolved status, so default_status was not read:\n%s", shown)
	}
}

// TestCreateExplicitStatusWins guards the other direction: making the
// default configurable must not disarm the flag. Scripts pass --status.
func TestCreateExplicitStatusWins(t *testing.T) {
	bin, home, env := statusVocabFixture(t, renamedVocabConfig)

	out, code := runTLC(t, bin, home, env, "task", "create", "explicit", "--status", "WORKING")
	if code != 0 {
		t.Fatalf("--status WORKING rejected (exit %d):\n%s", code, out)
	}

	shown := createdStatus(t, bin, home, env)
	if !strings.Contains(shown, "WORKING") && !strings.Contains(shown, "Working") {
		t.Errorf("explicit --status did not win over the configured default:\n%s", shown)
	}
	if strings.Contains(shown, "Open") {
		t.Errorf("explicit --status was overridden by the configured default:\n%s", shown)
	}
}

// TestCreateRejectsUnknownStatus pins that an empty flag default did not
// turn the validation off with it. The rejection must still name the
// CONFIGURED vocabulary, not the built-in four.
func TestCreateRejectsUnknownStatus(t *testing.T) {
	bin, home, env := statusVocabFixture(t, renamedVocabConfig)

	out, code := runTLC(t, bin, home, env, "task", "create", "bogus", "--status", "NOSUCH")
	if code == 0 {
		t.Fatalf("--status NOSUCH should be rejected:\n%s", out)
	}
	if !strings.Contains(out, "NOSUCH") {
		t.Errorf("rejection should name the offending value, got:\n%s", out)
	}
	for _, want := range []string{"OPEN", "WORKING", "SHIPPED", "WONTFIX"} {
		if !strings.Contains(out, want) {
			t.Errorf("rejection should list the configured vocabulary (missing %s), got:\n%s", want, out)
		}
	}
}

// TestCreateDefaultConfigUnchanged is the no-regression case: a config
// that declares no statuses must behave exactly as before, landing in
// TODO. This is what stops the fix from being a behavior change for
// every existing user.
func TestCreateDefaultConfigUnchanged(t *testing.T) {
	bin, home, env := statusVocabFixture(t, defaultStatusConfig)

	out, code := runTLC(t, bin, home, env, "task", "create", "plain")
	if code != 0 {
		t.Fatalf("bare create failed under the default config (exit %d):\n%s", code, out)
	}

	shown := createdStatus(t, bin, home, env)
	if !strings.Contains(shown, "TODO") && !strings.Contains(shown, "To Do") {
		t.Errorf("default config no longer creates into TODO:\n%s", shown)
	}
}

// TestCreateNoPhantomDefaultStatusWarning pins the second half of the
// defect. viper.SetDefault seeded task.default_status="TODO" into every
// merged config, so a user who renamed the vocabulary and never wrote the
// key still failed validation against a value they never chose.
func TestCreateNoPhantomDefaultStatusWarning(t *testing.T) {
	bin, home, env := statusVocabFixture(t, renamedVocabNoDefaultConfig)

	out, code := runTLC(t, bin, home, env, "task", "create", "quiet please")
	if code != 0 {
		t.Fatalf("bare create failed (exit %d):\n%s", code, out)
	}
	if strings.Contains(out, "default_status") {
		t.Errorf("config the user never wrote must not be validated against:\n%s", out)
	}
	if strings.Contains(out, "Invalid configuration") {
		t.Errorf("a valid config warned as invalid:\n%s", out)
	}
}

// TestCreateStatusUsageDoesNotPinConfig guards a trap this fix walked
// into. Rendering the resolved default into --status help needs the
// config, and the obvious way to get it — core.DefaultWorkflowE — caches
// in a sync.Once. Called from the pre-argv help/usage pass, it froze the
// workflow before `-c key=value` overrides were merged, so every later
// consumer in the same process silently lost them.
//
// The probe is a state-machine override on an unrelated command: the
// transition is illegal in the file and legal only via -c. It fails if
// the usage helper ever goes back through the caching accessor.
func TestCreateStatusUsageDoesNotPinConfig(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	dbPath := filepath.Join(home, "tasks.db")
	cfgPath := writeVocabConfig(t, home, dbPath, customStatusConfig)
	env := statusVocabEnv(t, home, dbPath, cfgPath)

	runTLCOK(t, bin, home, env, "task", "create", "override probe")

	// TODO -> DONE is absent from that config's rules (it routes through
	// IN_PROGRESS), so the file alone must refuse it.
	if out, code := runTLC(t, bin, home, env, "task", "update", "T-0001", "--status", "DONE"); code == 0 {
		t.Fatalf("control: TODO -> DONE should be forbidden by the file:\n%s", out)
	}

	// The config file is re-named explicitly: a `-c key=value` override
	// displaces TLC_CONFIG, so passing only the override would drop the
	// vocabulary along with it. Lower-cased key because viper lower-cases
	// every map key it stores, and normalizeStateMachineKeys re-cases it
	// against the declared statuses.
	out, code := runTLC(t, bin, home, env, "task", "update", "T-0001",
		"--status", "DONE", "-c", cfgPath, "-c", "task.state_machine.rules.todo=[DONE]")
	if code != 0 {
		t.Fatalf("-c state-machine override was dropped, so config was pinned before argv (exit %d):\n%s", code, out)
	}
}

// TestCreateHelpNamesResolvedDefault covers the help surface. With an
// empty flag default cobra prints no "(default ...)" of its own, so help
// would otherwise imply that omitting --status leaves the status unset.
// The rendered default must track the user's vocabulary, and kit's
// enum suffix must survive the rewrite.
func TestCreateHelpNamesResolvedDefault(t *testing.T) {
	bin, home, env := statusVocabFixture(t, renamedVocabConfig)

	out := runTLCOK(t, bin, home, env, "task", "create", "--help")

	if !strings.Contains(out, "default OPEN") {
		t.Errorf("--status help should name the resolved default OPEN, got:\n%s", out)
	}
	if strings.Contains(out, "default TODO") {
		t.Errorf("--status help names a status the config does not declare:\n%s", out)
	}
	// kit appends its own "(one of: ...)" to the same usage string; a
	// rewrite that clobbered the whole string would drop the vocabulary.
	if !strings.Contains(out, "one of: OPEN, WORKING, SHIPPED, WONTFIX") {
		t.Errorf("--status help lost kit's enum suffix:\n%s", out)
	}
}
