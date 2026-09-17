package vtodo_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	vstar "hop.top/vstar"
	"hop.top/vstar/hashing"
	"hop.top/vstar/supersession"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

const (
	ledgerTaskUID = "task_01h455vb4pex5vsknk084sn02q@tlc.local"
	claimedUID    = "journal:status:" + ledgerTaskUID + ":20260502T143000Z"
)

// logAt builds one log entry on the sample task.
func logAt(action string, at time.Time) *core.LogEntry {
	return &core.LogEntry{
		TaskID:    "task_01h455vb4pex5vsknk084sn02q",
		Timestamp: at,
		By:        "alice",
		Action:    action,
		Note:      "note for " + action,
	}
}

// buildLedger exports the sample task with the given log entries and
// returns the calendar and its single VJOURNAL.
func buildLedger(t *testing.T, logs []*core.LogEntry, opts ...vtodo.Option) (vstar.Calendar, vstar.Component) {
	t.Helper()
	opts = append(opts, vtodo.WithIncludeLogs(true))
	cal, err := vtodo.BuildVCalendar([]*core.Task{sampleTask()}, nil, logs, opts...)
	require.NoError(t, err)
	journals := cal.Filter(vstar.CompJournal)
	require.Len(t, journals, 1)
	return cal, journals[0]
}

// TestSupersession_StatusTransitionShape: a status transition is the
// spec 02 supersession entry, all five properties present, built
// against the task in the same calendar, still carrying tlc's own
// journal properties, hashed last, and projectable by the library.
func TestSupersession_StatusTransitionShape(t *testing.T) {
	cal, j := buildLedger(t, []*core.LogEntry{logAt(core.ActionClaimed, fixedTime)})
	requireValidExport(t, cal)

	require.Equal(t, claimedUID, j.UID())
	require.Equal(t, "20260502T143000Z", j.DTSTAMPRaw())
	rel, ok := j.Get("RELATED-TO")
	require.True(t, ok)
	require.Equal(t, ledgerTaskUID, rel.Value)
	require.Empty(t, rel.Params, "Supersedes writes the bare form; RFC 5545 defaults RELTYPE to PARENT")
	cat, ok := j.Get("CATEGORIES")
	require.True(t, ok)
	require.Equal(t, supersession.CategoryStatusSupersession, cat.Value)
	eff, ok := j.Get(supersession.PropEffectiveStatus)
	require.True(t, ok)
	require.Equal(t, string(vstar.TodoInProcess), eff.Value)
	requireVerifies(t, j, "supersession journal")
	require.Equal(t, hashing.XVSTARHashProperty, j.Props[len(j.Props)-1].Name, "hash closes the block")

	// tlc's own properties are not lost to the shape.
	require.Equal(t, vtodo.ConceptStatus, conceptOf(t, j))
	for name, want := range map[string]string{
		vtodo.XPropLogAction: core.ActionClaimed,
		vtodo.XPropLogBy:     "alice",
		vtodo.XPropLogTaskID: "task_01h455vb4pex5vsknk084sn02q",
		"SUMMARY":            "note for CLAIMED",
		"CREATED":            "20260502T143000Z",
	} {
		p, ok := j.Get(name)
		require.True(t, ok, name)
		require.Equal(t, want, p.Value, name)
	}
	require.Equal(t, 1, len(j.GetAll("RELATED-TO")), "one edge, not one per shape")

	// The library projects the ledger onto the task.
	task, ok := cal.Find(ledgerTaskUID)
	require.True(t, ok)
	got, ok := supersession.Superseded(task, cal.Components)
	require.True(t, ok)
	require.Equal(t, string(vstar.TodoInProcess), got)
}

