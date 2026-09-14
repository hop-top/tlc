package vtodo_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

// roundTrip encodes the supplied entities and immediately re-parses the
// result. Stability of repeated encode→decode→encode cycles is the
// primary contract this package exposes.
func roundTrip(
	t *testing.T,
	tasks []*core.Task,
	tracks []*core.Track,
	logs []*core.LogEntry,
	opts ...vtodo.Option,
) *vtodo.ParseResult {
	t.Helper()
	cal, err := vtodo.BuildVCalendar(tasks, tracks, logs, opts...)
	require.NoError(t, err)
	res, err := vtodo.ParseVCalendar(strings.NewReader(mustSerialize(t, cal)), opts...)
	require.NoError(t, err)
	return res
}

func TestRoundTrip_TaskAllFields(t *testing.T) {
	in := sampleTask()
	res := roundTrip(t, []*core.Task{in}, nil, nil)
	require.Len(t, res.Tasks, 1)
	got := res.Tasks[0]

	require.Equal(t, in.ID, got.ID)
	require.Equal(t, in.Title, got.Title)
	require.Equal(t, in.Description, got.Description)
	require.Equal(t, in.Status, got.Status)
	require.Equal(t, in.Priority, got.Priority)
	require.Equal(t, in.Effort, got.Effort)
	require.Equal(t, in.Tags, got.Tags)
	require.Equal(t, in.Reference, got.Reference)
	require.Equal(t, in.RRule, got.RRule)
	require.Equal(t, in.Seq, got.Seq)
	require.Equal(t, *in.AssignedTo, *got.AssignedTo)
	require.Equal(t, in.CreatedAt.UTC(), got.CreatedAt.UTC())
	require.Equal(t, in.UpdatedAt.UTC(), got.UpdatedAt.UTC())
	require.Equal(t, in.DueAt.UTC(), got.DueAt.UTC())
	require.Equal(t, in.RemindAt.UTC(), got.RemindAt.UTC())
}

func TestRoundTrip_StatusEveryValue(t *testing.T) {
	cases := []core.TaskStatus{
		core.StatusTodo,
		core.StatusInProgress,
		core.StatusDone,
		core.StatusSkipped,
	}
	for _, s := range cases {
		t.Run(string(s), func(t *testing.T) {
			task := &core.Task{
				ID:        "task_01h455vb4pex5vsknk084sn02q",
				Title:     "x",
				Status:    s,
				CreatedAt: fixedTime,
			}
			res := roundTrip(t, []*core.Task{task}, nil, nil)
			require.Len(t, res.Tasks, 1)
			require.Equal(t, s, res.Tasks[0].Status)
		})
	}
}

func TestRoundTrip_PriorityEveryValue(t *testing.T) {
	for _, p := range []core.Priority{
		core.PriorityP0, core.PriorityP1, core.PriorityP2, core.PriorityP3,
	} {
		t.Run(string(p), func(t *testing.T) {
			task := &core.Task{
				ID:        "task_01h455vb4pex5vsknk084sn02q",
				Title:     "x",
				Status:    core.StatusTodo,
				Priority:  p,
				CreatedAt: fixedTime,
			}
			res := roundTrip(t, []*core.Task{task}, nil, nil)
			require.Equal(t, p, res.Tasks[0].Priority)
		})
	}
}

func TestRoundTrip_TrackHierarchy(t *testing.T) {
	track := &core.Track{
		ID:        "track_01h455vbqkfsn02nk084ksn02q",
		Slug:      "auth-rewrite",
		Title:     "Auth rewrite",
		Type:      core.TrackTypeFeature,
		Status:    core.TrackStatusActive,
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	}
	t1 := sampleTask()
	t1.TrackID = ptr(track.ID)

	t2 := *sampleTask()
	t2.ID = "task_01h455vb4pex5vsknk084sn0aw"
	t2.Title = "Audit token claims"
	t2.TrackID = ptr(track.ID)

	t3 := *sampleTask()
	t3.ID = "task_01h455vb4pex5vsknk084sn0ax"
	t3.Title = "Bench rotation throughput"
	t3.TrackID = ptr(track.ID)

	res := roundTrip(
		t,
		[]*core.Task{t1, &t2, &t3},
		[]*core.Track{track},
		nil,
	)
	require.Len(t, res.Tracks, 1)
	require.Len(t, res.Tasks, 3)
	require.Equal(t, track.ID, res.Tracks[0].ID)
	require.Equal(t, track.Slug, res.Tracks[0].Slug)
	require.Equal(t, track.Type, res.Tracks[0].Type)
	for _, got := range res.Tasks {
		require.NotNil(t, got.TrackID, "task %s missing TrackID", got.ID)
		require.Equal(t, track.ID, *got.TrackID)
	}
}

func TestRoundTrip_DependsOn(t *testing.T) {
	blocker := "task_01h455vb4pex5vsknk084sn0az"
	task := sampleTask()
	task.Meta = map[string]interface{}{
		"blocked_by": []string{blocker},
	}
	res := roundTrip(t, []*core.Task{task}, nil, nil)
	require.Len(t, res.Tasks, 1)
	got := res.Tasks[0]
	require.NotNil(t, got.Meta)
	bb, ok := got.Meta["blocked_by"].([]string)
	require.True(t, ok, "blocked_by should be []string, got %T", got.Meta["blocked_by"])
	require.Equal(t, []string{blocker}, bb)
}

