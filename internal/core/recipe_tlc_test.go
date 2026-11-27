package core

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const tlcRecipeDoc = `
recipe: shipit
version: 1.0.0
vars:
  area: {default: core}
  level: {default: P1}
track:
  title: "Ship {{area}}"
  type: feature
  meta:
    owner: "{{area}}-team"
    cost: 3
steps:
  - id: plan
    title: Plan
    tags: [planning, "area:{{area}}"]
    meta: {ticket: "{{area}}-1", weight: 2}
    priority: "{{level}}"
  - id: build
    depends_on: [plan]
    x-other: {keep: me}
`

func TestTLCKeys_ParseIntoSingleExtension(t *testing.T) {
	r := mustParse(t, tlcRecipeDoc)
	if len(r.Ext) != 1 || r.Ext[extTLC] == nil {
		t.Fatalf("recipe Ext = %v; want only %q", r.Ext, extTLC)
	}
	track, err := RecipeTrackOf(r)
	if err != nil || track == nil {
		t.Fatalf("RecipeTrackOf = %+v, %v", track, err)
	}
	if track.Title != "Ship {{area}}" || track.Type != "feature" || track.Meta["owner"] != "{{area}}-team" {
		t.Errorf("track = %+v", track)
	}
	plan, err := StepExtrasOf(r.Steps[0])
	if err != nil {
		t.Fatal(err)
	}
	want := StepExtras{
		Tags: []string{"planning", "area:{{area}}"}, Meta: map[string]any{"ticket": "{{area}}-1", "weight": 2}, Priority: "{{level}}",
	}
	if !reflect.DeepEqual(plan, want) {
		t.Errorf("plan extras = %+v; want %+v", plan, want)
	}
	build := r.Steps[1]
	if x, _ := StepExtrasOf(build); !x.IsZero() {
		t.Errorf("build extras = %+v; want none", x)
	}
	if !reflect.DeepEqual(build.Ext["x-other"], map[string]any{"keep": "me"}) {
		t.Errorf("x-other = %v; want it passed through untouched", build.Ext["x-other"])
	}
	for k := range plan.Meta {
		if strings.HasPrefix(k, tlcInflight) {
			t.Errorf("in-flight key %q leaked", k)
		}
	}
}

func TestTLCKeys_JSONDocument(t *testing.T) {
	doc := `{"recipe":"j","version":"1.0.0","track":{"title":"T","meta":{"k":"v"}},
	 "steps":[{"id":"a","tags":["x"],"priority":"P2","meta":{"m":1}}]}`
	r, err := ParseRecipe(strings.NewReader(doc), "j.json")
	if err != nil {
		t.Fatal(err)
	}
	track, _ := RecipeTrackOf(r)
	x, _ := StepExtrasOf(r.Steps[0])
	if track == nil || track.Meta["k"] != "v" || x.Priority != "P2" || !reflect.DeepEqual(x.Tags, []string{"x"}) {
		t.Errorf("track=%+v extras=%+v", track, x)
	}
}

func TestTLCKeys_ReservedExtensionRejected(t *testing.T) {
	cases := map[string]string{
		"flow root": "recipe: r\nversion: 1.0.0\nx-tlc: {track: {title: t}}\nsteps:\n  - id: a\n",
		"step":      "recipe: r\nversion: 1.0.0\nsteps:\n  - id: a\n    x-tlc: {tags: [a]}\n",
		"nested":    "recipe: r\nversion: 1.0.0\nsteps:\n  - id: blk\n    repeat: 2\n    steps:\n      - id: a\n        x-tlc: {tags: [a]}\n",
		"in-flight": "recipe: r\nversion: 1.0.0\nsteps:\n  - id: a\n    x-tlc.tags: [a]\n",
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseRecipe(strings.NewReader(doc), "r.yaml")
			if err == nil || !strings.Contains(err.Error(), "reserved") || !strings.Contains(err.Error(), "x-tlc") {
				t.Fatalf("err = %v; want x-tlc reported as reserved", err)
			}
		})
	}
	_, err := ParseRecipe(strings.NewReader(`{"recipe":"r","version":"1.0.0","steps":[{"id":"a","x-tlc":{}}]}`), "r.json")
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("json err = %v; want reserved", err)
	}
}

