package vtodo_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	vstar "hop.top/vstar"
	"hop.top/vstar/helpers"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

// oneTodo builds a calendar for one task and returns its VTODO.
func oneTodo(t *testing.T, task *core.Task, opts ...vtodo.Option) (vstar.Calendar, vstar.Component) {
	t.Helper()
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil, opts...)
	require.NoError(t, err)
	requireValidExport(t, cal)
	todos := cal.Filter(vstar.CompTodo)
	require.Len(t, todos, 1)
	return cal, todos[0]
}

// alarmTriggers lists the TRIGGER wire lines of every VALARM under c.
func alarmTriggers(t *testing.T, c vstar.Component) []string {
	t.Helper()
	out := make([]string, 0, len(c.Sub))
	for _, sub := range c.Sub {
		require.Equal(t, vstar.CompAlarm, sub.Type)
		p, ok := sub.Get("TRIGGER")
		require.True(t, ok, "VALARM %s has no TRIGGER", sub.UID())
		line := "TRIGGER"
		for _, par := range p.Params {
			line += ";" + par.Name + "=" + par.Value
		}
		out = append(out, line+":"+p.Value)
	}
	return out
}

// --- date-only DUE -------------------------------------------------

// TestDateOnly_DueRoundTrip: a task flagged due_date_only exports
// DUE;VALUE=DATE, never a midnight DATE-TIME, and decodes back to the
// same DueAt and flag; the second export is byte-identical.
func TestDateOnly_DueRoundTrip(t *testing.T) {
	task := sampleTask()
	task.RemindAt = nil
	task.DueAt = ptr(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC))
	task.Meta = map[string]interface{}{vtodo.MetaDueDateOnly: true}

	cal, todo := oneTodo(t, task, vtodo.WithExportTime(fixedTime))
	out := mustSerialize(t, cal)
	require.Contains(t, out, "\r\nDUE;VALUE=DATE:20260515\r\n")
	require.NotContains(t, out, "20260515T000000Z", "a DATE is never promoted to midnight")
	require.True(t, todo.IsDateOnly("DUE"))
	d, ok := todo.DUEDate()
	require.True(t, ok)
	require.Equal(t, vstar.Date{Year: 2026, Month: time.May, Day: 15}, d)
	require.NotContains(t, out, vtodo.XPropMeta, "the flag is derived from VALUE=DATE, not carried in X-TLC-META")

	res, err := vtodo.ParseVCalendar(strings.NewReader(out))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	got := res.Tasks[0]
	require.NotNil(t, got.DueAt)
	require.Equal(t, *task.DueAt, got.DueAt.UTC())
	require.Equal(t, true, got.Meta[vtodo.MetaDueDateOnly])

	again, err := vtodo.BuildVCalendar([]*core.Task{got}, nil, nil, vtodo.WithExportTime(fixedTime))
	require.NoError(t, err)
	require.Equal(t, out, mustSerialize(t, again))
}

// TestDateOnly_ForeignLowercaseValueParam: RFC 5545 §3.2 makes both
// the parameter name and a value-type token case-insensitive, so a
// foreign DUE;VALUE=date decodes the same way.
func TestDateOnly_ForeignLowercaseValueParam(t *testing.T) {
	got := parseOneTask(t, calWith(
		"DTSTAMP:20260502T143000Z",
		"DUE;value=date:20260515",
	))
	require.NotNil(t, got.DueAt)
	require.Equal(t, time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC), got.DueAt.UTC())
	require.Equal(t, true, got.Meta[vtodo.MetaDueDateOnly])
}

