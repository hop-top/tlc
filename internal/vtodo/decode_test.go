package vtodo_test

import (
	"strings"
	"testing"
	"time"

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
	requireValidExport(t, cal)
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

// TestCategories_OnePropertyPerTag asserts the emitted wire form is one
// CATEGORIES property PER tag, never a comma-joined list. The joined
// form is RFC-legal in the abstract but unusable through this codec:
// it escapes every comma in a TEXT value, so `security,auth` reaches
// the wire as `security\,auth`, which any RFC 5545 reader takes as a
// single category. Trimming, empties and repeats are handled on the
// way out so the count is exactly the distinct tags.
func TestCategories_OnePropertyPerTag(t *testing.T) {
	task := sampleTask()
	task.Tags = []string{"security", " auth ", "", "security", "backend"}
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)

	require.Equal(t, 3, strings.Count(out, "\r\nCATEGORIES:"),
		"expected one CATEGORIES property per distinct tag in:\n%s", out)
	require.Contains(t, out, "\r\nCATEGORIES:security\r\n")
	require.Contains(t, out, "\r\nCATEGORIES:auth\r\n")
	require.Contains(t, out, "\r\nCATEGORIES:backend\r\n")
	require.NotContains(t, out, "\\,", "no comma-joined CATEGORIES on the wire")
}

// TestCategories_RepeatedPropertiesDecode reads the shape tlc writes.
// helpers.Categories alone cannot read it: it resolves the property
// through Component.Get, which returns only the first match, so
// decoding would silently yield just ["security"]. Dropping a user's
// tags with no error is the failure this test exists to catch.
func TestCategories_RepeatedPropertiesDecode(t *testing.T) {
	src := categoriesICS("CATEGORIES:security\r\nCATEGORIES:auth\r\nCATEGORIES:backend\r\n")

	res, err := vtodo.ParseVCalendar(strings.NewReader(src))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	require.Equal(t, []string{"security", "auth", "backend"}, res.Tasks[0].Tags)
}

// TestCategories_BackwardCompatMixedShapes covers a file that carries
// both shapes at once -- one property holding a comma-joined list (a
// foreign producer, or the interim tlc release that wrote that form)
// plus a standalone property. A merge of the two written by different
// tool versions is exactly how this arises in the wild.
func TestCategories_BackwardCompatMixedShapes(t *testing.T) {
	src := categoriesICS("CATEGORIES:security\\,auth\r\nCATEGORIES:backend\r\n")

	res, err := vtodo.ParseVCalendar(strings.NewReader(src))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	require.Equal(t, []string{"security", "auth", "backend"}, res.Tasks[0].Tags)
}

// TestCategories_RepeatedPropertiesDedupe pins the dedupe applied across
// repeated properties: a hand-edited file may name the same tag twice,
// and a task must not come back with a duplicate tag. First-seen order
// wins, matching the encoder's dedupe so the value is stable under a
// decode→encode round trip.
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
// wire it does not survive: the encoder writes `CATEGORIES:a\,b` (one
// property, comma escaped), but the decoder splits every property's
// value on comma because it must also read the comma-joined shape, so
// "a,b" comes back as two tags.
//
// The test pins the loss so a future change that makes commas
// round-trip (or that rejects them at validation) fails here loudly
// and gets a deliberate decision instead of passing unnoticed.
func TestCategories_TagContainingComma(t *testing.T) {
	require.NoError(t, core.ValidateTags([]string{"a,b"}),
		"open tag policy is expected to admit a comma; revisit this test if that changes")

	in := sampleTask()
	in.Tags = []string{"a,b", "c"}

	res := roundTrip(t, []*core.Task{in}, nil, nil)
	require.Equal(t, []string{"a", "b", "c"}, res.Tasks[0].Tags,
		"a comma inside a tag is indistinguishable from the list separator")
}

