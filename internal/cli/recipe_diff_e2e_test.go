package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

const diffRecipeV1 = `recipe: release
version: 1.0.0
steps:
  - id: plan
    title: Plan
    description: p
  - id: build
    kind: exec
    exec: {argv: [make]}
    depends_on: [plan]
  - id: old
    title: Old
    depends_on: [plan]
`

const diffRecipeV2 = `recipe: release
version: 1.1.0
steps:
  - id: plan
    title: Plan
    description: p
  - id: build
    kind: exec
    exec: {argv: [make]}
    depends_on: [plan]
  - id: docs
    title: Docs
    depends_on: [plan]
`

// diffRow returns the table line for a step id.
func diffRow(t *testing.T, out, step string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 1 && fields[1] == step {
			return line
		}
	}
	t.Fatalf("no row for step %q in:\n%s", step, out)
	return ""
}

func TestRecipeDiff_E2E_States(t *testing.T) {
	withTestLock(func() {
		tmp := filepath.Dir(resetTestDB(t))
		viper.Set("recipe.dir", "recipes")
		path := writeRecipeFixture(t, filepath.Join(tmp, "recipes"), "release.yaml", diffRecipeV1)
		loc := recipeLocator()
		r, err := loc.Locate("release")
		if err != nil {
			t.Fatalf("locate: %v", err)
		}
		steps, err := core.Expand(r, loc)
		if err != nil {
			t.Fatalf("expand: %v", err)
		}
		ctx := context.Background()
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer func() { _ = s.Close() }()
		_, res := seedMaterializedTrack(t, ctx, s, r, steps, nil)
		taskOf := map[string]string{}
		for _, c := range res.Created {
			taskOf[c.StepID] = c.TaskID
		}
		if err := s.DeleteTask(ctx, taskOf["build"]); err != nil {
			t.Fatalf("delete build: %v", err)
		}
		if err := os.WriteFile(path, []byte(diffRecipeV2), 0o644); err != nil {
			t.Fatalf("rewrite recipe: %v", err)
		}

		out, err := runRecipeCmd(t, "diff", e2eRecipeTrackSlug)
		if err != nil {
			t.Fatalf("recipe diff: %v\n%s", err, out)
		}
		for _, want := range []string{"release@1.1.0", res.RunID, "drift", "1.0.0", "STEP", "KIND", "TASK", "STATUS", "STATE"} {
			if !contains(out, want) {
				t.Errorf("output lacks %q:\n%s", want, out)
			}
		}
		if row := diffRow(t, out, "plan"); !contains(row, "materialized") || !contains(row, "T-0001") || !contains(row, "TODO") {
			t.Errorf("plan row = %q; want materialized with alias and status", row)
		}
		if row := diffRow(t, out, "build"); !contains(row, "deleted") || !contains(row, "exec") {
			t.Errorf("build row = %q; want deleted", row)
		}
		if row := diffRow(t, out, "docs"); !contains(row, "missing") {
			t.Errorf("docs row = %q; want missing", row)
		}
		if row := diffRow(t, out, "old"); !contains(row, "extra") || !contains(row, "T-0003") {
			t.Errorf("old row = %q; want extra with its task", row)
		}

		out, err = runRecipeJSON(t, "diff", e2eRecipeTrackSlug)
		if err != nil {
			t.Fatalf("recipe diff (json): %v\n%s", err, out)
		}
		var view struct {
			Recipe     string   `json:"recipe"`
			Version    string   `json:"version"`
			Run        string   `json:"run"`
			RunVersion string   `json:"run_version"`
			Drift      []string `json:"drift"`
			Steps      []struct {
				Step  string `json:"step"`
				State string `json:"state"`
				Task  string `json:"task"`
			} `json:"steps"`
		}
		if err := json.Unmarshal([]byte(out), &view); err != nil {
			t.Fatalf("json: %v\n%s", err, out)
		}
		if view.Recipe != "release" || view.Version != "1.1.0" || view.Run != res.RunID || view.RunVersion != "1.0.0" || len(view.Drift) != 2 {
			t.Errorf("json header = %+v", view)
		}
		states := map[string]string{}
		for _, row := range view.Steps {
			states[row.Step] = row.State
		}
		want := map[string]string{"plan": "materialized", "build": "deleted", "docs": "missing", "old": "extra"}
		for step, state := range want {
			if states[step] != state {
				t.Errorf("json state[%s] = %q; want %q (all: %v)", step, states[step], state, states)
			}
		}

		// --recipe accepts a path outside the search path.
		out, err = runRecipeCmd(t, "diff", e2eRecipeTrackSlug, "--recipe", path)
		if err != nil || !contains(diffRow(t, out, "docs"), "missing") {
			t.Errorf("--recipe path: err = %v\n%s", err, out)
		}

		// Without the file the run's recipe name cannot be resolved.
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove: %v", err)
		}
		_, err = runRecipeCmd(t, "diff", e2eRecipeTrackSlug)
		if err == nil || !contains(err.Error(), "--recipe") {
			t.Errorf("missing file: err = %v; want a --recipe hint", err)
		}

		// A track without runs has nothing to compare.
		seedTrack(t, ctx, s, s, &core.Track{Slug: "empty-track", Title: "Empty", Type: "feature"})
		_, err = runRecipeCmd(t, "diff", "empty-track")
		if err == nil || !contains(err.Error(), "no recipe runs") {
			t.Errorf("empty track: err = %v", err)
		}
	})
}
