package cli

// End-to-end coverage for the config-generated label axes.
//
// Driven through a real config FILE and the real binary, for the reason
// spelled out in status_vocabulary_e2e_test.go: the axes are generated
// from EFFECTIVE config, and effective config only exists once the
// binary has layered file, env and flags. A test calling
// labels.GetTemplates in-process would exercise generation against
// whatever the ambient provider happened to hold and could not see a
// vocabulary failing to reach the templates at all.
//
// Every config below pins storage.db_path. An unpinned probe resolves
// the DB through the global project registry rather than the cwd, and
// would read — and can mutate — an unrelated real database.

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// customLabelStatusConfig renames the whole status vocabulary and adds a
// second non-terminal, non-initial status. None of the built-in names
// survive, so any TODO/IN_PROGRESS/DONE/SKIPPED in the output is a
// hardcoded list leaking rather than a coincidence.
//
// Colours are declared per status, and deliberately mixed: a bare name
// (purple), a hex with a leading # (#FF00AA), and names on the statuses
// that must NOT produce a label at all.
const customLabelStatusConfig = `storage:
  db_path: %s
task:
  statuses:
    - name: BACKLOG
      label: Backlog
      role: initial
      color: gray
      tls_marker: " "
    - name: DOING
      label: Doing
      role: active
      color: "#FF00AA"
      tls_marker: ">"
    - name: IN_REVIEW
      label: In Review
      color: purple
      tls_marker: "?"
    - name: SHIPPED
      label: Shipped
      is_terminal: true
      role: completed
      color: green
      tls_marker: "x"
    - name: DROPPED
      label: Dropped
      is_terminal: true
      role: skipped
      color: black
      tls_marker: "-"
  state_machine:
    rules:
      BACKLOG: [DOING]
      DOING: [IN_REVIEW, SHIPPED, DROPPED]
      IN_REVIEW: [SHIPPED]
`

// customLabelPriorityConfig renames the priority vocabulary. The names
// share no substring with P0..P3 or with the critical/high/medium/low
// aliases, so a leaked built-in is unambiguous.
const customLabelPriorityConfig = `storage:
  db_path: %s
task:
  priorities:
    - name: URGENT
      label: Urgent
      color: red
    - name: NORMAL
      label: Normal
      color: blue
    - name: LATER
      label: Later
      color: gray
`

// defaultLabelConfig declares no vocabularies, so the built-ins apply.
const defaultLabelConfig = `storage:
  db_path: %s
`

// labelLineRe parses one `label init` line: "  - name (COLOR): desc".
var labelLineRe = regexp.MustCompile(`^\s*-\s+(\S+)\s+\(([^)]*)\):`)

type seededLabel struct {
	name  string
	color string
}

// parseLabelInit turns `label init` output into structured labels, so
// assertions can talk about colour as well as name.
func parseLabelInit(out string) []seededLabel {
	var got []seededLabel
	for _, line := range strings.Split(out, "\n") {
		if m := labelLineRe.FindStringSubmatch(line); m != nil {
			got = append(got, seededLabel{name: m[1], color: m[2]})
		}
	}
	return got
}

// runLabelInit runs `label init --type generic` under one config and
// returns the parsed labels. --type is forced rather than detected: the
// tempdir cwd has no project markers, and these cases are about the
// shared axes, not about detection.
func runLabelInit(t *testing.T, template string) []seededLabel {
	t.Helper()
	bin, home, env := statusVocabFixture(t, template)
	out := runTLCOK(t, bin, home, env, "label", "init", "--type", "generic")
	got := parseLabelInit(out)
	if len(got) == 0 {
		t.Fatalf("label init produced no parseable labels:\n%s", out)
	}
	return got
}

func labelNames(ls []seededLabel) map[string]string {
	m := make(map[string]string, len(ls))
	for _, l := range ls {
		m[l.name] = l.color
	}
	return m
}

