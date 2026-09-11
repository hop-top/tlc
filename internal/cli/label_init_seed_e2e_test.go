package cli

// End-to-end coverage for `label init` recording the template's
// `domain:*` values into `task.tags.allowed`.
//
// Driven through the real binary and a real config FILE, like the rest
// of the label e2e suite, because the whole point is what lands ON DISK
// after the command has run and what the NEXT process makes of it. An
// in-process test could assert the slice a helper returned but not that
// the file the next run reads says the same thing, and it is the second
// run — the one that enforces the closed policy — that the feature
// exists for.
//
// Every config pins storage.db_path for the reason label_axes_e2e_test.go
// gives: an unpinned probe resolves the DB through the global project
// registry rather than the cwd, and would read a real database.

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// closedTagPolicyConfig is the config the seeding feature exists for: a
// project that has declared a closed vocabulary and therefore wants its
// domain axis enumerated rather than wildcarded.
const closedTagPolicyConfig = `storage:
  db_path: %s
task:
  tags:
    policy: closed
`

// closedTagPolicyWithAllowedConfig adds entries the user wrote by hand.
// Seeding must preserve them: a command that replaced this list would
// silently revoke a vocabulary the user had chosen.
const closedTagPolicyWithAllowedConfig = `storage:
  db_path: %s
task:
  tags:
    policy: closed
    allowed:
      - area:billing
      - domain:cli
`

// openTagPolicyConfig declares no policy, so `open` applies. Seeding is
// gated on `closed`, so this config must come back untouched.
const openTagPolicyConfig = `storage:
  db_path: %s
task:
  tags:
    policy: open
`

