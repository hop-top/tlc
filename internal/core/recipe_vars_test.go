package core

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseVarFlags(t *testing.T) {
	got, err := ParseVarFlags([]string{"pr=42,depth=deep", "note=a=b", "team=platform"})
	if err != nil {
		t.Fatalf("ParseVarFlags: %v", err)
	}
	want := map[string]string{"pr": "42", "depth": "deep", "note": "a=b", "team": "platform"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("vars = %v; want %v", got, want)
	}
	for _, bad := range []string{"novalue", "=x", "a=1,,b=2", "a=1,b"} {
		if _, err := ParseVarFlags([]string{bad}); err == nil {
			t.Errorf("%q accepted; want error", bad)
		}
	}
}

func TestResolveRecipeVars(t *testing.T) {
	r := &Recipe{Name: "x", Vars: map[string]VarDef{
		"pr":    {Required: true},
		"depth": {Default: "standard"},
		"n":     {Default: 3},
	}}
	got, err := ResolveRecipeVars(r, map[string]string{"pr": "42"})
	if err != nil {
		t.Fatalf("ResolveRecipeVars: %v", err)
	}
	if got["pr"] != "42" || got["depth"] != "standard" || got["n"] != 3 {
		t.Errorf("vars = %v", got)
	}

	_, err = ResolveRecipeVars(r, nil)
	if err == nil || !strings.Contains(err.Error(), "pr") || !strings.Contains(err.Error(), "--var") {
		t.Errorf("missing required: err = %v; want it to name pr and the --var fix", err)
	}
	_, err = ResolveRecipeVars(r, map[string]string{"pr": "1", "bogus": "x"})
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Errorf("unknown var: err = %v", err)
	}
}

func TestRender_Scopes(t *testing.T) {
	scope := Scope{
		Vars:    map[string]any{"pr": "42", "n": 3},
		Subject: map[string]any{"id": "task_s", "title": "Fix login"},
		Run:     map[string]any{"iteration": 2},
		Steps:   map[string]map[string]any{"planning": {"due": "2026-10-01", "title": "Plan"}},
	}
	cases := map[string]string{
		"Review {{pr}} #{{ n }}":                 "Review 42 #3",
		"{{vars.pr}} again":                      "42 again",
		"for {{subject.title}} ({{subject.id}})": "for Fix login (task_s)",
		"round {{run.iteration}}":                "round 2",
		"after {{steps.planning.due}}":           "after 2026-10-01",
		"keep {{results.lint.stdout}} verbatim":  "keep {{results.lint.stdout}} verbatim",
		"no refs":                                "no refs",
	}
	for in, want := range cases {
		got, err := Render(in, scope)
		if err != nil {
			t.Errorf("Render(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("Render(%q) = %q; want %q", in, got, want)
		}
	}
	for _, bad := range []string{"{{missing}}", "{{subject.nope}}", "{{steps.zzz.due}}", "{{steps.planning.nope}}"} {
		if _, err := Render(bad, scope); err == nil {
			t.Errorf("Render(%q) succeeded; want error", bad)
		}
	}
}

func TestRenderStep_WalksTemplatableFields(t *testing.T) {
	scope := Scope{Vars: map[string]any{"pr": "42", "dir": "svc"}, Subject: map[string]any{"description": "desc"}}
	step := RecipeStep{
		ID: "lint", Title: "Lint {{pr}}", Description: "{{subject.description}}",
		Assignee: "@{{pr}}", Due: DueSpec{Raw: "{{pr}}"},
		When: "results.build.exit_code == 0",
		Exec: &ExecSpec{Argv: []string{"lint", "--pr={{pr}}"}, Cwd: "{{dir}}", Env: map[string]string{"PR": "{{pr}}"}},
		Gate: &StepGate{Contract: "c-{{pr}}"},
		With: map[string]any{"target": "{{pr}}", "n": 1},
	}
	got, err := RenderStep(step, scope)
	if err != nil {
		t.Fatalf("RenderStep: %v", err)
	}
	if got.Title != "Lint 42" || got.Description != "desc" || got.Assignee != "@42" || got.Due.Raw != "42" {
		t.Errorf("scalars: %+v", got)
	}
	if got.Exec.Argv[1] != "--pr=42" || got.Exec.Cwd != "svc" || got.Exec.Env["PR"] != "42" || got.Gate.Contract != "c-42" {
		t.Errorf("nested: %+v %+v", got.Exec, got.Gate)
	}
	if got.With["target"] != "42" || got.With["n"] != 1 {
		t.Errorf("with: %v", got.With)
	}
	// The input is not mutated: rendering returns a copy.
	if step.Title != "Lint {{pr}}" || step.Exec.Argv[1] != "--pr={{pr}}" {
		t.Errorf("RenderStep mutated its input: %+v", step)
	}
}