// TestLabelInitDefaultAxes is the default-vocabulary acceptance case:
// the full type axis, the priority aliases the plugins emit, every
// effort, the active status, and no bare label anywhere.
func TestLabelInitDefaultAxes(t *testing.T) {
	got := labelNames(runLabelInit(t, defaultLabelConfig))

	want := []string{
		"type:feat", "type:fix", "type:refactor", "type:docs", "type:test",
		"type:chore", "type:perf", "type:build", "type:ci", "type:style",
		"type:breaking",
		"priority:critical", "priority:high", "priority:medium", "priority:low",
		"effort:xs", "effort:s", "effort:m", "effort:l", "effort:xl",
		"status:in-progress", "status:blocked",
	}
	for _, w := range want {
		if _, ok := got[w]; !ok {
			t.Errorf("default label init missing %q", w)
		}
	}
	for name := range got {
		if !strings.Contains(name, ":") {
			t.Errorf("bare label %q seeded; sync discards colon-free labels", name)
		}
	}
}

// TestLabelInitCustomStatusVocabulary is the headline generation case.
// A hardcoded status list cannot pass it: the expected names exist only
// in the config file, and the forbidden ones are precisely what a
// hardcoded list would emit.
func TestLabelInitCustomStatusVocabulary(t *testing.T) {
	got := labelNames(runLabelInit(t, customLabelStatusConfig))

	// Non-terminal, non-initial statuses get a label.
	for _, w := range []string{"status:doing", "status:in-review"} {
		if _, ok := got[w]; !ok {
			t.Errorf("missing %q; status axis is not following the configured vocabulary", w)
		}
	}

	// The built-in names must not appear at all — none is declared.
	for _, gone := range []string{
		"status:todo", "status:in-progress", "status:done", "status:skipped",
	} {
		if _, ok := got[gone]; ok {
			t.Errorf("%q leaked; the built-in status list is still hardcoded", gone)
		}
	}

	// The initial status and both terminal ones carry no label: open vs
	// closed issue state already encodes them, and a second encoding
	// would be free to disagree with the first.
	for _, gone := range []string{"status:backlog", "status:shipped", "status:dropped"} {
		if _, ok := got[gone]; ok {
			t.Errorf("%q emitted; initial and terminal statuses are encoded by issue state", gone)
		}
	}

	// status:blocked survives a renamed vocabulary because blocked is
	// not a status at all — it is Task.BlockedReason, orthogonal to the
	// axis — and every sync plugin pushes and pulls the label.
	if _, ok := got["status:blocked"]; !ok {
		t.Error("status:blocked missing; it is independent of the status vocabulary")
	}
}

// TestLabelInitStatusColoursFollowConfig pins the colour flow. An
// invented palette would survive every name assertion above, so colour
// needs its own case: a declared hex passes through, a declared name
// resolves, and neither is the old hardcoded 1D76DB by accident.
func TestLabelInitStatusColoursFollowConfig(t *testing.T) {
	got := labelNames(runLabelInit(t, customLabelStatusConfig))

	if c := got["status:doing"]; c != "FF00AA" {
		t.Errorf("status:doing colour = %q, want FF00AA from the declared #FF00AA", c)
	}
	// purple resolves through the name table; the assertion is that it
	// is neither empty nor a raw colour name reaching the forge.
	if c := got["status:in-review"]; c == "" || c == "purple" {
		t.Errorf("status:in-review colour = %q, want a resolved hex for the declared 'purple'", c)
	}
}

// TestLabelInitCustomPriorityVocabulary: a renamed priority vocabulary
// must reach the labels, and P0-P3's aliases must not survive it.
func TestLabelInitCustomPriorityVocabulary(t *testing.T) {
	got := labelNames(runLabelInit(t, customLabelPriorityConfig))

	for _, w := range []string{"priority:urgent", "priority:normal", "priority:later"} {
		if _, ok := got[w]; !ok {
			t.Errorf("missing %q; priority axis is not following the configured vocabulary", w)
		}
	}
	for _, gone := range []string{
		"priority:critical", "priority:high", "priority:medium", "priority:low",
		"priority:p0", "priority:p1", "priority:p2", "priority:p3",
	} {
		if _, ok := got[gone]; ok {
			t.Errorf("%q leaked; the built-in priority list is still hardcoded", gone)
		}
	}
}

