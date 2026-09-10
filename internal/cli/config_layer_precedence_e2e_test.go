package cli

// End-to-end coverage for CONFIG-LAYER PRECEDENCE of the task status
// vocabulary and state machine.
//
// status_vocabulary_e2e_test.go proves the CLI honours *a* config file.
// It says nothing about which file wins when several are in play, and the
// cascade in root.go's initConfig has five layers. Every case below drives
// the real binary against real files on disk, for the same reason the
// vocabulary tests do: internal/core/workflow_test.go passed for months
// against a CLI that never handed the engine any config, because it built
// TaskConfig structs directly. A struct-level test cannot see a layering
// bug either — layering happens in viper, above the engine entirely.
//
// Two hazards shape the fixtures here:
//
//  1. storage.db_path is pinned in every generated config. The DB resolves
//     through the global project registry, not the cwd, so an unpinned
//     probe silently reads an unrelated real database.
//  2. Running tlc against a config file REWRITES IT IN PLACE, expanding
//     defaults and lower-casing state-machine keys. Every helper below
//     therefore writes fresh files per invocation, and no assertion reads
//     a config file back after a run.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// layerVocabTemplate renders a config declaring TODO -> <marker> -> DONE.
// The marker status is the probe: whichever layer's marker appears in the
// CLI's own vocabulary reporting is the layer that won.
func layerVocabTemplate(marker, dbPath string) string {
	return `storage:
  db_path: ` + dbPath + `
task:
  default_status: TODO
  statuses:
    - name: TODO
      label: To Do
      role: initial
    - name: ` + marker + `
      label: Marker ` + marker + `
      role: active
    - name: DONE
      label: Done
      is_terminal: true
      role: completed
  state_machine:
    rules:
      TODO: [` + marker + `]
      ` + marker + `: [DONE]
`
}

// writeLayerConfig materialises a config at path, creating parents.
func writeLayerConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// layerEnv wires a subprocess run to a private HOME/XDG and a pinned DB.
// It deliberately does NOT set TLC_CONFIG: these tests exercise the
// cascade that fires when no explicit config is named, so the env
// equivalent of -c must stay out of the way. e2eEnv already strips an
// inherited TLC_CONFIG.
//
// TLC_MODE is pinned to standalone so DetectMode cannot flip to hop mode
// because some ancestor of the tempdir happens to contain .hop/ — the
// mode picks which filenames findAllConfigsForMode looks for.
func layerEnv(t *testing.T, home, dbPath string) []string {
	t.Helper()
	return append(e2eEnv(t, home, dbPath), "TLC_MODE=standalone")
}

// vocabularyOf reports the status vocabulary the CLI itself believes in
// after the whole config cascade has run.
//
// The probe is `task list --status <nonsense>`, whose rejection message
// enumerates the resolved vocabulary. This runs on the EXECUTE path,
// which is where the cascade is fully assembled: cobra has parsed argv,
// so -c tokens are visible to initConfig. Reading the vocabulary out of
// the binary rather than out of a config file is the point — the file on
// disk is what the user wrote, this is what the CLI resolved.
func vocabularyOf(t *testing.T, bin, cwd string, env []string, args ...string) string {
	t.Helper()
	full := append([]string{"task", "list", "--status", nonsenseStatus}, args...)
	out, code := runTLC(t, bin, cwd, env, full...)
	if code == 0 {
		t.Fatalf("probe status %q should have been rejected:\n%s", nonsenseStatus, out)
	}
	i := strings.Index(out, "valid values:")
	if i < 0 {
		t.Fatalf("no vocabulary in rejection output:\n%s", out)
	}
	rest := out[i+len("valid values:"):]
	if j := strings.IndexAny(rest, "\n"); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest)
}

// nonsenseStatus is a value no vocabulary declares, so every rejection of
// it enumerates whatever set the CLI resolved.
const nonsenseStatus = "ZZZNOSUCHSTATUS"

