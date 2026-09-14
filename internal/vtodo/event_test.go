package vtodo_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	vstar "hop.top/vstar"
	"hop.top/vstar/hashing"
	"hop.top/vstar/validate"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

const (
	eventTaskUID = "task_01h455vb4pex5vsknk084sn02q@tlc.local"
	runID        = "run_01h455vb4pex5vsknk084sn0r1"
	runUID       = runID + "@tlc.local"
)

func at(h int) time.Time { return time.Date(2026, 5, 2, h, 0, 0, 0, time.UTC) }

// turnLog builds one log entry on the sample task by an actor.
func turnLog(action, by string, t time.Time) *core.LogEntry {
	return &core.LogEntry{TaskID: "task_01h455vb4pex5vsknk084sn02q", Timestamp: t, By: by, Action: action}
}

func sampleRun() *core.RecipeRun {
	return &core.RecipeRun{
		ID:        runID,
		ProjectID: "proj-one",
		RecipeID:  "ship-feature",
		Version:   "1.2.0",
		Hash:      "sha256:feedface",
		TrackID:   "track_01h455vbqkfsn02nk084ksn02q",
		CreatedBy: "alice",
		CreatedAt: at(9),
	}
}

// events returns the VEVENTs of cal in calendar order.
func events(cal vstar.Calendar) []vstar.Component { return cal.Filter(vstar.CompEvent) }

// requireEventShape checks what every emitted VEVENT must carry: UID,
// DTSTAMP, DTSTART, exactly one concept token, X-VSTAR-HASH last and
// verifying.
func requireEventShape(t *testing.T, ev vstar.Component, concept string) {
	t.Helper()
	require.Equal(t, vstar.CompEvent, ev.Type)
	require.NotEmpty(t, ev.UID())
	_, ok := ev.DTSTAMP()
	require.True(t, ok, "DTSTAMP")
	_, ok = ev.DTSTART(vstar.Calendar{})
	require.True(t, ok, "DTSTART")
	require.Equal(t, concept, conceptOf(t, ev))
	require.Equal(t, hashing.XVSTARHashProperty, ev.Props[len(ev.Props)-1].Name)
	requireVerifies(t, ev, string(ev.Type)+" "+ev.UID())
}

// TestTurn_OpenClaimFromTaskRow: with no exported log, ClaimedAt yields
// one open turn (no DTEND) held by the task's assignee.
func TestTurn_OpenClaimFromTaskRow(t *testing.T) {
	task := sampleTask()
	task.ClaimedAt = ptr(at(10))
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.NoError(t, err)
	requireValidExport(t, cal)

	evs := events(cal)
	require.Len(t, evs, 1)
	ev := evs[0]
	requireEventShape(t, ev, vtodo.ConceptTurn)
	require.Equal(t, "turn-task_01h455vb4pex5vsknk084sn02q-20260502T100000Z@tlc.local", ev.UID())
	start, _ := ev.DTSTART(vstar.Calendar{})
	require.Equal(t, at(10), start)
	_, hasEnd := ev.Get("DTEND")
	require.False(t, hasEnd, "an open turn has no DTEND")
	_, hasStatus := ev.Get("STATUS")
	require.False(t, hasStatus, "no VEVENT status enum at this vstar version")
	stamp, _ := ev.DTSTAMP()
	require.Equal(t, at(10), stamp, "an open turn's last modification is its start")
	out := mustSerialize(t, cal)
	require.Contains(t, out, "\r\nSUMMARY:Replace JWT signer\r\n")
	require.Contains(t, out, "RELATED-TO;RELTYPE=PARENT:"+eventTaskUID)
	require.Contains(t, out, "X-TLC-ASSIGNEE:alice")
}

// TestTurn_WindowsFromLog: an exported log yields every turn, closed
// ones with DTEND, the player being the opener's actor; an entry that
// is neither an opener nor a closer bounds nothing.
func TestTurn_WindowsFromLog(t *testing.T) {
	task := sampleTask()
	logs := []*core.LogEntry{
		turnLog(core.ActionClaimed, "alice", at(10)),
		turnLog(core.ActionComment, "carol", at(11)),
		turnLog(core.ActionReleased, "alice", at(12)),
		turnLog(core.ActionClaimed, "bob", at(14)),
	}
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, logs, vtodo.WithIncludeLogs(true))
	require.NoError(t, err)
	requireValidExport(t, cal)

	evs := events(cal)
	require.Len(t, evs, 2)
	closed, open := evs[0], evs[1]
	requireEventShape(t, closed, vtodo.ConceptTurn)
	requireEventShape(t, open, vtodo.ConceptTurn)

	end, ok := closed.DTEND(vstar.Calendar{})
	require.True(t, ok)
	require.Equal(t, at(12), end)
	stamp, _ := closed.DTSTAMP()
	require.Equal(t, at(12), stamp, "a closed turn's last modification is its end")
	require.Equal(t, "turn-task_01h455vb4pex5vsknk084sn02q-20260502T100000Z@tlc.local", closed.UID())
	p, _ := closed.Get(vtodo.XPropAssignee)
	require.Equal(t, "alice", p.Value)

	_, hasEnd := open.Get("DTEND")
	require.False(t, hasEnd)
	require.Equal(t, "turn-task_01h455vb4pex5vsknk084sn02q-20260502T140000Z@tlc.local", open.UID())
	p, _ = open.Get(vtodo.XPropAssignee)
	require.Equal(t, "bob", p.Value)
}

