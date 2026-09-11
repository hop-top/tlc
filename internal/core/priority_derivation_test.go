package core

// Unit coverage for the derivation engine itself, with a PINNED clock.
//
// The e2e suite drives the real binary through a real config file and is
// the authority on behavior; these cover the cases a subprocess cannot
// reach cheaply — a task aged 30 days, a boundary crossed by one second —
// without making the test suite wait for either.

import (
	"testing"
	"time"

	"hop.top/tlc/internal/config"
)

// fixedNow is the instant every test here pretends it is. Pinned rather
// than time.Now so a `min_age` case does not need a task genuinely
// created a month ago, and so a boundary case cannot flake by running
// across the boundary it is testing.
var fixedNow = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func testDeriver(t *testing.T, cfg *config.TaskConfig) *PriorityDeriver {
	t.Helper()
	d, err := NewPriorityDeriver(cfg)
	if err != nil {
		t.Fatalf("NewPriorityDeriver: %v", err)
	}
	if d == nil {
		t.Fatal("NewPriorityDeriver returned nil for a non-empty rule set")
	}
	d.SetNow(func() time.Time { return fixedNow })
	return d
}

func TestPrioritySourceDefaults(t *testing.T) {
	cases := []struct {
		name string
		task *Task
		want string
	}{
		{
			name: "unmarked with a priority reads manual",
			task: &Task{Priority: PriorityP1},
			want: PrioritySourceManual,
		},
		{
			name: "unmarked with no priority reads derived",
			task: &Task{},
			want: PrioritySourceDerived,
		},
		{
			name: "explicit manual marker wins",
			task: &Task{Meta: map[string]any{MetaPrioritySource: PrioritySourceManual}},
			want: PrioritySourceManual,
		},
		{
			name: "explicit derived marker wins over a set priority",
			task: &Task{
				Priority: PriorityP1,
				Meta:     map[string]any{MetaPrioritySource: PrioritySourceDerived},
			},
			want: PrioritySourceDerived,
		},
		{
			name: "garbage marker falls back to the unmarked reading",
			task: &Task{
				Priority: PriorityP1,
				Meta:     map[string]any{MetaPrioritySource: "nonsense"},
			},
			want: PrioritySourceManual,
		},
		{
			name: "nil task",
			task: nil,
			want: PrioritySourceDerived,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PrioritySource(tc.task); got != tc.want {
				t.Errorf("PrioritySource = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestNilDeriverIsANoOp pins the shape that makes "derivation off behaves
// exactly as before" true by construction: every entry point on a nil
// deriver returns the do-nothing answer rather than panicking or
// inventing a verdict.
func TestNilDeriverIsANoOp(t *testing.T) {
	d, err := NewPriorityDeriver(&config.TaskConfig{})
	if err != nil {
		t.Fatalf("NewPriorityDeriver on an empty config: %v", err)
	}
	if d != nil {
		t.Fatalf("empty rule set produced a non-nil deriver")
	}

	task := &Task{ID: "T-0001", Status: StatusTodo}
	if got := d.Derive(task, nil); got.Outcome != OutcomeUnchanged {
		t.Errorf("nil deriver Derive outcome = %v, want unchanged", got.Outcome)
	}
	if got := d.Apply(task, nil); got.Changed() {
		t.Errorf("nil deriver Apply reported a change")
	}
	if got := d.DeriveForCreate(task); got.Changed() {
		t.Errorf("nil deriver DeriveForCreate reported a change")
	}
	if d.DerivesOnCreate() {
		t.Errorf("nil deriver claims to derive on create")
	}
	if task.Priority != "" || task.Meta != nil {
		t.Errorf("nil deriver mutated the task: priority=%q meta=%v", task.Priority, task.Meta)
	}
}

func TestMinAgeCondition(t *testing.T) {
	cfg := &config.TaskConfig{
		PriorityDeriv: config.PriorityDerivationConfig{
			Rules: []config.PriorityDerivationRule{
				{Name: "stale", MinAge: 30 * 24 * time.Hour, Then: "P1"},
			},
		},
	}
	d := testDeriver(t, cfg)

	// One second short of the threshold: no match. One second past: match.
	// Boundaries are where an off-by-one lives, and a pinned clock is the
	// only way to test them without a flake.
	young := &Task{ID: "A", Status: StatusTodo, CreatedAt: fixedNow.Add(-30*24*time.Hour + time.Second)}
	old := &Task{ID: "B", Status: StatusTodo, CreatedAt: fixedNow.Add(-30*24*time.Hour - time.Second)}

	if got := d.Derive(young, nil); got.Changed() {
		t.Errorf("task one second below min_age derived to %q", got.To)
	}
	got := d.Derive(old, nil)
	if !got.Changed() || got.To != PriorityP1 {
		t.Errorf("task past min_age: outcome=%v to=%q, want derived/P1", got.Outcome, got.To)
	}
}

func TestDueWithinTreatsOverdueAsInside(t *testing.T) {
	cfg := &config.TaskConfig{
		PriorityDeriv: config.PriorityDerivationConfig{
			Rules: []config.PriorityDerivationRule{
				{Name: "imminent", DueWithin: 24 * time.Hour, Then: "P0"},
			},
		},
	}
	d := testDeriver(t, cfg)

	overdue := fixedNow.Add(-72 * time.Hour)
	soon := fixedNow.Add(2 * time.Hour)
	far := fixedNow.Add(72 * time.Hour)

	// An overdue task is not "further out than 24h" — it is already past.
	// A rule that compared only the absolute distance would exclude it,
	// which is exactly backwards for the most urgent task in the set.
	for _, tc := range []struct {
		name string
		due  *time.Time
		want bool
	}{
		{"overdue", &overdue, true},
		{"due soon", &soon, true},
		{"due far out", &far, false},
		{"no due date", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := &Task{ID: "T", Status: StatusTodo, DueAt: tc.due}
			if got := d.Derive(task, nil).Changed(); got != tc.want {
				t.Errorf("derived = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCountDependentsIgnoresAbsentBlockers(t *testing.T) {
	target := &Task{ID: "T-0001"}
	a := &Task{ID: "T-0002", Meta: map[string]any{"blocked_by": []string{"T-0001"}}}
	b := &Task{ID: "T-0003", Meta: map[string]any{"blocked_by": []string{"T-0001", "other/T-0009"}}}

	counts := CountDependents([]*Task{target, a, b})
	if counts["T-0001"] != 2 {
		t.Errorf("dependents of T-0001 = %d, want 2", counts["T-0001"])
	}
	// A blocker naming a task outside the set must not appear at all:
	// counting it would make the answer depend on which projects
	// happened to be loaded.
	if n, ok := counts["other/T-0009"]; ok {
		t.Errorf("counted an out-of-set blocker: %d", n)
	}
}

func TestTerminalAndActiveGuards(t *testing.T) {
	rules := []config.PriorityDerivationRule{{Name: "all", Always: true, Then: "P3"}}

	guarded := testDeriver(t, &config.TaskConfig{
		PriorityDeriv: config.PriorityDerivationConfig{Rules: rules},
	})
	lifted := testDeriver(t, &config.TaskConfig{
		PriorityDeriv: config.PriorityDerivationConfig{Rules: rules, IncludeActive: true},
	})

	active := func() *Task { return &Task{ID: "T", Status: StatusInProgress} }
	done := func() *Task { return &Task{ID: "T", Status: StatusDone} }

	if got := guarded.Derive(active(), nil); got.Outcome != OutcomeSkippedActive {
		t.Errorf("mid-flight guard: outcome = %v, want skipped-active", got.Outcome)
	}
	if got := lifted.Derive(active(), nil); got.Outcome != OutcomeDerived {
		t.Errorf("include_active: outcome = %v, want derived", got.Outcome)
	}
	// Terminal is unconditional: include_active lifts the mid-flight
	// guard, not the finished-work one.
	if got := guarded.Derive(done(), nil); got.Outcome != OutcomeSkippedTerminal {
		t.Errorf("terminal guard: outcome = %v, want skipped-terminal", got.Outcome)
	}
	if got := lifted.Derive(done(), nil); got.Outcome != OutcomeSkippedTerminal {
		t.Errorf("terminal guard under include_active: outcome = %v, want skipped-terminal", got.Outcome)
	}
}

// TestMatchingRuleNamingTheCurrentValueIsNotAChange pins idempotence at
// the engine level: a rule that agrees with the stored value produces no
// write, so a second pass does not churn UpdatedAt.
func TestMatchingRuleNamingTheCurrentValueIsNotAChange(t *testing.T) {
	d := testDeriver(t, &config.TaskConfig{
		PriorityDeriv: config.PriorityDerivationConfig{
			Rules: []config.PriorityDerivationRule{{Name: "all", Always: true, Then: "P3"}},
		},
	})

	task := &Task{
		ID:       "T",
		Status:   StatusTodo,
		Priority: PriorityP3,
		Meta:     map[string]any{MetaPrioritySource: PrioritySourceDerived},
	}
	got := d.Derive(task, nil)
	if got.Changed() {
		t.Errorf("re-deriving the stored value reported a change")
	}
	if got.RuleName != "all" {
		t.Errorf("rule name = %q, want the matching rule reported anyway", got.RuleName)
	}
}

// TestNewPriorityDeriverRejectsInvalidRuleSet pins that construction is
// the gate: a caller cannot obtain a deriver over a rule set that config
// validation refuses, so no command can half-apply one.
func TestNewPriorityDeriverRejectsInvalidRuleSet(t *testing.T) {
	cases := map[string][]config.PriorityDerivationRule{
		"duplicate name": {
			{Name: "x", Always: true, Then: "P0"},
			{Name: "x", MinAge: time.Hour, Then: "P1"},
		},
		"identical conditions": {
			{Name: "a", DueWithin: time.Hour, Then: "P0"},
			{Name: "b", DueWithin: time.Hour, Then: "P1"},
		},
		"unknown priority": {
			{Name: "a", Always: true, Then: "NOPE"},
		},
		"no condition": {
			{Name: "a", Then: "P0"},
		},
		"always plus another condition": {
			{Name: "a", Always: true, MinAge: time.Hour, Then: "P0"},
		},
		"unnamed": {
			{Always: true, Then: "P0"},
		},
	}

	for name, rules := range cases {
		t.Run(name, func(t *testing.T) {
			d, err := NewPriorityDeriver(&config.TaskConfig{
				PriorityDeriv: config.PriorityDerivationConfig{Rules: rules},
			})
			if err == nil {
				t.Fatalf("invalid rule set accepted; deriver = %v", d)
			}
			if d != nil {
				t.Errorf("a deriver was returned alongside the error")
			}
		})
	}
}

// TestDeriveForCreateLeavesAnExplicitPriorityAlone pins the create-time
// half of the manual guard: at create, "has a priority" means the user
// typed -p, and nothing may second-guess that.
func TestDeriveForCreateLeavesAnExplicitPriorityAlone(t *testing.T) {
	d := testDeriver(t, &config.TaskConfig{
		PriorityDeriv: config.PriorityDerivationConfig{
			OnCreate: true,
			Rules:    []config.PriorityDerivationRule{{Name: "all", Always: true, Then: "P3"}},
		},
	})

	typed := &Task{ID: "T", Status: StatusTodo, Priority: PriorityP0}
	if got := d.DeriveForCreate(typed); got.Changed() {
		t.Errorf("create-time derivation overwrote an explicit -p: %v", got)
	}
	if typed.Priority != PriorityP0 {
		t.Errorf("priority = %q, want P0", typed.Priority)
	}

	untyped := &Task{ID: "U", Status: StatusTodo}
	if got := d.DeriveForCreate(untyped); !got.Changed() || untyped.Priority != PriorityP3 {
		t.Errorf("create-time derivation did not fire: %v, priority=%q", got, untyped.Priority)
	}
	if PrioritySource(untyped) != PrioritySourceDerived {
		t.Errorf("derived task not marked derived: %v", untyped.Meta)
	}
}

// TestOnCreateOffMeansNoCreateTimeDerivation pins the opt-in.
func TestOnCreateOffMeansNoCreateTimeDerivation(t *testing.T) {
	d := testDeriver(t, &config.TaskConfig{
		PriorityDeriv: config.PriorityDerivationConfig{
			Rules: []config.PriorityDerivationRule{{Name: "all", Always: true, Then: "P3"}},
		},
	})
	task := &Task{ID: "T", Status: StatusTodo}
	if got := d.DeriveForCreate(task); got.Changed() {
		t.Errorf("on_create off still derived: %v", got)
	}
	if task.Priority != "" {
		t.Errorf("priority = %q, want empty", task.Priority)
	}
}

// TestReportSortIsStable pins that the report's ORDER is deterministic,
// not merely its verdicts. A caller fed rows in storage order must render
// the same text as one fed them reversed.
func TestReportSortIsStable(t *testing.T) {
	forward := &DerivationReport{}
	reverse := &DerivationReport{}
	ids := []string{"T-0003", "T-0001", "T-0002"}
	for _, id := range ids {
		forward.Add(DerivationResult{TaskID: id})
	}
	for i := len(ids) - 1; i >= 0; i-- {
		reverse.Add(DerivationResult{TaskID: ids[i]})
	}
	forward.Sort()
	reverse.Sort()

	for i := range forward.Results {
		if forward.Results[i].TaskID != reverse.Results[i].TaskID {
			t.Fatalf("sorted reports differ at %d: %q vs %q",
				i, forward.Results[i].TaskID, reverse.Results[i].TaskID)
		}
	}
	if forward.Results[0].TaskID != "T-0001" {
		t.Errorf("first sorted ID = %q, want T-0001", forward.Results[0].TaskID)
	}
}

// TestClearPrioritySourceRoundTrip pins the opt-back-in gesture at the
// engine level: after a clear, a previously-manual task is derivable
// again.
func TestClearPrioritySourceRoundTrip(t *testing.T) {
	d := testDeriver(t, &config.TaskConfig{
		PriorityDeriv: config.PriorityDerivationConfig{
			Rules: []config.PriorityDerivationRule{{Name: "all", Always: true, Then: "P3"}},
		},
	})

	task := &Task{ID: "T", Status: StatusTodo, Priority: PriorityP0}
	MarkPriorityManual(task)
	if got := d.Derive(task, nil); got.Outcome != OutcomeSkippedManual {
		t.Fatalf("manual task not protected: %v", got.Outcome)
	}

	task.Priority = ""
	ClearPrioritySource(task)
	got := d.Apply(task, nil)
	if !got.Changed() || task.Priority != PriorityP3 {
		t.Errorf("after clear: outcome=%v priority=%q, want derived/P3", got.Outcome, task.Priority)
	}
	if v, ok := task.Meta[MetaPriorityRule].(string); !ok || v != "all" {
		t.Errorf("priority_rule = %v, want all", task.Meta[MetaPriorityRule])
	}
}
