package vtodo_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	vstar "hop.top/vstar"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

// fixedTime is a deterministic timestamp used across encode tests.
var fixedTime = time.Date(2026, 5, 2, 14, 30, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

// mustSerialize encodes cal via vtodo.Serialize and fails the test on
// error. Replaces *ics.Calendar.Serialize() from the pre-vstar codec
// era (T-1230).
func mustSerialize(t *testing.T, cal vstar.Calendar) string {
	t.Helper()
	s, err := vtodo.Serialize(cal)
	require.NoError(t, err)
	return s
}

func sampleTask() *core.Task {
	due := fixedTime.Add(24 * time.Hour)
	remind := fixedTime.Add(12 * time.Hour)
	assignee := "alice"
	return &core.Task{
		ID:          "task_01h455vb4pex5vsknk084sn02q",
		Seq:         42,
		Title:       "Replace JWT signer",
		Description: "Rotate to ES256 across services",
		Status:      core.StatusInProgress,
		AssignedTo:  &assignee,
		Tags:        []string{"security", "auth"},
		Reference:   "tlc://hop-top/tlc/task_01h455vb4pex5vsknk084sn02q",
		Effort:      core.EffortM,
		Priority:    core.PriorityP1,
		CreatedAt:   fixedTime,
		UpdatedAt:   fixedTime.Add(time.Hour),
		DueAt:       &due,
		RemindAt:    &remind,
		RRule:       "FREQ=DAILY;INTERVAL=2",
	}
}

func TestBuildVCalendar_Envelope(t *testing.T) {
	cal, err := vtodo.BuildVCalendar(nil, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.Contains(t, out, "BEGIN:VCALENDAR")
	require.Contains(t, out, "END:VCALENDAR")
	require.Contains(t, out, "VERSION:2.0")
	require.Contains(t, out, "PRODID:-//tlc//vtodo//EN")
	require.Contains(t, out, "CALSCALE:GREGORIAN")
	require.Contains(t, out, "METHOD:PUBLISH")
}

func TestBuildVCalendar_TaskFields(t *testing.T) {
	task := sampleTask()
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)

	require.Contains(t, out, "BEGIN:VTODO")
	require.Contains(t, out, "UID:task_01h455vb4pex5vsknk084sn02q@tlc.local")
	require.Contains(t, out, "SUMMARY:Replace JWT signer")
	require.Contains(t, out, "DESCRIPTION:Rotate to ES256 across services")
	require.Contains(t, out, "STATUS:IN-PROCESS")
	require.Contains(t, out, "PRIORITY:3")
	require.Contains(t, out, "URL:tlc://hop-top/tlc/task_01h455vb4pex5vsknk084sn02q")
	require.Contains(t, out, "X-TLC-EFFORT:M")
	require.Contains(t, out, "DUE:20260503T143000Z")
	require.Contains(t, out, "RRULE:FREQ=DAILY;INTERVAL=2")
	// One CATEGORIES property holding a comma-separated list, per RFC
	// 5545 3.8.1.2 -- not one property per tag. The separator comma is
	// backslash-escaped on the wire because the codec escapes every
	// comma in a TEXT value; the parser unescapes it symmetrically, so
	// the round-trip in TestCategories_RoundTrip is what pins meaning.
	require.Contains(t, out, "CATEGORIES:security\\,auth")
	require.NotContains(t, out, "CATEGORIES:security\r\n")
	// Assignee without an email goes into X-TLC-ASSIGNEE.
	require.Contains(t, out, "X-TLC-ASSIGNEE:alice")
	// Email-shaped assignee → ATTENDEE.
	require.NotContains(t, out, "ATTENDEE:")
	// VALARM block carries the absolute reminder.
	require.Contains(t, out, "BEGIN:VALARM")
	require.Contains(t, out, "ACTION:DISPLAY")
	require.Contains(t, out, "TRIGGER;VALUE=DATE-TIME:20260503T023000Z")
	require.Contains(t, out, "END:VALARM")
}

func TestBuildVCalendar_AssigneeEmailGoesToAttendee(t *testing.T) {
	task := sampleTask()
	task.AssignedTo = ptr("alice@example.com")
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.Contains(t, out, "ATTENDEE:mailto:alice@example.com")
	require.NotContains(t, out, "X-TLC-ASSIGNEE:alice@example.com")
}

func TestBuildVCalendar_StatusMapping(t *testing.T) {
	cases := []struct {
		status core.TaskStatus
		want   string
	}{
		{core.StatusTodo, "STATUS:NEEDS-ACTION"},
		{core.StatusInProgress, "STATUS:IN-PROCESS"},
		{core.StatusDone, "STATUS:COMPLETED"},
		{core.StatusSkipped, "STATUS:CANCELLED"},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			task := &core.Task{
				ID:        "task_01h455vb4pex5vsknk084sn02q",
				Title:     "x",
				Status:    tc.status,
				CreatedAt: fixedTime,
			}
			cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
			require.NoError(t, err)
			require.Contains(t, mustSerialize(t, cal), tc.want)
		})
	}
}