// helpVocabularyOf reports the vocabulary stamped onto the --status flag
// enum in help output. This is a DIFFERENT resolution path from
// vocabularyOf: Execute() runs initConfig before cobra parses argv, so
// the help restamp sees only the layers reachable without argv — files
// on disk and TLC_CONFIG from the environment, but not -c tokens.
func helpVocabularyOf(t *testing.T, bin, cwd string, env []string, args ...string) string {
	t.Helper()
	full := append([]string{"task", "update", "--help"}, args...)
	out := runTLCOK(t, bin, cwd, env, full...)
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "--status") {
			continue
		}
		i := strings.Index(line, "one of:")
		if i < 0 {
			continue
		}
		rest := line[i+len("one of:"):]
		if j := strings.Index(rest, ")"); j >= 0 {
			rest = rest[:j]
		}
		return strings.TrimSpace(rest)
	}
	t.Fatalf("no --status enum in help output:\n%s", out)
	return ""
}

// assertVocabulary fails unless the resolved vocabulary contains want and
// contains none of the notWant markers. Both halves matter: asserting only
// the winner passes when a broken merge unions every layer together.
func assertVocabulary(t *testing.T, got, want string, notWant ...string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("resolved vocabulary %q does not contain the winning marker %q", got, want)
	}
	for _, bad := range notWant {
		if strings.Contains(got, bad) {
			t.Errorf("resolved vocabulary %q leaked losing layer's marker %q", got, bad)
		}
	}
}

// TestLayerProjectConfigBeatsBuiltinDefaults is the floor of the cascade:
// a project config must displace the compiled-in TODO/IN_PROGRESS/DONE/
// SKIPPED set entirely, not merely add to it.
func TestLayerProjectConfigBeatsBuiltinDefaults(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	work := filepath.Join(home, "work")
	dbPath := filepath.Join(home, "tasks.db")
	env := layerEnv(t, home, dbPath)

	writeLayerConfig(t,
		filepath.Join(work, ".tlc", "config.yaml"),
		layerVocabTemplate("PROJECTONLY", dbPath))

	got := vocabularyOf(t, bin, work, env)
	// IN_PROGRESS and SKIPPED are built-ins with no counterpart in the
	// project config: their survival would mean the layers unioned.
	assertVocabulary(t, got, "PROJECTONLY", "IN_PROGRESS", "SKIPPED")
}

// TestLayerClosestProjectConfigBeatsAncestor pins the direction of the
// project cascade. root.go merges root-most first so the closest file
// overwrites; reverse that loop and an ancestor's vocabulary silently
// governs a nested directory.
//
// Both resolution paths are asserted deliberately. The help path is the
// one that actually observes the merge loop's ordering — see
// TestLayerAncestorConfigDiscardedOnExecutePath for why the execute path
// cannot, and reverse the loop at root.go's `for i := len(configs) - 1`
// to watch the help half of this test go red.
func TestLayerClosestProjectConfigBeatsAncestor(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	root := filepath.Join(home, "work")
	nested := filepath.Join(root, "a", "b")
	dbPath := filepath.Join(home, "tasks.db")
	env := layerEnv(t, home, dbPath)

	writeLayerConfig(t,
		filepath.Join(root, ".tlc", "config.yaml"),
		layerVocabTemplate("ANCESTOR", dbPath))
	writeLayerConfig(t,
		filepath.Join(nested, ".tlc", "config.yaml"),
		layerVocabTemplate("CLOSEST", dbPath))

	// From the nested dir the closest config wins, on both paths.
	assertVocabulary(t, vocabularyOf(t, bin, nested, env), "CLOSEST", "ANCESTOR")
	assertVocabulary(t, helpVocabularyOf(t, bin, nested, env), "CLOSEST", "ANCESTOR")

	// From the ancestor dir the nested config is not on the walk at all,
	// so the ancestor's own vocabulary governs. Without this half, an
	// implementation that simply always preferred "CLOSEST" would pass.
	assertVocabulary(t, vocabularyOf(t, bin, root, env), "ANCESTOR", "CLOSEST")
	assertVocabulary(t, helpVocabularyOf(t, bin, root, env), "ANCESTOR", "CLOSEST")
}

