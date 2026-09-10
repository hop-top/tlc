package cli

// End-to-end coverage for config-driven priority derivation.
//
// Driven through a real config FILE and the real binary, for the reason
// spelled out in priority_vocabulary_e2e_test.go: struct-level tests pass
// while the CLI's own gate rejects or rewrites the value in the field,
// because they never enter through the door the user does. Every case
// below spawns the binary with a config file whose db_path is pinned — an
// unpinned probe resolves the DB through the global project registry, not
// the cwd, and would read (and mutate) an unrelated real database.

import (
	"encoding/json"
	"strings"
	"testing"
)

// derivationConfig declares a three-rule set exercising three different
// condition kinds, plus create-time derivation.
//
// Rule order matters and is asserted: `imminent` is declared before
// `blocker`, so a task that is both due soon AND blocking others must
// come out P0, not P1.
const derivationConfig = `storage:
  db_path: %s
task:
  priority_derivation:
    on_create: true
    rules:
      - name: imminent
        due_within: 24h
        then: P0
      - name: blocker
        min_dependents: 2
        then: P1
      - name: tagged-chore
        tag: chore
        then: P3
`

// derivationNoRulesConfig declares derivation with no rules at all, which
// must behave exactly as a build with no derivation feature.
const derivationNoRulesConfig = `storage:
  db_path: %s
`

// derivationDuplicateNameConfig declares two rules with the same name.
const derivationDuplicateNameConfig = `storage:
  db_path: %s
task:
  priority_derivation:
    rules:
      - name: dupe
        due_within: 24h
        then: P0
      - name: dupe
        min_age: 72h
        then: P1
`

// derivationIdenticalCondConfig declares two rules whose conditions are
// identical and whose verdicts differ. Which one "wins" would be an
// artefact of typing order, so it must be refused rather than resolved.
const derivationIdenticalCondConfig = `storage:
  db_path: %s
task:
  priority_derivation:
    rules:
      - name: first
        due_within: 24h
        then: P0
      - name: second
        due_within: 24h
        then: P2
`

// derivationUnknownPriorityConfig names a priority outside the
// vocabulary in `then`.
const derivationUnknownPriorityConfig = `storage:
  db_path: %s
task:
  priority_derivation:
    rules:
      - name: bogus
        due_within: 24h
        then: NOSUCH
`

// derivationNoConditionConfig declares a rule with no condition, which
// would silently claim every task if treated as a catch-all.
const derivationNoConditionConfig = `storage:
  db_path: %s
task:
  priority_derivation:
    rules:
      - name: empty
        then: P0
`

// derivationCatchAllConfig is one catch-all rule naming P3, with
// on_create deliberately OFF.
//
// A catch-all is the strongest fixture for the guard assertions: it
// matches every task, so any derivation that reaches a protected task
// changes it and the assertion cannot pass by coincidence. on_create off
// means a task starts with no priority, which is what lets each test
// drive the reprioritise pass explicitly rather than racing create.
const derivationCatchAllConfig = `storage:
  db_path: %s
task:
  priority_derivation:
    rules:
      - name: catch-all
        always: true
        then: P3
`

// derivationIncludeActiveConfig is derivationCatchAllConfig with the
// mid-flight guard lifted.
const derivationIncludeActiveConfig = `storage:
  db_path: %s
task:
  priority_derivation:
    include_active: true
    rules:
      - name: catch-all
        always: true
        then: P3
`

// derivFixture builds the isolated world for one case, reusing the
// vocabulary suite's config writer and env builder.
func derivFixture(t *testing.T, template string) (bin, home string, env []string) {
	t.Helper()
	return statusVocabFixture(t, template)
}