func TestTLCKeys_ReservedMetaRejectedAtParse(t *testing.T) {
	step := "recipe: r\nversion: 1.0.0\nsteps:\n  - id: a\n    meta: {blocked_by: [x]}\n"
	if _, err := ParseRecipe(strings.NewReader(step), "r.yaml"); err == nil || !strings.Contains(err.Error(), "blocked_by") {
		t.Fatalf("step meta err = %v; want the reserved key named", err)
	}
	track := "recipe: r\nversion: 1.0.0\ntrack: {title: t, meta: {plans: x}}\nsteps:\n  - id: a\n"
	if _, err := ParseRecipe(strings.NewReader(track), "r.yaml"); err == nil || !strings.Contains(err.Error(), "plans") {
		t.Fatalf("track meta err = %v; want the reserved key named", err)
	}
}

func TestTLCKeys_BadShapesRejected(t *testing.T) {
	for name, doc := range map[string]string{
		"tags not a list":   "recipe: r\nversion: 1.0.0\nsteps:\n  - id: a\n    tags: {a: b}\n",
		"meta not a map":    "recipe: r\nversion: 1.0.0\nsteps:\n  - id: a\n    meta: [a]\n",
		"unknown track key": "recipe: r\nversion: 1.0.0\ntrack: {titel: t}\nsteps:\n  - id: a\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseRecipe(strings.NewReader(doc), "r.yaml"); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestTLCKeys_UnknownKeyStillReported(t *testing.T) {
	doc := "recipe: r\nversion: 1.0.0\ntrack: {title: t}\nsteps:\n  - id: a\n    tags: [x]\n    bogus: 1\n"
	_, err := ParseRecipe(strings.NewReader(doc), "r.yaml")
	if err == nil || !strings.Contains(err.Error(), "bogus") || strings.Contains(err.Error(), "x-tlc") {
		t.Fatalf("err = %v; want the unknown key named, in recipe vocabulary", err)
	}
}

func TestTLCKeys_ThroughIncludeAndRepeat(t *testing.T) {
	child := mustParse(t, `
recipe: child
version: 1.0.0
steps:
  - id: scan
    tags: [security]
    meta: {scanner: "{{tool}}"}
    priority: P0
vars: {tool: {default: grype}}
`)
	parent := mustParse(t, `
recipe: parent
version: 1.0.0
vars: {who: {default: me}}
steps:
  - id: sec
    include: child@1.0.0
  - id: rounds
    repeat: 2
    steps:
      - id: review
        tags: [review]
        meta: {owner: "{{who}}"}
`)
	steps, err := Expand(parent, memLocator{"child@1.0.0": child})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]StepExtras{}
	for _, s := range steps {
		x, err := StepExtrasOf(s)
		if err != nil {
			t.Fatal(err)
		}
		got[s.ID] = x
	}
	if len(steps) != 3 {
		t.Fatalf("steps = %v", stepIDs(steps))
	}
	var scanID string
	for id, x := range got {
		if x.Priority == "P0" {
			scanID = id
		}
	}
	if scanID == "" || !strings.HasSuffix(scanID, "scan") || !reflect.DeepEqual(got[scanID].Tags, []string{"security"}) {
		t.Errorf("included step extras lost: %v", got)
	}
	reviews := 0
	for id, x := range got {
		if strings.Contains(id, "review") {
			reviews++
			if !reflect.DeepEqual(x.Tags, []string{"review"}) {
				t.Errorf("%s tags = %v", id, x.Tags)
			}
		}
	}
	if reviews != 2 {
		t.Errorf("review steps = %d; want 2 unrolled", reviews)
	}
}

func TestTLCKeys_RenderedWithRecipeVars(t *testing.T) {
	r := mustParse(t, tlcRecipeDoc)
	steps, err := Expand(r, memLocator{})
	if err != nil {
		t.Fatal(err)
	}
	vars, err := ResolveRecipeVars(r, map[string]string{"area": "api", "level": "P0"})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderSteps(steps, Scope{Vars: vars})
	if err != nil {
		t.Fatal(err)
	}
	x, _ := StepExtrasOf(rendered[0])
	if x.Priority != "P0" || x.Meta["ticket"] != "api-1" || !reflect.DeepEqual(x.Tags, []string{"planning", "area:api"}) {
		t.Errorf("rendered extras = %+v", x)
	}
	if raw, _ := StepExtrasOf(steps[0]); raw.Priority != "{{level}}" {
		t.Errorf("rendering mutated the expanded step: %+v", raw)
	}
	track, _ := RecipeTrackOf(r)
	meta, err := RenderRecipeMap(track.Meta, Scope{Vars: vars})
	if err != nil || meta["owner"] != "api-team" || meta["cost"] != 3 {
		t.Errorf("track meta = %v, %v", meta, err)
	}
}