func TestRoundTrip_LogsWithFlag(t *testing.T) {
	task := sampleTask()
	log := &core.LogEntry{
		TaskID:    task.ID,
		Timestamp: fixedTime,
		By:        "alice",
		Action:    "CLAIMED",
		Note:      "starting work",
	}
	res := roundTrip(
		t,
		[]*core.Task{task}, nil, []*core.LogEntry{log},
		vtodo.WithIncludeLogs(true),
	)
	require.Len(t, res.Logs, 1)
	got := res.Logs[0]
	require.Equal(t, log.TaskID, got.TaskID)
	require.Equal(t, log.By, got.By)
	require.Equal(t, log.Action, got.Action)
	require.Equal(t, log.Note, got.Note)
	require.Equal(t, log.Timestamp.UTC(), got.Timestamp.UTC())
}

func TestRoundTrip_LogsSkippedByDefault(t *testing.T) {
	task := sampleTask()
	log := &core.LogEntry{
		TaskID:    task.ID,
		Timestamp: fixedTime,
		Action:    "CLAIMED",
	}
	res := roundTrip(t, []*core.Task{task}, nil, []*core.LogEntry{log})
	require.Empty(t, res.Logs)
}

func TestParseVCalendar_ForeignUIDStashedInMeta(t *testing.T) {
	// Build a hand-crafted calendar with a UID that is *not* a tlc
	// typeid. The decoder should mint a fresh ID and remember the
	// original.
	ics := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//other//cal//EN",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"BEGIN:VTODO",
		"UID:foo@example.com",
		"SUMMARY:External task",
		"STATUS:NEEDS-ACTION",
		"DTSTAMP:20260502T143000Z",
		"END:VTODO",
		"END:VCALENDAR",
		"",
	}, "\r\n")
	res, err := vtodo.ParseVCalendar(strings.NewReader(ics))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	got := res.Tasks[0]
	require.True(t, core.IsTaskID(got.ID), "minted ID should be a tlc typeid, got %q", got.ID)
	require.Equal(t, "External task", got.Title)
	require.Equal(t, "foo@example.com", got.Meta["external_uid"])
}

func TestParseVCalendar_RRuleValidation(t *testing.T) {
	// An unsupported FREQ should be dropped, not propagated raw.
	icsBytes := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//tlc//vtodo//EN",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"BEGIN:VTODO",
		"UID:task_01h455vb4pex5vsknk084sn02q@tlc.local",
		"SUMMARY:Bad rule",
		"DTSTAMP:20260502T143000Z",
		"RRULE:FREQ=YEARLY;INTERVAL=1",
		"END:VTODO",
		"END:VCALENDAR",
		"",
	}, "\r\n")
	res, err := vtodo.ParseVCalendar(strings.NewReader(icsBytes))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	require.Empty(t, res.Tasks[0].RRule, "unsupported FREQ must not survive decode")
}

// TestRoundTrip_LiteralBackslashSequences pins the boundary between the
// codec's RFC 5545 §3.3.11 TEXT escaping and this package's model. The
// codec escapes on emit and unescapes on parse, so any extra unescape
// pass here would reinterpret a literal backslash sequence written by a
// user (for example the two characters `\` and `n` inside a code
// snippet) as the escape it merely resembles.
func TestRoundTrip_LiteralBackslashSequences(t *testing.T) {
	const literal = `seq \n stays two chars; \, stays two chars; \\ pair; trailing \`

	in := sampleTask()
	in.Title = literal
	in.Description = literal
	in.Tags = []string{`tag\nnot-newline`}

	res := roundTrip(t, []*core.Task{in}, nil, nil)
	require.Len(t, res.Tasks, 1)
	got := res.Tasks[0]

	require.Equal(t, literal, got.Title)
	require.Equal(t, literal, got.Description)
	require.Equal(t, in.Tags, got.Tags)
	require.NotContains(t, got.Description, "\n", "literal backslash-n must not become a newline")
}

// TestRoundTrip_LiteralBackslashInTrackAndLog covers the same boundary
// for the track SUMMARY and log note decode paths.
func TestRoundTrip_LiteralBackslashInTrackAndLog(t *testing.T) {
	const literal = `path C:\new\table and a \, comma`

	tr := &core.Track{
		ID:        "track_01h455vbqkfsn02nk084ksn02q",
		Slug:      "escapes",
		Title:     literal,
		Type:      core.TrackTypeFeature,
		Status:    core.TrackStatusActive,
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	}
	task := sampleTask()
	trackID := tr.ID
	task.TrackID = &trackID

	log := &core.LogEntry{
		ID:        1,
		TaskID:    task.ID,
		Action:    "note",
		By:        "alice",
		Note:      literal,
		Timestamp: fixedTime,
	}

	res := roundTrip(
		t,
		[]*core.Task{task},
		[]*core.Track{tr},
		[]*core.LogEntry{log},
		vtodo.WithIncludeLogs(true),
	)

	require.Len(t, res.Tracks, 1)
	require.Equal(t, literal, res.Tracks[0].Title)

	require.Len(t, res.Logs, 1)
	require.Equal(t, literal, res.Logs[0].Note)
}
