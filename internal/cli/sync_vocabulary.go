package cli

import (
	"strings"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// syncVocabulary is the effective task vocabulary rendered as forge
// labels, sent to a sync plugin alongside every pull and push.
//
// WHY the host sends this at all: the vocabulary is a runtime value,
// read from the user's config. A plugin cannot know it. Before this
// existed the four mappers each hardcoded the built-in vocabulary, so a
// repo declaring `task.priorities: [URGENT, NORMAL, LATER]` had
// `label init` seed priority:urgent while pull could not map it back —
// the typed Priority field was lost, the value degraded to a tag, and
// the next push omitted the label, churning every cycle.
//
// WHY labels rather than raw definitions: the name-to-label rule carries
// the built-in P0..P3 -> critical..low alias, which exists so generated
// labels match what plugins have always pushed. Sending definitions
// would force every plugin — including three in separate modules that
// cannot import this one — to reimplement that rule, which is the
// duplication being removed.
type syncVocabulary struct {
	PriorityToLabel  map[string]string `json:"priority_to_label,omitempty"`
	EffortToLabel    map[string]string `json:"effort_to_label,omitempty"`
	ActiveStatuses   map[string]string `json:"active_statuses,omitempty"`
	InitialStatus    string            `json:"initial_status,omitempty"`
	TerminalStatuses map[string]string `json:"terminal_statuses,omitempty"`
}

// builtinPriorityLabelValue is the rank-indexed alias the built-in
// priority names carry on the forge. Kept in step with the label axis:
// generation preserves the alias for the built-ins and falls back to the
// declared name for anything else, which is what makes a
// URGENT/NORMAL/LATER vocabulary yield priority:urgent/normal/later.
var builtinPriorityLabelValue = map[string]string{
	"P0": "critical",
	"P1": "high",
	"P2": "medium",
	"P3": "low",
}

// labelValue renders a vocabulary NAME as the value half of a label.
// Config names are shouty and underscored (IN_PROGRESS); forge labels
// are lowercase and hyphenated (status:in-progress).
func labelValue(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

// isBuiltinPriorityVocabulary reports whether the declared priorities
// ARE the built-in set. The alias applies only then: a custom
// vocabulary that happens to reuse the name P0 means its own P0, and
// silently renaming it to `critical` would be the guess this change
// exists to remove.
func isBuiltinPriorityVocabulary(defs []config.PriorityDefinition) bool {
	builtins := config.GetDefaultPriorities()
	if len(defs) != len(builtins) {
		return false
	}
	for i, p := range defs {
		if p.Name != builtins[i].Name {
			return false
		}
	}
	return true
}

// buildSyncVocabulary resolves the effective vocabulary for the wire.
func buildSyncVocabulary() *syncVocabulary {
	priorities := core.ConfiguredPriorityDefinitions()
	builtin := isBuiltinPriorityVocabulary(priorities)

	v := &syncVocabulary{
		PriorityToLabel:  make(map[string]string, len(priorities)),
		EffortToLabel:    map[string]string{},
		ActiveStatuses:   map[string]string{},
		TerminalStatuses: map[string]string{},
	}

	for _, p := range priorities {
		value := labelValue(p.Name)
		if builtin {
			if alias, ok := builtinPriorityLabelValue[p.Name]; ok {
				value = alias
			}
		}
		v.PriorityToLabel[p.Name] = "priority:" + value
	}

	for _, e := range core.ConfiguredEffortDefinitions() {
		v.EffortToLabel[e.Name] = "effort:" + labelValue(e.Name)
	}

	// Statuses split three ways. Only the non-terminal, non-initial ones
	// earn a label: open/closed already encodes terminality, and an
	// untouched open issue already encodes the initial status. Labeling
	// either would restate the issue state in a label and let the two
	// disagree after an edit on the forge side.
	for _, s := range core.ConfiguredTaskStatusDefinitions() {
		switch {
		case s.IsTerminal:
			// SKIPPED-like statuses close as not_planned; everything
			// else that ends the task closes as completed.
			reason := "completed"
			if s.Role == config.RoleSkipped {
				reason = "not_planned"
			}
			v.TerminalStatuses[s.Name] = reason
		case s.Role == config.RoleInitial:
			v.InitialStatus = s.Name
		default:
			v.ActiveStatuses[s.Name] = "status:" + labelValue(s.Name)
		}
	}

	return v
}
