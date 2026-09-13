package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// recipeFixFlowFixture requires a task subject and reads subject.* into
// its titles, so the binding is observable on the created tasks.
const recipeFixFlowFixture = `recipe: fix-flow
version: 1.0.0
requires: {subject: task}
steps:
  - id: repro
    title: "Reproduce {{subject.id}}: {{subject.title}}"
  - id: fix
    title: "Fix {{subject.id}}"
    depends_on: [repro]
  - id: verify
    title: "Verify {{subject.id}} in {{subject.track}}"
    depends_on: [fix]
`

// seedSubjectTask stores a plain task, optionally in a track, and returns
// it with its allocated sequence so the T-NNNN alias is known.
func seedSubjectTask(t *testing.T, ctx context.Context, s *storage.SQLiteStorage, title, trackID string) *core.Task {
	t.Helper()
	now := time.Now().UTC()
	task := &core.Task{ID: core.NewTaskID(), Title: title, Status: core.StatusTodo, CreatedAt: now, UpdatedAt: now}
	if trackID != "" {
		task.TrackID = &trackID
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask %q: %v", title, err)
	}
	return mustGetTask(t, ctx, s, task.ID)
}

func TestTaskCreateRecipe_E2E_ForSubject(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "fix-flow.yaml", recipeFixFlowFixture)
		s := openTestStorage(t)
		trackID := seedTrack(t, ctx, s, s, &core.Track{Slug: "auth", Title: "Auth", Type: core.TrackTypeFeature})
		subject := seedSubjectTask(t, ctx, s, "Login breaks", trackID)
		alias := formatTaskAlias(subject)

		out, err := runRecipeCLI(t, "task", "create", "--recipe", "fix-flow", "--for", alias)
		if err != nil {
			t.Fatalf("task create --for: %v\n%s", err, out)
		}
		assertContains(t, out, "Materialized fix-flow@1.0.0 for "+alias+" into L-", "created 3 task(s)")

		tasks := tasksOfTrack(t, ctx, s, trackID)
		if len(tasks) != 4 { // subject + 3 steps
			t.Fatalf("track has %d tasks; want the subject plus 3", len(tasks))
		}
		byStep := tasksByStep(tasks)
		if got := byStep["repro"].Title; got != "Reproduce "+alias+": Login breaks" {
			t.Errorf("repro title = %q; want subject.id and subject.title rendered", got)
		}
		if got := byStep["verify"].Title; got != "Verify "+alias+" in auth" {
			t.Errorf("verify title = %q; want subject.track rendered", got)
		}
		if prov := byStep["fix"].Provenance(); prov.Subject != subject.ID {
			t.Errorf("provenance subject = %q; want %s", prov.Subject, subject.ID)
		}
		blocked := mustGetTask(t, ctx, s, subject.ID).BlockedBy()
		if len(blocked) != 1 || blocked[0] != byStep["verify"].ID {
			t.Errorf("subject blocked_by = %v; want the run's leaf %s", blocked, byStep["verify"].ID)
		}
		runs := recipeRunsOf(t, ctx, s, "fix-flow", trackID)
		if len(runs) != 1 || runs[0].SubjectType != core.SubjectTask || runs[0].SubjectID != subject.ID {
			t.Errorf("run = %+v; want subject task %s", runs[0], subject.ID)
		}
	})
}

func TestTaskCreateRecipe_E2E_IntoTrack(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "review.yaml", recipeReviewFixture)
		s := openTestStorage(t)
		trackID := seedTrack(t, ctx, s, s, &core.Track{Slug: "auth", Title: "Auth", Type: core.TrackTypeFeature})

		out, err := runRecipeCLI(t, "task", "create", "--recipe", "review", "--var", "pr=5", "--track", "auth")
		if err != nil {
			t.Fatalf("task create --track: %v\n%s", err, out)
		}
		assertContains(t, out, "Materialized review@1.0.0 into L-", "created 3 task(s)")
		if got := len(tasksOfTrack(t, ctx, s, trackID)); got != 3 {
			t.Errorf("track has %d tasks; want 3", got)
		}
		runs := recipeRunsOf(t, ctx, s, "review", trackID)
		if len(runs) != 1 || runs[0].SubjectType != "" || runs[0].TrackID != trackID {
			t.Errorf("run = %+v; want no subject and the track", runs[0])
		}
	})
}

func TestTaskCreateRecipe_E2E_Standalone(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "review.yaml", recipeReviewFixture)

		out, err := runRecipeCLI(t, "task", "create", "--recipe", "review", "--var", "pr=5")
		if err != nil {
			t.Fatalf("task create --recipe: %v\n%s", err, out)
		}
		assertContains(t, out, "Materialized review@1.0.0: created 3 task(s)")
		s := openTestStorage(t)
		tasks := allTasks(t, ctx, s)
		if len(tasks) != 3 {
			t.Fatalf("%d tasks; want 3", len(tasks))
		}
		for _, task := range tasks {
			if task.TrackID != nil || task.RunID == "" {
				t.Errorf("%s: track %v run %q; want trackless with a run", task.StepID, task.TrackID, task.RunID)
			}
		}
		runs := recipeRunsOf(t, ctx, s, "review", "")
		if len(runs) != 1 || runs[0].TrackID != "" {
			t.Errorf("runs = %+v; want one trackless run", runs)
		}

		// A title argument has no meaning next to --recipe.
		if _, err = runRecipeCLI(t, "task", "create", "Extra", "--recipe", "review", "--var", "pr=5"); err == nil || !strings.Contains(err.Error(), "title") {
			t.Errorf("err = %v; want a rejection of the title argument", err)
		}
	})
}

func TestTaskCreateRecipe_E2E_RequiresSubjectEnforced(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "fix-flow.yaml", recipeFixFlowFixture)
		s := openTestStorage(t)
		seedTrack(t, ctx, s, s, &core.Track{Slug: "auth", Title: "Auth", Type: core.TrackTypeFeature})

		_, err := runRecipeCLI(t, "task", "create", "--recipe", "fix-flow")
		if err == nil || !strings.Contains(err.Error(), "requires a task subject") || !strings.Contains(err.Error(), "--for") {
			t.Errorf("no subject: err = %v; want the requirement and the --for fix", err)
		}
		_, err = runRecipeCLI(t, "task", "create", "--recipe", "fix-flow", "--for", "auth")
		if err == nil || !strings.Contains(err.Error(), "is a track") {
			t.Errorf("track subject: err = %v; want 'is a track'", err)
		}
		_, err = runRecipeCLI(t, "task", "create", "--recipe", "fix-flow", "--for", "T-0099")
		if err == nil || !strings.Contains(err.Error(), "T-0099") || !strings.Contains(err.Error(), "not found") {
			t.Errorf("missing subject: err = %v; want not found", err)
		}
		if got := len(allTasks(t, ctx, s)); got != 0 {
			t.Errorf("%d tasks written by rejected invocations", got)
		}
	})
}
