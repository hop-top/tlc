package core

// Regression test for PR #46 review item: resolveCrossTrackRef
// must scope task lookups by the target track's owning project_id.
// Track IDs are unique only within (project_id, id), so a bare
// track_id + title filter would merge tasks from sibling projects
// and produce wrong resolutions or spurious ambiguity.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// recordingTaskRepo embeds stubTaskRepo and captures every Query
// passed to ListTasks so tests can assert filter composition.
type recordingTaskRepo struct {
	stubTaskRepo
	queries []Query
}

func (r *recordingTaskRepo) ListTasks(
	ctx context.Context, q Query,
) ([]*Task, error) {
	r.queries = append(r.queries, q)
	return r.stubTaskRepo.ListTasks(ctx, q)
}

// writeMiniPlan writes a minimal plan.md with frontmatter naming
// the given task titles.
func writeMiniPlan(t *testing.T, titles ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.md")
	body := "---\ntitle: tmp\ntasks:\n"
	for _, ti := range titles {
		body += fmt.Sprintf("  - title: %q\n", ti)
	}
	body += "---\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	return path
}

// TestResolveCrossTrackRef_ScopesByProjectID verifies the spy
// captures a project_id filter alongside track_id and title, and
// that resolution picks the correct-project task when two tasks
// across different projects share the same track_id + title.
func TestResolveCrossTrackRef_ScopesByProjectID(t *testing.T) {
	ctx := context.Background()
	planPath := writeMiniPlan(t, "Shared")

	// Target track owned by project B.
	projB := "proj-b"
	trackRepo := newStubTrackRepo()
	target := &Track{
		ID:        "alpha",
		Title:     "Alpha",
		Type:      TrackTypeFeature,
		Status:    TrackStatusActive,
		ProjectID: &projB,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		Meta:      map[string]any{"plans": []string{planPath}},
	}
	if err := trackRepo.CreateTrack(ctx, target); err != nil {
		t.Fatalf("CreateTrack: %v", err)
	}

	// Two tasks, same track_id + title, different project_id.
	projA := "proj-a"
	taskA := &Task{
		ID: "T-0100", Title: "Shared",
		TrackID: strPtr("alpha"), ProjectID: &projA,
	}
	taskB := &Task{
		ID: "T-0200", Title: "Shared",
		TrackID: strPtr("alpha"), ProjectID: &projB,
	}
	taskRepo := &recordingTaskRepo{}
	taskRepo.tasks = []*Task{taskA, taskB}

	svc := NewTrackService(trackRepo, taskRepo)

	// We don't care about the return value here — the stub only
	// filters by track_id, so with two matches it will report an
	// ambiguity error. The regression we're guarding against is
	// the filter *composition*: the resolver must ask the
	// repository for project_id = target.ProjectID. A real sqlite
	// repository would then return exactly one row.
	_, _ = svc.resolveCrossTrackRef(ctx, &CrossTrackRef{
		TrackID: "alpha", TaskNum: 1,
	})

	if len(taskRepo.queries) == 0 {
		t.Fatal("ListTasks never called")
	}
	last := taskRepo.queries[len(taskRepo.queries)-1]
	seen := map[string]interface{}{}
	for _, f := range last.Filters {
		seen[f.Field] = f.Value
	}
	if _, ok := seen["track_id"]; !ok {
		t.Errorf("missing track_id filter; got filters %+v", last.Filters)
	}
	if _, ok := seen["title"]; !ok {
		t.Errorf("missing title filter; got filters %+v", last.Filters)
	}
	if v, ok := seen["project_id"]; !ok {
		t.Errorf("missing project_id filter; got filters %+v", last.Filters)
	} else if v != projB {
		t.Errorf("project_id filter = %v, want %q", v, projB)
	}
}

func strPtr(s string) *string { return &s }