// TestSupersession_NonStatusKeepsPlainShape: an observation is history,
// not a ledger entry; it keeps the log-… UID, with the same bare edge.
func TestSupersession_NonStatusKeepsPlainShape(t *testing.T) {
	cal, j := buildLedger(t, []*core.LogEntry{logAt(core.ActionComment, fixedTime)})
	requireValidExport(t, cal)

	require.Equal(t, "log-task_01h455vb4pex5vsknk084sn02q-COMMENT-20260502T143000Z@tlc.local", j.UID())
	_, ok := j.Get("CATEGORIES")
	require.False(t, ok)
	_, ok = j.Get(supersession.PropEffectiveStatus)
	require.False(t, ok)
	rel, ok := j.Get("RELATED-TO")
	require.True(t, ok)
	require.Equal(t, ledgerTaskUID, rel.Value)
	require.Empty(t, rel.Params, "the plain shape is the same bare edge; spec 02 omits RELTYPE=PARENT")
	require.Equal(t, vtodo.ConceptObservation, conceptOf(t, j))
	task, ok := cal.Find(ledgerTaskUID)
	require.True(t, ok)
	_, ok = supersession.Superseded(task, cal.Components)
	require.False(t, ok, "nothing supersedes the task")
}

// TestSupersession_TargetAbsentKeepsPlainShape: a status transition
// whose task is not in the export would be an orphan supersession entry
// (VS031, blocking), so it travels as history instead.
func TestSupersession_TargetAbsentKeepsPlainShape(t *testing.T) {
	cal, err := vtodo.BuildVCalendar(nil, nil,
		[]*core.LogEntry{logAt(core.ActionClaimed, fixedTime)}, vtodo.WithIncludeLogs(true))
	require.NoError(t, err)
	requireValidExport(t, cal)
	j := cal.Filter(vstar.CompJournal)[0]
	require.Equal(t, "log-task_01h455vb4pex5vsknk084sn02q-CLAIMED-20260502T143000Z@tlc.local", j.UID())
	_, ok := j.Get(supersession.PropEffectiveStatus)
	require.False(t, ok)
	require.Equal(t, vtodo.ConceptStatus, conceptOf(t, j), "the concept is about the action, not the shape")
}

// TestSupersession_EffectiveStatusVocabulary pins the value written as
// X-VSTAR-EFFECTIVE-STATUS: the RFC 5545 VTODO STATUS set, through the
// role each transition lands the task in, the projection the task
// builder makes for STATUS.
func TestSupersession_EffectiveStatusVocabulary(t *testing.T) {
	custom := vtodo.WithStatusDefinitions(customStatuses())
	cases := []struct {
		action string
		want   vstar.TodoStatus
		opts   []vtodo.Option
	}{
		{core.ActionClaimed, vstar.TodoInProcess, nil},
		{core.ActionReleased, vstar.TodoNeedsAction, nil},
		{core.ActionRetry, vstar.TodoNeedsAction, nil},
		{"REOPENED", vstar.TodoNeedsAction, nil},
		{core.ActionBlocked, vstar.TodoNeedsAction, nil},
		{core.ActionUnblocked, vstar.TodoNeedsAction, nil},
		{core.ActionDone, vstar.TodoCompleted, nil},
		{core.ActionSkipped, vstar.TodoCancelled, nil},
		{"claimed", vstar.TodoInProcess, nil},
		// A configured status name resolves through its role.
		{string(core.StatusInProgress), vstar.TodoInProcess, nil},
		{string(core.StatusTodo), vstar.TodoNeedsAction, nil},
		{"in_review", vstar.TodoInProcess, []vtodo.Option{custom}},
		{"SHIPPED", vstar.TodoCompleted, []vtodo.Option{custom}},
		{"DROPPED", vstar.TodoCancelled, []vtodo.Option{custom}},
		{"BACKLOG", vstar.TodoNeedsAction, []vtodo.Option{custom}},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			_, j := buildLedger(t, []*core.LogEntry{logAt(tc.action, fixedTime)}, tc.opts...)
			eff, ok := j.Get(supersession.PropEffectiveStatus)
			require.True(t, ok, "%s must be a supersession entry", tc.action)
			require.Equal(t, string(tc.want), eff.Value)
		})
	}
}