// TestTurn_ReclaimClosesPreviousWindow: a takeover of a stale claim
// ends the stale turn at the reclaim instant and starts the next.
func TestTurn_ReclaimClosesPreviousWindow(t *testing.T) {
	logs := []*core.LogEntry{
		turnLog(core.ActionClaimed, "alice", at(10)),
		turnLog(core.ActionReclaimed, "bob", at(13)),
		turnLog(core.ActionDone, "bob", at(15)),
	}
	cal, err := vtodo.BuildVCalendar([]*core.Task{sampleTask()}, nil, logs, vtodo.WithIncludeLogs(true))
	require.NoError(t, err)
	evs := events(cal)
	require.Len(t, evs, 2)
	end, ok := evs[0].DTEND(vstar.Calendar{})
	require.True(t, ok)
	require.Equal(t, at(13), end)
	start, _ := evs[1].DTSTART(vstar.Calendar{})
	require.Equal(t, at(13), start)
	end, ok = evs[1].DTEND(vstar.Calendar{})
	require.True(t, ok)
	require.Equal(t, at(15), end)
}

// TestTurn_EveryCloserEndsTheWindow pins the closer set.
func TestTurn_EveryCloserEndsTheWindow(t *testing.T) {
	for _, closer := range []string{
		core.ActionDone, core.ActionReleased, core.ActionSkipped, core.ActionApproved,
		core.ActionRejected, core.ActionRetry, core.ActionBlocked,
	} {
		t.Run(closer, func(t *testing.T) {
			logs := []*core.LogEntry{
				turnLog(core.ActionClaimed, "alice", at(10)),
				turnLog(closer, "alice", at(11)),
			}
			cal, err := vtodo.BuildVCalendar([]*core.Task{sampleTask()}, nil, logs, vtodo.WithIncludeLogs(true))
			require.NoError(t, err)
			evs := events(cal)
			require.Len(t, evs, 1)
			end, ok := evs[0].DTEND(vstar.Calendar{})
			require.True(t, ok, "%s must close the turn", closer)
			require.Equal(t, at(11), end)
		})
	}
	t.Run("UNBLOCKED does not", func(t *testing.T) {
		logs := []*core.LogEntry{
			turnLog(core.ActionClaimed, "alice", at(10)),
			turnLog(core.ActionUnblocked, "alice", at(11)),
		}
		cal, err := vtodo.BuildVCalendar([]*core.Task{sampleTask()}, nil, logs, vtodo.WithIncludeLogs(true))
		require.NoError(t, err)
		evs := events(cal)
		require.Len(t, evs, 1)
		_, ok := evs[0].Get("DTEND")
		require.False(t, ok)
	})
}

// TestTurn_LogWinsOverClaimedAt: when the exported log opens a window,
// the task row is not consulted, so a claim never appears twice.
func TestTurn_LogWinsOverClaimedAt(t *testing.T) {
	task := sampleTask()
	task.ClaimedAt = ptr(at(10).Add(3 * time.Second)) // the row's clock, a hair off the log's
	logs := []*core.LogEntry{turnLog(core.ActionClaimed, "alice", at(10))}
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, logs, vtodo.WithIncludeLogs(true))
	require.NoError(t, err)
	evs := events(cal)
	require.Len(t, evs, 1)
	start, _ := evs[0].DTSTART(vstar.Calendar{})
	require.Equal(t, at(10), start)
}

