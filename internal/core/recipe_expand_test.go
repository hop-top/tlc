package core

import (
	"strings"
	"testing"
)

// memLocator serves recipes from memory for expansion tests.
type memLocator map[string]*Recipe

func (m memLocator) Locate(ref string) (*Recipe, error) {
	if r, ok := m[ref]; ok {
		return r, nil
	}
	return nil, &RecipeNotFoundError{Ref: ref}
}

func (m memLocator) List() ([]RecipeInfo, error) { return nil, nil }

func mustParse(t *testing.T, yamlText string) *Recipe {
	t.Helper()
	r, err := ParseRecipe(strings.NewReader(yamlText), "mem.yaml")
	if err != nil {
		t.Fatalf("ParseRecipe: %v\n%s", err, yamlText)
	}
	return r
}

func stepIDs(steps []RecipeStep) []string {
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = s.ID
	}
	return out
}

func TestExpand_IncludePrefixesIDsDepsAndRefs(t *testing.T) {
	child := mustParse(t, `
recipe: security-scan
version: 1.0.0
vars: {target: {required: true}}
steps:
  - id: scan
    kind: exec
    exec: {argv: [scan, "{{target}}"]}
  - id: report
    depends_on: [scan]
    description: "scan of {{target}}: {{results.scan.stdout}}"
    when: "results.scan.exit_code == 0"
`)
	parent := mustParse(t, `
recipe: review
version: 1.0.0
vars: {pr: {required: true}}
steps:
  - id: lint
    kind: exec
    exec: {argv: [lint]}
  - id: sec
    include: security-scan@1.0.0
    with: {target: "{{pr}}"}
    depends_on: [lint]
  - id: wrap
    depends_on: [sec]
    description: "{{results.sec/report.stdout}}"
`)
	steps, err := Expand(parent, memLocator{"security-scan@1.0.0": child})
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	want := []string{"lint", "sec/scan", "sec/report", "wrap"}
	if strings.Join(stepIDs(steps), ",") != strings.Join(want, ",") {
		t.Fatalf("ids = %v; want %v", stepIDs(steps), want)
	}
	byID := map[string]RecipeStep{}
	for i, s := range steps {
		byID[s.ID] = s
		if s.Ordinal != i+1 {
			t.Errorf("%s ordinal = %d; want %d", s.ID, s.Ordinal, i+1)
		}
	}
	// The child's first step inherits the include's dependencies; the
	// child's internal edge is prefixed; the parent's edge on the include
	// points at every child leaf.
	if got := byID["sec/scan"].DependsOn; strings.Join(got, ",") != "lint" {
		t.Errorf("sec/scan depends_on = %v; want [lint]", got)
	}
	if got := byID["sec/report"].DependsOn; strings.Join(got, ",") != "sec/scan" {
		t.Errorf("sec/report depends_on = %v; want [sec/scan]", got)
	}
	if got := byID["wrap"].DependsOn; strings.Join(got, ",") != "sec/report" {
		t.Errorf("wrap depends_on = %v; want [sec/report] (child leaves)", got)
	}
	// results.* refs inside the child are rewritten to the prefixed ids;
	// `with` bindings are rendered into the child's var references.
	if d := byID["sec/report"].Description; !strings.Contains(d, "{{results.sec/scan.stdout}}") || !strings.Contains(d, "{{pr}}") {
		t.Errorf("child description not rewritten: %q", d)
	}
	if w := byID["sec/report"].When; w != "results.sec/scan.exit_code == 0" {
		t.Errorf("child when not rewritten: %q", w)
	}
	if argv := byID["sec/scan"].Exec.Argv; argv[1] != "{{pr}}" {
		t.Errorf("with binding not applied to child vars: %v", argv)
	}
}