// TestLayerAncestorConfigDiscardedOnExecutePath pins the second defect
// the layering investigation turned up, and explains why the merge loop's
// ordering is invisible to the execute path.
//
// initConfig merges every project config from root-most to closest and
// then calls viper.SetConfigFile(configs[0]) so ConfigFileUsed() names
// the closest one. Later, core.detectProjectOnce takes that filename and
// calls viper.ReadInConfig() on it (internal/core/project.go). ReadInConfig
// REPLACES viper's config map with that single file rather than merging
// into it — so every ancestor layer the cascade just assembled is thrown
// away, and the execute path ends up honouring the closest file alone.
//
// The visible consequence: a key an ancestor declares and the closest
// config does not mention reaches --help but not the command itself. A
// user who sets task.id_format once at a repo root gets it in help text
// and the built-in default everywhere it matters.
//
// Asserted as-is so a fix flips this test rather than passing unnoticed.
func TestLayerAncestorConfigDiscardedOnExecutePath(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	root := filepath.Join(home, "work")
	nested := filepath.Join(root, "a", "b")
	dbPath := filepath.Join(home, "tasks.db")
	env := layerEnv(t, home, dbPath)

	// The ancestor declares a custom id format; the closest config is
	// silent about it, so a true cascade would let the ancestor's value
	// through.
	writeLayerConfig(t, filepath.Join(root, ".tlc", "config.yaml"),
		"storage:\n  db_path: "+dbPath+"\ntask:\n  id_format: ANC-{seq:04d}\n")
	writeLayerConfig(t, filepath.Join(nested, ".tlc", "config.yaml"),
		"storage:\n  db_path: "+dbPath+"\n")

	out := runTLCOK(t, bin, nested, env, "task", "create", "cascade probe")
	if strings.Contains(out, "ANC-") {
		t.Fatalf("ancestor config now survives to the execute path; the "+
			"cascade is a real merge. Delete this test and assert the "+
			"ancestor's key reaches the command instead. Got:\n%s", out)
	}
	if !strings.Contains(out, "T-") {
		t.Fatalf("expected the built-in T- id format, got:\n%s", out)
	}
}

// TestLayerClosestProjectConfigBeatsAncestorStateMachine is the same
// precedence question asked of the state machine rather than the status
// list. Vocabulary and rules travel through different viper shapes — a
// list and a map — and viper merges the two differently, so proving one
// does not prove the other.
func TestLayerClosestProjectConfigBeatsAncestorStateMachine(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	root := filepath.Join(home, "work")
	nested := filepath.Join(root, "a", "b")
	dbPath := filepath.Join(home, "tasks.db")
	env := layerEnv(t, home, dbPath)

	// Same vocabulary in both files; only the rules differ, so the status
	// list cannot be what decides the outcome.
	statuses := `storage:
  db_path: ` + dbPath + `
task:
  default_status: TODO
  statuses:
    - name: TODO
      label: To Do
      role: initial
    - name: MID
      label: Middle
      role: active
    - name: DONE
      label: Done
      is_terminal: true
      role: completed
  state_machine:
    rules:
`
	// Ancestor forbids the TODO -> DONE shortcut; the closest config
	// permits it. If the ancestor wins, the update below is refused.
	writeLayerConfig(t, filepath.Join(root, ".tlc", "config.yaml"),
		statuses+"      TODO: [MID]\n      MID: [DONE]\n")
	writeLayerConfig(t, filepath.Join(nested, ".tlc", "config.yaml"),
		statuses+"      TODO: [MID, DONE]\n      MID: [DONE]\n")

	runTLCOK(t, bin, nested, env, "task", "create", "rules probe")

	out, code := runTLC(t, bin, nested, env, "task", "update", "T-0001", "--status", "DONE")
	if code != 0 {
		t.Fatalf("closest config permits TODO -> DONE but it was refused (exit %d):\n%s", code, out)
	}

	// And the ancestor's stricter rule still governs its own directory.
	runTLCOK(t, bin, root, env, "task", "create", "ancestor probe")
	id := "T-0002"
	out, code = runTLC(t, bin, root, env, "task", "update", id, "--status", "DONE")
	if code == 0 {
		t.Fatalf("ancestor config forbids TODO -> DONE but it was allowed:\n%s", out)
	}
	if !strings.Contains(out, "transition") {
		t.Errorf("refusal should come from the state machine, got:\n%s", out)
	}
}

