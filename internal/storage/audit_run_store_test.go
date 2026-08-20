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

func newAuditTestStore(t *testing.T) *SQLiteStorage {
	t.Helper()
	dir := t.TempDir()
	s, err := NewSQLiteStorage(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func auditFixture(tool, runID string, started time.Time) *core.AuditRun {
	finished := started.Add(10 * time.Minute)
	return &core.AuditRun{
		Tool:       tool,
		RunID:      runID,
		Subject:    "skills/example/SKILL.md",
		StartedAt:  started,
		FinishedAt: &finished,
		Outcome:    "rejected",
		Metrics:    map[string]any{"baseline": 0.7, "best": 0.74},
		Steps: []core.AuditStep{
			{Seq: 1, Name: "generation 1", Status: "rejected", Detail: "below gate"},
			{Seq: 2, Name: "generation 2", Status: "rejected"},
		},
	}
}

func TestAuditRunStore_RoundTrip(t *testing.T) {
	s := newAuditTestStore(t)
	ctx := context.Background()
	started := time.Now().UTC().Truncate(time.Second)

	r := auditFixture("looptool", "run-001", started)
	require.NoError(t, s.UpsertAuditRun(ctx, r))

	got, err := s.GetAuditRuns(ctx, "run-001", "", "")
	require.NoError(t, err)
	require.Len(t, got, 1)

	g := got[0]
	assert.Equal(t, "looptool", g.Tool)
	assert.Equal(t, "run-001", g.RunID)
	assert.Equal(t, "skills/example/SKILL.md", g.Subject)
	assert.Equal(t, "rejected", g.Outcome)
	assert.True(t, g.StartedAt.Equal(started))
	require.NotNil(t, g.FinishedAt)
	assert.True(t, g.FinishedAt.Equal(started.Add(10*time.Minute)))
	assert.InDelta(t, 0.7, g.Metrics["baseline"].(float64), 1e-9)
	require.Len(t, g.Steps, 2)
	assert.Equal(t, "generation 1", g.Steps[0].Name)
	assert.Equal(t, "below gate", g.Steps[0].Detail)
	assert.Equal(t, 2, g.Steps[1].Seq)
}

func TestAuditRunStore_UpsertReplaces(t *testing.T) {
	s := newAuditTestStore(t)
	ctx := context.Background()
	started := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, s.UpsertAuditRun(ctx, auditFixture("looptool", "run-001", started)))

	updated := auditFixture("looptool", "run-001", started)
	updated.Outcome = "promoted"
	updated.Steps = []core.AuditStep{
		{Seq: 1, Name: "generation 1", Status: "accepted"},
	}
	require.NoError(t, s.UpsertAuditRun(ctx, updated))

	got, err := s.GetAuditRuns(ctx, "run-001", "looptool", "")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "promoted", got[0].Outcome)
	require.Len(t, got[0].Steps, 1, "steps replaced wholesale, not appended")
	assert.Equal(t, "accepted", got[0].Steps[0].Status)
}

func TestAuditRunStore_ListFiltersOrderLimit(t *testing.T) {
	s := newAuditTestStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)

	older := auditFixture("looptool", "run-old", base.Add(-2*time.Hour))
	newer := auditFixture("looptool", "run-new", base)
	other := auditFixture("othertool", "run-x", base.Add(-1*time.Hour))
	other.Subject = "prompts/other.md"
	for _, r := range []*core.AuditRun{older, newer, other} {
		require.NoError(t, s.UpsertAuditRun(ctx, r))
	}

	// Newest first, all rows.
	all, err := s.ListAuditRuns(ctx, core.AuditRunQuery{})
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, "run-new", all[0].RunID)
	assert.Equal(t, "run-x", all[1].RunID)
	assert.Equal(t, "run-old", all[2].RunID)
	assert.Nil(t, all[0].Steps, "list omits steps")

	// Tool filter.
	loops, err := s.ListAuditRuns(ctx, core.AuditRunQuery{Tool: "looptool"})
	require.NoError(t, err)
	require.Len(t, loops, 2)

	// Subject filter.
	subj, err := s.ListAuditRuns(ctx, core.AuditRunQuery{Subject: "prompts/other.md"})
	require.NoError(t, err)
	require.Len(t, subj, 1)
	assert.Equal(t, "othertool", subj[0].Tool)

	// Limit.
	limited, err := s.ListAuditRuns(ctx, core.AuditRunQuery{Limit: 1})
	require.NoError(t, err)
	require.Len(t, limited, 1)
	assert.Equal(t, "run-new", limited[0].RunID)
}

func TestAuditRunStore_GetAmbiguousAcrossTools(t *testing.T) {
	s := newAuditTestStore(t)
	ctx := context.Background()
	started := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, s.UpsertAuditRun(ctx, auditFixture("looptool", "run-001", started)))
	require.NoError(t, s.UpsertAuditRun(ctx, auditFixture("othertool", "run-001", started)))

	both, err := s.GetAuditRuns(ctx, "run-001", "", "")
	require.NoError(t, err)
	assert.Len(t, both, 2)

	one, err := s.GetAuditRuns(ctx, "run-001", "othertool", "")
	require.NoError(t, err)
	require.Len(t, one, 1)
	assert.Equal(t, "othertool", one[0].Tool)

	none, err := s.GetAuditRuns(ctx, "run-missing", "", "")
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestAuditRunStore_ProjectScoping(t *testing.T) {
	s := newAuditTestStore(t)
	ctx := context.Background()
	started := time.Now().UTC().Truncate(time.Second)

	inProj := auditFixture("looptool", "run-p", started)
	inProj.ProjectID = "org/projA"
	require.NoError(t, s.UpsertAuditRun(ctx, inProj))
	require.NoError(t, s.UpsertAuditRun(ctx, auditFixture("looptool", "run-g", started.Add(-time.Minute))))

	scoped, err := s.ListAuditRuns(ctx, core.AuditRunQuery{ProjectID: "org/projA"})
	require.NoError(t, err)
	require.Len(t, scoped, 1)
	assert.Equal(t, "run-p", scoped[0].RunID)

	all, err := s.ListAuditRuns(ctx, core.AuditRunQuery{})
	require.NoError(t, err)
	assert.Len(t, all, 2)
}
