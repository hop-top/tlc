package cli

import (
	"context"
	"encoding/json"
	"testing"

	"hop.top/tlc/internal/core"
)

func TestRecipeRuns_E2E_TableFiltersAndJSON(t *testing.T) {
	withTestLock(func() {
		resetTestDB(t)
		ctx := context.Background()
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer func() { _ = s.Close() }()
		release := &core.Recipe{Name: "release", Version: "1.0.0", Hash: "sha256:r"}
		track, first := seedMaterializedTrack(t, ctx, s, release, e2eRecipeSteps(), map[string]any{"pr": "1234"})
		hotfix := &core.Recipe{Name: "hotfix", Version: "0.2.0", Hash: "sha256:h"}
		second := materializeInto(t, ctx, s, track, hotfix, []core.RecipeStep{
			{ID: "patch", Title: "Patch", Description: "p", Ordinal: 1},
		}, nil)

		out, err := runRecipeCmd(t, "runs")
		if err != nil {
			t.Fatalf("recipe runs: %v\n%s", err, out)
		}
		for _, want := range []string{
			"RUN", "RECIPE", "VERSION", "TRACK", "SUBJECT", "TASKS", "CREATED", "BY",
			first.RunID, second.RunID, "release", "1.0.0", "hotfix", "0.2.0", e2eRecipeTrackSlug, "test",
		} {
			if !contains(out, want) {
				t.Errorf("table lacks %q:\n%s", want, out)
			}
		}

		out, err = runRecipeCmd(t, "runs", "release")
		if err != nil || !contains(out, first.RunID) || contains(out, second.RunID) {
			t.Errorf("by recipe: err = %v\n%s", err, out)
		}
		out, err = runRecipeCmd(t, "runs", "release@9.9.9")
		if err != nil || !contains(out, "No recipe runs") {
			t.Errorf("pinned version with no run: err = %v\n%s", err, out)
		}
		out, err = runRecipeCmd(t, "runs", "--track", e2eRecipeTrackSlug, "--all-projects")
		if err != nil || !contains(out, first.RunID) || !contains(out, second.RunID) {
			t.Errorf("by track: err = %v\n%s", err, out)
		}
		if _, err := runRecipeCmd(t, "runs", "--track", "nope-track"); err == nil {
			t.Error("unknown --track: expected an error")
		}

		out, err = runRecipeJSON(t, "runs")
		if err != nil {
			t.Fatalf("recipe runs (json): %v\n%s", err, out)
		}
		var rows []struct {
			ID      string `json:"id"`
			Recipe  string `json:"recipe_id"`
			TrackID string `json:"track_id"`
			Tasks   int    `json:"tasks"`
		}
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatalf("json: %v\n%s", err, out)
		}
		if len(rows) != 2 || rows[0].ID != second.RunID || rows[0].Tasks != 1 || rows[1].ID != first.RunID || rows[1].Tasks != 3 {
			t.Errorf("json rows = %+v; want newest first with task counts", rows)
		}
		if rows[0].TrackID != track.ID {
			t.Errorf("json track_id = %q; want %q", rows[0].TrackID, track.ID)
		}
	})
}