// TestLayerExplicitConfigFileBeatsProjectConfig covers the -c <path>
// layer. Explicit beats ambient: a path the user names on the command
// line has to override the project file they happen to be standing in,
// or -c is decorative.
func TestLayerExplicitConfigFileBeatsProjectConfig(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	work := filepath.Join(home, "work")
	dbPath := filepath.Join(home, "tasks.db")
	env := layerEnv(t, home, dbPath)

	writeLayerConfig(t,
		filepath.Join(work, ".tlc", "config.yaml"),
		layerVocabTemplate("PROJECT", dbPath))

	// A private copy per invocation: the run rewrites whatever file it
	// resolves, so a shared fixture would be a different file by the
	// second assertion.
	flagCfg := filepath.Join(t.TempDir(), "explicit.yaml")
	writeLayerConfig(t, flagCfg, layerVocabTemplate("EXPLICIT", dbPath))

	got := vocabularyOf(t, bin, work, env, "-c", flagCfg)
	assertVocabulary(t, got, "EXPLICIT", "PROJECT")
}

// TestLayerExplicitConfigFilesMergeInArgumentOrder pins the ordering
// *within* the -c file layer: later -c files merge on top of earlier
// ones, so the last one named wins.
func TestLayerExplicitConfigFilesMergeInArgumentOrder(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	work := filepath.Join(home, "work")
	dbPath := filepath.Join(home, "tasks.db")
	env := layerEnv(t, home, dbPath)
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}

	dir := t.TempDir()
	first := filepath.Join(dir, "first.yaml")
	second := filepath.Join(dir, "second.yaml")
	writeLayerConfig(t, first, layerVocabTemplate("FIRSTFILE", dbPath))
	writeLayerConfig(t, second, layerVocabTemplate("SECONDFILE", dbPath))

	got := vocabularyOf(t, bin, work, env, "-c", first, "-c", second)
	assertVocabulary(t, got, "SECONDFILE", "FIRSTFILE")
}

// kvStatusOverride is a -c key=value token replacing the whole status
// list with TODO -> KVMARK -> DONE. viper parses the value as JSON, which
// is how a user supplies a structured override on the command line.
const kvStatusOverride = `task.statuses=[` +
	`{"name":"TODO","label":"To Do","role":"initial"},` +
	`{"name":"KVMARK","label":"KV Marker","role":"active"},` +
	`{"name":"DONE","label":"Done","role":"completed","is_terminal":true}]`

// TestLayerKeyValueOverrideBeatsEveryFile is the top of the cascade:
// -c key=value is applied with viper.Set after every file has merged, so
// it must win over the project config AND over an explicit -c file.
func TestLayerKeyValueOverrideBeatsEveryFile(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	work := filepath.Join(home, "work")
	dbPath := filepath.Join(home, "tasks.db")
	env := layerEnv(t, home, dbPath)

	writeLayerConfig(t,
		filepath.Join(work, ".tlc", "config.yaml"),
		layerVocabTemplate("PROJECT", dbPath))

	// Control: with only the project config in play, PROJECT governs.
	// Without this the assertions below would also pass if the override
	// had merely knocked the CLI back to its built-in defaults.
	assertVocabulary(t, vocabularyOf(t, bin, work, env), "PROJECT", "KVMARK")

	// Over the project config.
	assertVocabulary(t,
		vocabularyOf(t, bin, work, env, "-c", kvStatusOverride),
		"KVMARK", "PROJECT")

	// And over an explicit -c file, which is itself the strongest file
	// layer — so this pins the override above every file, not just the
	// ambient one.
	flagCfg := filepath.Join(t.TempDir(), "explicit.yaml")
	writeLayerConfig(t, flagCfg, layerVocabTemplate("EXPLICIT", dbPath))
	assertVocabulary(t,
		vocabularyOf(t, bin, work, env, "-c", flagCfg, "-c", kvStatusOverride),
		"KVMARK", "EXPLICIT", "PROJECT")
}