func TestTLCKeys_WrittenBackAsPlainKeys(t *testing.T) {
	r := mustParse(t, tlcRecipeDoc)
	data, err := MarshalRecipe(r)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "x-tlc") {
		t.Fatalf("x-tlc leaked into the document:\n%s", text)
	}
	for _, want := range []string{"track:", "tags:", "priority:", "meta:", "x-other:"} {
		if !strings.Contains(text, want) {
			t.Errorf("document lacks %q:\n%s", want, text)
		}
	}
	if strings.Index(text, "track:") > strings.Index(text, "steps:") {
		t.Errorf("track: should precede steps:\n%s", text)
	}
	back, err := ParseRecipe(strings.NewReader(text), "back.yaml")
	if err != nil {
		t.Fatalf("round trip: %v\n%s", err, text)
	}
	if !reflect.DeepEqual(back.Steps[0].Ext, r.Steps[0].Ext) || !reflect.DeepEqual(back.Ext, r.Ext) {
		t.Errorf("round trip changed extensions:\n%v\n%v", back.Steps[0].Ext, r.Steps[0].Ext)
	}
}

func TestTLCKeys_PlainStepEncodings(t *testing.T) {
	r := mustParse(t, tlcRecipeDoc)
	steps := PlainSteps(r.Steps)
	j, err := json.Marshal(steps[0])
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(j, &obj); err != nil {
		t.Fatal(err)
	}
	if _, bad := obj[extTLC]; bad || obj["priority"] != "{{level}}" || obj["tags"] == nil || obj["meta"] == nil {
		t.Errorf("json step = %s", j)
	}
	y, err := yaml.Marshal(steps[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(y), "x-tlc") || !strings.Contains(string(y), "tags:") {
		t.Errorf("yaml step =\n%s", y)
	}
	bare := RecipeStep{ID: "p"}
	plain, _ := json.Marshal(PlainStep{bare})
	if want, _ := json.Marshal(bare); string(plain) != string(want) {
		t.Errorf("step without extras = %s; want %s", plain, want)
	}
}

func TestTLCKeys_CapturedRecipeWritesTrackBlock(t *testing.T) {
	rec := &Recipe{Name: "c", Version: "0.1.0", Steps: []RecipeStep{{ID: "a", Title: "A"}}}
	if err := SetRecipeTrack(rec, &RecipeTrack{Title: "T", Meta: map[string]any{"k": "v"}}); err != nil {
		t.Fatal(err)
	}
	data, err := MarshalRecipe(rec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "x-tlc") || !strings.Contains(string(data), "track:") {
		t.Fatalf("document:\n%s", data)
	}
}

// --- materialization ----------------------------------------------------

func extrasStep(t *testing.T, id string, x StepExtras) RecipeStep {
	t.Helper()
	s := RecipeStep{ID: id, Title: id, Ordinal: 1}
	if err := SetStepExtras(&s, x); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMaterialize_StepTagsMetaPriority(t *testing.T) {
	f := newMatFixture()
	steps := []RecipeStep{extrasStep(t, "plan", StepExtras{
		Tags: []string{"planning", "area:api"}, Meta: map[string]any{"ticket": "api-1", "weight": 2}, Priority: "P1",
	}), {ID: "bare", Title: "bare", Ordinal: 2}}
	res := f.materialize(t, matInput(steps))
	plan := f.task(t, stepTaskIDs(res)["plan"])
	if !reflect.DeepEqual(plan.Tags, []string{"planning", "area:api"}) || plan.Priority != "P1" {
		t.Errorf("tags=%v priority=%q", plan.Tags, plan.Priority)
	}
	if plan.Meta["ticket"] != "api-1" || plan.Meta["weight"] != 2 {
		t.Errorf("meta = %v", plan.Meta)
	}
	if plan.Meta[metaRecipe] == nil {
		t.Errorf("recipe provenance lost next to user meta: %v", plan.Meta)
	}
	bare := f.task(t, stepTaskIDs(res)["bare"])
	if len(bare.Tags) != 0 || bare.Priority != "" || bare.Meta["ticket"] != nil {
		t.Errorf("bare task picked up fields: %+v", bare)
	}
}

func TestMaterialize_StepMetaKeepsDependencyEdges(t *testing.T) {
	f := newMatFixture()
	a := extrasStep(t, "a", StepExtras{Meta: map[string]any{"k": "v"}})
	b := extrasStep(t, "b", StepExtras{Meta: map[string]any{"k": "w"}})
	b.DependsOn, b.Ordinal = []string{"a"}, 2
	res := f.materialize(t, matInput([]RecipeStep{a, b}))
	ids := stepTaskIDs(res)
	assertBlockedBy(t, f.task(t, ids["b"]), ids["a"])
	if f.task(t, ids["b"]).Meta["k"] != "w" {
		t.Errorf("meta = %v", f.task(t, ids["b"]).Meta)
	}
}

func TestMaterialize_ReservedMetaCreatesNothing(t *testing.T) {
	for _, dry := range []bool{false, true} {
		f := newMatFixture()
		ok := extrasStep(t, "ok", StepExtras{Meta: map[string]any{"fine": 1}})
		bad := extrasStep(t, "bad", StepExtras{Meta: map[string]any{"blocked_by": []any{"x"}}})
		bad.Ordinal = 2
		in := matInput([]RecipeStep{ok, bad})
		in.DryRun = dry
		_, err := f.m.Materialize(context.Background(), in)
		if err == nil || !strings.Contains(err.Error(), "blocked_by") || !strings.Contains(err.Error(), "bad") {
			t.Fatalf("dry=%v err = %v; want the step and the reserved key", dry, err)
		}
		if len(f.repo.Tasks) != 0 || len(f.runs.runs) != 0 {
			t.Errorf("dry=%v tasks=%d runs=%d; want nothing written", dry, len(f.repo.Tasks), len(f.runs.runs))
		}
	}
}

func TestMaterialize_InvalidPriorityCreatesNothing(t *testing.T) {
	f := newMatFixture()
	s := extrasStep(t, "a", StepExtras{Priority: "urgent-ish"})
	_, err := f.m.Materialize(context.Background(), matInput([]RecipeStep{s}))
	if err == nil || !strings.Contains(err.Error(), "priority") || !strings.Contains(err.Error(), "urgent-ish") {
		t.Fatalf("err = %v; want the priority named", err)
	}
	if len(f.repo.Tasks) != 0 || len(f.runs.runs) != 0 {
		t.Errorf("tasks=%d runs=%d; want nothing written", len(f.repo.Tasks), len(f.runs.runs))
	}
}

func TestMaterialize_WhenDroppedStepFieldsNeverLand(t *testing.T) {
	f := newMatFixture()
	keep := extrasStep(t, "keep", StepExtras{Tags: []string{"kept"}})
	gone := extrasStep(t, "gone", StepExtras{Tags: []string{"dropped"}, Meta: map[string]any{"only": "gone"}})
	gone.When, gone.Ordinal = "vars.ship == false", 2
	kept := DropFalseWhen([]RecipeStep{keep, gone}, map[string]any{"ship": true})
	res := f.materialize(t, matInput(kept.Steps))
	if len(res.Created) != 1 {
		t.Fatalf("created = %v; want only keep", res.Created)
	}
	for _, task := range f.repo.Tasks {
		if task.Meta["only"] != nil || reflect.DeepEqual(task.Tags, []string{"dropped"}) {
			t.Errorf("dropped step's fields landed on %+v", task)
		}
	}
}

func TestReconcile_ExistingTasksKeepTheirFields(t *testing.T) {
	f := newMatFixture()
	first := extrasStep(t, "a", StepExtras{Tags: []string{"old"}, Meta: map[string]any{"v": 1}, Priority: "P2"})
	res := f.materialize(t, matInput([]RecipeStep{first}))
	id := stepTaskIDs(res)["a"]

	changed := extrasStep(t, "a", StepExtras{Tags: []string{"new"}, Meta: map[string]any{"v": 2}, Priority: "P0"})
	f.reconcile(t, matInput([]RecipeStep{changed}))
	task := f.task(t, id)
	if !reflect.DeepEqual(task.Tags, []string{"old"}) || task.Meta["v"] != 1 || task.Priority != "P2" {
		t.Errorf("reconcile rewrote an existing task: tags=%v meta=%v priority=%q", task.Tags, task.Meta, task.Priority)
	}
}
