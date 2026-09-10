package core

// Config-driven priority derivation.
//
// Priority used to be manual-only: `task create -p` / `task update -p`
// and nothing else. This makes it command-managed — a rule set in
// `task.priority_derivation` can supply a priority for tasks nobody has
// triaged — WITHOUT taking the field away from the person who did.
//
// The whole design turns on one property: a priority a human set is
// never overwritten by a rule. Everything else here (the manual marker,
// the mid-flight guard, the explicit command) exists to hold it.
//
// WHEN derivation runs is the load-bearing decision, so it is stated
// here rather than buried:
//
//   - On an explicit `tlc task reprioritise`. This is the only path that
//     changes an EXISTING priority. A priority that moves under the user
//     with no action they took is the worst outcome this feature can
//     produce, and an explicit verb is the only shape that cannot
//     produce it.
//   - On create, for a task with no priority, and only when the user
//     opts in with `on_create: true`. There is no existing value to
//     contradict and the result is printed in the create output, so it
//     surprises nobody — but it is still a value they did not type, so
//     it is opt-in rather than default.
//   - NEVER on read. A read path that derives would make `task list` and
//     `task show` disagree with the stored row, make two consecutive
//     `list`s disagree with each other as the clock moves a task across
//     a `due_within` boundary, and give no moment at which the user
//     could have consented. It would also make every read a write, or
//     else a lie.
//   - NEVER on a background pass. Same objection as read, plus the user
//     is not present to see it happen.
//
// Rule precedence is DECLARATION ORDER, first match wins, and identical
// conditions are refused at validation rather than resolved. That is the
// lesson `GetWorkflowForTags` paid for: it used map iteration order,
// measured a 36/4 split across identical inputs, and settled on refusing
// ambiguity by name. Here the config schema is a list, so order exists
// and is the user's own; what a list cannot rule out is two rules that
// test exactly the same thing, and those are rejected by name in
// config.ValidatePriorityDerivation.

import (
	"fmt"
	"sort"
	"time"

	"hop.top/tlc/internal/config"
)

// MetaPrioritySource is the task-meta key recording WHO set the current
// priority: a human, or a derivation rule.
//
// It lives in Meta rather than in a new Task column because Meta already
// exists, already round-trips through storage, and already carries
// exactly this kind of per-task annotation (blocked_by, eva). A schema
// migration to hold one enum would buy nothing.
const MetaPrioritySource = "priority_source"

// MetaPriorityRule is the task-meta key naming the rule that produced a
// derived priority. Written only alongside PrioritySourceDerived, so a
// user reading `task show --format json` can see not merely that a value
// was derived but WHICH rule derived it.
const MetaPriorityRule = "priority_rule"

// Priority provenance values stored under MetaPrioritySource.
const (
	// PrioritySourceManual: a human wrote this priority. Derivation
	// never overwrites it.
	PrioritySourceManual = "manual"
	// PrioritySourceDerived: a rule produced this priority. Derivation
	// may replace it with a newer verdict.
	PrioritySourceDerived = "derived"
)

// PrioritySource reports the provenance of a task's current priority.
//
// An UNMARKED task with a priority reads as manual, not as derived. That
// asymmetry is deliberate and it is the migration story: every task that
// existed before this feature has a priority somebody typed and no
// marker at all, and reading those as derived would hand the entire
// existing backlog to the rule set on the first reprioritise run. The
// safe default is the one that refuses to touch anything.
//
// An unmarked task with NO priority reads as derived — there is nothing
// to protect, and treating "" as manual would freeze every untriaged
// task out of derivation permanently, which is the feature's whole
// purpose.
func PrioritySource(t *Task) string {
	if t == nil {
		return PrioritySourceDerived
	}
	if t.Meta != nil {
		if v, ok := t.Meta[MetaPrioritySource].(string); ok {
			switch v {
			case PrioritySourceManual, PrioritySourceDerived:
				return v
			}
		}
	}
	if t.Priority == "" {
		return PrioritySourceDerived
	}
	return PrioritySourceManual
}

// MarkPriorityManual records that a human set this task's priority, so
// derivation will leave it alone.
//
// Called from the `-p <value>` write paths. Clearing the priority calls
// ClearPrioritySource instead — that is the documented way back onto
// derivation.
func MarkPriorityManual(t *Task) {
	if t == nil {
		return
	}
	if t.Meta == nil {
		t.Meta = make(map[string]interface{})
	}
	t.Meta[MetaPrioritySource] = PrioritySourceManual
	delete(t.Meta, MetaPriorityRule)
}

