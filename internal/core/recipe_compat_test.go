package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const legacyRecipeYAML = `recipe: legacy
version: 0.1.0
steps:
  - id: build
    title: "Build for {{run.recipe}}@{{ run.version }}"
    kind: exec
    exec: {argv: [make]}
  - id: ship
    title: Ship
    kind: human
    human: {timeout: 1h, on_timeout: reject}
    depends_on: [build]
    gate: {contract: release-ok, eva_url: "http://eva"}
`

// TestTaskSpec_StoredRowRoundTripsByteIdentical pins the persisted JSON of
// Task.Spec: a row written before the switch to hop.top/flo/spec decodes
// and re-encodes to the same bytes.
func TestTaskSpec_StoredRowRoundTripsByteIdentical(t *testing.T) {
	const stored = `{"agent":"planner","when":"results.a.exit_code == 0",` +
		`"exec":{"argv":["make","build"],"cwd":"svc","env":{"A":"1"},"timeout":"5m","stdout_max":2048},` +
		`"human":{"assignee":"@lead","timeout":"48h","on_timeout":"reject"},` +
		`"retry":{"max_attempts":3,"backoff":"30s"},` +
		`"gate":{"contract":"release-ok","eva_url":"http://eva"}}`
	var spec TaskSpec
	if err := json.Unmarshal([]byte(stored), &spec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if spec.Gate == nil || spec.Gate.Contract != "release-ok" || spec.Exec == nil || spec.Exec.StdoutMax != 2048 {
		t.Fatalf("decoded spec = %+v", spec)
	}
	out, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(out) != stored {
		t.Errorf("round trip changed the row:\n got %s\nwant %s", out, stored)
	}
}

func TestParseRecipe_AcceptsRecipeVocabulary(t *testing.T) {
	r, err := ParseRecipe(strings.NewReader(legacyRecipeYAML), "legacy.yaml")
	if err != nil {
		t.Fatalf("ParseRecipe: %v", err)
	}
	if r.Name != "legacy" || r.Steps[1].Check == nil || r.Steps[1].Check.Contract != "release-ok" {
		t.Errorf("parsed = %+v", r)
	}
	if want := rawHash([]byte(legacyRecipeYAML)); r.Hash != want {
		t.Errorf("Hash = %s; want the hash of the file as authored %s", r.Hash, want)
	}
	if !strings.Contains(r.Steps[0].Title, "{{run.flow}}") {
		t.Errorf("title = %q; want run.recipe mapped to run.flow", r.Steps[0].Title)
	}
	got, err := RenderStep(r.Steps[0], Scope{Run: map[string]any{"flow": "legacy", "version": "0.1.0"}})
	if err != nil || got.Title != "Build for legacy@0.1.0" {
		t.Errorf("render = %q, %v", got.Title, err)
	}
}

func TestParseRecipe_AcceptsRecipeVocabularyJSON(t *testing.T) {
	doc := `{"recipe":"legacy","version":"0.1.0","steps":[` +
		`{"id":"a","title":"A","kind":"human","human":{"timeout":"1h","on_timeout":"reject"},` +
		`"gate":{"contract":"c","eva_url":"u"}}]}`
	r, err := ParseRecipe(strings.NewReader(doc), "legacy.json")
	if err != nil || r.Name != "legacy" || r.Steps[0].Check == nil {
		t.Fatalf("ParseRecipe json: %v %+v", err, r)
	}
}

func TestParseRecipe_NestedGateAndLinesPreserved(t *testing.T) {
	doc := "recipe: nest\nversion: 1\nsteps:\n  - id: loop\n    repeat: 2\n    steps:\n" +
		"      - id: inner\n        title: x\n        gate: {contract: c, eva_url: u}\n" +
		"      - id: bad\n        title: y\n        bogus: 1\n"
	_, err := ParseRecipe(strings.NewReader(doc), "nest.yaml")
	if err == nil || !strings.Contains(err.Error(), "line 12") || strings.Contains(err.Error(), "gate") {
		t.Fatalf("err = %v; want the unknown key reported at its original line 12", err)
	}
	ok := strings.Replace(doc, "      - id: bad\n        title: y\n        bogus: 1\n", "", 1)
	r, err := ParseRecipe(strings.NewReader(ok), "nest.yaml")
	if err != nil {
		t.Fatalf("ParseRecipe: %v", err)
	}
	if c := r.Steps[0].Steps[0].Check; c == nil || c.Contract != "c" {
		t.Errorf("nested gate not mapped: %+v", r.Steps[0].Steps[0])
	}
}

func TestParseRecipe_ErrorsUseRecipeAndGateWording(t *testing.T) {
	cases := []struct {
		name, doc string
		want      []string
		notWant   []string
	}{
		{"missing name", "version: 1\nsteps:\n  - id: a\n    title: A\n", []string{`required field "recipe"`}, []string{"flow"}},
		{"no steps", "recipe: x\nversion: 1\n", []string{"recipe x: steps: at least one step"}, []string{"flow"}},
		{
			"gate contract", "recipe: x\nversion: 1\nsteps:\n  - id: a\n    title: A\n    gate: {eva_url: u}\n",
			[]string{"step a: gate.contract is required"},
			[]string{"check"},
		},
		{
			"step named check stays", "recipe: x\nversion: 1\nsteps:\n  - id: check\n    title: A\n    kind: nope\n",
			[]string{"step check: unknown kind"},
			[]string{"step gate"},
		},
		{
			"step named flow stays", "recipe: x\nversion: 1\nsteps:\n  - id: flow\n    title: A\n    kind: nope\n",
			[]string{"step flow: unknown kind"},
			[]string{"step recipe"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRecipe(strings.NewReader(tc.doc), "x.yaml")
			if err == nil {
				t.Fatal("want an error")
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("err = %q; want it to contain %q", err, w)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(err.Error(), w) {
					t.Errorf("err = %q; must not contain %q", err, w)
				}
			}
		})
	}
}

func TestRecipeWords(t *testing.T) {
	cases := map[string]string{
		`flow x: version is required`:           `recipe x: version is required`,
		`step a: check.contract is required`:    `step a: gate.contract is required`,
		`step check: unknown kind "check"`:      `step check: unknown kind "check"`,
		`open /tmp/flow/x.yaml: no such file`:   `open /tmp/flow/x.yaml: no such file`,
		`flow.yaml: bad`:                        `flow.yaml: bad`,
		`unknown run field "x" (run.flow only)`: `unknown run field "x" (run.recipe only)`,
		`the flow has 3 steps`:                  `the recipe has 3 steps`,
		`depends on a-flow-step and check_this`: `depends on a-flow-step and check_this`,
	}
	for in, want := range cases {
		if got := recipeWords(in); got != want {
			t.Errorf("recipeWords(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestMarshalRecipe_EmitsRecipeVocabulary(t *testing.T) {
	r, err := ParseRecipe(strings.NewReader(legacyRecipeYAML), "legacy.yaml")
	if err != nil {
		t.Fatalf("ParseRecipe: %v", err)
	}
	out, err := MarshalRecipe(r)
	if err != nil {
		t.Fatalf("MarshalRecipe: %v", err)
	}
	text := string(out)
	if !strings.HasPrefix(text, "recipe: legacy\n") || !strings.Contains(text, "gate:") ||
		strings.Contains(text, "flow:") || strings.Contains(text, "check:") {
		t.Errorf("marshaled recipe not in recipe vocabulary:\n%s", text)
	}
	if _, err := ParseRecipe(strings.NewReader(text), "again.yaml"); err != nil {
		t.Errorf("marshaled recipe does not re-parse: %v", err)
	}
}

func TestDirLocator_RecipeVocabularyFilesAndWording(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.yaml")
	if err := os.WriteFile(path, []byte(legacyRecipeYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	builtin := fstest.MapFS{"shipped.yaml": {Data: []byte("recipe: shipped\nversion: 2.0.0\nsteps:\n  - id: a\n    title: A\n")}}
	loc := &DirLocator{Dirs: []string{dir}, Builtin: builtin}

	r, err := loc.Locate("legacy")
	if err != nil || r.Path != path || r.Source != dir || r.Hash != rawHash([]byte(legacyRecipeYAML)) {
		t.Fatalf("Locate legacy: %v %+v", err, r)
	}
	b, err := loc.Locate("shipped@2.0.0")
	if err != nil || b.Source != BuiltinSource || b.Hash != rawHash([]byte("recipe: shipped\nversion: 2.0.0\nsteps:\n  - id: a\n    title: A\n")) {
		t.Fatalf("Locate shipped: %v %+v", err, b)
	}
	infos, err := loc.List()
	if err != nil || len(infos) != 2 || infos[0].Path != path || infos[0].Source != dir || infos[1].Source != BuiltinSource {
		t.Fatalf("List = %+v, %v", infos, err)
	}
}

func TestDirLocator_MissingAndInvalidRefs(t *testing.T) {
	loc := &DirLocator{Dirs: []string{t.TempDir()}}
	_, err := loc.Locate("nope")
	var nf *RecipeNotFoundError
	if !errors.As(err, &nf) || nf.Ref != "nope" || !IsRecipeNotFound(err) ||
		!strings.HasPrefix(err.Error(), `recipe "nope" not found`) {
		t.Errorf("missing: %v", err)
	}
	if _, err = loc.Locate("Bad Name"); err == nil || !strings.Contains(err.Error(), "invalid recipe reference") {
		t.Errorf("bad ref: %v", err)
	}
}

func TestDirLocator_BrokenFileNamesRealPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.yaml")
	if err := os.WriteFile(path, []byte("recipe: broken\nversion: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (&DirLocator{Dirs: []string{dir}}).Locate("broken")
	if err == nil || !strings.HasPrefix(err.Error(), path+": ") || !strings.Contains(err.Error(), "recipe broken: steps") {
		t.Errorf("err = %v; want it to name %s in recipe wording", err, path)
	}
}

// TestMaterialize_ResultsReferenceIsAnEdge: a step that reads
// results.<step> without depends_on still waits for that step.
func TestMaterialize_ResultsReferenceIsAnEdge(t *testing.T) {
	doc := `recipe: implicit
version: 1
steps:
  - id: scan
    title: Scan
    kind: exec
    exec: {argv: [scan]}
  - id: report
    title: "Report {{results.scan.stdout}}"
  - id: gated
    title: Gated
    when: "results.scan.exit_code == 0"
`
	r, err := ParseRecipe(strings.NewReader(doc), "implicit.yaml")
	if err != nil {
		t.Fatalf("ParseRecipe: %v", err)
	}
	for _, steps := range [][]RecipeStep{r.Steps, FoldImplicitDeps(r.Steps, nil)} {
		f := newMatFixture()
		in := matInput(steps)
		in.Recipe = r
		in.DroppedDeps = nil
		res, err := f.m.Materialize(context.Background(), in)
		if err != nil {
			t.Fatalf("Materialize: %v", err)
		}
		ids := stepTaskIDs(res)
		assertBlockedBy(t, f.task(t, ids["scan"]))
		assertBlockedBy(t, f.task(t, ids["report"]), ids["scan"])
		assertBlockedBy(t, f.task(t, ids["gated"]), ids["scan"])
	}
}

func TestReconcile_ResultsReferenceToExistingStepIsAnEdge(t *testing.T) {
	f := newMatFixture()
	steps := []RecipeStep{
		{ID: "scan", Title: "Scan", Ordinal: 1},
		{ID: "report", Title: "Report {{results.scan.stdout}}", Ordinal: 2},
	}
	in := matInput(steps)
	in.DroppedDeps = nil
	first := in
	first.Steps = steps[:1]
	res := f.materialize(t, first)
	scanID := stepTaskIDs(res)["scan"]

	in.Existing = map[string]string{"scan": scanID}
	res2 := f.reconcile(t, in)
	assertBlockedBy(t, f.task(t, stepTaskIDs(res2)["report"]), scanID)
}

func TestConditions_CELNeedsEngine(t *testing.T) {
	_, err := EvalConditionEnv("results.a.ok && size(vars.x) > 1", CondEnv{})
	if !errors.Is(err, ErrNeedsCEL) {
		t.Errorf("err = %v; want ErrNeedsCEL", err)
	}
	ok, err := EvalConditionEnv("results.a.exit_code == 0", CondEnv{
		Results: map[string]map[string]any{"a": {"exit_code": float64(0)}},
	})
	if err != nil || !ok {
		t.Errorf("single comparison = %v, %v; want true", ok, err)
	}
}

// TestRecipeRunScope_BindsEveryRunFieldValidationAccepts: run.started_at
// validates, so rendering must bind it instead of erroring.
func TestRecipeRunScope_BindsEveryRunFieldValidationAccepts(t *testing.T) {
	doc := "recipe: r\nversion: 1\nsteps:\n  - id: a\n" +
		"    title: \"{{run.id}} {{run.recipe}} {{run.version}} {{run.started_at}}\"\n"
	r, err := ParseRecipe(strings.NewReader(doc), "r.yaml")
	if err != nil {
		t.Fatalf("ParseRecipe: %v", err)
	}
	started := time.Date(2026, 11, 18, 9, 30, 0, 0, time.FixedZone("x", 3600))
	steps, err := RenderSteps(r.Steps, Scope{Run: RecipeRunScope("run_1", r.Name, r.Version, started)})
	if err != nil {
		t.Fatalf("RenderSteps: %v", err)
	}
	if got, want := steps[0].Title, "run_1 r 1 2026-11-18T08:30:00Z"; got != want {
		t.Errorf("title = %q; want %q", got, want)
	}
}
