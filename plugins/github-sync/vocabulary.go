package main

import "strings"

// Vocabulary is the effective task vocabulary the HOST resolved from
// config and sent over the wire, already rendered as forge labels.
//
// The host sends labels rather than raw config on purpose. Turning a
// config name into a label is not a formatting detail — it carries the
// built-in P0..P3 -> priority:critical..low alias, which exists only so
// generated labels match what plugins have always pushed. Re-deriving
// that rule here would put a second copy of it in every plugin module,
// which is the duplication this change removes. The plugin does no
// derivation at all: it looks values up.
//
// A nil or empty Vocabulary means "the host told us nothing", and every
// consumer falls back to the built-in tables below. That fallback is
// what makes an old host talking to a new plugin, and a new host talking
// to an old plugin, both behave exactly as they did before.
type Vocabulary struct {
	// PriorityToLabel maps a priority NAME (P0, URGENT) to its forge
	// label (priority:critical, priority:urgent).
	PriorityToLabel map[string]string `json:"priority_to_label,omitempty"`

	// EffortToLabel maps an effort NAME (XS, TINY) to its forge label.
	EffortToLabel map[string]string `json:"effort_to_label,omitempty"`

	// ActiveStatuses maps a status NAME to its forge label, for the
	// statuses that earn one: non-terminal and non-initial. Open/closed
	// already encodes the rest of the axis, so labeling a terminal or
	// initial status would restate the issue state in a label and let
	// the two disagree.
	//
	// status:blocked is deliberately absent: blocked is not a status in
	// tlc, it is an orthogonal BlockedReason a task carries WHILE in
	// some status, so it cannot be derived from this vocabulary.
	ActiveStatuses map[string]string `json:"active_statuses,omitempty"`

	// InitialStatus is the NAME of the status a freshly pulled, unlabeled
	// open issue lands in. Empty means the built-in TODO.
	InitialStatus string `json:"initial_status,omitempty"`

	// TerminalStatuses maps a terminal status NAME to the reason a closed
	// issue carries for it. Empty means the built-in DONE/SKIPPED pair.
	TerminalStatuses map[string]string `json:"terminal_statuses,omitempty"`
}

// activeStatusFor resolves a forge label to the active status NAME it
// stands for.
func (v *Vocabulary) activeStatusFor(label string) (string, bool) {
	if v != nil && len(v.ActiveStatuses) > 0 {
		return invertLookup(v.ActiveStatuses, label)
	}
	if strings.EqualFold(label, builtinActiveLabel) {
		return builtinActiveStatus, true
	}
	return "", false
}

// Fixed label and vocabulary names used when the host sends nothing.
const (
	// blockedLabel is the label for the blocked flag. Not part of the
	// vocabulary for the reason given on ActiveStatuses.
	blockedLabel = "status:blocked"

	// builtinActiveStatus and builtinActiveLabel are the single active
	// status of the built-in vocabulary and its label.
	builtinActiveStatus = "IN_PROGRESS"
	builtinActiveLabel  = "status:in-progress"

	// builtinInitialStatus is the built-in vocabulary's initial status.
	builtinInitialStatus = "TODO"
)

// priorityLabel returns the label for a priority name, and whether one
// exists. Falls back to the built-in table when the host sent nothing.
func (v *Vocabulary) priorityLabel(name string) (string, bool) {
	if v != nil && len(v.PriorityToLabel) > 0 {
		l, ok := v.PriorityToLabel[name]
		return l, ok
	}
	l, ok := priorityToLabel[name]
	return l, ok
}

// effortLabel is the effort-axis counterpart of priorityLabel.
func (v *Vocabulary) effortLabel(name string) (string, bool) {
	if v != nil && len(v.EffortToLabel) > 0 {
		l, ok := v.EffortToLabel[name]
		return l, ok
	}
	l, ok := effortToLabel[name]
	return l, ok
}

// statusLabel returns the label for an active status name. With no
// vocabulary only IN_PROGRESS earns one, which is what the plugin did
// before this change.
func (v *Vocabulary) statusLabel(name string) (string, bool) {
	if v != nil && len(v.ActiveStatuses) > 0 {
		l, ok := v.ActiveStatuses[name]
		return l, ok
	}
	if name == builtinActiveStatus {
		return builtinActiveLabel, true
	}
	return "", false
}

// priorityFor resolves a forge label back to a priority NAME. The
// reverse of priorityLabel, built by inverting the same map so the two
// directions cannot drift apart.
func (v *Vocabulary) priorityFor(label string) (string, bool) {
	if v != nil && len(v.PriorityToLabel) > 0 {
		return invertLookup(v.PriorityToLabel, label)
	}
	p, ok := labelToPriority[strings.ToLower(label)]
	return p, ok
}

// effortFor is the effort-axis counterpart of priorityFor.
func (v *Vocabulary) effortFor(label string) (string, bool) {
	if v != nil && len(v.EffortToLabel) > 0 {
		return invertLookup(v.EffortToLabel, label)
	}
	e, ok := labelToEffort[strings.ToLower(label)]
	return e, ok
}

// invertLookup finds the key whose value equals label, case-insensitively
// on the label side because forges do not agree on label case.
func invertLookup(m map[string]string, label string) (string, bool) {
	for name, l := range m {
		if strings.EqualFold(l, label) {
			return name, true
		}
	}
	return "", false
}