// TestDateOnly_DistinctFromMidnightDateTime: DUE:20260515T000000Z is
// an instant, not a day. It decodes without the flag and re-exports as
// the DATE-TIME it was.
func TestDateOnly_DistinctFromMidnightDateTime(t *testing.T) {
	got := parseOneTask(t, calWith(
		"DTSTAMP:20260502T143000Z",
		"DUE:20260515T000000Z",
	))
	require.NotNil(t, got.DueAt)
	require.Equal(t, time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC), got.DueAt.UTC())
	_, flagged := got.Meta[vtodo.MetaDueDateOnly]
	require.False(t, flagged, "a midnight instant is not a date-only value")

	cal, err := vtodo.BuildVCalendar([]*core.Task{got}, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.Contains(t, out, "\r\nDUE:20260515T000000Z\r\n")
	require.NotContains(t, out, "VALUE=DATE:")
}

// TestDateOnly_UntaggedDateIsNotPromoted: an eight-octet DUE without
// VALUE=DATE is a malformed DATE-TIME (the default value type), not a
// DATE. The library refuses it in both readers and so does tlc. This
// is the assertion the removed hand-rolled fallback would fail: it
// read the bare digits as midnight UTC.
func TestDateOnly_UntaggedDateIsNotPromoted(t *testing.T) {
	got := parseOneTask(t, calWith(
		"DTSTAMP:20260502T143000Z",
		"DUE:20260515",
	))
	require.Nil(t, got.DueAt, "an untagged YYYYMMDD must not decode")
	_, flagged := got.Meta[vtodo.MetaDueDateOnly]
	require.False(t, flagged)
}