// TestLabelInitPriorityOrderIsRank pins declaration order into the
// output. Declaration order IS rank order, and the names are chosen so
// alphabetical order (LATER, NORMAL, URGENT) is its exact reverse — a
// sorted axis would present the least urgent priority first.
func TestLabelInitPriorityOrderIsRank(t *testing.T) {
	var seen []string
	for _, l := range runLabelInit(t, customLabelPriorityConfig) {
		if strings.HasPrefix(l.name, "priority:") {
			seen = append(seen, l.name)
		}
	}
	want := []string{"priority:urgent", "priority:normal", "priority:later"}
	if len(seen) != len(want) {
		t.Fatalf("priority axis = %v, want %v", seen, want)
	}
	for i, w := range want {
		if seen[i] != w {
			t.Errorf("priority axis position %d = %q, want %q (declaration order is rank)", i, seen[i], w)
		}
	}
}

// TestLabelInitColoursAreHex guards the config/forge seam across every
// vocabulary: config declares colours by NAME, a forge label needs six
// hex digits, and a name leaking through unresolved makes GitHub reject
// the label.
func TestLabelInitColoursAreHex(t *testing.T) {
	hex := regexp.MustCompile(`^[0-9A-Fa-f]{6}$`)
	for _, cfg := range []string{
		defaultLabelConfig, customLabelStatusConfig, customLabelPriorityConfig,
	} {
		for _, l := range runLabelInit(t, cfg) {
			if !hex.MatchString(l.color) {
				t.Errorf("label %q colour %q is not a 6-digit hex", l.name, l.color)
			}
		}
	}
}

// TestLabelInitDomainSetsPerType pins the pre-existing per-type domain
// sets through the real command, so generating the shared axes cannot
// regress the part of `label init` that already worked.
func TestLabelInitDomainSetsPerType(t *testing.T) {
	bin, home, env := statusVocabFixture(t, defaultLabelConfig)

	cases := map[string][]string{
		"go-binary":      {"domain:cli", "domain:core", "domain:config", "domain:io", "domain:storage"},
		"node-backend":   {"domain:api", "domain:db", "domain:auth", "domain:jobs"},
		"react-frontend": {"domain:frontend", "domain:components", "domain:state", "domain:api"},
		"python-mvc":     {"domain:models", "domain:views", "domain:api", "domain:migrations"},
		"library":        {"domain:api", "domain:internal", "domain:docs"},
		"monorepo":       {"domain:tooling", "domain:release", "domain:deps"},
		"infra":          {"domain:terraform", "domain:k8s", "domain:network", "domain:secrets"},
		"generic":        {"domain:core", "domain:api", "domain:docs", "domain:ci"},
	}
	for pType, want := range cases {
		out := runTLCOK(t, bin, home, env, "label", "init", "--type", pType)
		got := labelNames(parseLabelInit(out))
		for _, w := range want {
			if _, ok := got[w]; !ok {
				t.Errorf("--type %s missing %q:\n%s", pType, w, out)
			}
		}
	}
}