// calWith wraps the supplied VTODO body lines in a minimal VCALENDAR.
func calWith(lines ...string) string {
	out := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//tlc//vtodo//EN",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"BEGIN:VTODO",
		"UID:task_01h455vb4pex5vsknk084sn02q@tlc.local",
		"SUMMARY:x",
		"STATUS:NEEDS-ACTION",
	}
	out = append(out, lines...)
	out = append(out, "END:VTODO", "END:VCALENDAR", "")
	return strings.Join(out, "\r\n")
}

func parseOneTask(t *testing.T, ics string) *core.Task {
	t.Helper()
	res, err := vtodo.ParseVCalendar(strings.NewReader(ics))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	return res.Tasks[0]
}

// TestDecode_DateOnlyDueFallback covers the one non-form-#2 layout we
// still accept. vstar has no VALUE=DATE support at the pinned version,
// so a bare YYYYMMDD DUE would otherwise be silently dropped.
func TestDecode_DateOnlyDueFallback(t *testing.T) {
	got := parseOneTask(t, calWith(
		"DTSTAMP:20260502T143000Z",
		"DUE;VALUE=DATE:20260515",
	))
	require.NotNil(t, got.DueAt, "date-only DUE must still decode")
	require.Equal(
		t,
		time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC),
		got.DueAt.UTC(),
	)
}

// TestDecode_DateOnlyCreatedFallback proves the fallback is shared by
// the other time-bearing properties, not special-cased to DUE.
func TestDecode_DateOnlyCreatedFallback(t *testing.T) {
	got := parseOneTask(t, calWith(
		"DTSTAMP:20260502T143000Z",
		"CREATED;VALUE=DATE:20260501",
	))
	require.Equal(
		t,
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		got.CreatedAt.UTC(),
	)
}

// TestDecode_NonICalLayoutsRejected pins the strictness we inherit from
// vstar.ParseTime: RFC 3339 and bare local-time forms are NOT iCalendar
// DATE-TIME and must not be silently coerced.
func TestDecode_NonICalLayoutsRejected(t *testing.T) {
	for _, due := range []string{
		"2026-05-15T14:30:00Z", // RFC 3339
		"2026-05-15T14:30:00",  // ISO local
		"2026-05-15T14:30:00+02:00",
		"20260515T143000", // form #1, floating local — no zone
	} {
		t.Run(due, func(t *testing.T) {
			got := parseOneTask(t, calWith(
				"DTSTAMP:20260502T143000Z",
				"DUE:"+due,
			))
			require.Nil(t, got.DueAt, "non-iCalendar DUE %q must not decode", due)
		})
	}
}

// TestDecode_UTCFormRoundTrips is the happy path: RFC 5545 §3.3.5
// form #2 for every time-bearing property the decoder reads.
func TestDecode_UTCFormRoundTrips(t *testing.T) {
	got := parseOneTask(t, calWith(
		"CREATED:20260502T143000Z",
		"DTSTAMP:20260914T080000Z",
		"LAST-MODIFIED:20260502T153000Z",
		"DUE:20260503T143000Z",
		"BEGIN:VALARM",
		"ACTION:DISPLAY",
		"TRIGGER;VALUE=DATE-TIME:20260503T023000Z",
		"END:VALARM",
	))
	require.Equal(t, time.Date(2026, 5, 2, 14, 30, 0, 0, time.UTC), got.CreatedAt.UTC())
	require.Equal(t, time.Date(2026, 5, 2, 15, 30, 0, 0, time.UTC), got.UpdatedAt.UTC())
	require.NotNil(t, got.DueAt)
	require.Equal(t, time.Date(2026, 5, 3, 14, 30, 0, 0, time.UTC), got.DueAt.UTC())
	require.NotNil(t, got.RemindAt)
	require.Equal(t, time.Date(2026, 5, 3, 2, 30, 0, 0, time.UTC), got.RemindAt.UTC())
}

