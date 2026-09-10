package labels

import (
	"strings"
	"testing"
)

// allProjectTypes is every type `label init --type` accepts plus the
// detector's fallbacks, so a case that only holds for go-binary cannot
// pass by accident.
var allProjectTypes = []ProjectType{
	TypeGoBinary,
	TypeGoSocket,
	TypePythonMVC,
	TypeReactFrontend,
	TypeMicroservices,
	TypeGeneric,
}

// TestNoBareLabels is the drift regression, stated as the property the
// sync plugins actually enforce rather than as a list of names.
//
// A label without a colon is not a weaker label — it is discarded.
// github-sync's mapLabelsToTask drops it outright and the TLS parser's
// isMetaToken parses it into the task TITLE. Seeding one means `label
// init` creates a label in the user's forge that no pull can ever read
// back, which is exactly what `feat` and `fix` did.
func TestNoBareLabels(t *testing.T) {
	for _, pt := range allProjectTypes {
		for _, l := range GetTemplates(pt) {
			if !strings.Contains(l.Name, ":") {
				t.Errorf("%s: label %q has no dimension prefix; sync discards it", pt, l.Name)
			}
		}
	}
}

// TestTypeAxisCoversConventionalCommits pins the type axis to the commit
// vocabulary the repo itself enforces. A type the house rules allow in a
// commit subject but the label set cannot express is a gap a triager
// hits the first time they try to label the work.
func TestTypeAxisCoversConventionalCommits(t *testing.T) {
	// The allowed types from the house Conventional Commits rule, plus
	// breaking — which is a marker rather than a type, and was
	// unrepresentable before.
	want := []string{
		"type:feat", "type:fix", "type:refactor", "type:docs",
		"type:test", "type:chore", "type:perf", "type:build",
		"type:ci", "type:style", "type:breaking",
	}
	got := namesOf(GetTemplates(TypeGeneric))
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing %q from the type axis", w)
		}
	}
	// The pre-fix names must be gone, not merely joined by prefixed
	// twins: leaving them would keep seeding the labels sync discards.
	for _, gone := range []string{"feat", "fix"} {
		if got[gone] {
			t.Errorf("bare %q still seeded; it must be renamed, not duplicated", gone)
		}
	}
}

// TestEffortAxisPresent covers the axis all four sync plugins already
// both emit and consume while no template defined it — so `sync push`
// created effort:* labels with whatever colour the forge picked.
func TestEffortAxisPresent(t *testing.T) {
	got := namesOf(GetTemplates(TypeGeneric))
	for _, w := range []string{"effort:xs", "effort:s", "effort:m", "effort:l", "effort:xl"} {
		if !got[w] {
			t.Errorf("missing %q; the plugins map this value but nothing defines the label", w)
		}
	}
}

// TestEffortAxisColoursUnchanged pins the built-in effort swatches and
// prose across the move from a hardcoded presentation map to
// config-derived definitions.
//
// Colour is not cosmetic here: `sync push` writes it to the forge, so a
// swatch that moved re-colours every issue carrying that label. The
// descriptions are pinned for the same reason.
func TestEffortAxisColoursUnchanged(t *testing.T) {
	want := map[string]struct{ color, desc string }{
		"effort:xs": {"C2E0C6", "XS — extra small"},
		"effort:s":  {"9EDAB0", "S — small"},
		"effort:m":  {"7BC99B", "M — medium"},
		"effort:l":  {"4FA97F", "L — large"},
		"effort:xl": {"2E8B62", "XL — extra large"},
	}
	for _, l := range GetTemplates(TypeGeneric) {
		w, ok := want[l.Name]
		if !ok {
			continue
		}
		if l.Color != w.color {
			t.Errorf("%s colour = %q, want %q", l.Name, l.Color, w.color)
		}
		if l.Description != w.desc {
			t.Errorf("%s description = %q, want %q", l.Name, l.Description, w.desc)
		}
		delete(want, l.Name)
	}
	for name := range want {
		t.Errorf("missing effort label %q", name)
	}
}

// TestPriorityAxisMatchesPluginNames pins the priority label names to the
// spelling every plugin's priorityToLabel produces. These already agreed;
// the test exists so a future rename cannot silently break the agreement.
func TestPriorityAxisMatchesPluginNames(t *testing.T) {
	got := namesOf(GetTemplates(TypeGeneric))
	for _, w := range []string{"priority:critical", "priority:high", "priority:medium", "priority:low"} {
		if !got[w] {
			t.Errorf("missing %q, which the plugins emit on push", w)
		}
	}
}

// TestLabelColoursAreHex guards the seam between config and the forge.
// Config declares colours by NAME ("red", "blue"); a GitHub label needs
// six hex digits. Generation reads the former and must emit the latter,
// so a name leaking through unresolved is a real failure mode.
func TestLabelColoursAreHex(t *testing.T) {
	for _, pt := range allProjectTypes {
		for _, l := range GetTemplates(pt) {
			if !hexRe.MatchString(l.Color) {
				t.Errorf("%s: label %q colour %q is not a 6-digit hex", pt, l.Name, l.Color)
			}
		}
	}
}

// TestNoDuplicateLabelNames: a duplicate would make `label init` emit the
// same name twice and a forge reject or silently collapse the second.
func TestNoDuplicateLabelNames(t *testing.T) {
	for _, pt := range allProjectTypes {
		seen := map[string]bool{}
		for _, l := range GetTemplates(pt) {
			if seen[l.Name] {
				t.Errorf("%s: duplicate label %q", pt, l.Name)
			}
			seen[l.Name] = true
		}
	}
}

// TestDomainSetsUnchanged pins the pre-existing per-type domain sets, so
// reworking the shared axes cannot quietly drop them.
func TestDomainSetsUnchanged(t *testing.T) {
	cases := map[ProjectType][]string{
		TypeGoBinary:      {"domain:cli", "domain:core", "domain:config", "domain:io"},
		TypeReactFrontend: {"domain:frontend", "domain:components", "domain:hooks"},
		TypeGeneric:       {"domain:core", "domain:api"},
	}
	for pt, want := range cases {
		got := namesOf(GetTemplates(pt))
		for _, w := range want {
			if !got[w] {
				t.Errorf("%s: missing %q", pt, w)
			}
		}
	}
}

func namesOf(ls []Label) map[string]bool {
	m := make(map[string]bool, len(ls))
	for _, l := range ls {
		m[l.Name] = true
	}
	return m
}
