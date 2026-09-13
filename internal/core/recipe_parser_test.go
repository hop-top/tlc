package core

import (
	"strings"
	"testing"
)

const validRecipeYAML = `
recipe: code-review
version: 1.2.0
description: review a PR
requires: {subject: task}
vars:
  pr: {description: "PR number", required: true}
  depth: {default: standard}
agent: claude
track:
  title: "Review {{pr}}"
  type: delivery
  plan: |
    Review checklist.
steps:
  - id: lint
    kind: exec
    title: "Lint {{pr}}"
    exec: {argv: [golangci-lint, run], cwd: ., timeout: 5m}
  - id: review
    title: "Review {{pr}} ({{depth}})"
    description: "{{subject.description}} lint said {{results.lint.stdout}}"
    depends_on: [lint]
    retry: {max_attempts: 3, backoff: 30s}
    when: "results.lint.exit_code == 0"
    assignee: "@lead"
    due: "+2d"
  - id: sign-off
    kind: human
    human: {assignee: "@lead", timeout: 48h, on_timeout: reject}
    depends_on: [review]
`

func parseRecipeString(t *testing.T, yamlText string) (*Recipe, error) {
	t.Helper()
	return ParseRecipe(strings.NewReader(yamlText), "test.yaml")
}

func TestParseRecipe_YAML(t *testing.T) {
	r, err := parseRecipeString(t, validRecipeYAML)
	if err != nil {
		t.Fatalf("ParseRecipe: %v", err)
	}
	assertRecipeHeader(t, r)
	assertRecipeSteps(t, r)
}

func assertRecipeHeader(t *testing.T, r *Recipe) {
	t.Helper()
	if r.Name != "code-review" || r.Version != "1.2.0" || r.Requires.Subject != "task" || r.Agent != "claude" {
		t.Errorf("header = %+v", r)
	}
	if r.Track == nil || r.Track.Type != "delivery" || !strings.Contains(r.Track.Plan, "checklist") {
		t.Errorf("track block = %+v", r.Track)
	}
	if r.Hash == "" || r.Path != "test.yaml" {
		t.Errorf("hash/path not set: %q %q", r.Hash, r.Path)
	}
	if !r.Vars["pr"].Required || r.Vars["depth"].Default != "standard" {
		t.Errorf("vars = %+v", r.Vars)
	}
}

func assertRecipeSteps(t *testing.T, r *Recipe) {
	t.Helper()
	if len(r.Steps) != 3 || r.Steps[0].ID != "lint" || r.Steps[2].Kind != TaskKindHuman {
		t.Fatalf("steps = %+v", r.Steps)
	}
	lint, review := r.Steps[0], r.Steps[1]
	if lint.Exec == nil || lint.Exec.Timeout != "5m" || lint.Ordinal != 1 {
		t.Errorf("exec step not decoded: %+v", lint)
	}
	if review.Retry == nil || review.Retry.MaxAttempts != 3 || review.Due.Raw != "+2d" {
		t.Errorf("review step not decoded: %+v", review)
	}
}

func TestParseRecipe_JSON(t *testing.T) {
	r, err := ParseRecipe(strings.NewReader(`{"recipe":"one","version":"0.1.0","steps":[{"id":"a","title":"A"}]}`), "one.json")
	if err != nil {
		t.Fatalf("ParseRecipe json: %v", err)
	}
	if r.Name != "one" || len(r.Steps) != 1 {
		t.Errorf("recipe = %+v", r)
	}
}

// TestParseRecipe_ExtensionlessStillValidates closes the flow parser's
// hole: a file without an extension must go through validation too.
func TestParseRecipe_ExtensionlessStillValidates(t *testing.T) {
	_, err := ParseRecipe(strings.NewReader("recipe: x\nversion: 1\nsteps:\n  - id: a\n    depends_on: [nope]\n"), "recipe")
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("extensionless file skipped validation: %v", err)
	}
}

// TestParseRecipe_RejectsFlowShapes: the flow format's map-form steps and
// header keys are refused with a pointer to the recipe spec, not decoded
// into an empty recipe.
func TestParseRecipe_RejectsFlowShapes(t *testing.T) {
	cases := map[string]string{
		"map steps":  "recipe: x\nversion: 1\nsteps:\n  a:\n    title: A\n",
		"flow_id":    "flow_id: flow:x:1\nversion: 1\nsteps:\n  - id: a\n",
		"entry_step": "recipe: x\nversion: 1\nentry_step: a\nsteps:\n  - id: a\n",
	}
	for name, yamlText := range cases {
		_, err := parseRecipeString(t, yamlText)
		if err == nil || !strings.Contains(err.Error(), "flow") || !strings.Contains(err.Error(), "recipe") {
			t.Errorf("%s: err = %v; want a flows-are-now-recipes error", name, err)
		}
	}
}