// TestLayerKeyValueOverrideBeatsStateMachine asks the same question of the
// state machine. The rules live under a viper MAP, which merges key-wise
// rather than being replaced wholesale, so a key=value override that wins
// for a scalar does not automatically win here.
func TestLayerKeyValueOverrideBeatsStateMachine(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	work := filepath.Join(home, "work")
	dbPath := filepath.Join(home, "tasks.db")
	env := layerEnv(t, home, dbPath)

	// Project config forbids TODO -> DONE.
	writeLayerConfig(t,
		filepath.Join(work, ".tlc", "config.yaml"),
		layerVocabTemplate("MID", dbPath))

	runTLCOK(t, bin, work, env, "task", "create", "override probe")

	// Control: refused by the project's rules.
	out, code := runTLC(t, bin, work, env, "task", "update", "T-0001", "--status", "DONE")
	if code == 0 {
		t.Fatalf("control: project config forbids TODO -> DONE but it was allowed:\n%s", out)
	}

	// Overridden: the key=value layer rewrites the rule and the same
	// transition is now legal. Note the lower-cased key — viper
	// lower-cases every map key it stores, and normalizeStateMachineKeys
	// re-cases it against the declared statuses on the way out.
	out, code = runTLC(t, bin, work, env, "task", "update", "T-0001",
		"--status", "DONE", "-c", "task.state_machine.rules.todo=[DONE]")
	if code != 0 {
		t.Fatalf("-c task.state_machine.rules.todo=[DONE] should permit TODO -> DONE (exit %d):\n%s", code, out)
	}
}

// TestLayerUserConfigBeatsBuiltinDefaults covers the user-level layer,
// reachable hermetically because config.UserConfigDir resolves through
// kit/xdg.ConfigDir and therefore honours XDG_CONFIG_HOME, which e2eEnv
// already redirects into the test's private HOME.
//
// The system layer (/etc/tlc) is NOT covered: config.SystemConfigDir is a
// compile-time constant with no env seam, so exercising it would require
// writing to /etc as root. See TestLayerUserAndSystemAreAlternatives for
// what is asserted about the pair instead.
func TestLayerUserConfigBeatsBuiltinDefaults(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	work := filepath.Join(home, "work")
	dbPath := filepath.Join(home, "tasks.db")
	env := layerEnv(t, home, dbPath)
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}

	writeLayerConfig(t,
		filepath.Join(home, ".config", "tlc", "config.yaml"),
		layerVocabTemplate("USERLEVEL", dbPath))

	got := vocabularyOf(t, bin, work, env)
	assertVocabulary(t, got, "USERLEVEL", "IN_PROGRESS", "SKIPPED")
}

// TestLayerProjectConfigBeatsUserConfig places the two file layers that
// are both hermetically reachable against each other. The project
// cascade merges on top of whatever ReadInConfig picked up, so the
// project file must win.
func TestLayerProjectConfigBeatsUserConfig(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	work := filepath.Join(home, "work")
	dbPath := filepath.Join(home, "tasks.db")
	env := layerEnv(t, home, dbPath)

	writeLayerConfig(t,
		filepath.Join(home, ".config", "tlc", "config.yaml"),
		layerVocabTemplate("USERLEVEL", dbPath))
	writeLayerConfig(t,
		filepath.Join(work, ".tlc", "config.yaml"),
		layerVocabTemplate("PROJECTLEVEL", dbPath))

	assertVocabulary(t, vocabularyOf(t, bin, work, env), "PROJECTLEVEL", "USERLEVEL")

	// Outside the project tree the user config governs again, which
	// rules out "the project marker always wins regardless of cwd".
	outside := filepath.Join(home, "elsewhere")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	assertVocabulary(t, vocabularyOf(t, bin, outside, env), "USERLEVEL", "PROJECTLEVEL")
}