// TestSupersession_DecisionIsNotALedgerEntry: APPROVED records a verdict
// (decision class), not a status transition, so it keeps the plain
// shape even though it completes the task.
func TestSupersession_DecisionIsNotALedgerEntry(t *testing.T) {
	_, j := buildLedger(t, []*core.LogEntry{logAt(core.ActionApproved, fixedTime)})
	_, ok := j.Get(supersession.PropEffectiveStatus)
	require.False(t, ok)
	require.Equal(t, vtodo.ConceptDecision, conceptOf(t, j))
}

// TestSupersession_LatestWinsOnOwnLedger: two transitions project to
// the later one whatever the export order, because DTSTAMP is the log
// instant.
func TestSupersession_LatestWinsOnOwnLedger(t *testing.T) {
	cal, err := vtodo.BuildVCalendar([]*core.Task{sampleTask()}, nil, []*core.LogEntry{
		logAt(core.ActionDone, fixedTime.Add(2*time.Hour)),
		logAt(core.ActionClaimed, fixedTime),
	}, vtodo.WithIncludeLogs(true))
	require.NoError(t, err)
	requireValidExport(t, cal)
	task, ok := cal.Find(ledgerTaskUID)
	require.True(t, ok)
	got, ok := supersession.Superseded(task, cal.Components)
	require.True(t, ok)
	require.Equal(t, string(vstar.TodoCompleted), got)
}

// foreignLedger is a calendar another producer wrote: an original VTODO
// never mutated (STATUS still NEEDS-ACTION, no X-TLC-STATUS) and two
// supersession journals out of order, the later one completing it.
func foreignLedger(extraTodoLines ...string) string {
	lines := make([]string, 0, 8+len(extraTodoLines)+17)
	lines = append(
		lines,
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//other//ledger//EN",
		"BEGIN:VTODO",
		"UID:"+ledgerTaskUID,
		"DTSTAMP:20260502T120000Z",
		"SUMMARY:Foreign task",
		"STATUS:NEEDS-ACTION",
	)
	lines = append(lines, extraTodoLines...)
	lines = append(
		lines,
		"END:VTODO",
		"BEGIN:VJOURNAL",
		"UID:journal:status:"+ledgerTaskUID+":20260502T180000Z",
		"DTSTAMP:20260502T180000Z",
		"RELATED-TO:"+ledgerTaskUID,
		"CATEGORIES:status-supersession",
		"X-VSTAR-EFFECTIVE-STATUS:COMPLETED",
		"END:VJOURNAL",
		"BEGIN:VJOURNAL",
		"UID:journal:status:"+ledgerTaskUID+":20260502T140000Z",
		"DTSTAMP:20260502T140000Z",
		"RELATED-TO:"+ledgerTaskUID,
		"CATEGORIES:status-supersession",
		"X-VSTAR-EFFECTIVE-STATUS:IN-PROCESS",
		"END:VJOURNAL",
		"END:VCALENDAR",
		"",
	)
	return strings.Join(lines, "\r\n")
}

// TestSupersession_ForeignLedgerSetsStatus: on a foreign ledger the
// VTODO's own STATUS is the never-mutated original and the latest
// supersession entry is the truth.
func TestSupersession_ForeignLedgerSetsStatus(t *testing.T) {
	res, err := vtodo.ParseVCalendar(strings.NewReader(foreignLedger()))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	require.Equal(t, core.StatusDone, res.Tasks[0].Status, "latest ledger entry (COMPLETED) wins over STATUS:NEEDS-ACTION")
	require.Len(t, res.Logs, 2)
}

// TestSupersession_OwnStatusNameBeatsLedger: a VTODO tlc wrote carries
// X-TLC-STATUS with its current status; that snapshot is authoritative
// and the ledger behind it is history.
func TestSupersession_OwnStatusNameBeatsLedger(t *testing.T) {
	res, err := vtodo.ParseVCalendar(strings.NewReader(foreignLedger("X-TLC-STATUS:IN_PROGRESS")))
	require.NoError(t, err)
	require.Equal(t, core.StatusInProgress, res.Tasks[0].Status)
}

