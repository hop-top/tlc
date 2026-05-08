package uri

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
	"hop.top/tlc/internal/uriutil"
)

// ---------------------------------------------------------------------------
// splitProjectTask unit tests
// ---------------------------------------------------------------------------

func TestSplitProjectTask(t *testing.T) {
	cases := []struct {
		space     string
		id        string
		wantProj  string
		wantTask  string
	}{
		// simple single-segment project
		{"tlc", "T-0001", "tlc", "T-0001"},
		// two-segment project ID (host + one path prefix)
		{"hop-top", "tlc/T-0001", "hop-top/tlc", "T-0001"},
		// three-segment project ID
		{"org", "dept/proj/T-0042", "org/dept/proj", "T-0042"},
		// no space, no slash – bare task ID
		{"", "T-0001", "", "T-0001"},
		// no space, single slash – one-segment project
		{"", "proj/T-0002", "proj", "T-0002"},
	}
	for _, tc := range cases {
		proj, task := uriutil.SplitProjectTask(tc.space, tc.id)
		assert.Equal(t, tc.wantProj, proj, "space=%q id=%q → projectID", tc.space, tc.id)
		assert.Equal(t, tc.wantTask, task, "space=%q id=%q → taskID", tc.space, tc.id)
	}
}

// newTestStorage creates a storage in an isolated temp dir and changes the
// working directory there so core.DetectProject does not pick up the repo's
// .tlc/config.yaml and scope queries to a project_id.
func newTestStorage(t *testing.T) *storage.SQLiteStorage {
	t.Helper()
	tmp := t.TempDir()

	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	core.ResetDetectionCache()
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
		core.ResetDetectionCache()
	})

	s, err := storage.NewSQLiteStorage(filepath.Join(tmp, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestResolveTask_ByCanonicalID(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	task := &core.Task{ID: "T-0001", Title: "hello", Status: core.StatusTodo}
	require.NoError(t, s.CreateTask(ctx, task))

	res, err := NewResolver(s).ResolveTask(ctx, "T-0001")
	require.NoError(t, err)
	assert.Equal(t, "T-0001", res.Task.ID)
	assert.Same(t, s, res.Storage)
}

func TestResolveTask_LowercaseBareID(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	task := &core.Task{ID: "T-0034", Title: "case insensitive", Status: core.StatusTodo}
	require.NoError(t, s.CreateTask(ctx, task))

	res, err := NewResolver(s).ResolveTask(ctx, "t-0034")
	require.NoError(t, err)
	assert.Equal(t, "T-0034", res.Task.ID)
}

func TestResolveTask_LowercaseInProjectURI(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()
	newRegisteredProject(t, s, "hop-top/tlc")

	res, err := NewResolver(s).ResolveTask(ctx, "task://hop-top/tlc/t-0001")
	require.NoError(t, err)
	assert.Equal(t, "T-0001", res.Task.ID)
}

func TestResolveTask_ByBareNumber(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	task := &core.Task{ID: "T-0042", Title: "bare number", Status: core.StatusTodo}
	require.NoError(t, s.CreateTask(ctx, task))

	res, err := NewResolver(s).ResolveTask(ctx, "42")
	require.NoError(t, err)
	assert.Equal(t, "T-0042", res.Task.ID)
}

func TestResolveTask_ByAtPrefix(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	task := &core.Task{ID: "T-0007", Title: "at prefix", Status: core.StatusTodo}
	require.NoError(t, s.CreateTask(ctx, task))

	res, err := NewResolver(s).ResolveTask(ctx, "@T-0007")
	require.NoError(t, err)
	assert.Equal(t, "T-0007", res.Task.ID)
}

func TestResolveTask_NotFound(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	_, err := NewResolver(s).ResolveTask(ctx, "T-9999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "T-9999")
}

func TestResolveTask_EmptyInput(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	_, err := NewResolver(s).ResolveTask(ctx, "")
	require.Error(t, err)
}

func TestResolveTask_ByFullURI(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	task := &core.Task{ID: "T-0003", Title: "uri task", Status: core.StatusTodo}
	require.NoError(t, s.CreateTask(ctx, task))

	// tlc:// URI without space component falls through to local storage.
	res, err := NewResolver(s).ResolveTask(ctx, "tlc:///T-0003")
	require.NoError(t, err)
	assert.Equal(t, "T-0003", res.Task.ID)
}

// newRegisteredProject creates a second SQLite DB in a temp sub-dir, seeds a
// task into it, registers it under projectID in the registry storage s, and
// returns the project DB so the caller can close it.
func newRegisteredProject(
	t *testing.T,
	s *storage.SQLiteStorage,
	projectID string,
) *storage.SQLiteStorage {
	t.Helper()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "proj.db")

	proj, err := storage.NewSQLiteStorage(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = proj.Close() })

	ctx := context.Background()
	task := &core.Task{ID: "T-0001", Title: "remote task", Status: core.StatusTodo}
	require.NoError(t, proj.CreateTask(ctx, task))

	require.NoError(t, s.RegisterProject(ctx, projectID, dbPath, "", ""))
	return proj
}

func TestResolveTask_MultiSegmentProjectURI_Absolute(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()
	newRegisteredProject(t, s, "hop-top/tlc")

	// task://hop-top/tlc/T-0001  →  project=hop-top/tlc  task=T-0001
	res, err := NewResolver(s).ResolveTask(ctx, "task://hop-top/tlc/T-0001")
	require.NoError(t, err)
	assert.Equal(t, "T-0001", res.Task.ID)
}

func TestResolveTask_MultiSegmentProjectURI_Shorthand(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()
	newRegisteredProject(t, s, "hop-top/tlc")

	// hop-top/tlc/T-0001  →  project=hop-top/tlc  task=T-0001
	res, err := NewResolver(s).ResolveTask(ctx, "hop-top/tlc/T-0001")
	require.NoError(t, err)
	assert.Equal(t, "T-0001", res.Task.ID)
}

func TestResolveTask_SingleSegmentProjectURI(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()
	newRegisteredProject(t, s, "tlc")

	// tlc/T-0001  →  project=tlc  task=T-0001
	res, err := NewResolver(s).ResolveTask(ctx, "tlc/T-0001")
	require.NoError(t, err)
	assert.Equal(t, "T-0001", res.Task.ID)
}

func TestResolveTask_MultiSegmentProject_NotFound(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	// project not registered → clear error
	_, err := NewResolver(s).ResolveTask(ctx, "task://hop-top/tlc/T-0001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hop-top/tlc")
}
