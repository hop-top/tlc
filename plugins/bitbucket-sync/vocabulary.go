package main

import (
	"sort"
	"strings"
)

// Vocabulary is the effective task vocabulary the HOST resolved from
// config and sent over the wire, already rendered as forge labels.
//
// The host sends labels rather than raw config on purpose. Turning a
// config name into a label carries the built-in P0..P3 ->
// priority:critical..low alias, which exists so generated labels match
// what plugins have always pushed. Re-deriving that rule here would put
// a second copy of it in every plugin module, which is the duplication
// this change removes. The plugin does no derivation: it looks values up.
//
// This plugin is a separate Go module and cannot import the host's
// internal packages, so the contract between them is this JSON shape
// rather than a shared type. A nil or empty Vocabulary means the host
// told us nothing, and every consumer falls back to the built-in tables
// — which is what keeps an old host talking to a new plugin, and a new
// host talking to an old plugin, behaving exactly as they did before.
type Vocabulary struct {
	PriorityToLabel  map[string]string `json:"priority_to_label,omitempty"`
	EffortToLabel    map[string]string `json:"effort_to_label,omitempty"`
	ActiveStatuses   map[string]string `json:"active_statuses,omitempty"`
	InitialStatus    string            `json:"initial_status,omitempty"`
	TerminalStatuses map[string]string `json:"terminal_statuses,omitempty"`
}

// The built-in fallback tables. Bitbucket historically built these
// labels by lowercasing the value name — priority:p0, effort:xs — rather
// than through the priority:critical..low alias the other forges use.
// That disagreed with what `label init` seeds, but it is what this
// plugin has always emitted, so the fallback reproduces it exactly and
// only a host-sent vocabulary changes it. Pull still accepts both
// spellings, as it always did.
var builtinPriorityToLabel = map[string]string{
	"P0": "priority:p0",
	"P1": "priority:p1",
	"P2": "priority:p2",
	"P3": "priority:p3",
}

var builtinEffortToLabel = map[string]string{
	"XS": "effort:xs",
	"S":  "effort:s",
	"M":  "effort:m",
	"L":  "effort:l",
	"XL": "effort:xl",
}

var builtinLabelToPriority = map[string]string{
	"priority:p0":       "P0",
	"priority:critical": "P0",
	"priority:p1":       "P1",
	"priority:high":     "P1",
	"priority:p2":       "P2",
	"priority:medium":   "P2",
	"priority:p3":       "P3",
	"priority:low":      "P3",
}

var builtinLabelToEffort = map[string]string{
	"effort:xs": "XS",
	"effort:s":  "S",
	"effort:m":  "M",
	"effort:l":  "L",
	"effort:xl": "XL",
}

// blockedLabel is the fixed label for the blocked flag. Blocked is not a
// status in tlc: it is an orthogonal reason a task carries WHILE in some
// status, so it is not derived from the vocabulary.
const blockedLabel = "status:blocked"

// priorityLabel returns the label for a priority name.
func (v *Vocabulary) priorityLabel(name string) (string, bool) {
	if v != nil && len(v.PriorityToLabel) > 0 {
		l, ok := v.PriorityToLabel[name]
		return l, ok
	}
	l, ok := builtinPriorityToLabel[name]
	return l, ok
}

// effortLabel is the effort-axis counterpart of priorityLabel.
func (v *Vocabulary) effortLabel(name string) (string, bool) {
	if v != nil && len(v.EffortToLabel) > 0 {
		l, ok := v.EffortToLabel[name]
		return l, ok
	}
	l, ok := builtinEffortToLabel[name]
	return l, ok
}

// statusLabel returns the label for an active status name.
func (v *Vocabulary) statusLabel(name string) (string, bool) {
	if v != nil && len(v.ActiveStatuses) > 0 {
		l, ok := v.ActiveStatuses[name]
		return l, ok
	}
	if name == "IN_PROGRESS" {
		return "status:in-progress", true
	}
	return "", false
}

// priorityFor resolves a forge label back to a priority NAME.
func (v *Vocabulary) priorityFor(label string) (string, bool) {
	if v != nil && len(v.PriorityToLabel) > 0 {
		return invertLookup(v.PriorityToLabel, label)
	}
	p, ok := builtinLabelToPriority[strings.ToLower(label)]
	return p, ok
}

// effortFor is the effort-axis counterpart of priorityFor.
func (v *Vocabulary) effortFor(label string) (string, bool) {
	if v != nil && len(v.EffortToLabel) > 0 {
		return invertLookup(v.EffortToLabel, label)
	}
	e, ok := builtinLabelToEffort[strings.ToLower(label)]
	return e, ok
}

// activeStatusFor resolves a forge label to the active status NAME it
// stands for.
//
// The NAME matters, not a boolean: an open issue labeled status:doing
// under a DOING vocabulary must pull back as DOING. Collapsing that to a
// flag and hardcoding IN_PROGRESS would round-trip the label correctly
// and still write the wrong status into the store.
func (v *Vocabulary) activeStatusFor(label string) (string, bool) {
	if v != nil && len(v.ActiveStatuses) > 0 {
		return invertLookup(v.ActiveStatuses, label)
	}
	if strings.EqualFold(label, "status:in-progress") {
		return "IN_PROGRESS", true
	}
	return "", false
}

// initialStatus is the status an untouched open issue lands in.
func (v *Vocabulary) initialStatus() string {
	if v != nil && v.InitialStatus != "" {
		return v.InitialStatus
	}
	return "TODO"
}

// isTerminal reports whether a status ends the task, and the reason the
// forge should record for it.
func (v *Vocabulary) isTerminal(name string) (string, bool) {
	if v != nil && len(v.TerminalStatuses) > 0 {
		reason, ok := v.TerminalStatuses[name]
		return reason, ok
	}
	switch name {
	case "DONE":
		return "completed", true
	case "SKIPPED":
		return "not_planned", true
	}
	return "", false
}

// terminalFor returns the vocabulary's status name for a close reason,
// so a closed issue pulls back into a declared terminal status rather
// than the built-in DONE/SKIPPED pair.
func (v *Vocabulary) terminalFor(reason string) string {
	if v != nil && len(v.TerminalStatuses) > 0 {
		// Sorted so two terminal statuses sharing a reason resolve the
		// same way on every call: map iteration order would otherwise
		// make the pulled status vary run to run.
		names := make([]string, 0, len(v.TerminalStatuses))
		for name := range v.TerminalStatuses {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if v.TerminalStatuses[name] == reason {
				return name
			}
		}
	}
	if reason == "not_planned" {
		return "SKIPPED"
	}
	return "DONE"
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