// TestSupersession_UnknownVocabularyFallsThrough: an effective status
// outside the RFC 5545 VTODO set is opaque; the VTODO's STATUS decides.
func TestSupersession_UnknownVocabularyFallsThrough(t *testing.T) {
	ics := strings.Replace(foreignLedger(),
		"X-VSTAR-EFFECTIVE-STATUS:COMPLETED", "X-VSTAR-EFFECTIVE-STATUS:shipped", 1)
	res, err := vtodo.ParseVCalendar(strings.NewReader(ics))
	require.NoError(t, err)
	require.Equal(t, core.StatusTodo, res.Tasks[0].Status)
}

// TestSupersession_ReExportPreservesEffectiveStatus: a foreign
// supersession journal (no X-TLC-LOG-ACTION) keeps its effective status
// through decode and re-export, as a supersession entry, and the
// preserved value never leaks into X-TLC-META.
func TestSupersession_ReExportPreservesEffectiveStatus(t *testing.T) {
	res, err := vtodo.ParseVCalendar(strings.NewReader(foreignLedger()))
	require.NoError(t, err)
	require.Len(t, res.Logs, 2)
	require.Equal(t, "COMPLETED", res.Logs[0].Meta[vtodo.MetaEffectiveStatusKey])
	require.Equal(t, "IN-PROCESS", res.Logs[1].Meta[vtodo.MetaEffectiveStatusKey])
	require.Empty(t, res.Logs[0].Action)

	cal, err := vtodo.BuildVCalendar(res.Tasks, nil, res.Logs, vtodo.WithIncludeLogs(true))
	require.NoError(t, err)
	requireValidExport(t, cal)
	out := mustSerialize(t, cal)
	require.Contains(t, out, "X-VSTAR-EFFECTIVE-STATUS:COMPLETED")
	require.Contains(t, out, "X-VSTAR-EFFECTIVE-STATUS:IN-PROCESS")
	require.Equal(t, 2, strings.Count(out, "CATEGORIES:status-supersession"))
	// The UID folds across wire lines, so it is checked on the model.
	_, ok := cal.Find("journal:status:" + ledgerTaskUID + ":20260502T180000Z")
	require.True(t, ok, "the supersession UID scheme is re-derived from task UID and instant")
	require.NotContains(t, out, "X-TLC-META", "effective_status is a derived key")
	require.NotContains(t, out, "effective_status")
	journals := cal.Filter(vstar.CompJournal)
	for _, j := range journals {
		require.Equal(t, vtodo.ConceptStatus, conceptOf(t, j))
	}
	// And the ledger still projects the same truth.
	task, ok := cal.Find(ledgerTaskUID)
	require.True(t, ok)
	got, ok := supersession.Superseded(task, cal.Components)
	require.True(t, ok)
	require.Equal(t, "COMPLETED", got)
}

// TestSupersession_OwnRoundTripIsMetaClean: tlc's own supersession
// journals decode to the Meta they were exported from, because the
// action reproduces the wire value, and a second export is
// byte-identical.
func TestSupersession_OwnRoundTripIsMetaClean(t *testing.T) {
	logs := []*core.LogEntry{
		logAt(core.ActionClaimed, fixedTime),
		logAt(core.ActionDone, fixedTime.Add(2*time.Hour)),
		logAt(core.ActionComment, fixedTime.Add(time.Hour)),
	}
	cal, err := vtodo.BuildVCalendar([]*core.Task{sampleTask()}, nil, logs, vtodo.WithIncludeLogs(true))
	require.NoError(t, err)
	first := mustSerialize(t, cal)

	res, err := vtodo.ParseVCalendar(strings.NewReader(first))
	require.NoError(t, err)
	require.Len(t, res.Logs, 3)
	for _, le := range res.Logs {
		require.Nil(t, le.Meta, "%s: %#v", le.Action, le.Meta)
	}
	cal2, err := vtodo.BuildVCalendar(res.Tasks, nil, res.Logs, vtodo.WithIncludeLogs(true))
	require.NoError(t, err)
	require.Equal(t, first, mustSerialize(t, cal2))
}

