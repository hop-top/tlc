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
// era.
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
	// One CATEGORIES property per tag (RFC 5545 3.8.1.2 allows either
	// shape). The comma-joined form is not usable: the codec escapes
	// every comma in a TEXT value, so it reaches the wire as
	// `security\,auth`, one category to any RFC 5545 reader.
	require.Contains(t, out, "CATEGORIES:security\r\n")
	require.Contains(t, out, "CATEGORIES:auth\r\n")
	require.NotContains(t, out, "\\,")
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

func TestBuildVCalendar_TrackParentEdges(t *testing.T) {
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
	// Each task carries the PARENT edge, bare (RFC 5545 §3.2.15
	// default); the track carries no CHILD back-reference (spec 02).
	require.Equal(t, 2, strings.Count(out, "\r\nRELATED-TO:track_01h455vbqkfsn02nk084ksn02q@tlc.local\r\n"))
	require.NotContains(t, out, "RELTYPE=CHILD")
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
		// CLAIMED is a status transition and its task is in the export,
		// so the journal is a supersession entry: RELATED-TO is the bare
		// form supersession.Supersedes writes (RFC 5545 §3.2.15 defaults
		// RELTYPE to PARENT; the spec 02 example uses the same shape).
		require.Contains(t, out, "\r\nRELATED-TO:task_01h455vb4pex5vsknk084sn02q@tlc.local\r\n")
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

// TestBuildVCalendar_DTSTAMPIsLastModified pins the deviation from RFC
// 5545 §3.8.7.2 recorded in docs/VSTAR-CONFORMANCE.md: DTSTAMP is the
// entity's last-modified instant, not the export clock, so unchanged
// content keeps its X-VSTAR-HASH from one export to the next. CREATED
// keeps CreatedAt.
func TestBuildVCalendar_DTSTAMPIsLastModified(t *testing.T) {
	exportAt := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	task := sampleTask() // UpdatedAt == fixedTime + 1h, CreatedAt == fixedTime
	require.NotEqual(t, exportAt, task.UpdatedAt)

	cal, err := vtodo.BuildVCalendar(
		[]*core.Task{task}, nil, nil,
		vtodo.WithExportTime(exportAt),
	)
	require.NoError(t, err)
	out := mustSerialize(t, cal)

	require.Contains(t, out, "DTSTAMP:20260502T153000Z", "DTSTAMP must be UpdatedAt")
	require.Contains(t, out, "CREATED:20260502T143000Z", "CREATED must keep CreatedAt")
	require.NotContains(t, out, "DTSTAMP:20260914T080000Z", "DTSTAMP must not be the export clock")
	require.NotContains(t, out, "DTSTAMP:20260502T143000Z", "DTSTAMP must not mirror CreatedAt")

	// The wire check above is satisfied by ANY DTSTAMP in the document,
	// the VALARM's included; pin the VTODO's own through the model.
	todos := cal.Filter(vstar.CompTodo)
	require.Len(t, todos, 1)
	stamp, ok := todos[0].DTSTAMP()
	require.True(t, ok)
	require.Equal(t, task.UpdatedAt, stamp, "the VTODO itself must carry UpdatedAt")
}

// TestBuildVCalendar_DTSTAMPStableAcrossExports proves two exports of
// the same unchanged entity under different export clocks are
// byte-identical, X-VSTAR-HASH included. This is the property the
// DTSTAMP decision buys: the hash detects content changes, not exports.
func TestBuildVCalendar_DTSTAMPStableAcrossExports(t *testing.T) {
	task := sampleTask()
	first := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	second := first.Add(90 * time.Minute)

	cal1, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil, vtodo.WithExportTime(first))
	require.NoError(t, err)
	cal2, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil, vtodo.WithExportTime(second))
	require.NoError(t, err)

	out1, out2 := mustSerialize(t, cal1), mustSerialize(t, cal2)
	require.Equal(t, out1, out2)
	require.Contains(t, out1, "DTSTAMP:20260502T153000Z")
	require.NotContains(t, out1, "20260914")
}

// TestBuildVCalendar_DTSTAMPDefaultsToNow proves a timestamp-less entity
// falls back to the export clock, which defaults to wall-clock time
// when no WithExportTime is supplied.
func TestBuildVCalendar_DTSTAMPDefaultsToNow(t *testing.T) {
	task := sampleTask()
	task.CreatedAt = time.Time{}
	task.UpdatedAt = time.Time{}
	before := time.Now().UTC().Add(-time.Second).Truncate(time.Second)
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.NoError(t, err)
	after := time.Now().UTC().Add(time.Second)

	todos := cal.Filter(vstar.CompTodo)
	require.Len(t, todos, 1)
	stamp, ok := todos[0].DTSTAMP()
	require.True(t, ok, "DTSTAMP must parse as RFC 5545 form #2")
	require.False(t, stamp.Before(before), "DTSTAMP %v before %v", stamp, before)
	require.False(t, stamp.After(after), "DTSTAMP %v after %v", stamp, after)
}

