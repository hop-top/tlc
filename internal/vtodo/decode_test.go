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

// categoriesICS wraps body (raw VTODO property lines) in a minimal
// VCALENDAR so a hand-written CATEGORIES shape can be parsed.
func categoriesICS(body string) string {
	return "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//tlc//vtodo//EN\r\n" +
		"BEGIN:VTODO\r\n" +
		"UID:task_01h455vb4pex5vsknk084sn02q@tlc.local\r\n" +
		"SUMMARY:Tagged\r\n" +
		body +
		"END:VTODO\r\n" +
		"END:VCALENDAR\r\n"
}

// TestCategories_RoundTrip pins the contract that matters to a caller:
// tags survive export→import as the same slice, in the same order.
// Order is load-bearing -- tlc tags are ordered and callers compare the
// slice directly, so a set-like reordering would be a regression.
func TestCategories_RoundTrip(t *testing.T) {
	in := sampleTask()
	in.Tags = []string{"security", "auth", "backend", "priority:P1"}

	res := roundTrip(t, []*core.Task{in}, nil, nil)
	require.Len(t, res.Tasks, 1)
	require.Equal(t, in.Tags, res.Tasks[0].Tags)
}

// TestCategories_SinglePropertyForm asserts the emitted wire form is ONE
// CATEGORIES property carrying a comma-separated list (RFC 5545
// 3.8.1.2), not the one-property-per-tag shape tlc used to write.
func TestCategories_SinglePropertyForm(t *testing.T) {
	task := sampleTask()
	task.Tags = []string{"security", "auth"}
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)

	require.Equal(t, 1, strings.Count(out, "CATEGORIES"),
		"expected exactly one CATEGORIES property in:\n%s", out)
	// The separator comma is escaped because the codec escapes every
	// comma in a TEXT value; the parser reverses it, which is why the
	// round-trip test above is the real guarantee.
	require.Contains(t, out, "CATEGORIES:security\\,auth")
}

// TestCategories_BackwardCompatRepeatedProperties is the compatibility
// guarantee. Every .ics tlc wrote before this change -- and the
// committed fixtures -- carry one CATEGORIES property PER tag.
// helpers.Categories alone cannot read that shape: it resolves the
// property through Component.Get, which returns only the first match,
// so decoding would silently yield just ["security"]. Dropping a user's
// tags with no error is the failure this test exists to catch.
func TestCategories_BackwardCompatRepeatedProperties(t *testing.T) {
	src := categoriesICS("CATEGORIES:security\r\nCATEGORIES:auth\r\nCATEGORIES:backend\r\n")

	res, err := vtodo.ParseVCalendar(strings.NewReader(src))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	require.Equal(t, []string{"security", "auth", "backend"}, res.Tasks[0].Tags)
}

// TestCategories_BackwardCompatMixedShapes covers a file that carries
// both shapes at once -- one property holding a list plus a second
// standalone property. A merge of the two written by different tool
// versions is exactly how this arises in the wild.
func TestCategories_BackwardCompatMixedShapes(t *testing.T) {
	src := categoriesICS("CATEGORIES:security\\,auth\r\nCATEGORIES:backend\r\n")

	res, err := vtodo.ParseVCalendar(strings.NewReader(src))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	require.Equal(t, []string{"security", "auth", "backend"}, res.Tasks[0].Tags)
}

// TestCategories_RepeatedPropertiesDedupe pins the dedupe applied across
// repeated properties: an old file may name the same tag twice, and a
// task must not come back with a duplicate tag. First-seen order wins,
// matching helpers.SetCategories on the write side so the value is
// stable under a decode→encode round trip.
func TestCategories_RepeatedPropertiesDedupe(t *testing.T) {
	src := categoriesICS("CATEGORIES:security\r\nCATEGORIES:auth\r\nCATEGORIES:security\r\n")

	res, err := vtodo.ParseVCalendar(strings.NewReader(src))
	require.NoError(t, err)
	require.Equal(t, []string{"security", "auth"}, res.Tasks[0].Tags)
}

// TestCategories_CaseSensitive pins case-sensitive tag identity.
// helpers treats CATEGORIES as user-facing labels, so "Security" and
// "security" are distinct tags and neither dedupes the other away.
func TestCategories_CaseSensitive(t *testing.T) {
	in := sampleTask()
	in.Tags = []string{"Security", "security"}

	res := roundTrip(t, []*core.Task{in}, nil, nil)
	require.Equal(t, []string{"Security", "security"}, res.Tasks[0].Tags)
}

// TestCategories_Empty asserts a task with no tags emits no CATEGORIES
// property at all, rather than an empty one.
func TestCategories_Empty(t *testing.T) {
	task := sampleTask()
	task.Tags = nil
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)

	require.NotContains(t, out, "CATEGORIES")

	res := roundTrip(t, []*core.Task{task}, nil, nil)
	require.Empty(t, res.Tasks[0].Tags)
}

// TestCategories_TagContainingComma documents a REAL limitation rather
// than asserting desired behavior.
//
// core.ValidateTags imposes no character restrictions under the default
// `open` policy -- it returns nil without inspecting the tags at all --
// so a tag containing a comma is accepted on the write path. On the
// wire it cannot survive: RFC 5545 3.3.11 makes the comma the value
// separator for multi-value TEXT, and the codec escapes every comma in
// a CATEGORIES value uniformly, giving the separator and an in-tag
// comma an identical encoding. So "a,b" comes back as two tags.
//
// This is not a regression introduced here -- the previous
// one-property-per-tag encoder split on comma at decode and lost the
// same tag. The test pins the loss so a future change that makes
// commas round-trip (or that rejects them at validation) fails here
// loudly and gets a deliberate decision instead of passing unnoticed.
func TestCategories_TagContainingComma(t *testing.T) {
	require.NoError(t, core.ValidateTags([]string{"a,b"}),
		"open tag policy is expected to admit a comma; revisit this test if that changes")

	in := sampleTask()
	in.Tags = []string{"a,b", "c"}

	res := roundTrip(t, []*core.Task{in}, nil, nil)
	require.Equal(t, []string{"a", "b", "c"}, res.Tasks[0].Tags,
		"a comma inside a tag is indistinguishable from the list separator")
}