func TestValidateRecipe_Rules(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string // substring of the error
	}{
		{"missing name", "version: 1\nsteps:\n  - id: a\n", "recipe"},
		{"bad name", "recipe: Code_Review\nversion: 1\nsteps:\n  - id: a\n", "name"},
		{"missing version", "recipe: x\nsteps:\n  - id: a\n", "version"},
		{"no steps", "recipe: x\nversion: 1\n", "steps"},
		{"bad subject", "recipe: x\nversion: 1\nrequires: {subject: sprint}\nsteps:\n  - id: a\n", "subject"},
		{"reserved var", "recipe: x\nversion: 1\nvars:\n  subject: {default: 1}\nsteps:\n  - id: a\n", "reserved"},
		{"required with default", "recipe: x\nversion: 1\nvars:\n  pr: {required: true, default: 1}\nsteps:\n  - id: a\n", "contradict"},
		{"duplicate id", "recipe: x\nversion: 1\nsteps:\n  - id: a\n  - id: a\n", "duplicate"},
		{"slash in id", "recipe: x\nversion: 1\nsteps:\n  - id: a/b\n", "id"},
		{"missing dep", "recipe: x\nversion: 1\nsteps:\n  - id: a\n    depends_on: [b]\n", "b"},
		{"cycle", "recipe: x\nversion: 1\nsteps:\n  - id: a\n    depends_on: [b]\n  - id: b\n    depends_on: [a]\n", "cycle"},
		{"exec without argv", "recipe: x\nversion: 1\nsteps:\n  - id: a\n    kind: exec\n", "argv"},
		{"unknown kind", "recipe: x\nversion: 1\nsteps:\n  - id: a\n    kind: robot\n", "kind"},
		{"bad on_timeout", "recipe: x\nversion: 1\nsteps:\n  - id: a\n    kind: human\n    human: {on_timeout: shrug}\n", "on_timeout"},
		{"bad timeout", "recipe: x\nversion: 1\nsteps:\n  - id: a\n    kind: exec\n    exec: {argv: [true], timeout: soon}\n", "timeout"},
		{"when unknown step", "recipe: x\nversion: 1\nsteps:\n  - id: a\n    when: \"results.b.exit_code == 0\"\n", "b"},
		{"when step not upstream", "recipe: x\nversion: 1\nsteps:\n  - id: a\n  - id: b\n    when: \"results.a.exit_code == 0\"\n", "depends_on"},
		{"when bad grammar", "recipe: x\nversion: 1\nsteps:\n  - id: a\n    when: \"results.a\"\n", "when"},
		{"template unknown var", "recipe: x\nversion: 1\nsteps:\n  - id: a\n    title: \"{{missing}}\"\n", "missing"},
		{"template results not upstream", "recipe: x\nversion: 1\nsteps:\n  - id: a\n  - id: b\n    title: \"{{results.a.stdout}}\"\n", "depends_on"},
		{"steps ref unknown", "recipe: x\nversion: 1\nsteps:\n  - id: a\n    title: \"{{steps.zzz.title}}\"\n", "zzz"},
		{"steps self ref", "recipe: x\nversion: 1\nsteps:\n  - id: a\n    title: \"{{steps.a.title}}\"\n", "itself"},
		{"include with steps", "recipe: x\nversion: 1\nsteps:\n  - id: a\n    include: other@1\n    steps: [{id: b}]\n", "include"},
		{"repeat without steps", "recipe: x\nversion: 1\nsteps:\n  - id: a\n    repeat: 2\n", "repeat"},
		{"with on non-include", "recipe: x\nversion: 1\nsteps:\n  - id: a\n    with: {x: 1}\n", "with"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseRecipeString(t, tc.yaml)
			if err == nil {
				t.Fatalf("expected error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v; want it to mention %q", err, tc.want)
			}
		})
	}
}

// TestValidateRecipe_AcceptsUpstreamResultsAndStepsRefs: references to
// results of a dependency and to fields of another step are valid.
func TestValidateRecipe_AcceptsUpstreamResultsAndStepsRefs(t *testing.T) {
	r, err := parseRecipeString(t, `
recipe: x
version: 1
vars: {sprint_end: {required: true}}
steps:
  - id: planning
    due: "{{sprint_end}}"
  - id: test
    kind: exec
    exec: {argv: [go, test]}
  - id: retro
    depends_on: [test]
    due: "{{steps.planning.due}} + 5d"
    when: "results.test.exit_code == 0"
    description: "Retro for {{steps.planning.due}}; tests said {{results.test.stdout}}"
`)
	if err != nil {
		t.Fatalf("valid recipe rejected: %v", err)
	}
	if len(r.Steps) != 3 {
		t.Errorf("steps = %d", len(r.Steps))
	}
}
