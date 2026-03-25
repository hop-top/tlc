package uri

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

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

	s, err := storage.NewSQLiteStorage(tmp + "/test.db")
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
