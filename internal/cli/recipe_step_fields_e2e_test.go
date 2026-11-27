package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// recipeFieldsFixture carries every tlc-owned key: track meta, and per
// step tags, meta and priority, one step inside a repeat block, one
// templated, and one a create-time when: leaves out.
const recipeFieldsFixture = `recipe: fields
version: 1.0.0
vars:
  area: {default: core}
  level: {default: P1}
track:
  title: "Fields {{area}}"
  type: feature
  meta:
    owner: "{{area}}-team"
    budget: 5
steps:
  - id: plan
    title: Plan
    tags: [planning, "area:{{area}}"]
    meta: {ticket: "{{area}}-1", weight: 2}
    priority: "{{level}}"
  - id: rounds
    repeat: 2
    depends_on: [plan]
    steps:
      - id: review
        tags: [review]
        meta: {reviewer: "{{area}}-lead"}
  - id: never
    when: "vars.area == 'never'"
    tags: [dropped]
    meta: {dropped: yes}
`

func TestTrackCreateRecipe_E2E_StepFieldsAndTrackMeta(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "fields.yaml", recipeFieldsFixture)

		out, err := runRecipeCLI(t, "track", "create", "--recipe", "fields", "--var", "area=api", "--var", "level=P0")
		if err != nil {
			t.Fatalf("track create --recipe: %v\n%s", err, out)
		}
		s := openTestStorage(t)
		track := mustTrack(t, ctx, s, "fields-api")
		if track.Meta["owner"] != "api-team" || track.Meta["budget"] != float64(5) && track.Meta["budget"] != 5 {
			t.Errorf("track meta = %v; want the recipe's rendered meta", track.Meta)
		}
		tasks := tasksOfTrack(t, ctx, s, track.ID)
		var plan *core.Task
		reviews := 0
		for _, task := range tasks {
			switch {
			case task.StepID == "plan":
				plan = task
			case strings.HasSuffix(task.StepID, "review"):
				reviews++
				if len(task.Tags) != 1 || task.Tags[0] != "review" || task.Meta["reviewer"] != "api-lead" {
					t.Errorf("%s tags=%v meta=%v", task.StepID, task.Tags, task.Meta)
				}
			default:
				t.Errorf("unexpected task %s (%v)", task.StepID, task.Tags)
			}
		}
		if reviews != 2 || plan == nil {
			t.Fatalf("tasks = %d, plan = %v; want plan and 2 unrolled reviews", len(tasks), plan)
		}
		if strings.Join(plan.Tags, ",") != "planning,area:api" || plan.Priority != core.PriorityP0 {
			t.Errorf("plan tags=%v priority=%q", plan.Tags, plan.Priority)
		}
		if plan.Meta["ticket"] != "api-1" {
			t.Errorf("plan meta = %v", plan.Meta)
		}
		if core.PrioritySource(plan) != core.PrioritySourceManual || plan.Meta[core.MetaPrioritySource] != core.PrioritySourceManual {
			t.Errorf("priority source = %v; want the create gate's manual marker", plan.Meta[core.MetaPrioritySource])
		}
	})
}

func TestTrackCreateRecipe_E2E_StepFieldsDryRun(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "fields.yaml", recipeFieldsFixture)

		out, err := runRecipeCLI(t, "track", "create", "--recipe", "fields", "--dry-run")
		if err != nil {
			t.Fatalf("dry run: %v\n%s", err, out)
		}
		assertContains(t, out, "Dry run", "created 3 task(s)")
		if n := len(allTasks(t, ctx, openTestStorage(t))); n != 0 {
			t.Errorf("dry run left %d tasks", n)
		}
	})
}

func TestRecipeFields_E2E_ReservedKeysRefused(t *testing.T) {
	cases := map[string]string{
		"step meta":  "recipe: bad\nversion: 1.0.0\nsteps:\n  - id: a\n    meta: {blocked_by: [x]}\n",
		"track meta": "recipe: bad\nversion: 1.0.0\ntrack: {title: t, meta: {plans: x}}\nsteps:\n  - id: a\n",
		"x-tlc":      "recipe: bad\nversion: 1.0.0\nx-tlc: {track: {title: t}}\nsteps:\n  - id: a\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			viper.Reset()
			defer viper.Reset()
			p := writeRecipeFixture(t, t.TempDir(), "bad.yaml", body)
			out, err := runRecipeCmd(t, "validate", p)
			if err == nil || !strings.Contains(out, "Validation failed") {
				t.Fatalf("validate err=%v out=%q; want a failure", err, out)
			}
		})
	}
	for name, body := range cases {
		t.Run("create "+name, func(t *testing.T) {
			withTestLock(func() {
				ctx, cleanup := setupTestDir(t)
				defer cleanup()
				plantRecipe(t, "bad.yaml", body)
				if _, err := runRecipeCLI(t, "track", "create", "Bad", "--recipe", "bad"); err == nil {
					t.Fatal("track create --recipe succeeded; want the reserved key refused")
				}
				s := openTestStorage(t)
				tracks, err := s.ListTracks(ctx, core.TrackQuery{AllProjects: true})
				if err != nil || len(tracks) != 0 || len(allTasks(t, ctx, s)) != 0 {
					t.Errorf("tracks=%d tasks=%d err=%v; want nothing created", len(tracks), len(allTasks(t, ctx, s)), err)
				}
			})
		})
	}
}