// TestSupersession_DisagreeingWireValuePreserved: when the wire value
// disagrees with what the action derives, the wire wins and is carried.
func TestSupersession_DisagreeingWireValuePreserved(t *testing.T) {
	ics := strings.Replace(foreignLedger(),
		"X-VSTAR-EFFECTIVE-STATUS:COMPLETED",
		"X-TLC-LOG-ACTION:CLAIMED\r\nX-VSTAR-EFFECTIVE-STATUS:COMPLETED", 1)
	res, err := vtodo.ParseVCalendar(strings.NewReader(ics))
	require.NoError(t, err)
	require.Equal(t, core.ActionClaimed, res.Logs[0].Action)
	require.Equal(t, "COMPLETED", res.Logs[0].Meta[vtodo.MetaEffectiveStatusKey])
	cal, err := vtodo.BuildVCalendar(res.Tasks, nil, res.Logs[:1], vtodo.WithIncludeLogs(true))
	require.NoError(t, err)
	require.Contains(t, mustSerialize(t, cal), "X-VSTAR-EFFECTIVE-STATUS:COMPLETED")
}

// TestSupersession_HashLastAfterSupersedes: Supersedes hashes last, but
// the tlc properties added afterwards would leave that digest stale
// and mid-block; finalize must still close the builder.
func TestSupersession_HashLastAfterSupersedes(t *testing.T) {
	_, j := buildLedger(t, []*core.LogEntry{logAt(core.ActionClaimed, fixedTime)})
	require.Equal(t, hashing.XVSTARHashProperty, j.Props[len(j.Props)-1].Name)
	require.Len(t, j.GetAll(hashing.XVSTARHashProperty), 1)
	requireVerifies(t, j, "supersession journal")
}

// TestTrack_CompletedCarriesCompleted: a completed or archived track
// pairs STATUS=COMPLETED with COMPLETED (spec 05 §5), so an undated one
// no longer trips VS040; an open track carries no COMPLETED.
func TestTrack_CompletedCarriesCompleted(t *testing.T) {
	for _, status := range []core.TrackStatus{core.TrackStatusCompleted, core.TrackStatusArchived} {
		t.Run(string(status), func(t *testing.T) {
			track := hashTrack()
			track.Status = status
			track.UpdatedAt = fixedTime.Add(3 * time.Hour)
			require.Nil(t, track.DueAt)
			cal, err := vtodo.BuildVCalendar(nil, []*core.Track{track}, nil)
			require.NoError(t, err)
			r := requireValidExport(t, cal)
			require.Empty(t, r.Allowed, "no VS040 for an undated completed track: %v", keys(r.Allowed))
			out := mustSerialize(t, cal)
			require.Contains(t, out, "\r\nSTATUS:COMPLETED\r\n")
			require.Contains(t, out, "\r\nCOMPLETED:20260502T173000Z\r\n", "COMPLETED is the last modification")
		})
	}
	t.Run("active", func(t *testing.T) {
		cal, err := vtodo.BuildVCalendar(nil, []*core.Track{hashTrack()}, nil)
		require.NoError(t, err)
		require.NotContains(t, mustSerialize(t, cal), "\r\nCOMPLETED:")
	})
}

// TestMeta_LastSyncHashNeverExported: the sync layer's content-hash
// baseline is local bookkeeping; it must not ride inside X-TLC-META.
func TestMeta_LastSyncHashNeverExported(t *testing.T) {
	task := sampleTask()
	task.Meta = map[string]interface{}{
		core.MetaLastSyncHash: "sha256:abc",
		"owner":               "alice",
	}
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.Contains(t, out, `X-TLC-META:{"owner":"alice"}`)
	require.NotContains(t, out, core.MetaLastSyncHash)
	require.NotContains(t, out, "sha256:abc")
}