// TestBuildVCalendar_TrackAndLogDTSTAMPFollowEntity covers the two
// other top-level components: a track stamps its UpdatedAt, a journal
// its own Timestamp (a log entry never changes after it is written).
func TestBuildVCalendar_TrackAndLogDTSTAMPFollowEntity(t *testing.T) {
	exportAt := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	track := &core.Track{
		ID:        "track_01h455vbqkfsn02nk084ksn02q",
		Title:     "Auth rewrite",
		Status:    core.TrackStatusActive,
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime.Add(3 * time.Hour),
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

	require.NotContains(t, out, "DTSTAMP:20260914T080000Z", "no export-clock DTSTAMP")
	require.Contains(t, out, "DTSTAMP:20260502T173000Z", "track DTSTAMP is its UpdatedAt")
	require.Contains(t, out, "DTSTAMP:20260502T143000Z", "journal DTSTAMP is its Timestamp")
	require.Equal(t, 2, strings.Count(out, "DTSTAMP:"), out)
	// CREATED keeps the entity timestamps.
	require.Equal(t, 2, strings.Count(out, "CREATED:20260502T143000Z"), out)
}

// TestBuildVCalendar_CreatedEmittedWithoutDTSTAMPCoupling proves CREATED
// is driven solely by CreatedAt: a zero CreatedAt drops CREATED while
// DTSTAMP still lands (RFC 5545 requires it on every VTODO), walking
// the ladder UpdatedAt, then CreatedAt, then the export clock.
func TestBuildVCalendar_CreatedEmittedWithoutDTSTAMPCoupling(t *testing.T) {
	exportAt := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	build := func(created, updated time.Time) string {
		task := sampleTask()
		task.CreatedAt, task.UpdatedAt = created, updated
		cal, err := vtodo.BuildVCalendar(
			[]*core.Task{task}, nil, nil,
			vtodo.WithExportTime(exportAt),
		)
		require.NoError(t, err)
		return mustSerialize(t, cal)
	}

	out := build(time.Time{}, fixedTime.Add(time.Hour))
	require.NotContains(t, out, "CREATED:")
	require.Contains(t, out, "DTSTAMP:20260502T153000Z", "UpdatedAt wins")

	out = build(fixedTime, time.Time{})
	require.Contains(t, out, "CREATED:20260502T143000Z")
	require.Contains(t, out, "DTSTAMP:20260502T143000Z", "CreatedAt backs a zero UpdatedAt")

	out = build(time.Time{}, time.Time{})
	require.NotContains(t, out, "CREATED:")
	require.Contains(t, out, "DTSTAMP:20260914T080000Z", "the export clock is the last resort")
}

// TestBuildVCalendar_CompletedRoleEmitsDoneTriple proves a task whose
// status ROLE is completed goes through helpers.Complete: STATUS,
// COMPLETED and PERCENT-COMPLETE=100 land together, and STATUS is
// written exactly once. Without a completing log entry COMPLETED is
// the last modification.
func TestBuildVCalendar_CompletedRoleEmitsDoneTriple(t *testing.T) {
	task := sampleTask()
	task.Status = core.StatusDone
	task.UpdatedAt = time.Date(2026, 5, 3, 9, 0, 0, 0, time.UTC)

	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)

	require.Equal(t, 1, strings.Count(out, "\r\nSTATUS:"), out)
	require.Contains(t, out, "\r\nSTATUS:COMPLETED\r\n")
	require.Contains(t, out, "\r\nCOMPLETED:20260503T090000Z\r\n")
	require.Contains(t, out, "\r\nPERCENT-COMPLETE:100\r\n")
	require.Contains(t, out, "X-TLC-STATUS:DONE")
}

// TestBuildVCalendar_CompletedAtFromLog proves COMPLETED is the LATEST
// completing transition in the log, not UpdatedAt, whenever the log has
// one, that a reopening transition in between does not count, that
// another task's log is not confused with this one's, and that the log
// is consulted even though VJOURNAL export is off.
func TestBuildVCalendar_CompletedAtFromLog(t *testing.T) {
	task := sampleTask()
	task.Status = core.StatusDone
	task.UpdatedAt = time.Date(2026, 5, 3, 9, 0, 0, 0, time.UTC)
	at := func(h int) time.Time { return time.Date(2026, 5, 2, h, 0, 0, 0, time.UTC) }
	logs := []*core.LogEntry{
		{TaskID: task.ID, Action: string(core.StatusDone), Timestamp: at(18)},
		{TaskID: task.ID, Action: string(core.StatusInProgress), Timestamp: at(19)},
		{TaskID: task.ID, Action: string(core.StatusDone), Timestamp: at(20)},
		{TaskID: "task_01h455vb4pex5vsknk084sn0az", Action: string(core.StatusDone), Timestamp: at(23)},
	}

	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, logs)
	require.NoError(t, err)
	out := mustSerialize(t, cal)

	require.Contains(t, out, "\r\nCOMPLETED:20260502T200000Z\r\n")
	require.NotContains(t, out, "COMPLETED:20260503T090000Z")
	require.NotContains(t, out, "COMPLETED:20260502T230000Z")
	require.NotContains(t, out, "BEGIN:VJOURNAL")
}

