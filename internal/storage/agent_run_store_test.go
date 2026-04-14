package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hop.top/tlc/internal/core"
)

func TestAgentRunStore_CRUD(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteStorage(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	// Create.
	r := &core.AgentRunRecord{
		ID:         "run-001",
		Agent:      "claude",
		TargetType: "task",
		TargetID:   "T-0042",
		Status:     "running",
		StartedAt:  time.Now().UTC().Truncate(time.Second),
		CreatedBy:  "test",
	}
	require.NoError(t, s.CreateAgentRun(ctx, r))

	// Get.
	got, err := s.GetAgentRun(ctx, "run-001")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "claude", got.Agent)
	assert.Equal(t, "running", got.Status)
	assert.Equal(t, "T-0042", got.TargetID)

	// Update.
	now := time.Now().UTC().Truncate(time.Second)
	r.Status = "succeeded"
	r.ExitCode = 0
	r.EndedAt = &now
	require.NoError(t, s.UpdateAgentRun(ctx, r))

	got, err = s.GetAgentRun(ctx, "run-001")
	require.NoError(t, err)
	assert.Equal(t, "succeeded", got.Status)
	assert.NotNil(t, got.EndedAt)

	// List.
	runs, err := s.ListAgentRuns(ctx, "", 10)
	require.NoError(t, err)
	assert.Len(t, runs, 1)

	// List with filter.
	runs, err = s.ListAgentRuns(ctx, "failed", 10)
	require.NoError(t, err)
	assert.Len(t, runs, 0)

	runs, err = s.ListAgentRuns(ctx, "succeeded", 10)
	require.NoError(t, err)
	assert.Len(t, runs, 1)
}

func TestAgentRunStore_GetNotFound(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteStorage(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer func() { _ = s.Close() }()

	got, err := s.GetAgentRun(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}