func TestBuildVCalendar_PriorityMapping(t *testing.T) {
	cases := []struct {
		priority core.Priority
		want     string
		absent   bool
	}{
		{core.PriorityP0, "PRIORITY:1", false},
		{core.PriorityP1, "PRIORITY:3", false},
		{core.PriorityP2, "PRIORITY:5", false},
		{core.PriorityP3, "PRIORITY:7", false},
		{"", "PRIORITY:", true},
	}
	for _, tc := range cases {
		t.Run(string(tc.priority), func(t *testing.T) {
			task := &core.Task{
				ID:        "task_01h455vb4pex5vsknk084sn02q",
				Title:     "x",
				Status:    core.StatusTodo,
				Priority:  tc.priority,
				CreatedAt: fixedTime,
			}
			cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
			require.NoError(t, err)
			out := mustSerialize(t, cal)
			if tc.absent {
				require.NotContains(t, out, "PRIORITY:")
			} else {
				require.Contains(t, out, tc.want)
			}
		})
	}
}

func TestBuildVCalendar_EmptyRRuleNoLine(t *testing.T) {
	task := sampleTask()
	task.RRule = ""
	task.DueAt = nil
	task.RemindAt = nil
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.NoError(t, err)
	require.NotContains(t, mustSerialize(t, cal), "RRULE:")
}

func TestBuildVCalendar_TrackChildLinks(t *testing.T) {
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
	t2.ID = "task_01h455vb4pex5vsknk084sn0az"
	t2.TrackID = ptr(track.ID)

	cal, err := vtodo.BuildVCalendar([]*core.Task{t1, &t2}, []*core.Track{track}, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	// Track CHILD links to both tasks.
	require.Contains(t, out, "RELATED-TO;RELTYPE=CHILD:task_01h455vb4pex5vsknk084sn02q@tlc.local")
	require.Contains(t, out, "RELATED-TO;RELTYPE=CHILD:task_01h455vb4pex5vsknk084sn0az@tlc.local")
	// Each task PARENT link back.
	require.Contains(t, out, "RELATED-TO;RELTYPE=PARENT:track_01h455vbqkfsn02nk084ksn02q@tlc.local")
	// Track marker for decode classification.
	require.Contains(t, out, "X-TLC-IS-TRACK:TRUE")
	require.Contains(t, out, "X-TLC-TRACK-SLUG:auth-rewrite")
	require.Contains(t, out, "X-TLC-TRACK-TYPE:feature")
}

func TestBuildVCalendar_DependsOn(t *testing.T) {
	blocker := "task_01h455vb4pex5vsknk084sn0az"
	task := sampleTask()
	task.Meta = map[string]interface{}{
		"blocked_by": []string{blocker},
	}
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.NoError(t, err)
	require.Contains(
		t, mustSerialize(t, cal),
		"RELATED-TO;RELTYPE=DEPENDS-ON:task_01h455vb4pex5vsknk084sn0az@tlc.local",
	)
}

func TestBuildVCalendar_LogsGated(t *testing.T) {
	task := sampleTask()
	logs := []*core.LogEntry{{
		TaskID:    task.ID,
		Timestamp: fixedTime,
		By:        "alice",
		Action:    "CLAIMED",
		Note:      "starting work",
	}}

	t.Run("default skips VJOURNAL", func(t *testing.T) {
		cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, logs)
		require.NoError(t, err)
		require.NotContains(t, mustSerialize(t, cal), "BEGIN:VJOURNAL")
	})

	t.Run("WithIncludeLogs emits VJOURNAL", func(t *testing.T) {
		cal, err := vtodo.BuildVCalendar(
			[]*core.Task{task}, nil, logs,
			vtodo.WithIncludeLogs(true),
		)
		require.NoError(t, err)
		out := mustSerialize(t, cal)
		require.Contains(t, out, "BEGIN:VJOURNAL")
		require.Contains(t, out, "X-TLC-LOG-ACTION:CLAIMED")
		require.Contains(t, out, "X-TLC-LOG-BY:alice")
		require.Contains(
			t, out,
			"RELATED-TO;RELTYPE=PARENT:task_01h455vb4pex5vsknk084sn02q@tlc.local",
		)
	})
}