// TestTurn_ClaimedAtIsTheFallback: a log that opens nothing, or a log
// that is not exported, leaves ClaimedAt as the source.
func TestTurn_ClaimedAtIsTheFallback(t *testing.T) {
	task := sampleTask()
	task.ClaimedAt = ptr(at(16))
	t.Run("log opens nothing", func(t *testing.T) {
		logs := []*core.LogEntry{turnLog(core.ActionComment, "alice", at(10))}
		cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, logs, vtodo.WithIncludeLogs(true))
		require.NoError(t, err)
		evs := events(cal)
		require.Len(t, evs, 1)
		start, _ := evs[0].DTSTART(vstar.Calendar{})
		require.Equal(t, at(16), start)
	})
	t.Run("log not exported", func(t *testing.T) {
		logs := []*core.LogEntry{turnLog(core.ActionClaimed, "alice", at(10))}
		cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, logs)
		require.NoError(t, err)
		evs := events(cal)
		require.Len(t, evs, 1)
		start, _ := evs[0].DTSTART(vstar.Calendar{})
		require.Equal(t, at(16), start, "an unexported log bounds nothing a reader could see")
	})
}

// TestTurn_NoClaimNoEvent: a task that was never claimed emits no turn.
func TestTurn_NoClaimNoEvent(t *testing.T) {
	cal, err := vtodo.BuildVCalendar([]*core.Task{sampleTask()}, nil, nil)
	require.NoError(t, err)
	require.Empty(t, events(cal))
}

// TestTurn_ClaimedAtRoundTrips: the open turn is the wire form of
// ClaimedAt, so it decodes back onto the task; a closed turn does not.
func TestTurn_ClaimedAtRoundTrips(t *testing.T) {
	t.Run("open", func(t *testing.T) {
		task := sampleTask()
		task.ClaimedAt = ptr(at(10))
		res := roundTrip(t, []*core.Task{task}, nil, nil)
		require.NotNil(t, res.Tasks[0].ClaimedAt)
		require.Equal(t, at(10), res.Tasks[0].ClaimedAt.UTC())
	})
	t.Run("closed", func(t *testing.T) {
		logs := []*core.LogEntry{
			turnLog(core.ActionClaimed, "alice", at(10)),
			turnLog(core.ActionDone, "alice", at(12)),
		}
		res := roundTrip(t, []*core.Task{sampleTask()}, nil, logs, vtodo.WithIncludeLogs(true))
		require.Nil(t, res.Tasks[0].ClaimedAt)
	})
	t.Run("byte-identical re-export", func(t *testing.T) {
		task := sampleTask()
		task.ClaimedAt = ptr(at(10))
		cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
		require.NoError(t, err)
		first := mustSerialize(t, cal)
		res, err := vtodo.ParseVCalendar(strings.NewReader(first))
		require.NoError(t, err)
		cal2, err := vtodo.BuildVCalendar(res.Tasks, nil, nil)
		require.NoError(t, err)
		require.Equal(t, first, mustSerialize(t, cal2))
	})
}

// TestTurn_ForeignEventIsNotAClaim: a VEVENT related to a task that
// does not declare itself a turn does not set ClaimedAt.
func TestTurn_ForeignEventIsNotAClaim(t *testing.T) {
	ics := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//other//EN",
		"BEGIN:VTODO",
		"UID:" + eventTaskUID,
		"DTSTAMP:20260502T120000Z",
		"SUMMARY:x",
		"STATUS:NEEDS-ACTION",
		"END:VTODO",
		"BEGIN:VEVENT",
		"UID:meeting-1@other",
		"DTSTAMP:20260502T120000Z",
		"DTSTART:20260502T100000Z",
		"RELATED-TO:" + eventTaskUID,
		"END:VEVENT",
		"END:VCALENDAR",
		"",
	}, "\r\n")
	res, err := vtodo.ParseVCalendar(strings.NewReader(ics))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	require.Nil(t, res.Tasks[0].ClaimedAt)
}

// TestPlaythrough_Shape: a run is a VEVENT starting at its
// materialisation, with no end, carrying the recipe identity and the
// mission edge.
func TestPlaythrough_Shape(t *testing.T) {
	run := sampleRun()
	track := hashTrack()
	cal, err := vtodo.BuildVCalendar(nil, []*core.Track{track}, nil, vtodo.WithRecipeRuns([]*core.RecipeRun{run}))
	require.NoError(t, err)
	requireValidExport(t, cal)

	evs := events(cal)
	require.Len(t, evs, 1)
	ev := evs[0]
	requireEventShape(t, ev, vtodo.ConceptPlaythrough)
	require.Equal(t, runUID, ev.UID())
	start, _ := ev.DTSTART(vstar.Calendar{})
	require.Equal(t, at(9), start)
	stamp, _ := ev.DTSTAMP()
	require.Equal(t, at(9), stamp)
	_, hasEnd := ev.Get("DTEND")
	require.False(t, hasEnd, "a run has no end timestamp")
	for name, want := range map[string]string{
		"SUMMARY":                "ship-feature",
		vtodo.XPropRecipeID:      "ship-feature",
		vtodo.XPropRecipeVersion: "1.2.0",
		vtodo.XPropRecipeHash:    "sha256:feedface",
		vtodo.XPropProjectID:     "proj-one",
	} {
		p, ok := ev.Get(name)
		require.True(t, ok, name)
		require.Equal(t, want, p.Value, name)
	}
	require.Contains(t, mustSerialize(t, cal),
		"RELATED-TO;RELTYPE=PARENT:track_01h455vbqkfsn02nk084ksn02q@tlc.local\r\nX-TLC-PROJECT-ID")
}

