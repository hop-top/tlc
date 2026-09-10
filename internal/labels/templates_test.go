package labels

import (
	"slices"
	"strings"
	"testing"
)

// allProjectTypes is every type `label init --type` documents plus an
// unrecognised one, so a case that only holds for go-binary cannot pass
// by accident and the free-string fallback stays covered.
var allProjectTypes = append(AllProjectTypes(), ProjectType("not-a-real-type"))

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

// TestDomainSetsUnchanged pins the per-type domain labels that must
// survive any later rework of the shared axes.
//
// `domain:hooks` is gone from the react-frontend row on purpose rather
// than by omission: it was replaced by `domain:state` because it named
// one framework's spelling of a concern every UI framework has, so
// pinning it here would pin the thing the replacement removed.
// TestDomainSetsPerType below is the exact-set assertion that proves the
// removal, and TestReplacedDomainsAreGone proves it is not merely
// unpinned.
func TestDomainSetsUnchanged(t *testing.T) {
	cases := map[ProjectType][]string{
		TypeGoBinary:      {"domain:cli", "domain:core", "domain:config", "domain:io"},
		TypeReactFrontend: {"domain:frontend", "domain:components"},
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

// TestDomainSetsPerType pins the domain set of every advertised type,
// exactly — not as a subset. TestDomainSetsUnchanged above only asserts
// the wanted names are PRESENT, which a type falling through to
// `default` can satisfy by accident: generic's set is
// {domain:core, domain:api}, so a "want domain:api" case passes for a
// type that produced nothing of its own. Comparing the whole set is what
// makes the phantom visible.
func TestDomainSetsPerType(t *testing.T) {
	want := map[ProjectType][]string{
		TypeGoBinary:      {"domain:cli", "domain:core", "domain:config", "domain:io", "domain:storage"},
		TypeNodeBackend:   {"domain:api", "domain:db", "domain:auth", "domain:jobs"},
		TypeReactFrontend: {"domain:frontend", "domain:components", "domain:state", "domain:api"},
		TypePythonMVC:     {"domain:models", "domain:views", "domain:api", "domain:migrations"},
		TypeLibrary:       {"domain:api", "domain:internal", "domain:docs"},
		TypeMonorepo:      {"domain:tooling", "domain:release", "domain:deps"},
		TypeInfra:         {"domain:terraform", "domain:k8s", "domain:network", "domain:secrets"},
		TypeGeneric:       {"domain:core", "domain:api", "domain:docs", "domain:ci"},
	}
	// Every advertised type must appear above. Without this, adding a
	// type to AllProjectTypes and forgetting the row here would leave
	// the new type's exact set unpinned — and an exact-set test that
	// silently skips the type it was added for is worse than no test.
	for _, pt := range AllProjectTypes() {
		if _, ok := want[pt]; !ok {
			t.Errorf("%s is advertised but its exact domain set is not pinned here", pt)
		}
	}
	for pt, w := range want {
		got := domainsOf(GetTemplates(pt))
		if !slices.Equal(got, w) {
			t.Errorf("%s domains = %v, want %v", pt, got, w)
		}
	}
}

// TestReplacedDomainsAreGone proves a replaced label was removed rather
// than merely joined by its replacement.
//
// Dropping `domain:hooks` from the pinned sets makes the exact-set test
// pass either way is not true — slices.Equal would catch a leftover — but
// only for as long as react-frontend keeps an exact-set row. This states
// the removal as its own property so it survives independently, and
// names the reason: `label init` seeds what it lists, so a leftover
// label is one a user's forge keeps carrying after the vocabulary moved.
func TestReplacedDomainsAreGone(t *testing.T) {
	for _, pt := range allProjectTypes {
		got := namesOf(GetTemplates(pt))
		if got["domain:hooks"] {
			t.Errorf("%s still seeds domain:hooks; it was replaced by domain:state", pt)
		}
	}
}

// TestNeedsAxisPresentOnEveryType pins the `needs:*` axis to every type,
// new and old.
//
// It is asserted across all types rather than on generic alone because
// being project-shape-independent is the REASON it sits on `common`
// instead of in a switch case: "nobody has triaged this" is a fact about
// a task, not about a repo. Adding it to one type's domains would be the
// same mistake in a new place.
func TestNeedsAxisPresentOnEveryType(t *testing.T) {
	for _, pt := range allProjectTypes {
		got := namesOf(GetTemplates(pt))
		for _, w := range []string{"needs:triage", "needs:repro", "needs:decision"} {
			if !got[w] {
				t.Errorf("%s: missing %q", pt, w)
			}
		}
	}
}

// TestNeedsIsNotAStatus guards the boundary that justifies the axis
// existing at all.
//
// `needs:*` was added because `status:blocked` means blocked-by-TASK —
// it is generated from Task.BlockedReason, which tlc fills from task
// dependencies — while `needs:decision` means blocked-on-HUMAN. If a
// `needs:` value were ever also spelled as a status, the two would be
// two labels for one state and a triager would have no rule for picking
// between them.
func TestNeedsIsNotAStatus(t *testing.T) {
	for _, pt := range allProjectTypes {
		for _, l := range GetTemplates(pt) {
			if !strings.HasPrefix(l.Name, "needs:") {
				continue
			}
			twin := "status:" + strings.TrimPrefix(l.Name, "needs:")
			if namesOf(GetTemplates(pt))[twin] {
				t.Errorf("%s: %q duplicates %q; needs:* is blocked-on-human, status:* is blocked-by-task",
					pt, l.Name, twin)
			}
		}
	}
}

// TestUnknownTypeMatchesGeneric pins the free-string fallback to
// generic's set EXACTLY.
//
// `--type` is a free string, so a typo arrives here as a ProjectType and
// gets the default arm. That arm used to be a second copy of generic's
// literal, and this test exists because adding to generic while
// forgetting the copy would give a typo a smaller set than the generic
// it is documented to equal — a difference nothing else here would
// notice, since every other assertion about the fallback is a
// subset check.
func TestUnknownTypeMatchesGeneric(t *testing.T) {
	generic := domainsOf(GetTemplates(TypeGeneric))
	got := domainsOf(GetTemplates(ProjectType("not-a-real-type")))
	if !slices.Equal(got, generic) {
		t.Errorf("unknown type domains = %v, want generic's %v", got, generic)
	}
}

// TestEveryAdvertisedTypeIsDistinct is the defect, stated as the rule
// that was broken rather than as a list of names.
//
// `label init --type X` prints "Detected project type: X" and then the
// labels, so a type that falls through to `default` reports the user's
// choice back to them while ignoring it. There is no output that
// distinguishes "python-mvc has these domains" from "python-mvc was not
// recognised" — which is why an advertised type producing generic's set
// is worse than no such type at all.
//
// Generic is excluded because generic IS the default set.
func TestEveryAdvertisedTypeIsDistinct(t *testing.T) {
	generic := domainsOf(GetTemplates(TypeGeneric))
	for _, pt := range AllProjectTypes() {
		if pt == TypeGeneric {
			continue
		}
		if got := domainsOf(GetTemplates(pt)); slices.Equal(got, generic) {
			t.Errorf("--type %s produces generic's domains %v; it is advertised but falls through to default", pt, got)
		}
	}
}

// TestAllProjectTypesHaveOwnCase closes the gap in the other direction:
// AllProjectTypes drives the help text and `label templates`, so a type
// added there without a switch case would re-create the phantom. Every
// listed type must contribute at least one domain no other listed type
// has, which no `default` fallthrough can satisfy.
//
// Generic is exempt, and not as a convenience: its set is deliberately
// the common denominator — `domain:core` is shared with go-binary and
// `domain:api` with python-mvc — because generic names the shape we
// know nothing else about. A unique label there would be a claim about
// an unknown project.
func TestAllProjectTypesHaveOwnCase(t *testing.T) {
	counts := map[string]int{}
	for _, pt := range AllProjectTypes() {
		for _, d := range domainsOf(GetTemplates(pt)) {
			counts[d]++
		}
	}
	for _, pt := range AllProjectTypes() {
		if pt == TypeGeneric {
			continue
		}
		unique := false
		for _, d := range domainsOf(GetTemplates(pt)) {
			if counts[d] == 1 {
				unique = true
				break
			}
		}
		if !unique {
			t.Errorf("%s has no domain of its own; it shares every label with another type", pt)
		}
	}
}

// TestSharedAxesSurviveOnEveryType guards the axis generation that
// already landed: the domain work must not disturb type/priority/effort/
// status, which are generated rather than switched on project type.
func TestSharedAxesSurviveOnEveryType(t *testing.T) {
	for _, pt := range allProjectTypes {
		got := namesOf(GetTemplates(pt))
		for _, w := range []string{
			"type:feat", "type:breaking", "priority:critical",
			"effort:xs", "effort:xl", "status:in-progress",
		} {
			if !got[w] {
				t.Errorf("%s: missing generated axis label %q", pt, w)
			}
		}
	}
}

// domainsOf returns just the `domain:*` labels, in template order.
func domainsOf(ls []Label) []string {
	var out []string
	for _, l := range ls {
		if strings.HasPrefix(l.Name, "domain:") {
			out = append(out, l.Name)
		}
	}
	return out
}

func namesOf(ls []Label) map[string]bool {
	m := make(map[string]bool, len(ls))
	for _, l := range ls {
		m[l.Name] = true
	}
	return m
}
