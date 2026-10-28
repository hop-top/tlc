package cli

import (
	"encoding/json"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// recipeDirFixture builds a recipe dir with two versions of code-review
// (the newer one including security-scan) and the included recipe, and
// returns it relative to the working directory for recipe.dir.
func recipeDirFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeRecipeFixture(t, dir, "review.yaml", `recipe: code-review
version: 1.1.0
description: Review a PR
vars:
  pr: {required: true, description: PR number}
steps:
  - id: lint
    kind: exec
    exec: {argv: [make, lint]}
  - id: sec
    include: security-scan@1.0.0
    with: {target: "{{pr}}"}
    depends_on: [lint]
`)
	writeRecipeFixture(t, dir, "review-old.yaml", "recipe: code-review\nversion: 1.0.0\nsteps:\n  - id: lint\n")
	writeRecipeFixture(t, dir, "sec.yaml", `recipe: security-scan
version: 1.0.0
vars:
  target: {required: true}
steps:
  - id: scan
    kind: exec
    exec: {argv: [scan, "{{target}}"]}
`)
	return relRecipeDir(t, dir)
}

func TestRecipeList_E2E_TableJSONAndSourceFilter(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	dir := recipeDirFixture(t)
	viper.Set("recipe.dir", dir)

	// One layer only: the temp dir's long relative path plus the built-in
	// rows would push DESCRIPTION past the non-TTY fallback width.
	out, err := runRecipeCmd(t, "list", "--source", dir)
	if err != nil {
		t.Fatalf("recipe list: %v\n%s", err, out)
	}
	for _, want := range []string{"code-review", "1.1.0", "1.0.0", "security-scan", "Review a PR"} {
		if !contains(out, want) {
			t.Errorf("table output lacks %q:\n%s", want, out)
		}
	}

	out, err = runRecipeJSON(t, "list")
	if err != nil {
		t.Fatalf("recipe list (json): %v\n%s", err, out)
	}
	var infos []core.RecipeInfo
	if err := json.Unmarshal([]byte(out), &infos); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(infos) != 4 || infos[0].Name != "code-review" || infos[0].Version != "1.1.0" || infos[2].Name != "security-scan" || infos[3].Source != core.BuiltinSource {
		t.Errorf("json rows = %+v; want name asc, version desc, the built-in layer last", infos)
	}
	if infos[0].Source != dir {
		t.Errorf("source = %q; want the recipe.dir %q", infos[0].Source, dir)
	}

	// --source keeps only one layer: the embedded set alone.
	out, err = runRecipeCmd(t, "list", "--source", "builtin")
	if err != nil || !contains(out, "track-consolidate") || contains(out, "code-review") {
		t.Errorf("--source builtin: err = %v, out = %q; want only the built-in recipes", err, out)
	}
}

// The embedded set is the lowest layer: available with no directory on
// the search path, shadowed by a same-name recipe in any directory, and
// still reachable by a pinned version.
func TestRecipe_E2E_BuiltinLayerPrecedence(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	out, err := runRecipeCmd(t, "show", "track-consolidate")
	if err != nil || !contains(out, "track-consolidate@1.0.0") || !contains(out, "builtin:") {
		t.Fatalf("show built-in: err = %v\n%s", err, out)
	}
	if out, err := runRecipeCmd(t, "validate", "track-consolidate"); err != nil || !contains(out, "is valid") {
		t.Fatalf("validate built-in: err = %v\n%s", err, out)
	}

	dir := t.TempDir()
	writeRecipeFixture(t, dir, "mine.yaml", "recipe: track-consolidate\nversion: 0.0.1\nsteps:\n  - id: local\n")
	viper.Set("recipe.dir", relRecipeDir(t, dir))

	out, err = runRecipeCmd(t, "show", "track-consolidate")
	if err != nil || !contains(out, "track-consolidate@0.0.1") {
		t.Errorf("a project recipe must shadow the built-in, even at a lower version: err = %v\n%s", err, out)
	}
	out, err = runRecipeCmd(t, "show", "track-consolidate@1.0.0")
	if err != nil || !contains(out, "track-consolidate@1.0.0") || !contains(out, "builtin:") {
		t.Errorf("a pinned built-in version must still resolve: err = %v\n%s", err, out)
	}
}

func TestRecipeShow_E2E_TextAndExpandedJSON(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	dir := recipeDirFixture(t)
	viper.Set("recipe.dir", dir)

	out, err := runRecipeCmd(t, "show", "code-review")
	if err != nil {
		t.Fatalf("recipe show: %v\n%s", err, out)
	}
	for _, want := range []string{"code-review@1.1.0", "Review a PR", "pr", "PR number", "lint", "sec/scan", "exec"} {
		if !contains(out, want) {
			t.Errorf("text output lacks %q:\n%s", want, out)
		}
	}

	// JSON is the expanded view: included steps appear with prefixed ids,
	// ordinals assigned, and the with-binding spliced into the child.
	out, err = runRecipeJSON(t, "show", "code-review")
	if err != nil {
		t.Fatalf("recipe show (json): %v\n%s", err, out)
	}
	var view struct {
		Recipe  string            `json:"recipe"`
		Version string            `json:"version"`
		Source  string            `json:"source"`
		Steps   []core.RecipeStep `json:"steps"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if view.Recipe != "code-review" || view.Version != "1.1.0" || view.Source != dir {
		t.Errorf("header = %+v", view)
	}
	if len(view.Steps) != 2 || view.Steps[1].ID != "sec/scan" || view.Steps[1].Ordinal != 2 {
		t.Fatalf("steps = %+v; want lint, sec/scan with ordinals", view.Steps)
	}
	if view.Steps[1].Exec == nil || view.Steps[1].Exec.Argv[1] != "{{pr}}" || view.Steps[1].DependsOn[0] != "lint" {
		t.Errorf("included step not rewritten: %+v", view.Steps[1])
	}

	// A pinned version picks the older file.
	out, err = runRecipeJSON(t, "show", "code-review@1.0.0")
	if err != nil {
		t.Fatalf("pinned show: %v\n%s", err, out)
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil || view.Version != "1.0.0" || len(view.Steps) != 1 {
		t.Errorf("pinned view = %+v (%v)", view, err)
	}

	_, err = runRecipeCmd(t, "show", "nope")
	if err == nil || !contains(err.Error(), "nope") || !contains(err.Error(), dir) {
		t.Errorf("unknown recipe: err = %v; want the name and the search dir", err)
	}
}
