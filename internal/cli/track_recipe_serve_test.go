package cli

// HTTP coverage for POST /tracks with {recipe, vars}: the track is built
// and materialized through the same code the CLI's `track create --recipe`
// runs, resolved by name through the recipe search path only.

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

type recipeTrackResponse struct {
	core.Track
	RecipeRun *struct {
		Recipe   string   `json:"recipe"`
		Version  string   `json:"version"`
		RunID    string   `json:"run_id"`
		Created  []any    `json:"created"`
		Warnings []string `json:"warnings"`
	} `json:"recipe_run"`
}

func postRecipeTrack(t *testing.T, router http.Handler, body string) (int, recipeTrackResponse, string) {
	t.Helper()
	rec := doServeRequestRaw(t, router, "POST", "/tracks", body)
	var out recipeTrackResponse
	if rec.Code == http.StatusCreated {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v\n%s", err, rec.Body.String())
		}
	}
	return rec.Code, out, rec.Body.String()
}

func TestServe_TrackCreateFromRecipe(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "review.yaml", recipeReviewFixture)
		router := newTestServeRouter(t)

		code, got, body := postRecipeTrack(t, router, `{"recipe":"review","vars":{"pr":12,"depth":"deep"}}`)
		if code != http.StatusCreated {
			t.Fatalf("POST /tracks = %d %s", code, body)
		}
		if got.Title != "Review PR 12" || got.Type != core.TrackTypeFeature || got.Slug != "review-pr-12" {
			t.Errorf("track = %q / %q / %q; want the recipe's rendered title and type", got.Title, got.Type, got.Slug)
		}
		if got.RecipeRun == nil || got.RecipeRun.Recipe != "review" || len(got.RecipeRun.Created) != 3 || got.RecipeRun.RunID == "" {
			t.Fatalf("recipe_run = %+v; want review with 3 created steps", got.RecipeRun)
		}

		s := openTestStorage(t)
		byStep := tasksByStep(tasksOfTrack(t, ctx, s, got.ID))
		if len(byStep) != 3 {
			t.Fatalf("steps = %v; want 3 tasks in the track", byStep)
		}
		if title := byStep["review"].Title; title != "Review 12 (deep)" {
			t.Errorf("review title = %q; want vars rendered", title)
		}
		if prov := byStep["review"].Provenance(); prov.Recipe != "review" || byStep["review"].RunID != got.RecipeRun.RunID {
			t.Errorf("provenance = %+v run %q", prov, byStep["review"].RunID)
		}
		runs := recipeRunsOf(t, ctx, s, "review", got.ID)
		if len(runs) != 1 || runs[0].Vars["pr"] != "12" {
			t.Errorf("runs = %+v; want one run with pr=12", runs)
		}
	})
}

func TestServe_TrackCreateFromRecipe_TitleSlugAndPin(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "review.yaml", recipeReviewFixture)
		router := newTestServeRouter(t)

		code, got, body := postRecipeTrack(t, router,
			`{"recipe":"review@1.0.0","vars":{"pr":"a,b=c"},"title":"Mine","slug":"my-review","type":"fix"}`)
		if code != http.StatusCreated {
			t.Fatalf("POST /tracks = %d %s", code, body)
		}
		if got.Title != "Mine" || got.Slug != "my-review" || got.Type != "fix" {
			t.Errorf("track = %q / %q / %q; want the request's title, slug and type", got.Title, got.Slug, got.Type)
		}
	})
}

func TestServe_TrackCreateFromRecipe_ClientErrors(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "review.yaml", recipeReviewFixture)
		if err := os.WriteFile("stray", []byte(recipeReviewFixture), 0o644); err != nil {
			t.Fatalf("write stray: %v", err)
		}
		router := newTestServeRouter(t)

		cases := []struct {
			name, body, want string
		}{
			{"unknown recipe", `{"recipe":"nope","vars":{"pr":1}}`, "nope"},
			{"unknown pinned version", `{"recipe":"review@9.9.9","vars":{"pr":1}}`, "review"},
			{"missing required var", `{"recipe":"review"}`, "pr"},
			{"recipe with meta", `{"recipe":"review","vars":{"pr":1},"meta":{"a":"b"}}`, "meta"},
			{"path with separator", `{"recipe":"recipes/review.yaml","vars":{"pr":1}}`, "not a path"},
			{"absolute path", `{"recipe":"` + os.Getenv("PWD") + `/recipes/review.yaml","vars":{"pr":1}}`, "not a path"},
			{"file extension", `{"recipe":"review.yaml","vars":{"pr":1}}`, "not a path"},
			{"file next to cwd", `{"recipe":"stray","vars":{"pr":1}}`, "stray"},
			{"vars without recipe", `{"slug":"x","title":"x","type":"feature","vars":{"pr":1}}`, "recipe"},
			{"bad slug", `{"recipe":"review","vars":{"pr":1},"slug":"Not A Slug"}`, "slug"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rec := doServeRequestRaw(t, router, "POST", "/tracks", tc.body)
				if rec.Code != http.StatusUnprocessableEntity {
					t.Fatalf("status = %d %s; want 422", rec.Code, rec.Body.String())
				}
				var shape struct {
					Status  int    `json:"status"`
					Code    string `json:"code"`
					Message string `json:"message"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &shape); err != nil || shape.Code == "" || !strings.Contains(shape.Message, tc.want) {
					t.Errorf("body = %s; want the api error shape naming %q", rec.Body.String(), tc.want)
				}
			})
		}
		s := openTestStorage(t)
		if n := len(allTasks(t, ctx, s)); n != 0 {
			t.Errorf("rejected requests left %d tasks", n)
		}
	})
}