// MarkPriorityDerived records that rule `rule` produced this priority.
func MarkPriorityDerived(t *Task, rule string) {
	if t == nil {
		return
	}
	if t.Meta == nil {
		t.Meta = make(map[string]interface{})
	}
	t.Meta[MetaPrioritySource] = PrioritySourceDerived
	if rule != "" {
		t.Meta[MetaPriorityRule] = rule
	} else {
		delete(t.Meta, MetaPriorityRule)
	}
}

// ClearPrioritySource drops the provenance marker entirely, handing the
// task back to derivation.
//
// This is what `task update -p -` does, and it is the answer to "can a
// user opt back in after setting a priority by hand": yes, by clearing
// it. Clearing is the right gesture because it is already how every
// other optional field is un-set (`--due -`, `--assigned-to -`), and
// because it is honest — the value is gone, and the next derivation pass
// supplies one. There is deliberately no `-p auto` alias: a second way
// to spell the same thing is a second thing to document and keep in
// sync, and "clear it and let the rules decide" is what actually happens.
func ClearPrioritySource(t *Task) {
	if t == nil || t.Meta == nil {
		return
	}
	delete(t.Meta, MetaPrioritySource)
	delete(t.Meta, MetaPriorityRule)
}

// DerivationOutcome is why one task got, or did not get, a new priority.
type DerivationOutcome int

const (
	// OutcomeUnchanged: a rule matched but named the priority the task
	// already has, or no rule matched at all.
	OutcomeUnchanged DerivationOutcome = iota
	// OutcomeDerived: a rule matched and the priority changed.
	OutcomeDerived
	// OutcomeSkippedManual: a human set this priority; derivation
	// declined to touch it. The criterion this feature exists to hold.
	OutcomeSkippedManual
	// OutcomeSkippedActive: the task is in an `active`-role status and
	// `include_active` is off — the mid-flight guard.
	OutcomeSkippedActive
	// OutcomeSkippedTerminal: the task is done or skipped. Re-ranking
	// finished work is noise in every list it appears in.
	OutcomeSkippedTerminal
)

// String renders an outcome for the reprioritise report.
func (o DerivationOutcome) String() string {
	switch o {
	case OutcomeDerived:
		return "derived"
	case OutcomeSkippedManual:
		return "skipped (manual)"
	case OutcomeSkippedActive:
		return "skipped (in progress)"
	case OutcomeSkippedTerminal:
		return "skipped (terminal)"
	default:
		return "unchanged"
	}
}

// DerivationResult is the verdict for one task.
type DerivationResult struct {
	// TaskID is the durable typeid. Sorting and identity use this.
	TaskID string
	// Display is the human-readable alias ("T-0042") for rendering. Set
	// by the caller, which owns alias formatting; empty here means "fall
	// back to TaskID". Kept separate from TaskID so the ordering the
	// report is sorted by cannot drift with display formatting.
	Display  string
	Outcome  DerivationOutcome
	From     Priority
	To       Priority
	RuleName string
}

// Label renders the ID a user should see: the display alias when the
// caller supplied one, else the durable ID.
func (r DerivationResult) Label() string {
	if r.Display != "" {
		return r.Display
	}
	return r.TaskID
}

// Changed reports whether this result requires a write.
func (r DerivationResult) Changed() bool {
	return r.Outcome == OutcomeDerived
}

// PriorityDeriver evaluates a validated rule set against tasks.
//
// Built once per command run from the user's config. Construction
// re-runs config.ValidatePriorityDerivation, so a caller cannot evaluate
// a rule set that was only warned about: an invalid set produces an
// error here and the command refuses, rather than applying the rules it
// happened to be able to parse.
type PriorityDeriver struct {
	rules         []config.PriorityDerivationRule
	includeActive bool
	onCreate      bool

	// activeStatuses names the statuses carrying role "active", for the
	// mid-flight guard. Resolved from the same config the rules came
	// from rather than hardcoded to IN_PROGRESS, since the status
	// vocabulary is itself config-driven.
	activeStatuses map[string]bool
	// terminalStatuses names the statuses declared terminal.
	terminalStatuses map[string]bool

	// now is time.Now, indirected so tests can pin the clock. Every
	// time-dependent rule reads it exactly once per Derive call, so a
	// single evaluation cannot straddle a boundary.
	now func() time.Time
}

