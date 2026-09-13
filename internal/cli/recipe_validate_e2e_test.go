package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// writeRecipeFixture writes a recipe document under dir and returns its path.
func writeRecipeFixture(t *testing.T, dir, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

// relRecipeDir returns dir relative to the working directory: recipe.dir
// must be relative, and without a project config a relative dir resolves
// against the working directory.
func relRecipeDir(t *testing.T, dir string) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	rel, err := filepath.Rel(cwd, dir)
	if err != nil {
		t.Fatalf("rel: %v", err)
	}
	return rel
}

// runRecipeCmd runs `tlc recipe <args...>` in-process and returns the
// combined output. No storage is opened by any recipe command.
func runRecipeCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	resetRecipeFlags()
	cmd := newTestCmd()
	cmd.AddCommand(RecipeCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(append([]string{"recipe"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// runRecipeJSON runs a recipe command with output.format=json. The format
// is set through viper rather than --format because the shared command
// globals cache the first test root's persistent flags.
func runRecipeJSON(t *testing.T, args ...string) (string, error) {
	t.Helper()
	viper.Set("output.format", "json")
	defer viper.Set("output.format", "")
	return runRecipeCmd(t, args...)
}

const validRecipeFixture = `recipe: hello
version: 1.0.0
steps:
  - id: greet
    title: Say hello
  - id: wave
    depends_on: [greet]
`

func TestRecipeValidate_E2E_Valid(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	p := writeRecipeFixture(t, t.TempDir(), "hello.yaml", validRecipeFixture)

	out, err := runRecipeCmd(t, "validate", p)
	if err != nil {
		t.Fatalf("expected exit 0; got %v\n%s", err, out)
	}
	if !contains(out, "hello@1.0.0") || !contains(out, "valid") || !contains(out, "2 steps") {
		t.Errorf("output = %q; want the recipe key, 'valid' and the step count", out)
	}
}

func TestRecipeValidate_E2E_MissingDep(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	p := writeRecipeFixture(t, t.TempDir(), "bad.yaml", "recipe: bad\nversion: 1\nsteps:\n  - id: a\n    depends_on: [nope]\n")

	out, err := runRecipeCmd(t, "validate", p)
	if err == nil {
		t.Fatalf("expected exit 1; output:\n%s", out)
	}
	if !contains(err.Error(), "nope") || !contains(out, "Validation failed") {
		t.Errorf("err = %v, out = %q; want the dangling dependency named", err, out)
	}
}

func TestRecipeValidate_E2E_Cycle(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	p := writeRecipeFixture(t, t.TempDir(), "cyc.yaml", "recipe: cyc\nversion: 1\nsteps:\n  - id: a\n    depends_on: [b]\n  - id: b\n    depends_on: [a]\n")

	_, err := runRecipeCmd(t, "validate", p)
	if err == nil || !contains(err.Error(), "cycle") {
		t.Errorf("err = %v; want a cycle error", err)
	}
}

// TestRecipeValidate_E2E_ByNameExpandsIncludes: a name resolves through
// recipe.dir and validation covers expansion, so an include cycle is
// reported even though each file validates on its own.
func TestRecipeValidate_E2E_ByNameExpandsIncludes(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	dir := t.TempDir()
	writeRecipeFixture(t, dir, "loop.yaml", "recipe: loop\nversion: 1\nsteps:\n  - id: again\n    include: loop@1\n")
	rel := relRecipeDir(t, dir)
	viper.Set("recipe.dir", rel)

	_, err := runRecipeCmd(t, "validate", "loop")
	if err == nil || !contains(err.Error(), "include") {
		t.Errorf("err = %v; want an include cycle error", err)
	}
	_, err = runRecipeCmd(t, "validate", "missing")
	if err == nil || !contains(err.Error(), "missing") || !contains(err.Error(), rel) {
		t.Errorf("err = %v; want the unknown name and the search dir", err)
	}
}
