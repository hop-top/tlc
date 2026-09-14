package vtodo_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

// fixturesDir is the path from the package directory
// (`internal/vtodo`) to the shared `tests/fixtures/vtodo` tree.
const fixturesDir = "../../tests/fixtures/vtodo"

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixturesDir, name))
	require.NoError(t, err, "load fixture %s", name)
	return data
}

func TestFixture_SingleTask(t *testing.T) {
	data := loadFixture(t, "single-task.ics")
	res, err := vtodo.ParseVCalendar(strings.NewReader(string(data)))
	require.NoError(t, err)

	require.Len(t, res.Tasks, 1)
	require.Empty(t, res.Tracks)
	require.Empty(t, res.Logs)

	got := res.Tasks[0]
	require.Equal(t, "task_01h455vb4pex5vsknk084sn02q", got.ID)
	require.Equal(t, int64(42), got.Seq)
	require.Equal(t, "Replace JWT signer", got.Title)
	require.Equal(t, "Rotate to ES256 across services", got.Description)
	require.Equal(t, core.StatusInProgress, got.Status)
	require.Equal(t, core.PriorityP1, got.Priority)
	require.Equal(t, core.EffortM, got.Effort)
	require.Equal(t, []string{"security", "auth"}, got.Tags)
	require.Equal(
		t,
		"tlc://hop-top/tlc/task_01h455vb4pex5vsknk084sn02q",
		got.Reference,
	)
	require.NotNil(t, got.AssignedTo)
	require.Equal(t, "alice", *got.AssignedTo)
	require.NotNil(t, got.DueAt)
}

func TestFixture_TrackWithTasks(t *testing.T) {
	data := loadFixture(t, "track-with-tasks.ics")
	res, err := vtodo.ParseVCalendar(strings.NewReader(string(data)))
	require.NoError(t, err)

	require.Len(t, res.Tracks, 1)
	require.Len(t, res.Tasks, 3)

	tr := res.Tracks[0]
	require.Equal(t, "track_01h455vbqkfsn02nk084ksn02q", tr.ID)
	require.Equal(t, "auth-rewrite", tr.Slug)
	require.Equal(t, core.TrackTypeFeature, tr.Type)
	require.Equal(t, core.TrackStatusActive, tr.Status)

	for _, task := range res.Tasks {
		require.NotNil(t, task.TrackID, "task %s lost its TrackID", task.ID)
		require.Equal(t, tr.ID, *task.TrackID)
	}
}

func TestFixture_RecurringRRule(t *testing.T) {
	data := loadFixture(t, "recurring-rrule.ics")
	res, err := vtodo.ParseVCalendar(strings.NewReader(string(data)))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	require.Equal(t, "FREQ=DAILY;INTERVAL=2", res.Tasks[0].RRule)
}

func TestFixture_Dependencies(t *testing.T) {
	data := loadFixture(t, "with-dependencies.ics")
	res, err := vtodo.ParseVCalendar(strings.NewReader(string(data)))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 2)

	var dep *core.Task
	for _, task := range res.Tasks {
		if task.Title == "Wire signer to new key" {
			dep = task
		}
	}
	require.NotNil(t, dep, "expected dependent task in fixture")
	require.NotNil(t, dep.Meta)
	bb, ok := dep.Meta["blocked_by"].([]string)
	require.True(t, ok, "blocked_by missing from %v", dep.Meta)
	require.Equal(t, []string{"task_01h455vb4pex5vsknk084sn0az"}, bb)
}

func TestFixture_WithLogs(t *testing.T) {
	data := loadFixture(t, "with-logs.ics")
	res, err := vtodo.ParseVCalendar(strings.NewReader(string(data)))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	require.Len(t, res.Logs, 2)

	for _, log := range res.Logs {
		require.Equal(t, res.Tasks[0].ID, log.TaskID)
		require.Equal(t, "alice", log.By)
		require.NotEmpty(t, log.Action)
		require.NotEmpty(t, log.Note)
	}
}

// TestFixture_RoundTripStability asserts the encode→decode→encode cycle
// is stable: re-encoding the parsed result of a fixture must reproduce
// byte-for-byte the original fixture content. This is the primary
// guarantee of the package.
//
// DTSTAMP is the entity's own last-modified instant, so it round-trips
// from the decoded entity like every other property; the export clock
// is pinned only so a timestamp-less entity could never make the
// comparison non-deterministic.
func TestFixture_RoundTripStability(t *testing.T) {
	fixtureStamp := time.Date(2026, 5, 2, 14, 30, 0, 0, time.UTC)
	for _, name := range []string{
		"single-task.ics",
		"recurring-rrule.ics",
	} {
		t.Run(name, func(t *testing.T) {
			data := loadFixture(t, name)
			res, err := vtodo.ParseVCalendar(strings.NewReader(string(data)))
			require.NoError(t, err)

			cal, err := vtodo.BuildVCalendar(
				res.Tasks, res.Tracks, res.Logs,
				vtodo.WithExportTime(fixtureStamp),
			)
			require.NoError(t, err)
			got := mustSerialize(t, cal)
			require.Equal(t, normaliseEOL(string(data)), normaliseEOL(got))
		})
	}
}

func normaliseEOL(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}