// TestBuildVCalendar_OpenRoleHasNoDoneTriple is the inverse: a task that
// is not in a completed-role status carries neither COMPLETED nor
// PERCENT-COMPLETE.
func TestBuildVCalendar_OpenRoleHasNoDoneTriple(t *testing.T) {
	cal, err := vtodo.BuildVCalendar([]*core.Task{sampleTask()}, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.NotContains(t, out, "\r\nCOMPLETED:")
	require.NotContains(t, out, "PERCENT-COMPLETE")
}

// TestBuildVCalendar_AlarmCarriesUIDAndDTSTAMP proves the reminder
// VALARM is built through helpers.NewAbsoluteAlarm: a UID derived from
// its parent's, a DTSTAMP pinned to its parent's rather than the
// constructor's wall clock or the export clock, an absolute TRIGGER in
// the form spec-vstar 03 "TRIGGER conventions" asks for (VALUE=DATE-TIME
// explicit, UTC form #2, no RELATED), and an X-VSTAR-HASH. The dated
// task also carries its auto reminder, second; reminders_test.go
// covers that one.
func TestBuildVCalendar_AlarmCarriesUIDAndDTSTAMP(t *testing.T) {
	exportAt := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	task := sampleTask()
	cal, err := vtodo.BuildVCalendar(
		[]*core.Task{task}, nil, nil,
		vtodo.WithExportTime(exportAt),
	)
	require.NoError(t, err)
	todos := cal.Filter(vstar.CompTodo)
	require.Len(t, todos, 1)
	require.Len(t, todos[0].Sub, 2, "RemindAt alarm, then the auto reminder")
	alarm := todos[0].Sub[0]

	require.Equal(t, vstar.CompAlarm, alarm.Type)
	require.Equal(t, "task_01h455vb4pex5vsknk084sn02q-alarm@tlc.local", alarm.UID())
	stamp, ok := alarm.DTSTAMP()
	require.True(t, ok)
	require.Equal(t, task.UpdatedAt, stamp, "VALARM takes its parent's DTSTAMP")
	trig, ok := alarm.Get("TRIGGER")
	require.True(t, ok)
	require.Equal(t, []vstar.Param{{Name: "VALUE", Value: "DATE-TIME"}}, trig.Params,
		"exactly VALUE=DATE-TIME: RELATED is meaningless on an absolute trigger")
	require.Equal(t, "20260503T023000Z", trig.Value)
	require.True(t, strings.HasSuffix(trig.Value, "Z"), "absolute trigger must be UTC form #2")
	_, ok = alarm.Get("X-VSTAR-HASH")
	require.True(t, ok, "VALARM must carry X-VSTAR-HASH")
}

// TestBuildVCalendar_AlarmUIDFollowsDomain proves the alarm UID is
// derived from the parent UID as emitted, domain override included.
func TestBuildVCalendar_AlarmUIDFollowsDomain(t *testing.T) {
	cal, err := vtodo.BuildVCalendar(
		[]*core.Task{sampleTask()}, nil, nil,
		vtodo.WithUIDDomain("calendar.example.com"),
	)
	require.NoError(t, err)
	todos := cal.Filter(vstar.CompTodo)
	require.Len(t, todos, 1)
	require.Len(t, todos[0].Sub, 2)
	require.Equal(
		t,
		"task_01h455vb4pex5vsknk084sn02q-alarm-auto@calendar.example.com",
		todos[0].Sub[1].UID(),
	)
	require.Equal(
		t,
		"task_01h455vb4pex5vsknk084sn02q-alarm@calendar.example.com",
		todos[0].Sub[0].UID(),
	)
}

// TestBuildVCalendar_JournalOmitsDTSTART pins the decision taken when
// moving to helpers.NewJournal: the constructor also writes DTSTART, but
// a tlc journal's instant travels as CREATED and is not duplicated.
func TestBuildVCalendar_JournalOmitsDTSTART(t *testing.T) {
	logs := []*core.LogEntry{{
		TaskID:    "task_01h455vb4pex5vsknk084sn02q",
		Timestamp: fixedTime,
		By:        "alice",
		Action:    "CLAIMED",
		Note:      "starting work",
	}}
	cal, err := vtodo.BuildVCalendar(nil, nil, logs, vtodo.WithIncludeLogs(true))
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.Contains(t, out, "BEGIN:VJOURNAL")
	require.NotContains(t, out, "DTSTART")
	require.Contains(t, out, "CREATED:20260502T143000Z")
}

// TestBuildVCalendar_EmptyIDRejected proves an entity without an ID is
// refused rather than exported with an empty UID: every V* component
// must carry one, and the helpers constructors enforce it.
func TestBuildVCalendar_EmptyIDRejected(t *testing.T) {
	task := sampleTask()
	task.ID = ""
	_, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.ErrorIs(t, err, vstar.ErrMissingUID)

	track := &core.Track{Title: "no id", Status: core.TrackStatusActive}
	_, err = vtodo.BuildVCalendar(nil, []*core.Track{track}, nil)
	require.ErrorIs(t, err, vstar.ErrMissingUID)
}
