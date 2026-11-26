package core

import (
	"reflect"
	"strings"
	"testing"
)

func TestCreateTimeWhen(t *testing.T) {
	vars := map[string]any{"target": "staging", "n": 3, "on": true}
	cases := []struct {
		name        string
		when        string
		verdict     bool
		known       bool
		errContains string
	}{
		{"empty", "", true, false, ""},
		{"true", "vars.target == 'staging'", true, true, ""},
		{"false", "vars.target == 'production'", false, true, ""},
		{"bare identifier", "target != 'staging'", false, true, ""},
		{"wrapped in braces", "{{ vars.target == 'staging' }}", true, true, ""},
		{"number ordering", "vars.n > 2", true, true, ""},
		{"bool", "vars.on == false", false, true, ""},
		{"results is run-time", "results.lint.exit_code == 0", true, false, ""},
		{"results in quoted rhs stays run-time", "vars.target == 'results.x'", true, false, ""},
		{"CEL left to the executor", "vars.target == 'staging' && vars.n > 1", true, false, ""},
		{"undefined var", "vars.nope == 'x'", true, false, "unknown var key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verdict, known, err := CreateTimeWhen(RecipeStep{ID: "s", When: tc.when}, vars)
			if tc.errContains == "" && err != nil {
				t.Fatalf("err = %v", err)
			}
			if tc.errContains != "" && (err == nil || !strings.Contains(err.Error(), tc.errContains)) {
				t.Fatalf("err = %v; want it to contain %q", err, tc.errContains)
			}
			if known != tc.known || (known && verdict != tc.verdict) {
				t.Errorf("verdict/known = %v/%v; want %v/%v", verdict, known, tc.verdict, tc.known)
			}
			if !known && !verdict {
				t.Errorf("an undecided condition must never read as false")
			}
		})
	}
}

func dropSteps() []RecipeStep {
	return []RecipeStep{
		{ID: "validate", Ordinal: 1},
		{ID: "deploy-staging", Ordinal: 2, DependsOn: []string{"validate"}, When: "vars.target == 'staging'"},
		{ID: "verify-staging", Ordinal: 3, DependsOn: []string{"deploy-staging"}, When: "vars.target == 'staging'"},
		{ID: "deploy-prod", Ordinal: 4, DependsOn: []string{"validate"}, When: "vars.target == 'production'"},
		{ID: "report", Ordinal: 5, DependsOn: []string{"verify-staging", "deploy-prod"}},
		{ID: "after-results", Ordinal: 6, DependsOn: []string{"report"}, When: "results.report.exit_code == 0"},
	}
}

func stepIDsOf(steps []RecipeStep) []string {
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = s.ID
	}
	return out
}

func TestDropFalseWhen(t *testing.T) {
	in := dropSteps()
	got := DropFalseWhen(in, map[string]any{"target": "staging"})

	if want := []string{"validate", "deploy-staging", "verify-staging", "report", "after-results"}; !reflect.DeepEqual(stepIDsOf(got.Steps), want) {
		t.Errorf("kept = %v; want %v", stepIDsOf(got.Steps), want)
	}
	if want := []string{"deploy-prod"}; !reflect.DeepEqual(got.Dropped, want) {
		t.Errorf("dropped = %v; want %v", got.Dropped, want)
	}
	byID := map[string]RecipeStep{}
	for _, s := range got.Steps {
		byID[s.ID] = s
	}
	if deps := byID["report"].DependsOn; !reflect.DeepEqual(deps, []string{"verify-staging"}) {
		t.Errorf("report depends_on = %v; want the edge to the dropped step removed", deps)
	}
	if want := map[string][]string{"report": {"deploy-prod"}}; !reflect.DeepEqual(got.Deps, want) {
		t.Errorf("deps = %v; want %v", got.Deps, want)
	}
	// True and run-time conditions are kept verbatim for the executor.
	if byID["deploy-staging"].When != "vars.target == 'staging'" || byID["after-results"].When != "results.report.exit_code == 0" {
		t.Errorf("when rewritten: %q / %q", byID["deploy-staging"].When, byID["after-results"].When)
	}
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "deploy-prod") {
		t.Errorf("warnings = %v; want one naming deploy-prod", got.Warnings)
	}
	// The input is not mutated.
	if !reflect.DeepEqual(in, dropSteps()) {
		t.Errorf("input steps were modified")
	}
}

func TestDropFalseWhen_BothBranchesFalse(t *testing.T) {
	got := DropFalseWhen(dropSteps(), map[string]any{"target": "other"})
	if want := []string{"validate", "report", "after-results"}; !reflect.DeepEqual(stepIDsOf(got.Steps), want) {
		t.Errorf("kept = %v; want %v", stepIDsOf(got.Steps), want)
	}
	for _, s := range got.Steps {
		if s.ID == "report" && len(s.DependsOn) != 0 {
			t.Errorf("report depends_on = %v; want none left", s.DependsOn)
		}
	}
}

func TestDropFalseWhen_UndefinedVarKeepsStep(t *testing.T) {
	got := DropFalseWhen([]RecipeStep{{ID: "a", When: "vars.nope == 'x'"}}, map[string]any{})
	if len(got.Steps) != 1 || len(got.Dropped) != 0 {
		t.Fatalf("steps/dropped = %v/%v; an unevaluable when must not drop the step", got.Steps, got.Dropped)
	}
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "left to the executor") {
		t.Errorf("warnings = %v; want the evaluation error surfaced", got.Warnings)
	}
}