// NewPriorityDeriver builds a deriver from a task config.
//
// Returns (nil, nil) when no rules are declared: derivation off is not
// an error, and a nil deriver is a no-op at every call site, which is
// what makes "no derivation configured behaves byte-identically to
// before" true by construction rather than by inspection.
func NewPriorityDeriver(cfg *config.TaskConfig) (*PriorityDeriver, error) {
	if cfg == nil || len(cfg.PriorityDeriv.Rules) == 0 {
		return nil, nil
	}
	if err := cfg.ValidatePriorityDerivation(); err != nil {
		return nil, err //nolint:wrapcheck // the validator's message is the whole diagnosis; wrapping it adds noise
	}

	d := &PriorityDeriver{
		rules:            append([]config.PriorityDerivationRule(nil), cfg.PriorityDeriv.Rules...),
		includeActive:    cfg.PriorityDeriv.IncludeActive,
		onCreate:         cfg.PriorityDeriv.OnCreate,
		activeStatuses:   map[string]bool{},
		terminalStatuses: map[string]bool{},
		now:              func() time.Time { return time.Now().UTC() },
	}

	statuses := cfg.Statuses
	if len(statuses) == 0 {
		statuses = config.GetDefaultStatuses()
	}
	for _, s := range statuses {
		if s.Role == config.RoleActive {
			d.activeStatuses[s.Name] = true
		}
		if s.IsTerminal {
			d.terminalStatuses[s.Name] = true
		}
	}

	return d, nil
}

// DerivesOnCreate reports whether the user opted into create-time
// derivation. Nil-safe, so the create path can ask without branching.
func (d *PriorityDeriver) DerivesOnCreate() bool {
	return d != nil && d.onCreate
}

// SetNow pins the clock. Tests only.
func (d *PriorityDeriver) SetNow(fn func() time.Time) {
	if d != nil && fn != nil {
		d.now = fn
	}
}

// DependentCounts maps a task ID to the number of OTHER tasks that
// declare it in their blocked_by.
//
// Built by the caller from the task set it is about to evaluate, and
// passed in rather than computed here, so the deriver never issues a
// query of its own — internal/core has no storage handle, and a rule
// engine that silently fans out to the database is a rule engine whose
// cost is invisible at the call site.
type DependentCounts map[string]int

// CountDependents builds the dependent counts for a task set.
//
// Counts only blockers that are IN the set. A cross-project blocker
// reference names a task this pass cannot see, and counting it would
// make the answer depend on which projects happened to be loaded — the
// same class of nondeterminism the rule ordering avoids.
func CountDependents(tasks []*Task) DependentCounts {
	present := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		if t != nil {
			present[t.ID] = true
		}
	}
	counts := make(DependentCounts, len(tasks))
	for _, t := range tasks {
		if t == nil {
			continue
		}
		for _, blocker := range t.BlockedBy() {
			if present[blocker] {
				counts[blocker]++
			}
		}
	}
	return counts
}

// Derive computes the verdict for one task WITHOUT mutating it.
//
// Split from Apply so a dry run and a real run evaluate identical code:
// a `--dry-run` that walked a second path would be free to disagree with
// the run it previews, which is the one thing a preview must not do.
//
// A nil deriver returns OutcomeUnchanged, so callers need no nil guard.
func (d *PriorityDeriver) Derive(t *Task, deps DependentCounts) DerivationResult {
	res := DerivationResult{Outcome: OutcomeUnchanged}
	if d == nil || t == nil {
		return res
	}
	res.TaskID = t.ID
	res.From = t.Priority
	res.To = t.Priority

	if d.terminalStatuses[string(t.Status)] {
		res.Outcome = OutcomeSkippedTerminal
		return res
	}
	if !d.includeActive && d.activeStatuses[string(t.Status)] {
		res.Outcome = OutcomeSkippedActive
		return res
	}
	// The criterion that matters most. Checked AFTER the status guards
	// only because those are cheaper; checked before any rule is
	// evaluated, so no rule can even see a manually-prioritised task.
	if PrioritySource(t) == PrioritySourceManual {
		res.Outcome = OutcomeSkippedManual
		return res
	}

	now := d.now()
	for _, r := range d.rules {
		if !d.matches(r, t, deps, now) {
			continue
		}
		res.RuleName = r.Name
		next := Priority(r.Then)
		if next == t.Priority {
			// A rule matched and named the value already stored. Not a
			// change, and not a write: rewriting a row to the value it
			// already holds churns UpdatedAt, which feeds staleness and
			// every "recently touched" view.
			return res
		}
		res.To = next
		res.Outcome = OutcomeDerived
		return res
	}
	return res
}

// matches reports whether every condition on r holds for t. Conditions
// are ANDed; a rule with no condition cannot reach here, since
// validation rejects it.
//
// One predicate per condition rather than one long chain of ifs, so
// adding a condition is a self-contained edit in three places that a
// reviewer can see all of — the struct field, its conditionKey entry,
// and its predicate here — rather than a line buried in a growing
// function.
func (d *PriorityDeriver) matches(
	r config.PriorityDerivationRule, t *Task, deps DependentCounts, now time.Time,
) bool {
	if r.Always {
		return true
	}
	return matchesDueWithin(r, t, now) &&
		matchesMinAge(r, t, now) &&
		matchesMinDependents(r, t, deps) &&
		matchesTag(r, t)
}