func TestBuildVCalendar_CustomDomainAndProductID(t *testing.T) {
	task := sampleTask()
	cal, err := vtodo.BuildVCalendar(
		[]*core.Task{task}, nil, nil,
		vtodo.WithUIDDomain("calendar.example.com"),
		vtodo.WithProductID("-//example//cal//EN"),
	)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.Contains(t, out, "PRODID:-//example//cal//EN")
	require.Contains(
		t, out,
		"UID:task_01h455vb4pex5vsknk084sn02q@calendar.example.com",
	)
}

// TestBuildVCalendar_LongPRODIDFoldsCleanly proves CALSCALE/METHOD
// inject lands AFTER the folded continuation lines of a long PRODID,
// not between them. RFC 5545 §3.1 folds physical lines >75 octets at
// CRLF + SPACE; a naive "first CRLF after PRODID:" inject corrupted
// the header.
func TestBuildVCalendar_LongPRODIDFoldsCleanly(t *testing.T) {
	// Construct a PRODID well past 75 octets so the encoder folds it.
	longPRODID := "-//example//" + strings.Repeat("VERY-LONG-PRODUCT-NAME-", 5) + "//EN"
	require.Greater(t, len(longPRODID), 75, "PRODID must be long enough to fold")

	cal, err := vtodo.BuildVCalendar(
		nil, nil, nil,
		vtodo.WithProductID(longPRODID),
	)
	require.NoError(t, err)
	out := mustSerialize(t, cal)

	// PRODID must fold (CRLF then SPACE).
	require.Contains(t, out, "PRODID:")
	require.Contains(t, out, "\r\n ", "encoder must fold long PRODID")

	// CALSCALE and METHOD must each be on their own physical lines —
	// preceded by CRLF that is NOT followed by SPACE/TAB.
	require.Contains(t, out, "\r\nCALSCALE:GREGORIAN\r\n")
	require.Contains(t, out, "\r\nMETHOD:PUBLISH\r\n")
}

// TestBuildVCalendar_DTSTAMPIsExportTime proves DTSTAMP carries the
// moment the calendar instance was created (RFC 5545 §3.8.7.2), not the
// entity's creation timestamp. CREATED keeps CreatedAt.
func TestBuildVCalendar_DTSTAMPIsExportTime(t *testing.T) {
	exportAt := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	task := sampleTask() // CreatedAt == fixedTime, a different instant
	require.NotEqual(t, exportAt, task.CreatedAt)

	cal, err := vtodo.BuildVCalendar(
		[]*core.Task{task}, nil, nil,
		vtodo.WithExportTime(exportAt),
	)
	require.NoError(t, err)
	out := mustSerialize(t, cal)

	require.Contains(t, out, "DTSTAMP:20260914T080000Z", "DTSTAMP must be export time")
	require.Contains(t, out, "CREATED:20260502T143000Z", "CREATED must keep CreatedAt")
	require.NotContains(t, out, "DTSTAMP:20260502T143000Z", "DTSTAMP must not mirror CreatedAt")
}