func TestRecipeFields_E2E_OtherExtensionPassesThrough(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	p := writeRecipeFixture(t, t.TempDir(), "ok.yaml", "recipe: ok\nversion: 1.0.0\nx-team: {owner: me}\nsteps:\n  - id: a\n    x-note: hi\n")
	if out, err := runRecipeCmd(t, "validate", p); err != nil {
		t.Fatalf("validate: %v\n%s", err, out)
	}
}

func TestRecipeShow_E2E_PlainKeysNoExtension(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	p := writeRecipeFixture(t, t.TempDir(), "fields.yaml", recipeFieldsFixture)

	out, err := runRecipeJSON(t, "show", p)
	if err != nil {
		t.Fatalf("show: %v\n%s", err, out)
	}
	if strings.Contains(out, "x-tlc") {
		t.Fatalf("x-tlc in show output:\n%s", out)
	}
	var view struct {
		Track struct {
			Title string         `json:"title"`
			Meta  map[string]any `json:"meta"`
		} `json:"track"`
		Steps []struct {
			ID       string         `json:"id"`
			Tags     []string       `json:"tags"`
			Meta     map[string]any `json:"meta"`
			Priority string         `json:"priority"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if view.Track.Meta["owner"] != "{{area}}-team" || len(view.Steps) == 0 {
		t.Fatalf("view = %+v", view)
	}
	first := view.Steps[0]
	if first.ID != "plan" || first.Priority != "{{level}}" || len(first.Tags) != 2 || first.Meta["ticket"] != "{{area}}-1" {
		t.Errorf("plan step = %+v", first)
	}
	text, err := runRecipeCmd(t, "show", p)
	if err != nil || strings.Contains(text, "x-tlc") {
		t.Errorf("table show err=%v:\n%s", err, text)
	}
}

func TestServe_TrackCreateFromRecipe_StepFieldsAndTrackMeta(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "fields.yaml", recipeFieldsFixture)
		router := newTestServeRouter(t)

		code, got, body := postRecipeTrack(t, router, `{"recipe":"fields","vars":{"area":"web","level":"P2"}}`)
		if code != http.StatusCreated {
			t.Fatalf("POST /tracks = %d %s", code, body)
		}
		if got.Meta["owner"] != "web-team" {
			t.Errorf("response meta = %v", got.Meta)
		}
		s := openTestStorage(t)
		if track := mustTrack(t, ctx, s, got.ID); track.Meta["owner"] != "web-team" {
			t.Errorf("stored meta = %v", track.Meta)
		}
		byStep := tasksByStep(tasksOfTrack(t, ctx, s, got.ID))
		plan := byStep["plan"]
		if plan == nil || plan.Priority != core.PriorityP2 || strings.Join(plan.Tags, ",") != "planning,area:web" || plan.Meta["ticket"] != "web-1" {
			t.Errorf("plan = %+v", plan)
		}

		rec := doServeRequestRaw(t, router, "POST", "/tracks", `{"recipe":"fields","meta":{"a":"b"}}`)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("recipe + meta = %d; want 422", rec.Code)
		}
	})
}

func TestServe_TrackCreateFromRecipe_ReservedMetaCreatesNothing(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "bad.yaml", "recipe: bad\nversion: 1.0.0\ntrack: {title: t, type: feature, meta: {plans: x}}\nsteps:\n  - id: a\n")
		router := newTestServeRouter(t)
		rec := doServeRequestRaw(t, router, "POST", "/tracks", `{"recipe":"bad"}`)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "plans") {
			t.Fatalf("status = %d %s; want 422 naming the key", rec.Code, rec.Body.String())
		}
		s := openTestStorage(t)
		if tracks, _ := s.ListTracks(ctx, core.TrackQuery{AllProjects: true}); len(tracks) != 0 {
			t.Errorf("tracks = %d; want none", len(tracks))
		}
	})
}