// showTaskJSON reads a task back as JSON, so assertions land on what was
// STORED rather than on what a table happened to render.
func showTaskJSON(t *testing.T, bin, home string, env []string, id string) (priority, source, rule string) {
	t.Helper()
	out := runTLCOK(t, bin, home, env, "task", "show", id, "--format", "json")
	var payload struct {
		Task struct {
			Priority string         `json:"priority"`
			Meta     map[string]any `json:"meta"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode task show json: %v\n%s", err, out)
	}
	str := func(k string) string {
		if v, ok := payload.Task.Meta[k].(string); ok {
			return v
		}
		return ""
	}
	return payload.Task.Priority, str("priority_source"), str("priority_rule")
}

// TestDerivationAssignsPriorityToUntriagedTask is acceptance criterion 1:
// with derivation configured, a task carrying no manual priority gets a
// derived one per the rules.
func TestDerivationAssignsPriorityToUntriagedTask(t *testing.T) {
	bin, home, env := derivFixture(t, derivationConfig)

	// Created with no -p and with a due date inside the `imminent`
	// window, so create-time derivation must supply P0.
	out := runTLCOK(t, bin, home, env, "task", "create", "ship it", "--due", "in 2 hours")
	if !strings.Contains(out, "imminent") {
		t.Errorf("create output did not report the deriving rule:\n%s", out)
	}

	prio, source, rule := showTaskJSON(t, bin, home, env, "T-0001")
	if prio != "P0" {
		t.Errorf("derived priority = %q, want P0", prio)
	}
	if source != "derived" {
		t.Errorf("priority_source = %q, want derived", source)
	}
	if rule != "imminent" {
		t.Errorf("priority_rule = %q, want imminent", rule)
	}
}

// TestManualPrioritySurvivesDerivation is THE criterion. A priority a
// human set must survive a derivation pass that would otherwise change
// it. Silently overwriting someone's P3 erodes trust in the whole tool.
func TestManualPrioritySurvivesDerivation(t *testing.T) {
	bin, home, env := derivFixture(t, derivationCatchAllConfig)

	// P0 by hand. The only rule in this config is a catch-all naming P3,
	// so ANY derivation that reaches this task changes it — there is no
	// way for the assertion below to pass by coincidence.
	runTLCOK(t, bin, home, env, "task", "create", "hand-triaged", "-p", "P0")

	prio, source, _ := showTaskJSON(t, bin, home, env, "T-0001")
	if prio != "P0" || source != "manual" {
		t.Fatalf("after manual create: priority=%q source=%q, want P0/manual", prio, source)
	}

	out := runTLCOK(t, bin, home, env, "task", "reprioritise")
	if !strings.Contains(out, "skipped (manual)") && !strings.Contains(out, "1 skipped (manual)") {
		t.Errorf("reprioritise did not report the manual skip:\n%s", out)
	}

	prio, source, rule := showTaskJSON(t, bin, home, env, "T-0001")
	if prio != "P0" {
		t.Errorf("manual priority was CLOBBERED: got %q, want P0", prio)
	}
	if source != "manual" {
		t.Errorf("priority_source = %q, want manual", source)
	}
	if rule != "" {
		t.Errorf("priority_rule = %q on a manual task, want empty", rule)
	}
}

// TestManualPriorityViaUpdateSurvivesDerivation covers the other write
// door. `task update -p` is a separate gate from `task create -p`; both
// must mark provenance or the guard has a hole.
func TestManualPriorityViaUpdateSurvivesDerivation(t *testing.T) {
	bin, home, env := derivFixture(t, derivationCatchAllConfig)

	runTLCOK(t, bin, home, env, "task", "create", "later triaged")
	runTLCOK(t, bin, home, env, "task", "reprioritise")
	if prio, _, _ := showTaskJSON(t, bin, home, env, "T-0001"); prio != "P3" {
		t.Fatalf("pre-condition: derived priority = %q, want P3", prio)
	}

	// A human overrides the derived value.
	runTLCOK(t, bin, home, env, "task", "update", "T-0001", "-p", "P0")
	runTLCOK(t, bin, home, env, "task", "reprioritise")

	prio, source, _ := showTaskJSON(t, bin, home, env, "T-0001")
	if prio != "P0" || source != "manual" {
		t.Errorf("manual override lost: priority=%q source=%q, want P0/manual", prio, source)
	}
}

// TestClearingManualPriorityReturnsToDerivation is acceptance criterion 3:
// clearing a manual value hands the task back to derivation.
func TestClearingManualPriorityReturnsToDerivation(t *testing.T) {
	bin, home, env := derivFixture(t, derivationCatchAllConfig)

	runTLCOK(t, bin, home, env, "task", "create", "opt back in", "-p", "P0")
	runTLCOK(t, bin, home, env, "task", "reprioritise")
	if prio, _, _ := showTaskJSON(t, bin, home, env, "T-0001"); prio != "P0" {
		t.Fatalf("pre-condition: manual P0 not held, got %q", prio)
	}

	runTLCOK(t, bin, home, env, "task", "update", "T-0001", "-p", "-")
	prio, source, _ := showTaskJSON(t, bin, home, env, "T-0001")
	if prio != "" {
		t.Fatalf("clear left a priority behind: %q", prio)
	}
	if source != "" {
		t.Errorf("clear left a provenance marker behind: %q", source)
	}

	runTLCOK(t, bin, home, env, "task", "reprioritise")
	prio, source, rule := showTaskJSON(t, bin, home, env, "T-0001")
	if prio != "P3" {
		t.Errorf("after clear, derived priority = %q, want P3", prio)
	}
	if source != "derived" || rule != "catch-all" {
		t.Errorf("after clear: source=%q rule=%q, want derived/catch-all", source, rule)
	}
}

// TestDerivationIsDeterministic is acceptance criterion 4. Run the same
// pass repeatedly over the same data and the verdict must not move —
// including the ORDER of the report, which a map-iterating implementation
// would shuffle even when every individual verdict held.
func TestDerivationIsDeterministic(t *testing.T) {
	bin, home, env := derivFixture(t, derivationConfig)

	// A spread of tasks: one due imminently, one blocking two others, one
	// tagged chore, one matching nothing, plus the two blockees.
	runTLCOK(t, bin, home, env, "task", "create", "due soon", "--due", "in 2 hours")
	runTLCOK(t, bin, home, env, "task", "create", "the blocker")
	runTLCOK(t, bin, home, env, "task", "create", "a chore", "--tag", "chore")
	runTLCOK(t, bin, home, env, "task", "create", "plain")
	runTLCOK(t, bin, home, env, "task", "create", "blockee one", "--blocked-by", "T-0002")
	runTLCOK(t, bin, home, env, "task", "create", "blockee two", "--blocked-by", "T-0002")

	var first string
	for i := range 8 {
		out := runTLCOK(t, bin, home, env, "task", "reprioritise", "--dry-run", "--format", "json")
		if i == 0 {
			first = out
			continue
		}
		if out != first {
			t.Fatalf("run %d differed from run 0.\nfirst:\n%s\ngot:\n%s", i, first, out)
		}
	}

	// And the verdicts are the RIGHT ones, not merely stable: an
	// implementation that derived nothing at all would also be stable.
	if !strings.Contains(first, `"rule": "imminent"`) {
		t.Errorf("no imminent verdict in stable output:\n%s", first)
	}
	if !strings.Contains(first, `"rule": "blocker"`) {
		t.Errorf("no blocker verdict in stable output:\n%s", first)
	}
}

// TestRulePrecedenceIsDeclarationOrder pins first-match-wins. The blocker
// task below is BOTH due imminently and blocking two others; `imminent`
// is declared first, so P0 is the only defensible answer and P1 would
// mean precedence came from somewhere other than the config.
func TestRulePrecedenceIsDeclarationOrder(t *testing.T) {
	bin, home, env := derivFixture(t, derivationConfig)

	runTLCOK(t, bin, home, env, "task", "create", "both conditions", "--due", "in 2 hours")
	runTLCOK(t, bin, home, env, "task", "create", "blockee one", "--blocked-by", "T-0001")
	runTLCOK(t, bin, home, env, "task", "create", "blockee two", "--blocked-by", "T-0001")
	runTLCOK(t, bin, home, env, "task", "reprioritise")

	prio, _, rule := showTaskJSON(t, bin, home, env, "T-0001")
	if prio != "P0" || rule != "imminent" {
		t.Errorf("precedence broken: priority=%q rule=%q, want P0/imminent", prio, rule)
	}
}

// TestMidFlightTaskIsSkippedByDefault pins the mid-flight guard: a task
// someone is actively working must not be re-ranked under them.
func TestMidFlightTaskIsSkippedByDefault(t *testing.T) {
	bin, home, env := derivFixture(t, derivationCatchAllConfig)

	runTLCOK(t, bin, home, env, "task", "create", "in flight")
	runTLCOK(t, bin, home, env, "task", "claim", "T-0001")

	out := runTLCOK(t, bin, home, env, "task", "reprioritise")
	if !strings.Contains(out, "skipped (in progress)") {
		t.Errorf("reprioritise did not report the mid-flight skip:\n%s", out)
	}
	if prio, _, _ := showTaskJSON(t, bin, home, env, "T-0001"); prio != "" {
		t.Errorf("mid-flight task was reprioritised to %q; want untouched", prio)
	}
}

// TestIncludeActiveLiftsMidFlightGuard pins the opt-in half: a user who
// says include_active gets the in-progress task derived.
func TestIncludeActiveLiftsMidFlightGuard(t *testing.T) {
	bin, home, env := derivFixture(t, derivationIncludeActiveConfig)

	runTLCOK(t, bin, home, env, "task", "create", "in flight")
	runTLCOK(t, bin, home, env, "task", "claim", "T-0001")
	runTLCOK(t, bin, home, env, "task", "reprioritise")

	if prio, _, _ := showTaskJSON(t, bin, home, env, "T-0001"); prio != "P3" {
		t.Errorf("include_active did not lift the guard: priority = %q, want P3", prio)
	}
}

// TestDryRunWritesNothing pins that the preview is a preview.
func TestDryRunWritesNothing(t *testing.T) {
	bin, home, env := derivFixture(t, derivationCatchAllConfig)

	runTLCOK(t, bin, home, env, "task", "create", "untouched")
	out := runTLCOK(t, bin, home, env, "task", "reprioritise", "--dry-run")
	if !strings.Contains(out, "Dry run") {
		t.Errorf("dry run output not labelled:\n%s", out)
	}
	if !strings.Contains(out, "P3") {
		t.Errorf("dry run did not report the verdict it would apply:\n%s", out)
	}
	if prio, _, _ := showTaskJSON(t, bin, home, env, "T-0001"); prio != "" {
		t.Errorf("dry run WROTE a priority: %q", prio)
	}
}

// TestNoDerivationConfiguredIsUnchanged is acceptance criterion 5: with
// no rules declared, behaviour matches today's exactly. Creating without
// -p leaves the priority empty, creating with -p stores it, and the
// command says there is nothing to do rather than inventing a default.
func TestNoDerivationConfiguredIsUnchanged(t *testing.T) {
	bin, home, env := derivFixture(t, derivationNoRulesConfig)

	runTLCOK(t, bin, home, env, "task", "create", "no priority")
	if prio, _, _ := showTaskJSON(t, bin, home, env, "T-0001"); prio != "" {
		t.Errorf("unconfigured derivation invented a priority: %q", prio)
	}

	runTLCOK(t, bin, home, env, "task", "create", "explicit", "-p", "P1")
	if prio, _, _ := showTaskJSON(t, bin, home, env, "T-0002"); prio != "P1" {
		t.Errorf("explicit priority = %q, want P1", prio)
	}

	out := runTLCOK(t, bin, home, env, "task", "reprioritise")
	if !strings.Contains(out, "No priority derivation rules configured") {
		t.Errorf("unconfigured reprioritise did not say so:\n%s", out)
	}
	if prio, _, _ := showTaskJSON(t, bin, home, env, "T-0001"); prio != "" {
		t.Errorf("unconfigured reprioritise wrote a priority: %q", prio)
	}
}

// TestInvalidRuleSetIsReported is acceptance criterion 6. Each case is a
// rule set that cannot be applied coherently; every one must be REPORTED
// and refuse to run, never silently resolved into some subset of rules
// the tool happened to be able to parse.
func TestInvalidRuleSetIsReported(t *testing.T) {
	cases := []struct {
		name     string
		template string
		wants    []string
	}{
		{
			name:     "duplicate rule name",
			template: derivationDuplicateNameConfig,
			wants:    []string{"duplicate rule name", "dupe"},
		},
		{
			name:     "identical conditions",
			template: derivationIdenticalCondConfig,
			wants:    []string{"identical conditions", "first", "second"},
		},
		{
			name:     "then outside vocabulary",
			template: derivationUnknownPriorityConfig,
			wants:    []string{"NOSUCH", "priority vocabulary"},
		},
		{
			name:     "no condition",
			template: derivationNoConditionConfig,
			wants:    []string{"declares no condition", "always: true"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin, home, env := derivFixture(t, tc.template)

			out, code := runTLC(t, bin, home, env, "task", "reprioritise")
			if code == 0 {
				t.Fatalf("invalid rule set was accepted (exit 0):\n%s", out)
			}
			for _, want := range tc.wants {
				if !strings.Contains(out, want) {
					t.Errorf("error message missing %q:\n%s", want, out)
				}
			}
		})
	}
}

// TestReprioritiseNamedTasksOnly pins that arguments narrow the pass:
// a task nobody named must not be touched.
func TestReprioritiseNamedTasksOnly(t *testing.T) {
	bin, home, env := derivFixture(t, derivationCatchAllConfig)

	runTLCOK(t, bin, home, env, "task", "create", "named")
	runTLCOK(t, bin, home, env, "task", "create", "unnamed")
	runTLCOK(t, bin, home, env, "task", "reprioritise", "T-0001")

	if prio, _, _ := showTaskJSON(t, bin, home, env, "T-0001"); prio != "P3" {
		t.Errorf("named task not derived: %q", prio)
	}
	if prio, _, _ := showTaskJSON(t, bin, home, env, "T-0002"); prio != "" {
		t.Errorf("unnamed task was derived to %q; want untouched", prio)
	}
}

// TestDerivationIsIdempotent pins that a second pass over already-derived
// tasks writes nothing new. A pass that rewrote every row to the value it
// already held would churn UpdatedAt, which feeds staleness and every
// "recently touched" view.
func TestDerivationIsIdempotent(t *testing.T) {
	bin, home, env := derivFixture(t, derivationCatchAllConfig)

	runTLCOK(t, bin, home, env, "task", "create", "settle")
	first := runTLCOK(t, bin, home, env, "task", "reprioritise")
	if !strings.Contains(first, "1 derived") {
		t.Fatalf("first pass did not derive:\n%s", first)
	}

	second := runTLCOK(t, bin, home, env, "task", "reprioritise")
	if !strings.Contains(second, "0 derived") {
		t.Errorf("second pass re-derived an already-settled task:\n%s", second)
	}
}