// TestPlaythrough_TracklessRunHasNoEdge: a run through the world
// carries no mission edge; empty version and hash are omitted.
func TestPlaythrough_TracklessRunHasNoEdge(t *testing.T) {
	run := sampleRun()
	run.TrackID, run.Version, run.Hash = "", "", ""
	cal, err := vtodo.BuildVCalendar(nil, nil, nil, vtodo.WithRecipeRuns([]*core.RecipeRun{run}))
	require.NoError(t, err)
	requireValidExport(t, cal)
	ev := events(cal)[0]
	require.Empty(t, ev.GetAll("RELATED-TO"))
	for _, name := range []string{vtodo.XPropRecipeVersion, vtodo.XPropRecipeHash} {
		_, ok := ev.Get(name)
		require.False(t, ok, name)
	}
}

// TestPlaythrough_EmptyIDRejected: like every component, a run without
// an ID is refused rather than exported with an empty UID.
func TestPlaythrough_EmptyIDRejected(t *testing.T) {
	run := sampleRun()
	run.ID = ""
	_, err := vtodo.BuildVCalendar(nil, nil, nil, vtodo.WithRecipeRuns([]*core.RecipeRun{run}))
	require.ErrorIs(t, err, vstar.ErrMissingUID)
}

// TestPlaythrough_TaskEdge: a task materialised by a run points at the
// playthrough with X-TLC-RUN, which decodes back to RunID under our
// domain and is left alone under a foreign one.
func TestPlaythrough_TaskEdge(t *testing.T) {
	task := sampleTask()
	task.RunID = runID
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.Contains(t, out, "\r\nX-TLC-RUN:"+runUID+"\r\n")

	res, err := vtodo.ParseVCalendar(strings.NewReader(out))
	require.NoError(t, err)
	require.Equal(t, runID, res.Tasks[0].RunID)
	require.NotContains(t, res.Tasks[0].Meta, vtodo.MetaXTLCKey, "registered, never parked")

	foreign := strings.Replace(out, "X-TLC-RUN:"+runUID, "X-TLC-RUN:"+runID+"@someone-else.example", 1)
	res, err = vtodo.ParseVCalendar(strings.NewReader(foreign))
	require.NoError(t, err)
	require.Empty(t, res.Tasks[0].RunID)
}

// TestEvent_NoVS041: every VEVENT tlc emits carries DTSTART, so the
// gate never sees VS041, turns and playthroughs together.
func TestEvent_NoVS041(t *testing.T) {
	track := hashTrack()
	task := sampleTask()
	task.TrackID = ptr(track.ID)
	task.RunID = runID
	task.ClaimedAt = ptr(at(14))
	logs := []*core.LogEntry{
		turnLog(core.ActionClaimed, "alice", at(10)),
		turnLog(core.ActionReleased, "alice", at(12)),
		turnLog(core.ActionClaimed, "bob", at(14)),
	}
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, []*core.Track{track}, logs,
		vtodo.WithIncludeLogs(true), vtodo.WithRecipeRuns([]*core.RecipeRun{sampleRun()}))
	require.NoError(t, err)
	r := requireValidExport(t, cal)
	for _, d := range r.Allowed {
		require.NotEqual(t, validate.CodeVEVENTMissingDTSTART, d.Code)
	}
	require.Len(t, events(cal), 3)

	res, err := vtodo.ParseVCalendar(strings.NewReader(mustSerialize(t, cal)))
	require.NoError(t, err)
	require.Empty(t, res.Warnings)
	require.Equal(t, vtodo.ConceptTurn, res.Concepts["turn-task_01h455vb4pex5vsknk084sn02q-20260502T100000Z@tlc.local"])
	require.Equal(t, vtodo.ConceptTurn, res.Concepts["turn-task_01h455vb4pex5vsknk084sn02q-20260502T140000Z@tlc.local"])
	require.Equal(t, vtodo.ConceptPlaythrough, res.Concepts[runUID])
	require.NotNil(t, res.Tasks[0].ClaimedAt)
	require.Equal(t, at(14), res.Tasks[0].ClaimedAt.UTC())
}