// readAllowedTags parses task.tags.allowed back out of a config file.
//
// Parsed as YAML rather than grepped: the write goes through viper,
// which re-renders the whole document, so a substring check would pass
// on a value that landed under the wrong key.
func readAllowedTags(t *testing.T, cfgPath string) []string {
	t.Helper()

	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config %s: %v", cfgPath, err)
	}

	var doc struct {
		Task struct {
			Tags struct {
				Allowed []string `yaml:"allowed"`
			} `yaml:"tags"`
		} `yaml:"task"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse config %s: %v\n%s", cfgPath, err, raw)
	}
	return doc.Task.Tags.Allowed
}

// seedFixture builds the isolated world for one seeding case and hands
// back everything a case needs to run and then inspect the file.
func seedFixture(t *testing.T, template string) (bin, home, cfgPath string, env []string) {
	t.Helper()
	bin = buildTLCBinary(t)
	home = t.TempDir()
	dbPath := filepath.Join(home, "tasks.db")
	cfgPath = writeVocabConfig(t, home, dbPath, template)
	return bin, home, cfgPath, statusVocabEnv(t, home, dbPath, cfgPath)
}

// goBinaryDomains is the `domain:*` set the go-binary template declares.
// Named here rather than derived from internal/labels so the test pins
// the values a user actually gets: a test reading the same slice the
// command writes would pass even if that slice went empty.
var goBinaryDomains = []string{
	"domain:cli",
	"domain:config",
	"domain:core",
	"domain:io",
	"domain:storage",
}

// TestLabelInitSeedsDomainsIntoConfig is the headline case: under a
// closed policy, `label init` records the template's domain values so
// the config says what was seeded.
func TestLabelInitSeedsDomainsIntoConfig(t *testing.T) {
	bin, home, cfgPath, env := seedFixture(t, closedTagPolicyConfig)

	out := runTLCOK(t, bin, home, env, "label", "init", "--type", "go-binary")

	got := readAllowedTags(t, cfgPath)
	for _, want := range goBinaryDomains {
		if !slices.Contains(got, want) {
			t.Errorf("label init did not record %q in task.tags.allowed\ngot: %v\noutput:\n%s",
				want, got, out)
		}
	}
}

// TestLabelInitReportsWhereItWrote pins the stdout contract. A command
// that mutates a config file the user did not name must say which file,
// or the user has no way to find or undo the change.
func TestLabelInitReportsWhereItWrote(t *testing.T) {
	bin, home, cfgPath, env := seedFixture(t, closedTagPolicyConfig)

	out := runTLCOK(t, bin, home, env, "label", "init", "--type", "go-binary")

	if !strings.Contains(out, cfgPath) {
		t.Errorf("label init wrote %s without naming it on stdout:\n%s", cfgPath, out)
	}
	if !strings.Contains(out, "domain") {
		t.Errorf("label init did not report what it recorded:\n%s", out)
	}
}

// TestLabelInitSeedIsIdempotent is what the `kit/idempotent: yes`
// annotation promises. A second run must converge, not append a second
// copy of every domain.
func TestLabelInitSeedIsIdempotent(t *testing.T) {
	bin, home, cfgPath, env := seedFixture(t, closedTagPolicyConfig)

	runTLCOK(t, bin, home, env, "label", "init", "--type", "go-binary")
	first := readAllowedTags(t, cfgPath)

	runTLCOK(t, bin, home, env, "label", "init", "--type", "go-binary")
	second := readAllowedTags(t, cfgPath)

	if !slices.Equal(first, second) {
		t.Errorf("re-running label init changed task.tags.allowed\nfirst:  %v\nsecond: %v",
			first, second)
	}

	seen := make(map[string]int, len(second))
	for _, a := range second {
		seen[a]++
	}
	for a, n := range seen {
		if n > 1 {
			t.Errorf("entry %q appears %d times after two runs: %v", a, n, second)
		}
	}
}

// TestLabelInitPreservesExistingAllowed is the merge contract. Entries
// the user wrote must survive, and a domain they had already listed must
// not be duplicated by the seeding.
func TestLabelInitPreservesExistingAllowed(t *testing.T) {
	bin, home, cfgPath, env := seedFixture(t, closedTagPolicyWithAllowedConfig)

	runTLCOK(t, bin, home, env, "label", "init", "--type", "go-binary")

	got := readAllowedTags(t, cfgPath)

	// The hand-written non-domain entry survives untouched.
	if !slices.Contains(got, "area:billing") {
		t.Errorf("label init dropped the user's own entry %q: %v", "area:billing", got)
	}
	// Every seeded domain is present.
	for _, want := range goBinaryDomains {
		if !slices.Contains(got, want) {
			t.Errorf("label init did not record %q: %v", want, got)
		}
	}
	// The overlapping entry appears exactly once.
	n := 0
	for _, a := range got {
		if a == "domain:cli" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("domain:cli appears %d times, want 1: %v", n, got)
	}
}

// TestLabelInitSeedingIsDeterministic pins the on-disk order, so two
// projects seeded from the same template produce the same file and a
// re-seed produces no spurious diff.
func TestLabelInitSeedingIsDeterministic(t *testing.T) {
	binA, homeA, cfgA, envA := seedFixture(t, closedTagPolicyConfig)
	runTLCOK(t, binA, homeA, envA, "label", "init", "--type", "go-binary")

	binB, homeB, cfgB, envB := seedFixture(t, closedTagPolicyConfig)
	runTLCOK(t, binB, homeB, envB, "label", "init", "--type", "go-binary")

	a := readAllowedTags(t, cfgA)
	b := readAllowedTags(t, cfgB)
	if !slices.Equal(a, b) {
		t.Errorf("two seedings of the same template disagree\nA: %v\nB: %v", a, b)
	}
	if !slices.IsSorted(a) {
		t.Errorf("seeded task.tags.allowed is not in a stable sorted order: %v", a)
	}
}

// TestLabelInitDoesNotSeedUnderOpenPolicy is the gating contract.
// `allowed` is only consulted under a closed policy, so writing it into
// an open-policy config would mutate a file to no effect — surprising on
// a command the user reached for its listing.
func TestLabelInitDoesNotSeedUnderOpenPolicy(t *testing.T) {
	bin, home, cfgPath, env := seedFixture(t, openTagPolicyConfig)

	before, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	runTLCOK(t, bin, home, env, "label", "init", "--type", "go-binary")

	after, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("label init rewrote an open-policy config\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestSeededDomainAdmittedTypoRejected is the whole point: once the
// domains are recorded, a closed policy regains closure ON THE DOMAIN
// AXIS. A seeded value is accepted and a typo of one is not.
//
// Run as one case rather than two so the admitted and the rejected tag
// are proven against the SAME seeded config. Split across two fixtures,
// a seeding that wrote nothing at all would still show the typo
// rejected — for the wrong reason — in its own case.
func TestSeededDomainAdmittedTypoRejected(t *testing.T) {
	bin, home, _, env := seedFixture(t, closedTagPolicyConfig)

	runTLCOK(t, bin, home, env, "label", "init", "--type", "go-binary")

	out, code := runTLC(t, bin, home, env,
		"task", "create", "seeded domain", "--tag", "domain:storage")
	if code != 0 {
		t.Errorf("seeded tag domain:storage rejected under closed policy (exit %d):\n%s", code, out)
	}

	out, code = runTLC(t, bin, home, env,
		"task", "create", "typo domain", "--tag", "domain:strage")
	if code == 0 {
		t.Errorf("typo tag domain:strage was ADMITTED under a closed, seeded policy;"+
			" closure on the domain axis is not regained:\n%s", out)
	}
}

// TestLabelInitWritesNothingIntoSourceTree guards the CWD-relative
// hazard: a write that resolved its target by falling back to a
// `.tlc.yaml` next to the working directory would leak a file into
// whatever tree the command ran in — which is how a sibling fix on this
// branch leaked a config into the repo.
//
// The command runs from a tempdir that is NOT where its config lives, so
// any target resolved relative to the working directory lands somewhere
// this test can see. The config itself is discovered through the XDG
// cascade rather than named by TLC_CONFIG, so the target comes from the
// cascade — the resolution a real user gets — rather than from a path
// the test handed in.
func TestLabelInitWritesNothingIntoSourceTree(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	dbPath := filepath.Join(home, "tasks.db")

	// The config is DISCOVERED through the XDG cascade rather than named
	// by TLC_CONFIG, which is how a real user reaches this command. It is
	// also the weakest binding the seeding can run under: the gate reads
	// `policy: closed` from a file, so some file always exists by the
	// time a write happens, and the target is resolved from the cascade
	// rather than from a path the caller supplied.
	cfgDir := filepath.Join(home, ".config", "tlc")
	if err := os.MkdirAll(cfgDir, 0o750); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	body := strings.Replace(closedTagPolicyConfig, "%s", dbPath, 1)
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	env := e2eEnv(t, home, dbPath)

	workDir := t.TempDir()
	before := dirEntryNames(t, workDir)

	runTLCOK(t, bin, workDir, env, "label", "init", "--type", "go-binary")

	after := dirEntryNames(t, workDir)
	if !slices.Equal(before, after) {
		t.Errorf("label init created files in its working directory\nbefore: %v\nafter:  %v",
			before, after)
	}

	// Belt and braces: name the specific shapes a CWD fallback would take.
	for _, leaked := range []string{".tlc.yaml", ".tlc.yml", "config.yaml", "tlc.yaml"} {
		if _, err := os.Stat(filepath.Join(workDir, leaked)); err == nil {
			t.Errorf("label init leaked %s into its working directory", leaked)
		}
	}

	// The positive half of the same guarantee: the seeded values landed in
	// the DISCOVERED config, not merely somewhere other than the cwd.
	// Asserting only that the cwd stayed clean would also pass if the
	// command had written nothing at all, and a seeding that silently did
	// nothing is not the behavior this guards.
	//
	// Scoped to this run's own tempdirs rather than scanning the repo: the
	// source tree is shared with every other test in the package, so a
	// file found there is not evidence about `label init`. `sync init` has
	// its own CWD-relative `.tlc.yaml` fallback (internal/cli/sync.go), and
	// attributing its leak to this command would fail this test for a
	// defect it does not cover.
	seeded := readAllowedTags(t, filepath.Join(home, ".config", "tlc", "config.yaml"))
	for _, want := range goBinaryDomains {
		if !slices.Contains(seeded, want) {
			t.Errorf("seeding did not reach the discovered config: %q missing from %v",
				want, seeded)
		}
	}
}

// dirEntryNames lists a directory's entries, sorted, for before/after
// comparison.
func dirEntryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	return names
}