// TestDateOnly_TrackRoundTrip: the track VTODO takes the same path
// through Track.Meta.
func TestDateOnly_TrackRoundTrip(t *testing.T) {
	track := hashTrack()
	track.DueAt = ptr(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	track.Meta = map[string]any{vtodo.MetaDueDateOnly: true}
	cal, err := vtodo.BuildVCalendar(nil, []*core.Track{track}, nil, vtodo.WithExportTime(fixedTime))
	require.NoError(t, err)
	requireValidExport(t, cal)
	out := mustSerialize(t, cal)
	require.Contains(t, out, "\r\nDUE;VALUE=DATE:20260601\r\n")

	res, err := vtodo.ParseVCalendar(strings.NewReader(out))
	require.NoError(t, err)
	require.Len(t, res.Tracks, 1)
	require.NotNil(t, res.Tracks[0].DueAt)
	require.Equal(t, *track.DueAt, res.Tracks[0].DueAt.UTC())
	require.Equal(t, true, res.Tracks[0].Meta[vtodo.MetaDueDateOnly])
}

// --- relative triggers ---------------------------------------------

// TestRelativeTrigger_ResolvesAgainstDue: RELATED=END on a VTODO
// anchors to DUE. DTSTART is present too, so an implementation that
// ignored RELATED would land 15 minutes before the wrong instant.
func TestRelativeTrigger_ResolvesAgainstDue(t *testing.T) {
	got := parseOneTask(t, calWith(
		"DTSTAMP:20260502T143000Z",
		"DTSTART:20990601T090000Z",
		"DUE:20990601T170000Z",
		"BEGIN:VALARM",
		"ACTION:DISPLAY",
		"TRIGGER;RELATED=END:-PT15M",
		"END:VALARM",
	))
	require.NotNil(t, got.RemindAt)
	require.Equal(t, time.Date(2099, 6, 1, 16, 45, 0, 0, time.UTC), got.RemindAt.UTC())
}

// TestRelativeTrigger_ResolvesAgainstDtstart: no RELATED means START
// (RFC 5545 §3.2.14), the shape Apple Reminders and Thunderbird emit.
// DUE is present too, so the two anchors are told apart.
func TestRelativeTrigger_ResolvesAgainstDtstart(t *testing.T) {
	for _, trigger := range []string{
		"TRIGGER:-PT15M",
		"TRIGGER;RELATED=START:-PT15M",
		"TRIGGER;VALUE=DURATION:-PT15M",
	} {
		t.Run(trigger, func(t *testing.T) {
			got := parseOneTask(t, calWith(
				"DTSTAMP:20260502T143000Z",
				"DTSTART:20990601T090000Z",
				"DUE:20990601T170000Z",
				"BEGIN:VALARM",
				"ACTION:DISPLAY",
				trigger,
				"END:VALARM",
			))
			require.NotNil(t, got.RemindAt)
			require.Equal(t, time.Date(2099, 6, 1, 8, 45, 0, 0, time.UTC), got.RemindAt.UTC())
		})
	}
}

// TestRelativeTrigger_DateOnlyAnchor: the library will not resolve an
// offset against a DATE; tlc applies it to the day's midnight UTC, the
// same reading DueAt gets, so the reminder lands where the task is due.
func TestRelativeTrigger_DateOnlyAnchor(t *testing.T) {
	got := parseOneTask(t, calWith(
		"DTSTAMP:20260502T143000Z",
		"DUE;VALUE=DATE:20990601",
		"BEGIN:VALARM",
		"ACTION:DISPLAY",
		"TRIGGER;RELATED=END:-PT15M",
		"END:VALARM",
	))
	require.NotNil(t, got.RemindAt)
	require.Equal(t, time.Date(2099, 5, 31, 23, 45, 0, 0, time.UTC), got.RemindAt.UTC())
}

// TestRelativeTrigger_NoAnchorIsDropped: a relative trigger whose
// anchor the VTODO lacks resolves to nothing, not to year 1.
func TestRelativeTrigger_NoAnchorIsDropped(t *testing.T) {
	got := parseOneTask(t, calWith(
		"DTSTAMP:20260502T143000Z",
		"BEGIN:VALARM",
		"ACTION:DISPLAY",
		"TRIGGER:-PT15M",
		"END:VALARM",
	))
	require.Nil(t, got.RemindAt)
}

// --- several VALARMs -----------------------------------------------

// TestReminders_MultipleAlarmsPreserved: every VALARM is kept. The
// earliest future one is RemindAt, the rest travel in
// Meta["reminders"] sorted, and re-export writes each back as its own
// VALARM in time order. A second cycle is byte-identical.
func TestReminders_MultipleAlarmsPreserved(t *testing.T) {
	// Wire order is deliberately not time order.
	got := parseOneTask(t, calWith(
		"DTSTAMP:20260502T143000Z",
		"DUE:20990601T170000Z",
		"BEGIN:VALARM",
		"ACTION:DISPLAY",
		"TRIGGER;VALUE=DATE-TIME:20990601T120000Z",
		"END:VALARM",
		"BEGIN:VALARM",
		"ACTION:DISPLAY",
		"TRIGGER;RELATED=END:-PT1H",
		"END:VALARM",
		"BEGIN:VALARM",
		"ACTION:DISPLAY",
		"TRIGGER;VALUE=DATE-TIME:20990601T080000Z",
		"END:VALARM",
	))
	require.NotNil(t, got.RemindAt)
	require.Equal(t, time.Date(2099, 6, 1, 8, 0, 0, 0, time.UTC), got.RemindAt.UTC(), "earliest wins")
	require.Equal(t,
		[]string{"2099-06-01T12:00:00Z", "2099-06-01T16:00:00Z"},
		got.Meta[vtodo.MetaReminders],
		"the others are kept, resolved, in time order")

	got.UpdatedAt = fixedTime
	cal, todo := oneTodo(t, got, vtodo.WithExportTime(fixedTime))
	require.Equal(t, []string{
		"TRIGGER;VALUE=DATE-TIME:20990601T080000Z",
		"TRIGGER;VALUE=DATE-TIME:20990601T120000Z",
		"TRIGGER;VALUE=DATE-TIME:20990601T160000Z",
		"TRIGGER;RELATED=END:-PT12H",
	}, alarmTriggers(t, todo))
	require.Equal(t, "task_01h455vb4pex5vsknk084sn02q-alarm@tlc.local", todo.Sub[0].UID())
	require.Equal(t, "task_01h455vb4pex5vsknk084sn02q-alarm-20990601T120000Z@tlc.local", todo.Sub[1].UID())
	require.Equal(t, "task_01h455vb4pex5vsknk084sn02q-alarm-20990601T160000Z@tlc.local", todo.Sub[2].UID())
	require.Equal(t, "task_01h455vb4pex5vsknk084sn02q-alarm-auto@tlc.local", todo.Sub[3].UID())
	out := mustSerialize(t, cal)
	require.NotContains(t, out, vtodo.XPropMeta, "reminders is derived from the VALARMs, never in X-TLC-META")

	res, err := vtodo.ParseVCalendar(strings.NewReader(out))
	require.NoError(t, err)
	require.Empty(t, res.Warnings)
	require.Len(t, res.Tasks, 1)
	again, err := vtodo.BuildVCalendar(res.Tasks, nil, nil, vtodo.WithExportTime(fixedTime))
	require.NoError(t, err)
	require.Equal(t, out, mustSerialize(t, again))
}

// TestReminders_EarliestFutureIsNext: a reminder already in the past
// is not the one tlc fires next; the earliest future one is RemindAt
// and the past one is still preserved.
func TestReminders_EarliestFutureIsNext(t *testing.T) {
	got := parseOneTask(t, calWith(
		"DTSTAMP:20260502T143000Z",
		"BEGIN:VALARM",
		"ACTION:DISPLAY",
		"TRIGGER;VALUE=DATE-TIME:20200101T080000Z",
		"END:VALARM",
		"BEGIN:VALARM",
		"ACTION:DISPLAY",
		"TRIGGER;VALUE=DATE-TIME:20990601T080000Z",
		"END:VALARM",
	))
	require.NotNil(t, got.RemindAt)
	require.Equal(t, time.Date(2099, 6, 1, 8, 0, 0, 0, time.UTC), got.RemindAt.UTC())
	require.Equal(t, []string{"2020-01-01T08:00:00Z"}, got.Meta[vtodo.MetaReminders])
}

// TestReminders_DuplicateInstantsCollapse: two VALARMs firing at the
// same instant, in different forms, are one reminder.
func TestReminders_DuplicateInstantsCollapse(t *testing.T) {
	got := parseOneTask(t, calWith(
		"DTSTAMP:20260502T143000Z",
		"DUE:20990601T170000Z",
		"BEGIN:VALARM",
		"ACTION:DISPLAY",
		"TRIGGER;VALUE=DATE-TIME:20990601T160000Z",
		"END:VALARM",
		"BEGIN:VALARM",
		"ACTION:DISPLAY",
		"TRIGGER;RELATED=END:-PT1H",
		"END:VALARM",
	))
	require.NotNil(t, got.RemindAt)
	require.Equal(t, time.Date(2099, 6, 1, 16, 0, 0, 0, time.UTC), got.RemindAt.UTC())
	_, more := got.Meta[vtodo.MetaReminders]
	require.False(t, more)
}

// --- the auto reminder ---------------------------------------------

// TestAutoRemind_EmittedAsRelativeTrigger: a dated task without a
// reminder of its own still carries one VALARM, the derived reminder,
// as TRIGGER;RELATED=END:-PT12H with the spec-conformant omissions (no
// VALUE=DURATION, no RELATED=START), a UID, its parent's DTSTAMP and a
// hash; helpers.AlarmFiresAt resolves it to exactly Task.AutoRemindAt.
func TestAutoRemind_EmittedAsRelativeTrigger(t *testing.T) {
	task := sampleTask()
	task.RemindAt = nil
	cal, todo := oneTodo(t, task, vtodo.WithExportTime(fixedTime))

	require.Equal(t, []string{"TRIGGER;RELATED=END:-PT12H"}, alarmTriggers(t, todo))
	alarm := todo.Sub[0]
	require.Equal(t, "task_01h455vb4pex5vsknk084sn02q-alarm-auto@tlc.local", alarm.UID())
	stamp, ok := alarm.DTSTAMP()
	require.True(t, ok)
	require.Equal(t, task.UpdatedAt, stamp)
	requireVerifies(t, alarm, "auto reminder")

	fires, err := helpers.AlarmFiresAt(alarm, todo, cal)
	require.NoError(t, err)
	require.NotNil(t, task.AutoRemindAt())
	require.Equal(t, *task.AutoRemindAt(), fires, "the wire offset must be the one core derives")
}

// TestAutoRemind_DecodesWithoutDoubleStoring: tlc's own auto reminder
// comes back as nothing on the model: RemindAt stays nil and no
// Meta["reminders"] appears, because AutoRemindAt derives it from DUE
// again. A foreign -PT12H from DUE reads the same way.
func TestAutoRemind_DecodesWithoutDoubleStoring(t *testing.T) {
	task := sampleTask()
	task.RemindAt = nil
	res := roundTrip(t, []*core.Task{task}, nil, nil)
	require.Len(t, res.Tasks, 1)
	got := res.Tasks[0]
	require.Nil(t, got.RemindAt)
	_, more := got.Meta[vtodo.MetaReminders]
	require.False(t, more)
	require.Equal(t, *task.AutoRemindAt(), *got.AutoRemindAt())

	foreign := parseOneTask(t, calWith(
		"DTSTAMP:20260502T143000Z",
		"DUE:20990601T170000Z",
		"BEGIN:VALARM",
		"ACTION:DISPLAY",
		"TRIGGER;RELATED=END:-P0DT12H",
		"END:VALARM",
	))
	require.Nil(t, foreign.RemindAt, "an authored twelve-hours-before-due is the derived reminder")
}

// TestAutoRemind_ExplicitAbsoluteAtSameInstantIsKept: an absolute
// reminder a user set that happens to fall 12 hours before DUE is not
// the derived one; it round-trips as RemindAt beside the auto alarm.
func TestAutoRemind_ExplicitAbsoluteAtSameInstantIsKept(t *testing.T) {
	task := sampleTask() // RemindAt == DueAt - 12h
	require.Equal(t, *task.AutoRemindAt(), task.RemindAt.UTC())
	res := roundTrip(t, []*core.Task{task}, nil, nil)
	require.Len(t, res.Tasks, 1)
	require.NotNil(t, res.Tasks[0].RemindAt)
	require.Equal(t, task.RemindAt.UTC(), res.Tasks[0].RemindAt.UTC())
}

// TestAutoRemind_NoAutoRemindSuppresses: NoAutoRemind means no derived
// reminder, so no VALARM for it; an undated task has none either.
func TestAutoRemind_NoAutoRemindSuppresses(t *testing.T) {
	task := sampleTask()
	task.RemindAt = nil
	task.NoAutoRemind = true
	_, todo := oneTodo(t, task)
	require.Empty(t, todo.Sub)

	undated := sampleTask()
	undated.RemindAt = nil
	undated.DueAt = nil
	_, todo = oneTodo(t, undated)
	require.Empty(t, todo.Sub)
}

// TestAutoRemind_DateOnlyDueRoundTrips: the derived reminder on a
// date-only task resolves against the day's midnight on the way back,
// so it is recognized and not stored.
func TestAutoRemind_DateOnlyDueRoundTrips(t *testing.T) {
	task := sampleTask()
	task.RemindAt = nil
	task.DueAt = ptr(time.Date(2099, 6, 1, 0, 0, 0, 0, time.UTC))
	task.Meta = map[string]interface{}{vtodo.MetaDueDateOnly: true}
	res := roundTrip(t, []*core.Task{task}, nil, nil)
	require.Len(t, res.Tasks, 1)
	require.Nil(t, res.Tasks[0].RemindAt)
	require.Equal(t, *task.AutoRemindAt(), *res.Tasks[0].AutoRemindAt())
}
