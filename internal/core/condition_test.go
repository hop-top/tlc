package core

import (
	"strings"
	"testing"
)

func condEnv() CondEnv {
	return CondEnv{
		Inputs: map[string]any{"target": "staging"},
		Vars:   map[string]any{"depth": "deep", "n": 3},
		Results: map[string]map[string]any{
			"lint": {
				"exit_code": float64(0), // JSON numbers decode to float64
				"stdout":    "ok",
				"truncated": false,
				"nested":    map[string]any{"score": 0.9},
			},
			"review": {"verdict": "approved", "attempts": 2},
		},
	}
}

// TestEvalConditionEnv_Grammar pins the `when:` grammar the executor
// evaluates at readiness: results.<step>.<dotted path>, vars.<name>, the
// legacy inputs.<name>/bare forms, equality on any value, ordering on
// numbers only, and two-character operators winning over their prefixes.
func TestEvalConditionEnv_Grammar(t *testing.T) {
	cases := []struct {
		name    string
		expr    string
		want    bool
		wantErr bool
	}{
		{"empty is true", "", true, false},
		{"results string eq", "results.review.verdict == 'approved'", true, false},
		{"results string ne", `results.review.verdict != "approved"`, false, false},
		{"results number eq bare", "results.lint.exit_code == 0", true, false},
		{"results number eq quoted", "results.lint.exit_code == '0'", true, false},
		{"results lt", "results.lint.exit_code < 1", true, false},
		{"results le at boundary", "results.lint.exit_code <= 0", true, false},
		{"results ge false", "results.lint.exit_code >= 1", false, false},
		{"results gt nested float", "results.lint.nested.score > 0.5", true, false},
		{"results int attempts", "results.review.attempts >= 2", true, false},
		{"results bool", "results.lint.truncated == false", true, false},
		{"vars string", "vars.depth == deep", true, false},
		{"vars number ordering", "vars.n >= 3", true, false},
		{"legacy inputs", "inputs.target == 'staging'", true, false},
		{"legacy bare key", "target == staging", true, false},
		{"bare key falls back to vars", "depth == deep", true, false},
		{"unknown step", "results.build.exit_code == 0", false, true},
		{"unknown field", "results.lint.nope == 0", false, true},
		{"unknown nested field", "results.lint.nested.nope == 0", false, true},
		{"unknown var", "vars.missing == x", false, true},
		{"unknown bare key", "missing == x", false, true},
		{"ordering on non-number lhs", "results.review.verdict < 'b'", false, true},
		{"ordering on non-number rhs", "results.lint.exit_code < 'one'", false, true},
		{"missing operator", "results.lint.exit_code", false, true},
		{"empty rhs", "results.lint.exit_code == ", false, true},
	}
	env := condEnv()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EvalConditionEnv(tc.expr, env)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (got=%v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

// TestEvalConditionEnv_UnknownStepErrorNamesIt keeps the fail-loud contract
// actionable: the error says which step or key was not found.
func TestEvalConditionEnv_UnknownStepErrorNamesIt(t *testing.T) {
	_, err := EvalConditionEnv("results.build.exit_code == 0", condEnv())
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !strings.Contains(got, "build") {
		t.Errorf("error %q does not name the unknown step", got)
	}
}
