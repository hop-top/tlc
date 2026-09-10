package labels

// Generation of the label axes from effective config.
//
// The axes used to be retyped here as literals, and they drifted: `feat`
// and `fix` lost their dimension prefix, effort was never defined at
// all, and a user who renamed their statuses or priorities got labels
// naming a vocabulary they had abandoned. Retyping a vocabulary that
// already exists in config is the defect; deriving it is the fix, and
// derivation is what makes the drift unrepeatable rather than merely
// repaired.
//
// Only the axes that MIRROR a configured vocabulary are generated. The
// type axis is not one: `type:*` mirrors Conventional Commits, which is
// a spec rather than a tlc config surface, so it stays a literal in
// templates.go where it can be read against the spec.

import (
	"fmt"
	"regexp"
	"strings"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// hexRe matches the six hex digits a forge label colour must be.
var hexRe = regexp.MustCompile(`^[0-9A-Fa-f]{6}$`)

// Colour of the two labels that exist outside any configured vocabulary.
const (
	blockedLabelColor = "D93F0B"
	fallbackColor     = "EDEDED"
)

// namedColors resolves the colour NAMES config declares — `task.statuses`
// and `task.priorities` carry "red", "blue", "gray" — into the six hex
// digits a forge label needs.
//
// The two schemas genuinely differ: a config colour is a terminal
// rendering hint, shared with the TUI, where a name is the right level of
// abstraction; a forge label is an RGB swatch. Something must translate,
// and doing it here keeps `internal/config` from growing a
// forge-specific concern.
//
// A value that is already six hex digits passes through, so a user who
// wants an exact swatch can simply declare one. Anything unrecognised
// falls back to a neutral grey rather than being emitted raw: GitHub
// rejects a malformed colour outright, which would fail the whole
// `label init`, and one off-palette label is a much smaller harm than a
// command that refuses to run.
var namedColors = map[string]string{
	"black":   "24292F",
	"blue":    "1D76DB",
	"cyan":    "17A2B8",
	"gray":    "6A737D",
	"grey":    "6A737D",
	"green":   "0E8A16",
	"magenta": "B5289E",
	"orange":  "D93F0B",
	"purple":  "5319E7",
	"red":     "B60205",
	"white":   "F6F8FA",
	"yellow":  "FBCA04",
}

func resolveColor(declared string) string {
	if declared == "" {
		return fallbackColor
	}
	trimmed := strings.TrimPrefix(declared, "#")
	if hexRe.MatchString(trimmed) {
		return strings.ToUpper(trimmed)
	}
	if hex, ok := namedColors[strings.ToLower(declared)]; ok {
		return hex
	}
	return fallbackColor
}

// labelValue renders a vocabulary NAME as the value half of a label.
// Config names are shouty and underscored (IN_PROGRESS, IN_REVIEW);
// forge labels are lowercase and hyphenated (status:in-progress). This
// is the single place that convention is applied, so push, pull and
// seeding cannot spell the same status three ways.
func labelValue(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

// statusAxis generates the `status:*` labels from the effective workflow
// vocabulary.
//
// WHICH statuses get a label is decided from role and terminality, never
// from the name. github-sync's buildPushLabels labels only IN_PROGRESS
// and blocked, and that is not an arbitrary shortlist: a GitHub issue is
// open or closed, and that bit ALREADY encodes the rest of the axis. A
// terminal status is a closed issue, and the initial status is an open
// issue nobody has touched — labelling either would restate in a label
// what the issue state already says, and the two would then be free to
// disagree after an edit on the forge side.
//
// What open/closed cannot express is the difference between "open,
// untouched" and "open, someone is on it". That is exactly the set this
// selects: non-terminal, non-initial. With the built-in vocabulary it
// yields IN_PROGRESS alone, matching the plugins. With a vocabulary that
// adds IN_REVIEW it yields both, which is the behaviour a custom
// vocabulary should get and a name-matched shortlist could not give.
//
// Deriving from role also means a user who renames IN_PROGRESS to
// DOING keeps a working label, because `role: active` travelled with the
// rename.
func statusAxis(defs []config.StatusDefinition) []Label {
	out := make([]Label, 0, len(defs)+1)
	for _, s := range defs {
		if s.IsTerminal || s.Role == config.RoleInitial {
			continue
		}
		out = append(out, Label{
			Name:        "status:" + labelValue(s.Name),
			Color:       resolveColor(s.Color),
			Description: statusDescription(s),
		})
	}

	// `status:blocked` is NOT a status. tlc models blocked as
	// Task.BlockedReason, an orthogonal field a task carries WHILE it
	// sits in some status — which is why it cannot be derived from the
	// vocabulary above, and why deriving it would be wrong even if a
	// user happened to declare a status named BLOCKED.
	//
	// It is still on the status axis because that is where the wire
	// format already puts it: all four sync plugins push and pull
	// `status:blocked`, and the label has to exist with a defined colour
	// or `sync push` creates it with a forge default. Appended as a
	// fixed entry, after the generated ones, so its independence from
	// the vocabulary is visible rather than implied.
	out = append(out, Label{
		Name:        "status:blocked",
		Color:       blockedLabelColor,
		Description: "Blocked — task carries a blocked reason",
	})
	return out
}

func statusDescription(s config.StatusDefinition) string {
	if s.Description != "" {
		return s.Description
	}
	if s.Label != "" {
		return fmt.Sprintf("%s — %s", s.Name, s.Label)
	}
	return s.Name
}

// priorityAxis generates the `priority:*` labels from the configured
// priority vocabulary.
//
// Declaration order IS rank order, most urgent first, so the axis is
// emitted in that order and never sorted — sorting would destroy the
// only expression of rank the schema has.
//
// The built-in vocabulary is the special case, not the rule. Its names
// are P0..P3, but the labels every sync plugin's priorityToLabel emits
// are priority:critical..priority:low — a rank-indexed alias, chosen
// long ago so the forge label reads as urgency rather than as a tlc
// internal code. Generation preserves that alias for the built-ins
// (otherwise `label init` would seed priority:p0 that no push ever
// produces) and falls back to the declared name for anything else, which
// is what makes a URGENT/NORMAL/LATER vocabulary yield
// priority:urgent/normal/later.
var builtinPriorityLabelValue = map[string]string{
	"P0": "critical",
	"P1": "high",
	"P2": "medium",
	"P3": "low",
}

func priorityAxis(defs []config.PriorityDefinition) []Label {
	// The alias applies only when the vocabulary IS the built-in one. A
	// custom vocabulary that happens to reuse the name P0 means its own
	// P0, and silently renaming it to `critical` would be the same class
	// of guess this whole change exists to remove.
	builtin := isBuiltinPriorityVocabulary(defs)

	out := make([]Label, 0, len(defs))
	for _, p := range defs {
		value := labelValue(p.Name)
		if builtin {
			if alias, ok := builtinPriorityLabelValue[p.Name]; ok {
				value = alias
			}
		}
		out = append(out, Label{
			Name:        "priority:" + value,
			Color:       resolveColor(p.Color),
			Description: priorityDescription(p),
		})
	}
	return out
}

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

func priorityDescription(p config.PriorityDefinition) string {
	if p.Description != "" {
		return p.Description
	}
	if p.Label != "" {
		return fmt.Sprintf("%s — %s priority", p.Name, p.Label)
	}
	return fmt.Sprintf("%s priority", p.Name)
}

// effortLabelMeta carries the swatch and prose for each built-in effort.
// Effort is a closed Go enum rather than a config surface — core.Efforts
// has no configured counterpart — so the presentation lives here. The
// SET still comes from core.Efforts(), so adding a value there adds a
// label without a second edit.
var effortLabelMeta = map[core.Effort]struct{ color, desc string }{
	core.EffortXS: {"C2E0C6", "XS — extra small"},
	core.EffortS:  {"9EDAB0", "S — small"},
	core.EffortM:  {"7BC99B", "M — medium"},
	core.EffortL:  {"4FA97F", "L — large"},
	core.EffortXL: {"2E8B62", "XL — extra large"},
}

func effortAxis(efforts []core.Effort) []Label {
	out := make([]Label, 0, len(efforts))
	for _, e := range efforts {
		meta, ok := effortLabelMeta[e]
		if !ok {
			meta.color = fallbackColor
			meta.desc = string(e) + " effort"
		}
		out = append(out, Label{
			Name:        "effort:" + labelValue(string(e)),
			Color:       meta.color,
			Description: meta.desc,
		})
	}
	return out
}

// generatedAxes returns every config-derived axis, in the order
// `label init` prints them.
//
// It reads the NON-CACHING accessors deliberately. core.DefaultWorkflow*
// memoises its answer in a sync.Once, so any call from a path that runs
// before argv is parsed — help rendering, flag usage, shell completion —
// would pin the process to whatever config existed at that instant and
// silently discard every later `-c key=value` override, for every
// command in the process rather than just this one. `label templates`
// enumerates the built-in sets and is close enough to that class of path
// that reaching for the singleton here would be a live hazard, not a
// hypothetical one.
func generatedAxes() []Label {
	out := make([]Label, 0, 16)
	out = append(out, priorityAxis(core.ConfiguredPriorityDefinitions())...)
	out = append(out, effortAxis(core.Efforts())...)
	out = append(out, statusAxis(core.ConfiguredTaskStatusDefinitions())...)
	return out
}
