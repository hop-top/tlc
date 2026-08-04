package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"hop.top/tlc/internal/core"
)

// chdirForTest moves the process into dir for the duration of the test so
// core.DetectProject reports InProject=false (no .tlc/config.yaml above dir),
// exercising the storage layer's out-of-project scoping rules.
func chdirForTest(t *testing.T, dir string) {
	t.Helper()
	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })
}

// TestGetTaskAndListTasksAgreeOutsideProject pins the read-path invariant
// that GetTask and ListTasks select the same row when no project context is
// active.
//
// Two rows may legitimately share a T-NNNN alias because the seq counter is
// per project bucket and the schema only enforces UNIQUE (project_id, seq).
// Outside a project, GetTask prefers the global bucket and stops at LIMIT 1,
// while ListTasks returns every bucket unfiltered. A status write must land on
// the same row the list view renders under that alias, or the list keeps
// showing a stale status for a task that show reports as closed.
func TestGetTaskAndListTasksAgreeOutsideProject(t *testing.T) {
	resetProjectDetection()
	defer resetProjectDetection()

	ctx := context.Background()
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	s, err := NewSQLiteStorage(filepath.Join(tempDir, "scope.db"))
	require.NoError(t, err)
	defer s.Close()

	now := time.Now().UTC()
	globalBucket := ""
	projectBucket := "acme/widgets"

	globalTask := &core.Task{
		ID:        "task_scope_global_row_0001",
		Seq:       1,
		ProjectID: &globalBucket,
		Title:     "global bucket row",
		Status:    core.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
		Meta:      map[string]any{},
	}
	projectTask := &core.Task{
		ID:        "task_scope_project_row_0001",
		Seq:       1,
		ProjectID: &projectBucket,
		Title:     "project bucket row",
		Status:    core.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
		Meta:      map[string]any{},
	}
	require.NoError(t, s.CreateTask(ctx, globalTask))
	require.NoError(t, s.CreateTask(ctx, projectTask))

	// The write path resolves the task the same way `task show` does.
	resolved, err := s.GetTask(ctx, globalTask.ID)
	require.NoError(t, err)
	require.NotNil(t, resolved)

	entry, err := resolved.Transition(core.StatusInProgress, "tester", "claiming")
	require.NoError(t, err)
	require.NoError(t, s.UpdateTaskWithLog(ctx, resolved, entry))

	entry, err = resolved.Transition(core.StatusDone, "tester", "closing")
	require.NoError(t, err)
	require.NoError(t, s.UpdateTaskWithLog(ctx, resolved, entry))

	// Re-reading through the show path reports the new status.
	after, err := s.GetTask(ctx, globalTask.ID)
	require.NoError(t, err)
	require.Equal(t, core.StatusDone, after.Status)

	// The list path must report the same status for that same row.
	listed, err := s.ListTasks(ctx, core.Query{})
	require.NoError(t, err)

	byID := make(map[string]*core.Task, len(listed))
	for _, task := range listed {
		byID[task.ID] = task
	}

	row, ok := byID[globalTask.ID]
	require.True(t, ok, "list view dropped the row the write path updated")
	require.Equal(
		t, core.StatusDone, row.Status,
		"list view reports a stale status for the row GetTask resolved and updated",
	)

	// The sibling row sharing the alias must be untouched, and must not be
	// what a caller resolving that alias sees.
	sibling, ok := byID[projectTask.ID]
	require.True(t, ok)
	require.Equal(t, core.StatusTodo, sibling.Status)
}

// TestGetTaskBySeqIsScopedToRequestedProject pins that alias resolution stays
// inside the bucket the caller asked for. GetTaskBySeq located the row by
// (project_id, seq) and then re-resolved it through GetTask by bare id, which
// re-applied its own scoping rules and could return a different bucket's row.
func TestGetTaskBySeqIsScopedToRequestedProject(t *testing.T) {
	resetProjectDetection()
	defer resetProjectDetection()

	ctx := context.Background()
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	s, err := NewSQLiteStorage(filepath.Join(tempDir, "seq.db"))
	require.NoError(t, err)
	defer s.Close()

	now := time.Now().UTC()
	globalBucket := ""
	projectBucket := "acme/widgets"

	require.NoError(t, s.CreateTask(ctx, &core.Task{
		ID:        "task_seq_global_row_00000001",
		Seq:       1,
		ProjectID: &globalBucket,
		Title:     "global bucket row",
		Status:    core.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
		Meta:      map[string]any{},
	}))
	require.NoError(t, s.CreateTask(ctx, &core.Task{
		ID:        "task_seq_project_row_00000001",
		Seq:       1,
		ProjectID: &projectBucket,
		Title:     "project bucket row",
		Status:    core.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
		Meta:      map[string]any{},
	}))

	got, err := s.GetTaskBySeq(ctx, projectBucket, 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(
		t, "task_seq_project_row_00000001", got.ID,
		"alias lookup escaped the requested project bucket",
	)
	require.NotNil(t, got.ProjectID)
	require.Equal(t, projectBucket, *got.ProjectID)
}