// TestLayerUserAndSystemAreAlternatives documents — and pins — the fact
// that the user and system config directories are NOT merged layers.
//
// initConfig registers both via AddConfigPath (user first, system second)
// and then calls a single viper.ReadInConfig, which returns the FIRST
// path that holds a config file and stops. So a user config does not
// override a system config key-by-key: it suppresses the system file
// wholesale, including keys the user never mentioned.
//
// Writing to /etc/tlc needs root, so the pair cannot be exercised
// directly here. What CAN be pinned hermetically is the property that
// makes it true: with a user config present, a key absent from it falls
// through to the BUILT-IN default rather than to any other file layer.
// If ReadInConfig were ever changed to merge every registered path, a
// system file would start contributing such keys and this contract would
// need revisiting.
func TestLayerUserAndSystemAreAlternatives(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	work := filepath.Join(home, "work")
	dbPath := filepath.Join(home, "tasks.db")
	env := layerEnv(t, home, dbPath)
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}

	// A user config that pins the DB but declares no task vocabulary at
	// all. Everything under `task` must come from the built-ins.
	writeLayerConfig(t,
		filepath.Join(home, ".config", "tlc", "config.yaml"),
		"storage:\n  db_path: "+dbPath+"\n")

	got := vocabularyOf(t, bin, work, env)
	for _, want := range []string{"TODO", "IN_PROGRESS", "DONE", "SKIPPED"} {
		if !strings.Contains(got, want) {
			t.Errorf("vocabulary %q should fall through to the built-in %q", got, want)
		}
	}
}

// TestLayerExplicitConfigFileMissesHelpRestamp pins a real inconsistency
// in the cascade rather than a contract anyone designed.
//
// -c <file> reaches the EXECUTE path, where it correctly overrides the
// project config. It does NOT reach the --help restamp: Execute() calls
// initConfig() before cobra has parsed argv, so viper.GetStringSlice
// ("config") is still empty at that point and the -c token is invisible.
// TLC_CONFIG, documented as the env equivalent of -c, arrives through
// AutomaticEnv and so IS visible there.
//
// The result is that one invocation can describe two different
// vocabularies: `tlc task update --help -c custom.yaml` advertises the
// built-in set while `tlc task update --status X -c custom.yaml` in the
// same shell validates against the custom one. That is the exact failure
// shape TestConfiguredStatusInHelpAndCompletion was written to prevent,
// surviving on the -c path.
//
// Asserted as-is so a fix flips this test rather than passing unnoticed.
// When the restamp learns to pre-parse -c, replace the body with the
// equality both halves should satisfy.
func TestLayerExplicitConfigFileMissesHelpRestamp(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	work := filepath.Join(home, "work")
	dbPath := filepath.Join(home, "tasks.db")
	env := layerEnv(t, home, dbPath)
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}

	flagCfg := filepath.Join(t.TempDir(), "explicit.yaml")
	writeLayerConfig(t, flagCfg, layerVocabTemplate("EXPLICIT", dbPath))

	// Execute path honours it.
	assertVocabulary(t, vocabularyOf(t, bin, work, env, "-c", flagCfg),
		"EXPLICIT", "IN_PROGRESS", "SKIPPED")

	// Help path does not — it still advertises the built-in four.
	gotHelp := helpVocabularyOf(t, bin, work, env, "-c", flagCfg)
	if strings.Contains(gotHelp, "EXPLICIT") {
		t.Fatalf("help restamp now honours -c <file>; the two paths agree. "+
			"Replace this test with an equality assertion between "+
			"vocabularyOf and helpVocabularyOf. Got: %s", gotHelp)
	}
	for _, want := range []string{"TODO", "IN_PROGRESS", "DONE", "SKIPPED"} {
		if !strings.Contains(gotHelp, want) {
			t.Errorf("help vocabulary %q should still show the built-in %q", gotHelp, want)
		}
	}

	// TLC_CONFIG, the documented env equivalent of -c, DOES reach the
	// restamp. Pinning the asymmetry is what makes it a bug report
	// rather than an accepted limitation of naming a config at all.
	envCfg := filepath.Join(t.TempDir(), "viaenv.yaml")
	writeLayerConfig(t, envCfg, layerVocabTemplate("VIAENV", dbPath))
	envWithCfg := append(layerEnv(t, home, dbPath), "TLC_CONFIG="+envCfg)
	assertVocabulary(t, helpVocabularyOf(t, bin, work, envWithCfg),
		"VIAENV", "IN_PROGRESS", "SKIPPED")
}