// matchesDueWithin holds when the task's due date is no further out than
// the window.
//
// An overdue task satisfies EVERY due_within window: it is not "further
// out than 24h", it is already past. Comparing the remaining duration
// handles both sides with one expression; comparing an absolute distance
// would exclude the most urgent tasks in the set, which is exactly
// backwards. A task with no due date satisfies no window.
func matchesDueWithin(r config.PriorityDerivationRule, t *Task, now time.Time) bool {
	if r.DueWithin == 0 {
		return true
	}
	if t.DueAt == nil {
		return false
	}
	return t.DueAt.Sub(now) <= r.DueWithin
}

// matchesMinAge holds when the task is at least that old. A task with no
// creation timestamp has no age to compare and never matches, rather
// than reading as infinitely old.
func matchesMinAge(r config.PriorityDerivationRule, t *Task, now time.Time) bool {
	if r.MinAge == 0 {
		return true
	}
	if t.CreatedAt.IsZero() {
		return false
	}
	return now.Sub(t.CreatedAt) >= r.MinAge
}

// matchesMinDependents holds when enough other tasks are blocked on this
// one. Counts come from the caller's set; see CountDependents.
func matchesMinDependents(r config.PriorityDerivationRule, t *Task, deps DependentCounts) bool {
	return r.MinDependents == 0 || deps[t.ID] >= r.MinDependents
}

// matchesTag holds when the task carries the named tag.
func matchesTag(r config.PriorityDerivationRule, t *Task) bool {
	if r.Tag == "" {
		return true
	}
	for _, tag := range t.Tags {
		if tag == r.Tag {
			return true
		}
	}
	return false
}

// Apply runs Derive and, when the verdict is a change, writes the new
// priority and its provenance onto the task. Returns the same result
// Derive produced, so a caller reporting the run and a caller performing
// it read the identical value.
func (d *PriorityDeriver) Apply(t *Task, deps DependentCounts) DerivationResult {
	res := d.Derive(t, deps)
	if !res.Changed() {
		return res
	}
	t.Priority = res.To
	MarkPriorityDerived(t, res.RuleName)
	return res
}

// DeriveForCreate supplies a priority for a task being created that
// carries none, when the user opted into create-time derivation.
//
// Deliberately narrower than Apply: it returns immediately for a task
// that already has a priority, whatever its provenance. At create time
// "has a priority" means "the user typed -p", and nothing here may
// second-guess that. It also passes no dependent counts — a task that
// does not exist yet blocks nothing, so a `min_dependents` rule cannot
// fire for it, and pretending otherwise would need a database round trip
// to learn a number that is definitionally zero.
func (d *PriorityDeriver) DeriveForCreate(t *Task) DerivationResult {
	if d == nil || !d.onCreate || t == nil || t.Priority != "" {
		return DerivationResult{Outcome: OutcomeUnchanged}
	}
	return d.Apply(t, nil)
}

// DerivationReport summarises a whole reprioritise pass.
type DerivationReport struct {
	Results []DerivationResult
}

// Add appends a result, keeping the report in the caller's order.
func (rep *DerivationReport) Add(res DerivationResult) {
	rep.Results = append(rep.Results, res)
}

// Sort orders results by task ID so a run's output is stable regardless
// of the order storage happened to return rows in. Determinism is a
// property of the OUTPUT as well as of the verdicts.
func (rep *DerivationReport) Sort() {
	sort.SliceStable(rep.Results, func(i, j int) bool {
		return rep.Results[i].TaskID < rep.Results[j].TaskID
	})
}

// Counts tallies the report by outcome.
func (rep *DerivationReport) Counts() map[DerivationOutcome]int {
	out := make(map[DerivationOutcome]int, 5)
	for _, r := range rep.Results {
		out[r.Outcome]++
	}
	return out
}

// Summary renders the tally in a fixed order, so two runs over the same
// data produce byte-identical text.
func (rep *DerivationReport) Summary() string {
	c := rep.Counts()
	return fmt.Sprintf(
		"%d derived, %d unchanged, %d skipped (manual), %d skipped (in progress), %d skipped (terminal)",
		c[OutcomeDerived], c[OutcomeUnchanged], c[OutcomeSkippedManual],
		c[OutcomeSkippedActive], c[OutcomeSkippedTerminal],
	)
}
