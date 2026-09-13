package uri

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hop.top/tlc/internal/storage"
)

func TestGetRegistry_RegistersExpectedTypes(t *testing.T) {
	dir := t.TempDir()
	s, err := storage.NewSQLiteStorage(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer s.Close()

	reg, err := GetRegistry(s)
	require.NoError(t, err)
	require.NotNil(t, reg)

	types := reg.Types()
	assert.ElementsMatch(t,
		[]string{"project", "task", "track", "assignee", "tag", "recipe"}, types)
}

func TestGetRegistry_DoubleCallReturnsNewRegistry(t *testing.T) {
	dir := t.TempDir()
	s, err := storage.NewSQLiteStorage(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer s.Close()

	r1, err := GetRegistry(s)
	require.NoError(t, err)

	r2, err := GetRegistry(s)
	require.NoError(t, err)

	// Each call returns a fresh registry (no "already registered" error).
	assert.NotSame(t, r1, r2)
}

func TestRegisterTypes_TaskCompleter(t *testing.T) {
	dir := t.TempDir()
	s, err := storage.NewSQLiteStorage(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer s.Close()

	reg, err := GetRegistry(s)
	require.NoError(t, err)

	// Empty DB → zero suggestions, no error.
	suggestions, err := reg.Complete(context.Background(), "task", "")
	require.NoError(t, err)
	assert.Empty(t, suggestions)
}

func TestRegisterTypes_TagCompleter(t *testing.T) {
	dir := t.TempDir()
	s, err := storage.NewSQLiteStorage(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer s.Close()

	reg, err := GetRegistry(s)
	require.NoError(t, err)

	// Empty DB → zero suggestions, no error.
	suggestions, err := reg.Complete(context.Background(), "tag", "")
	require.NoError(t, err)
	assert.Empty(t, suggestions)
}

func TestRegisterTypes_ProjectCompleter(t *testing.T) {
	dir := t.TempDir()
	s, err := storage.NewSQLiteStorage(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer s.Close()

	reg, err := GetRegistry(s)
	require.NoError(t, err)

	// Empty DB → zero suggestions, no error.
	suggestions, err := reg.Complete(context.Background(), "project", "")
	require.NoError(t, err)
	assert.Empty(t, suggestions)
}