func TestExpand_IncludeErrors(t *testing.T) {
	child := mustParse(t, "recipe: c\nversion: 1\nvars: {target: {required: true}}\nsteps:\n  - id: s\n")
	for name, yamlText := range map[string]string{
		"missing include":  "recipe: p\nversion: 1\nsteps:\n  - id: x\n    include: nope@1\n",
		"unknown with key": "recipe: p\nversion: 1\nsteps:\n  - id: x\n    include: c@1\n    with: {bogus: 1}\n",
		"required unbound": "recipe: p\nversion: 1\nsteps:\n  - id: x\n    include: c@1\n",
	} {
		_, err := Expand(mustParse(t, yamlText), memLocator{"c@1": child})
		if err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	// Self-inclusion and deeper cycles are refused.
	loop := mustParse(t, "recipe: loop\nversion: 1\nsteps:\n  - id: again\n    include: loop@1\n")
	if _, err := Expand(loop, memLocator{"loop@1": loop}); err == nil || !strings.Contains(err.Error(), "include") {
		t.Errorf("include cycle: err = %v", err)
	}
}

func TestExpand_RepeatUnrollsWithDerivedWhen(t *testing.T) {
	r := mustParse(t, `
recipe: loop
version: 1
steps:
  - id: build
    kind: exec
    exec: {argv: [make]}
  - id: fix-loop
    depends_on: [build]
    repeat: 3
    until: "results.review.verdict == 'approved'"
    steps:
      - id: fix
        description: "fix round {{run.iteration}}"
      - id: review
        depends_on: [fix]
  - id: ship
    depends_on: [fix-loop]
`)
	steps, err := Expand(r, memLocator{})
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	want := []string{"build", "fix-loop/1/fix", "fix-loop/1/review", "fix-loop/2/fix", "fix-loop/2/review", "fix-loop/3/fix", "fix-loop/3/review", "ship"}
	if strings.Join(stepIDs(steps), ",") != strings.Join(want, ",") {
		t.Fatalf("ids = %v; want %v", stepIDs(steps), want)
	}
	byID := map[string]RecipeStep{}
	for _, s := range steps {
		byID[s.ID] = s
	}
	if d := byID["fix-loop/1/fix"].DependsOn; strings.Join(d, ",") != "build" {
		t.Errorf("first iteration depends_on = %v; want [build]", d)
	}
	if d := byID["fix-loop/2/fix"].DependsOn; strings.Join(d, ",") != "fix-loop/1/review" {
		t.Errorf("second iteration depends_on = %v; want the previous iteration's leaves", d)
	}
	if w := byID["fix-loop/1/fix"].When; w != "" {
		t.Errorf("first iteration has a when: %q", w)
	}
	if w := byID["fix-loop/2/fix"].When; w != "results.fix-loop/1/review.verdict != 'approved'" {
		t.Errorf("derived when = %q", w)
	}
	if w := byID["fix-loop/3/review"].When; w != "results.fix-loop/2/review.verdict != 'approved'" {
		t.Errorf("derived when on iteration 3 = %q", w)
	}
	if d := byID["fix-loop/2/fix"].Description; !strings.Contains(d, "round 2") {
		t.Errorf("run.iteration not bound: %q", d)
	}
	if d := byID["ship"].DependsOn; strings.Join(d, ",") != "fix-loop/3/review" {
		t.Errorf("ship depends_on = %v; want the last iteration's leaves", d)
	}
}

func TestExpand_RepeatRequiresUntilOrPlainRepeat(t *testing.T) {
	r := mustParse(t, "recipe: x\nversion: 1\nsteps:\n  - id: l\n    repeat: 2\n    steps:\n      - id: a\n")
	steps, err := Expand(r, memLocator{})
	if err != nil {
		t.Fatalf("plain repeat: %v", err)
	}
	if len(steps) != 2 || steps[1].When != "" {
		t.Errorf("plain repeat without until: %v / %q", stepIDs(steps), steps[1].When)
	}
}

// TestExpand_StepsRefsOrderRendering: steps.* references add implicit
// render-order edges without adding blocked_by; forward and cyclic
// references are errors.
func TestExpand_StepsRefsOrderRendering(t *testing.T) {
	r := mustParse(t, `
recipe: sprint
version: 1
vars: {sprint_end: {required: true}}
steps:
  - id: retro
    due: "{{steps.planning.due}} + 5d"
    description: "retro after {{steps.planning.title}}"
  - id: planning
    title: "Plan"
    due: "{{sprint_end}}"
`)
	steps, err := Expand(r, memLocator{})
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	rendered, err := RenderSteps(steps, Scope{Vars: map[string]any{"sprint_end": "2026-10-01"}})
	if err != nil {
		t.Fatalf("RenderSteps: %v", err)
	}
	byID := map[string]RecipeStep{}
	for _, s := range rendered {
		byID[s.ID] = s
	}
	if got := byID["retro"].Due.Raw; got != "2026-10-06" {
		t.Errorf("retro due = %q; want 2026-10-06 (planning due + 5d)", got)
	}
	if got := byID["retro"].Description; got != "retro after Plan" {
		t.Errorf("retro description = %q", got)
	}
	if len(byID["retro"].DependsOn) != 0 {
		t.Errorf("steps.* reference added blocked_by: %v", byID["retro"].DependsOn)
	}

	cyc := mustParse(t, "recipe: x\nversion: 1\nsteps:\n  - id: a\n    title: \"{{steps.b.title}}\"\n  - id: b\n    title: \"{{steps.a.title}}\"\n")
	if _, err := Expand(cyc, memLocator{}); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("steps.* cycle: err = %v", err)
	}
}

func TestRenderSteps_StructuredDue(t *testing.T) {
	r := mustParse(t, `
recipe: sprint
version: 1
steps:
  - id: planning
    due: "2026-10-01"
  - id: retro
    due: {after: planning, offset: 5d}
  - id: prep
    due: {after: planning, offset: -1d}
`)
	steps, err := Expand(r, memLocator{})
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	rendered, err := RenderSteps(steps, Scope{})
	if err != nil {
		t.Fatalf("RenderSteps: %v", err)
	}
	if rendered[1].Due.Raw != "2026-10-06" || rendered[2].Due.Raw != "2026-09-30" {
		t.Errorf("structured due = %q / %q; want 2026-10-06 / 2026-09-30", rendered[1].Due.Raw, rendered[2].Due.Raw)
	}
	// A natural-language base is left for the materializer's parser.
	nat := mustParse(t, "recipe: x\nversion: 1\nsteps:\n  - id: a\n    due: friday\n  - id: b\n    due: \"{{steps.a.due}} + 2d\"\n")
	es, _ := Expand(nat, memLocator{})
	rs, err := RenderSteps(es, Scope{})
	if err != nil {
		t.Fatalf("RenderSteps natural: %v", err)
	}
	if rs[1].Due.Raw != "friday + 2d" {
		t.Errorf("natural base was rewritten: %q", rs[1].Due.Raw)
	}
}