// TestLabelInitTypeIsNotIgnored is the phantom-type defect through the
// real binary.
//
// `label init --type X` echoes X back in its own header, so a type that
// fell through to the default arm looked indistinguishable from a type
// that worked: `--type python-mvc` printed "Detected project type:
// python-mvc" above the generic labels. Comparing each type's domain
// set against generic's is the only check that catches it — asserting
// the presence of individual names cannot, because the generic set is a
// subset of what several types legitimately emit.
func TestLabelInitTypeIsNotIgnored(t *testing.T) {
	bin, home, env := statusVocabFixture(t, defaultLabelConfig)

	domainsFor := func(pType string) []string {
		out := runTLCOK(t, bin, home, env, "label", "init", "--type", pType)
		var got []string
		for name := range labelNames(parseLabelInit(out)) {
			if strings.HasPrefix(name, "domain:") {
				got = append(got, name)
			}
		}
		sort.Strings(got)
		return got
	}

	generic := domainsFor("generic")
	for _, pType := range []string{
		"go-binary", "node-backend", "react-frontend", "python-mvc",
		"library", "monorepo", "infra",
	} {
		if got := domainsFor(pType); slices.Equal(got, generic) {
			t.Errorf("--type %s emitted generic's domains %v; the flag was accepted and ignored", pType, got)
		}
	}

	// An unrecognised value must emit generic's set EXACTLY, not merely
	// something. --type is a free string, so this is the arm a typo
	// reaches, and it is documented as equivalent to generic.
	if got := domainsFor("not-a-real-type"); !slices.Equal(got, generic) {
		t.Errorf("--type not-a-real-type emitted %v; want generic's %v", got, generic)
	}
}

// TestLabelInitHelpNamesOnlyWorkingTypes is acceptance in the user's
// terms: the help text is where a user learns which values --type
// takes, so a name there that produces generic output is the whole
// defect restated. go-socket and microservices were never in the help,
// but they were declared constants, and the constants are what a future
// contributor copies into it.
func TestLabelInitHelpNamesOnlyWorkingTypes(t *testing.T) {
	bin, home, env := statusVocabFixture(t, defaultLabelConfig)
	out := runTLCOK(t, bin, home, env, "label", "init", "--help")

	for _, want := range []string{
		"go-binary", "node-backend", "react-frontend", "python-mvc",
		"library", "monorepo", "infra", "generic",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("label init --help does not name working type %q:\n%s", want, out)
		}
	}
	for _, gone := range []string{"go-socket", "microservices"} {
		if strings.Contains(out, gone) {
			t.Errorf("label init --help still advertises removed type %q:\n%s", gone, out)
		}
	}
}

// TestLabelTemplatesListsEveryType keeps the second surface coherent:
// `label templates` enumerates the built-in templates, and generation
// must not break its grouping or drop a type.
func TestLabelTemplatesListsEveryType(t *testing.T) {
	bin, home, env := statusVocabFixture(t, defaultLabelConfig)
	out := runTLCOK(t, bin, home, env, "label", "templates")

	for _, want := range []string{
		"[go-binary]", "[node-backend]", "[react-frontend]", "[python-mvc]",
		"[library]", "[monorepo]", "[infra]", "[generic]",
		"domain:cli", "domain:frontend", "domain:models", "domain:migrations",
		"domain:jobs", "domain:internal", "domain:tooling", "domain:terraform",
		"type:feat", "effort:xl",
		"priority:critical", "status:in-progress",
		"needs:triage", "needs:decision",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("label templates output missing %q:\n%s", want, out)
		}
	}
	// The listing is the other place a phantom surfaces: a type printed
	// here with generic's labels under it reads as a real template.
	for _, gone := range []string{"[go-socket]", "[microservices]"} {
		if strings.Contains(out, gone) {
			t.Errorf("label templates still lists removed type %q:\n%s", gone, out)
		}
	}
}

// TestLabelTemplatesFollowsCustomVocabulary proves the generation
// reaches the OTHER surface too. `label templates` enumerates built-in
// project types, which reads like a static listing — exactly the kind of
// path that would keep a hardcoded axis after `label init` was fixed.
func TestLabelTemplatesFollowsCustomVocabulary(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customLabelStatusConfig)
	out := runTLCOK(t, bin, home, env, "label", "templates")

	if !strings.Contains(out, "status:doing") {
		t.Errorf("label templates does not follow the configured vocabulary:\n%s", out)
	}
	if strings.Contains(out, "status:in-progress") {
		t.Errorf("label templates leaked the built-in status list:\n%s", out)
	}
}
