package cli

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// gateRecipe has one human gate with a step that needs the approval and one
// that merely follows the gate.
const gateRecipe = `recipe: gate-probe
version: 1.0.0
steps:
  - id: prep
    kind: exec
    title: Prepare
    exec:
      argv: [echo, prepared]
  - id: review
    kind: human
    title: Review the change
    depends_on: [prep]
  - id: merge
    kind: exec
    title: Merge when approved
    depends_on: [review]
    when: "results.review.decision == 'approved'"
    exec:
      argv: [echo, merged]
  - id: after
    kind: exec
    title: Follow the gate
    depends_on: [review]
    exec:
      argv: [echo, followed]
`

// runGateProbe lays the gate recipe and one run manifest out and runs it.
func runGateProbe(t *testing.T, manifest string, extra ...string) (string, error) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	inRepoRoot(t)
	dir := t.TempDir()
	path := writeRecipeFixture(t, dir, "gate-probe.yaml", gateRecipe)
	writeRunFixture(t, dir, "gate-probe", "run", manifest)
	return runRecipeCmd(t, append([]string{"test", path, "run"}, extra...)...)
}

// An approved gate is decided, the run resumes past it, and the recorded
// decision is what the downstream when: reads.
func TestRecipeTest_Gates_ApproveRunsPastTheGate(t *testing.T) {
	out, err := runGateProbe(t, "expected_exit: 0\ngates:\n  review: approve\n")
	if err != nil {
		t.Fatalf("expected exit 0; got %v (code %d)\n%s", err, exitCodeOf(err), out)
	}
	for _, want := range []string{"Merge when approved", "Follow the gate", "result: PASS"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestRecipeTest_Gates_ApproveWithByAndNote(t *testing.T) {
	out, err := runGateProbe(t, `expected_exit: 0
gates:
  review:
    decision: approve
    by: lead
    note: fine
`)
	if err != nil || !strings.Contains(out, "result: PASS") {
		t.Fatalf("expected PASS; got %v\n%s", err, out)
	}
}

// A declared reject is the expected end state: the gate stays blocking, the
// steps behind it are never reached, and the run passes.
func TestRecipeTest_Gates_RejectBlocksTheRest(t *testing.T) {
	out, err := runGateProbe(t, `expected_exit: 0
gates:
  review:
    decision: reject
    reason: not ready
`)
	if err != nil {
		t.Fatalf("expected exit 0; got %v (code %d)\n%s", err, exitCodeOf(err), out)
	}
	if !strings.Contains(out, "result: PASS") {
		t.Errorf("output lacks PASS:\n%s", out)
	}
	for _, unwanted := range []string{"Merge when approved", "Follow the gate"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("step %q ran behind a rejected gate:\n%s", unwanted, out)
		}
	}
}

func TestRecipeTest_Gates_MissingDecisionFails(t *testing.T) {
	out, err := runGateProbe(t, "expected_exit: 0\n")
	if code := exitCodeOf(err); code != recipeTestExitFailed {
		t.Fatalf("exit = %d (%v); want %d\n%s", code, err, recipeTestExitFailed, out)
	}
	if !strings.Contains(out, `gate "review": no decision in test.yaml gates`) {
		t.Errorf("output does not name the undecided gate:\n%s", out)
	}
}

// A gate the --task subset never materializes needs no decision.
func TestRecipeTest_Gates_TaskSubsetBeforeGateNeedsNoDecision(t *testing.T) {
	out, err := runGateProbe(t, "expected_exit: 0\n", "--task", "1")
	if err != nil || !strings.Contains(out, "result: PASS") {
		t.Fatalf("expected PASS; got %v (code %d)\n%s", err, exitCodeOf(err), out)
	}
}

// The same decisions apply when recording: no prompt, no TTY.
func TestRecipeTest_Gates_RecordAppliesDecisionsNonInteractively(t *testing.T) {
	out, err := runGateProbe(t, "expected_exit: 0\ngates:\n  review: approve\n", "--record")
	if err != nil {
		t.Fatalf("expected exit 0; got %v (code %d)\n%s", err, exitCodeOf(err), out)
	}
	if !strings.Contains(out, "Merge when approved") || !strings.Contains(out, "result: PASS") {
		t.Errorf("recording did not run past the gate:\n%s", out)
	}
}

// A gate that was decided without the fixture (here the recipe's own
// timeout rejects it) does not match a declared approval.
func TestRecipeTest_Gates_DeclaredDecisionMustMatchEndState(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	inRepoRoot(t)
	dir := t.TempDir()
	path := writeRecipeFixture(t, dir, "gate-timeout.yaml", strings.NewReplacer(
		"gate-probe", "gate-timeout",
		"    title: Review the change\n", "    title: Review the change\n    human:\n      timeout: 1ms\n      on_timeout: reject\n",
	).Replace(gateRecipe))
	writeRunFixture(t, dir, "gate-timeout", "run", "expected_exit: 0\ngates:\n  review: approve\n")

	out, err := runRecipeCmd(t, "test", path, "run")
	if code := exitCodeOf(err); code != recipeTestExitFailed {
		t.Fatalf("exit = %d (%v); want %d\n%s", code, err, recipeTestExitFailed, out)
	}
	if !strings.Contains(out, `gate "review"`) {
		t.Errorf("output does not name the mismatched gate:\n%s", out)
	}
}

func TestRecipeTest_Gates_FixtureErrorsAreSetupFailures(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
	}{
		{"unknown manifest key", "expected_exit: 0\ngate:\n  review: approve\n"},
		{"gate step is not in the recipe", "gates:\n  nope: approve\n"},
		{"gate step is not a human step", "gates:\n  prep: approve\n"},
		{"reject without a reason", "gates:\n  review: reject\n"},
		{"decision other than approve or reject", "gates:\n  review:\n    decision: maybe\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runGateProbe(t, tc.manifest)
			if code := exitCodeOf(err); code != recipeTestExitSandbox {
				t.Errorf("exit = %d (%v); want %d\n%s", code, err, recipeTestExitSandbox, out)
			}
		})
	}
}

// A gate declared on a human step that the run's vars leave out is not an
// error and needs no decision, the same as a gate the --task subset skips:
// the step is never materialized, so the run passes with it absent.
func TestRecipeTest_Gates_DeclaredGateOnVarsDroppedStepNeedsNoDecision(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	inRepoRoot(t)
	dir := t.TempDir()
	path := writeRecipeFixture(t, dir, "gate-vars.yaml", `recipe: gate-vars
version: 1.0.0
vars:
  review:
    description: whether a human reviews
    required: true
steps:
  - id: prep
    kind: exec
    title: Prepare
    exec:
      argv: [echo, prepared]
  - id: signoff
    kind: human
    title: Sign off the change
    depends_on: [prep]
    when: "vars.review == 'yes'"
  - id: ship
    kind: exec
    title: Ship it
    depends_on: [signoff]
    exec:
      argv: [echo, shipped]
`)
	writeRunFixture(t, dir, "gate-vars", "run",
		"expected_exit: 0\nvars:\n  review: 'no'\ngates:\n  signoff: approve\n")

	out, err := runRecipeCmd(t, "test", path, "run")
	if err != nil {
		t.Fatalf("expected exit 0; got %v (code %d)\n%s", err, exitCodeOf(err), out)
	}
	if strings.Contains(out, "Sign off the change") || !strings.Contains(out, "Ship it") || !strings.Contains(out, "result: PASS") {
		t.Errorf("want the gate absent, its dependent run, PASS:\n%s", out)
	}
}