// TestDecode_UpdatedAtPrefersLastModified pins the documented
// precedence: LAST-MODIFIED wins, DTSTAMP is only a fallback. With
// DTSTAMP now carrying export time, leaking it into UpdatedAt whenever
// LAST-MODIFIED exists would corrupt the entity.
func TestDecode_UpdatedAtPrefersLastModified(t *testing.T) {
	t.Run("last-modified present", func(t *testing.T) {
		got := parseOneTask(t, calWith(
			"CREATED:20260502T143000Z",
			"DTSTAMP:20260914T080000Z",
			"LAST-MODIFIED:20260502T153000Z",
		))
		require.Equal(t, time.Date(2026, 5, 2, 15, 30, 0, 0, time.UTC), got.UpdatedAt.UTC())
	})
	t.Run("falls back to dtstamp", func(t *testing.T) {
		got := parseOneTask(t, calWith(
			"CREATED:20260502T143000Z",
			"DTSTAMP:20260914T080000Z",
		))
		require.Equal(t, time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC), got.UpdatedAt.UTC())
	})
}

// TestDecode_TrackUpdatedAtPrefersLastModified mirrors the task-side
// precedence check for the track decode path.
func TestDecode_TrackUpdatedAtPrefersLastModified(t *testing.T) {
	base := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//tlc//vtodo//EN",
		"BEGIN:VTODO",
		"UID:track_01h455vbqkfsn02nk084ksn02q@tlc.local",
		"SUMMARY:Auth rewrite",
		"STATUS:IN-PROCESS",
		"X-TLC-IS-TRACK:TRUE",
		"CREATED:20260502T143000Z",
		"DTSTAMP:20260914T080000Z",
	}
	withLM := append(append([]string{}, base...),
		"LAST-MODIFIED:20260502T153000Z", "END:VTODO", "END:VCALENDAR", "")
	res, err := vtodo.ParseVCalendar(strings.NewReader(strings.Join(withLM, "\r\n")))
	require.NoError(t, err)
	require.Len(t, res.Tracks, 1)
	require.Equal(t, time.Date(2026, 5, 2, 15, 30, 0, 0, time.UTC), res.Tracks[0].UpdatedAt.UTC())

	noLM := append(append([]string{}, base...), "END:VTODO", "END:VCALENDAR", "")
	res2, err := vtodo.ParseVCalendar(strings.NewReader(strings.Join(noLM, "\r\n")))
	require.NoError(t, err)
	require.Len(t, res2.Tracks, 1)
	require.Equal(t, time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC), res2.Tracks[0].UpdatedAt.UTC())
}

// TestDecode_JournalTimestampPrefersCreated pins the VJOURNAL
// precedence: CREATED carries the entry's own instant, DTSTAMP carries
// export time and is only a fallback. Preferring DTSTAMP would stamp
// every decoded log entry with the moment the file was written.
func TestDecode_JournalTimestampPrefersCreated(t *testing.T) {
	ics := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//tlc//vtodo//EN",
		"BEGIN:VJOURNAL",
		"UID:log-1@tlc.local",
		"SUMMARY:claimed",
		"DTSTAMP:20260914T080000Z",
		"CREATED:20260502T143000Z",
		"X-TLC-LOG-TASK:task_01h455vb4pex5vsknk084sn02q",
		"END:VJOURNAL",
		"END:VCALENDAR",
		"",
	}, "\r\n")
	res, err := vtodo.ParseVCalendar(strings.NewReader(ics))
	require.NoError(t, err)
	require.Len(t, res.Logs, 1)
	require.Equal(t, time.Date(2026, 5, 2, 14, 30, 0, 0, time.UTC), res.Logs[0].Timestamp.UTC())

	// No CREATED → fall back to DTSTAMP.
	noCreated := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//tlc//vtodo//EN",
		"BEGIN:VJOURNAL",
		"UID:log-1@tlc.local",
		"SUMMARY:claimed",
		"DTSTAMP:20260914T080000Z",
		"X-TLC-LOG-TASK:task_01h455vb4pex5vsknk084sn02q",
		"END:VJOURNAL",
		"END:VCALENDAR",
		"",
	}, "\r\n")
	res2, err := vtodo.ParseVCalendar(strings.NewReader(noCreated))
	require.NoError(t, err)
	require.Len(t, res2.Logs, 1)
	require.Equal(t, time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC), res2.Logs[0].Timestamp.UTC())
}
