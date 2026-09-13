package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// writeRunFixture lays out fixtures/<recipe>/<run>/ beside a recipe file.
func writeRunFixture(t *testing.T, dir, recipe, run, manifest string) {
	t.Helper()
	runDir := filepath.Join(dir, "fixtures", recipe, run)
	for _, sub := range []string{"contracts", "record"} {
		if err := os.MkdirAll(filepath.Join(runDir, sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	if err := os.WriteFile(filepath.Join(runDir, "test.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// inRepoRoot runs the test from the repository, since the sandbox clones
// the working directory and a temp dir is not a git repo.
func inRepoRoot(t *testing.T) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(testRepoRoot(t)); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

// exitCodeOf reports the exit code an ExitCodeError carries, or 0/-1.
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var ec *ExitCodeError
	if errors.As(err, &ec) {
		return ec.Code
	}
	return -1
}

const echoRecipe = `recipe: echo-probe
version: 1.0.0
description: One exec step that always succeeds.
steps:
  - id: say
    kind: exec
    title: Say hello
    exec:
      argv: [echo, hello]
      timeout: 10s
`

// TestRecipeTest_E2E_HappyPath runs a recipe whose only step is an exec
// that needs no cassette, so replay succeeds without a recording.
func TestRecipeTest_E2E_HappyPath(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	inRepoRoot(t)
	dir := t.TempDir()
	path := writeRecipeFixture(t, dir, "echo-probe.yaml", echoRecipe)
	writeRunFixture(t, dir, "echo-probe", "happy-path",
		"name: happy-path\ndescription: exec only\nexpected_exit: 0\n")

	out, err := runRecipeCmd(t, "test", path, "happy-path")
	if err != nil {
		t.Fatalf("expected exit 0; got %v (code %d)\n%s", err, exitCodeOf(err), out)
	}
	if !contains(out, "run: happy-path") || !contains(out, "result: PASS") {
		t.Errorf("output = %q; want the run name and PASS", out)
	}
}

// TestRecipeTest_E2E_NoRuns reports the fixture directory it looked in
// rather than failing, so a recipe without fixtures is not an error.
func TestRecipeTest_E2E_NoRuns(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	inRepoRoot(t)
	dir := t.TempDir()
	path := writeRecipeFixture(t, dir, "echo-probe.yaml", echoRecipe)

	out, err := runRecipeCmd(t, "test", path)
	if err != nil {
		t.Fatalf("no runs should not fail: %v\n%s", err, out)
	}
	if !contains(out, "no runs in") || !contains(out, "echo-probe") {
		t.Errorf("output = %q; want the fixture path it searched", out)
	}
}

// TestRecipeTest_E2E_UnknownRecipe is a setup failure, not a test failure.
func TestRecipeTest_E2E_UnknownRecipe(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	inRepoRoot(t)

	_, err := runRecipeCmd(t, "test", filepath.Join(t.TempDir(), "absent.yaml"))
	if code := exitCodeOf(err); code != recipeTestExitSandbox {
		t.Errorf("exit = %d (%v); want %d for an unresolvable recipe", code, err, recipeTestExitSandbox)
	}
}

// TestRecipeTest_E2E_MissingRequiredVar: a var the manifest does not bind
// cannot be prompted for in a test run, so the run cannot be set up.
func TestRecipeTest_E2E_MissingRequiredVar(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	inRepoRoot(t)
	dir := t.TempDir()
	path := writeRecipeFixture(t, dir, "needs-var.yaml", `recipe: needs-var
version: 1.0.0
vars:
  target:
    description: where to deploy
    required: true
steps:
  - id: say
    kind: exec
    title: "Say {{target}}"
    exec:
      argv: [echo, "{{target}}"]
`)
	writeRunFixture(t, dir, "needs-var", "happy-path",
		"name: happy-path\nexpected_exit: 0\n")

	_, err := runRecipeCmd(t, "test", path, "happy-path")
	if code := exitCodeOf(err); code != recipeTestExitSandbox {
		t.Errorf("exit = %d (%v); want %d when a required var is unbound", code, err, recipeTestExitSandbox)
	}
}

// TestRecipeTest_E2E_ManifestVarsBind: the same recipe passes once the
// manifest binds the var, and the value reaches the step.
func TestRecipeTest_E2E_ManifestVarsBind(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	inRepoRoot(t)
	dir := t.TempDir()
	path := writeRecipeFixture(t, dir, "needs-var.yaml", `recipe: needs-var
version: 1.0.0
vars:
  target:
    description: where to deploy
    required: true
steps:
  - id: say
    kind: exec
    title: "Say {{target}}"
    exec:
      argv: [echo, "{{target}}"]
`)
	writeRunFixture(t, dir, "needs-var", "happy-path",
		"name: happy-path\nexpected_exit: 0\nvars:\n  target: staging\n")

	out, err := runRecipeCmd(t, "test", path, "happy-path")
	if err != nil {
		t.Fatalf("expected exit 0; got %v\n%s", err, out)
	}
	if !contains(out, "Say staging") || !contains(out, "result: PASS") {
		t.Errorf("output = %q; want the rendered title and PASS", out)
	}
}

// TestRecipeTest_E2E_TaskSelection materializes only the selected step.
func TestRecipeTest_E2E_TaskSelection(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	inRepoRoot(t)
	dir := t.TempDir()
	path := writeRecipeFixture(t, dir, "two-step.yaml", `recipe: two-step
version: 1.0.0
steps:
  - id: first
    kind: exec
    title: First step
    exec:
      argv: [echo, one]
  - id: second
    kind: exec
    title: Second step
    depends_on: [first]
    exec:
      argv: [echo, two]
`)
	writeRunFixture(t, dir, "two-step", "happy-path",
		"name: happy-path\nexpected_exit: 0\n")

	out, err := runRecipeCmd(t, "test", path, "happy-path", "--task", "1")
	if err != nil {
		t.Fatalf("expected exit 0; got %v\n%s", err, out)
	}
	if !contains(out, "First step") || contains(out, "Second step") {
		t.Errorf("output = %q; want only the selected step to run", out)
	}
}

// TestRecipeTest_E2E_ExpectedExitMismatch: a run that declares a nonzero
// expected exit fails when everything succeeds.
func TestRecipeTest_E2E_ExpectedExitMismatch(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	inRepoRoot(t)
	dir := t.TempDir()
	path := writeRecipeFixture(t, dir, "echo-probe.yaml", echoRecipe)
	writeRunFixture(t, dir, "echo-probe", "expects-failure",
		"name: expects-failure\nexpected_exit: 1\n")

	out, err := runRecipeCmd(t, "test", path, "expects-failure")
	if code := exitCodeOf(err); code != recipeTestExitFailed {
		t.Errorf("exit = %d (%v); want %d\n%s", code, err, recipeTestExitFailed, out)
	}
	if !contains(out, "expected exit 1") {
		t.Errorf("output = %q; want the expectation named", out)
	}
}