// TestBuildVCalendar_DTSTAMPChangesAcrossExports proves two exports of
// the same unchanged entity carry different DTSTAMPs. The pre-fix code
// pinned DTSTAMP to CreatedAt, so it never moved.
func TestBuildVCalendar_DTSTAMPChangesAcrossExports(t *testing.T) {
	task := sampleTask()
	first := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	second := first.Add(90 * time.Minute)

	cal1, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil, vtodo.WithExportTime(first))
	require.NoError(t, err)
	cal2, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil, vtodo.WithExportTime(second))
	require.NoError(t, err)

	require.Contains(t, mustSerialize(t, cal1), "DTSTAMP:20260914T080000Z")
	require.Contains(t, mustSerialize(t, cal2), "DTSTAMP:20260914T093000Z")
}

// TestBuildVCalendar_DTSTAMPDefaultsToNow proves the export clock
// defaults to wall-clock time when no WithExportTime is supplied.
func TestBuildVCalendar_DTSTAMPDefaultsToNow(t *testing.T) {
	before := time.Now().UTC().Add(-time.Second).Truncate(time.Second)
	cal, err := vtodo.BuildVCalendar([]*core.Task{sampleTask()}, nil, nil)
	require.NoError(t, err)
	after := time.Now().UTC().Add(time.Second)

	todos := cal.Filter(vstar.CompTodo)
	require.Len(t, todos, 1)
	stamp, ok := todos[0].DTSTAMP()
	require.True(t, ok, "DTSTAMP must parse as RFC 5545 form #2")
	require.False(t, stamp.Before(before), "DTSTAMP %v before %v", stamp, before)
	require.False(t, stamp.After(after), "DTSTAMP %v after %v", stamp, after)
}

// TestBuildVCalendar_TrackAndLogDTSTAMPIsExportTime covers the two
// other components that emit DTSTAMP.
func TestBuildVCalendar_TrackAndLogDTSTAMPIsExportTime(t *testing.T) {
	exportAt := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	track := &core.Track{
		ID:        "track_01h455vbqkfsn02nk084ksn02q",
		Title:     "Auth rewrite",
		Status:    core.TrackStatusActive,
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	}
	logs := []*core.LogEntry{{
		TaskID:    "task_01h455vb4pex5vsknk084sn02q",
		Timestamp: fixedTime,
		By:        "alice",
		Action:    "CLAIMED",
	}}
	cal, err := vtodo.BuildVCalendar(
		nil, []*core.Track{track}, logs,
		vtodo.WithIncludeLogs(true),
		vtodo.WithExportTime(exportAt),
	)
	require.NoError(t, err)
	out := mustSerialize(t, cal)

	require.NotContains(t, out, "DTSTAMP:20260502T143000Z")
	require.Equal(t, 2, strings.Count(out, "DTSTAMP:20260914T080000Z"),
		"both VTODO and VJOURNAL carry export-time DTSTAMP:\n%s", out)
	// CREATED keeps the entity timestamps.
	require.Equal(t, 2, strings.Count(out, "CREATED:20260502T143000Z"), out)
}

// TestBuildVCalendar_CreatedEmittedWithoutDTSTAMPCoupling proves CREATED
// is driven solely by CreatedAt: a zero CreatedAt drops CREATED but
// DTSTAMP is still emitted (RFC 5545 requires DTSTAMP on every VTODO).
func TestBuildVCalendar_CreatedEmittedWithoutDTSTAMPCoupling(t *testing.T) {
	exportAt := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	task := sampleTask()
	task.CreatedAt = time.Time{}

	cal, err := vtodo.BuildVCalendar(
		[]*core.Task{task}, nil, nil,
		vtodo.WithExportTime(exportAt),
	)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.NotContains(t, out, "CREATED:")
	require.Contains(t, out, "DTSTAMP:20260914T080000Z")
}
