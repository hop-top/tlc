package vtodo_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	vstar "hop.top/vstar"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

// conceptOf returns the X-TLC-CONCEPT value of c, failing when the
// component carries anything other than exactly one.
func conceptOf(t *testing.T, c vstar.Component) string {
	t.Helper()
	props := c.GetAll(vtodo.XPropConcept)
	require.Len(t, props, 1, "%s %s must declare exactly one concept", c.Type, c.UID())
	return props[0].Value
}

// TestConcept_TrackIsMission: a track declares mission and keeps the
// track marker, which still does the job the token cannot (entity type).
func TestConcept_TrackIsMission(t *testing.T) {
	cal, err := vtodo.BuildVCalendar(nil, []*core.Track{hashTrack()}, nil)
	require.NoError(t, err)
	todos := cal.Filter(vstar.CompTodo)
	require.Len(t, todos, 1)
	require.Equal(t, vtodo.ConceptMission, conceptOf(t, todos[0]))
	marker, ok := todos[0].Get(vtodo.XPropTrackKind)
	require.True(t, ok, "X-TLC-IS-TRACK must still be emitted")
	require.Equal(t, "TRUE", marker.Value)
}

// TestConcept_StandaloneTaskIsMission: a task with no track is the
// goal itself, not a parentless assignment; it carries no PARENT edge.
func TestConcept_StandaloneTaskIsMission(t *testing.T) {
	task := sampleTask()
	require.Nil(t, task.TrackID)
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.NoError(t, err)
	todos := cal.Filter(vstar.CompTodo)
	require.Len(t, todos, 1)
	require.Equal(t, vtodo.ConceptMission, conceptOf(t, todos[0]))
	require.Empty(t, todos[0].GetAll("RELATED-TO"))
	_, ok := todos[0].Get(vtodo.XPropTrackKind)
	require.False(t, ok, "a task never carries the track marker")
}

// TestConcept_TrackedTaskIsAssignment: a task inside a track declares
// assignment next to the PARENT edge that already implied it.
func TestConcept_TrackedTaskIsAssignment(t *testing.T) {
	track := hashTrack()
	task := sampleTask()
	task.TrackID = ptr(track.ID)
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, []*core.Track{track}, nil)
	require.NoError(t, err)
	c, ok := cal.Find("task_01h455vb4pex5vsknk084sn02q@tlc.local")
	require.True(t, ok)
	require.Equal(t, vtodo.ConceptAssignment, conceptOf(t, c))
	out := mustSerialize(t, cal)
	require.Contains(t, out, "RELATED-TO;RELTYPE=PARENT:track_01h455vbqkfsn02nk084ksn02q@tlc.local")
}

// TestConcept_OneTokenPerComponent: every top-level component declares
// exactly one concept; the VALARM, a single-concept component, none.
func TestConcept_OneTokenPerComponent(t *testing.T) {
	cal := fullCalendar(t)
	alarms := 0
	for _, c := range cal.Components {
		conceptOf(t, c)
		for _, sub := range c.Sub {
			require.Equal(t, vstar.CompAlarm, sub.Type)
			require.Empty(t, sub.GetAll(vtodo.XPropConcept), "VALARM must not declare a concept")
			alarms++
		}
	}
	require.Equal(t, 1, alarms)
}

// journalConceptFor builds one log entry with the given action and
// returns the concept its VJOURNAL declares.
func journalConceptFor(t *testing.T, action string, opts ...vtodo.Option) string {
	t.Helper()
	logs := []*core.LogEntry{{
		TaskID:    "task_01h455vb4pex5vsknk084sn02q",
		Timestamp: fixedTime,
		By:        "alice",
		Action:    action,
	}}
	opts = append(opts, vtodo.WithIncludeLogs(true))
	cal, err := vtodo.BuildVCalendar(nil, nil, logs, opts...)
	require.NoError(t, err)
	journals := cal.Filter(vstar.CompJournal)
	require.Len(t, journals, 1)
	return conceptOf(t, journals[0])
}

// TestConcept_JournalSubTypes pins the sub-typing rule, one row per
// action class, including the configured status names and the
// case-insensitive match, under the default vocabulary.
func TestConcept_JournalSubTypes(t *testing.T) {
	cases := map[string]string{
		// decision: a human-gate verdict.
		core.ActionApproved: vtodo.ConceptDecision,
		core.ActionRejected: vtodo.ConceptDecision,
		// status: the fixed transition constants...
		core.ActionClaimed:   vtodo.ConceptStatus,
		core.ActionReleased:  vtodo.ConceptStatus,
		core.ActionDone:      vtodo.ConceptStatus,
		core.ActionSkipped:   vtodo.ConceptStatus,
		core.ActionRetry:     vtodo.ConceptStatus,
		"REOPENED":           vtodo.ConceptStatus,
		core.ActionBlocked:   vtodo.ConceptStatus,
		core.ActionUnblocked: vtodo.ConceptStatus,
		// ...and any configured status name, the form
		// Task.TransitionWithWorkflow writes, matched case-insensitively.
		string(core.StatusInProgress): vtodo.ConceptStatus,
		string(core.StatusTodo):       vtodo.ConceptStatus,
		"in_progress":                 vtodo.ConceptStatus,
		"claimed":                     vtodo.ConceptStatus,
		// action: something a player did other than move the status.
		core.ActionCreated:    vtodo.ConceptAction,
		core.ActionReclaimed:  vtodo.ConceptAction,
		core.ActionExecStart:  vtodo.ConceptAction,
		core.ActionSyncPushed: vtodo.ConceptAction,
		core.ActionMigrated:   vtodo.ConceptAction,
		// observation: something noticed; no state change.
		core.ActionComment:      vtodo.ConceptObservation,
		core.ActionFailure:      vtodo.ConceptObservation,
		core.ActionSyncConflict: vtodo.ConceptObservation,
		core.ActionSyncError:    vtodo.ConceptObservation,
		// journal: the umbrella.
		"PROGRESS":      vtodo.ConceptJournal,
		"task.reopened": vtodo.ConceptJournal,
		"SYSTEM_BOOT":   vtodo.ConceptJournal,
		"":              vtodo.ConceptJournal,
	}
	for action, want := range cases {
		t.Run(action, func(t *testing.T) {
			require.Equal(t, want, journalConceptFor(t, action))
		})
	}
}

// TestConcept_DecisionPrecedesStatus: a vocabulary declaring a status
// named APPROVED does not turn the verdict into a status journal, and a
// custom name classifies as status only under the vocabulary that
// declares it.
func TestConcept_DecisionPrecedesStatus(t *testing.T) {
	vocab := vtodo.WithStatusDefinitions([]config.StatusDefinition{
		{Name: "BACKLOG", Role: config.RoleInitial},
		{Name: "IN_REVIEW", Role: config.RoleActive},
		{Name: "APPROVED", Role: config.RoleCompleted},
	})
	require.Equal(t, vtodo.ConceptDecision, journalConceptFor(t, "APPROVED", vocab))
	require.Equal(t, vtodo.ConceptStatus, journalConceptFor(t, "IN_REVIEW", vocab))
	require.Equal(t, vtodo.ConceptStatus, journalConceptFor(t, "in_review", vocab))
	// IN_PROGRESS is a built-in name but not in this vocabulary and not
	// a transition constant, so it is unclassifiable here.
	require.Equal(t, vtodo.ConceptJournal, journalConceptFor(t, string(core.StatusInProgress), vocab))
}

// trackOrTask parses ics and reports which slice the single VTODO
// landed in.
func trackOrTask(t *testing.T, ics string) string {
	t.Helper()
	res, err := vtodo.ParseVCalendar(strings.NewReader(ics))
	require.NoError(t, err)
	switch {
	case len(res.Tracks) == 1 && len(res.Tasks) == 0:
		return "track"
	case len(res.Tasks) == 1 && len(res.Tracks) == 0:
		return "task"
	}
	t.Fatalf("expected exactly one entity, got %d tasks and %d tracks", len(res.Tasks), len(res.Tracks))
	return ""
}

// conflictingTodo builds a VTODO whose three discriminators can be set
// independently, so each precedence step can be isolated.
func conflictingTodo(uid, token string, marker bool) string {
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//probe//EN",
		"BEGIN:VTODO",
		"UID:" + uid,
		"DTSTAMP:20260502T143000Z",
		"SUMMARY:conflict",
		"STATUS:NEEDS-ACTION",
	}
	if token != "" {
		lines = append(lines, "X-TLC-CONCEPT:"+token)
	}
	if marker {
		lines = append(lines, "X-TLC-IS-TRACK:TRUE")
	}
	lines = append(lines, "END:VTODO", "END:VCALENDAR", "")
	return strings.Join(lines, "\r\n")
}

// TestConcept_DecodePrecedence proves token beats marker beats UID
// prefix, with inputs where each pair disagrees.
func TestConcept_DecodePrecedence(t *testing.T) {
	const taskUID = "task_01h455vb4pex5vsknk084sn02q@tlc.local"
	const trackUID = "track_01h455vbqkfsn02nk084ksn02q@tlc.local"

	t.Run("emitted assignment token beats an injected track marker", func(t *testing.T) {
		// Encode a tracked task, then forge the marker onto it. The
		// only thing standing between this VTODO and the Tracks slice is
		// the token the encoder wrote.
		track := hashTrack()
		task := sampleTask()
		task.TrackID = ptr(track.ID)
		cal, err := vtodo.BuildVCalendar([]*core.Task{task}, []*core.Track{track}, nil)
		require.NoError(t, err)
		out := mustSerialize(t, cal)
		forged := strings.Replace(out,
			"UID:"+taskUID+"\r\n",
			"UID:"+taskUID+"\r\nX-TLC-IS-TRACK:TRUE\r\n", 1)
		require.NotEqual(t, out, forged)

		res, err := vtodo.ParseVCalendar(strings.NewReader(forged))
		require.NoError(t, err)
		require.Len(t, res.Tasks, 1, "the assignment must decode as a task")
		require.Len(t, res.Tracks, 1)
		require.Equal(t, task.ID, res.Tasks[0].ID)
	})

	t.Run("assignment token beats marker and track prefix together", func(t *testing.T) {
		require.Equal(t, "task", trackOrTask(t, conflictingTodo(trackUID, "assignment", true)))
	})
	t.Run("token is read case-insensitively", func(t *testing.T) {
		require.Equal(t, "task", trackOrTask(t, conflictingTodo(trackUID, "Assignment", true)))
	})
	t.Run("mission token is not decisive: marker beats task prefix", func(t *testing.T) {
		require.Equal(t, "track", trackOrTask(t, conflictingTodo(taskUID, "mission", true)))
	})
	t.Run("mission token without marker on a task UID is a standalone task", func(t *testing.T) {
		require.Equal(t, "task", trackOrTask(t, conflictingTodo(taskUID, "mission", false)))
	})
	t.Run("mission token without marker falls back to the track prefix", func(t *testing.T) {
		require.Equal(t, "track", trackOrTask(t, conflictingTodo(trackUID, "mission", false)))
	})
	t.Run("no token: marker beats task prefix", func(t *testing.T) {
		require.Equal(t, "track", trackOrTask(t, conflictingTodo(taskUID, "", true)))
	})
	t.Run("no token, no marker: prefix decides", func(t *testing.T) {
		require.Equal(t, "track", trackOrTask(t, conflictingTodo(trackUID, "", false)))
		require.Equal(t, "task", trackOrTask(t, conflictingTodo(taskUID, "", false)))
	})
	t.Run("unknown token is not decisive", func(t *testing.T) {
		require.Equal(t, "track", trackOrTask(t, conflictingTodo(taskUID, "turn", true)))
	})
}

// TestConcept_DecodeExposesTokens: what each component declared is
// readable from the parse result, keyed by wire UID, and nothing is
// recorded for the alarm.
func TestConcept_DecodeExposesTokens(t *testing.T) {
	out := mustSerialize(t, fullCalendar(t))
	res, err := vtodo.ParseVCalendar(strings.NewReader(out))
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"track_01h455vbqkfsn02nk084ksn02q@tlc.local":                             vtodo.ConceptMission,
		"task_01h455vb4pex5vsknk084sn02q@tlc.local":                              vtodo.ConceptAssignment,
		"log-task_01h455vb4pex5vsknk084sn02q-CLAIMED-20260502T143000Z@tlc.local": vtodo.ConceptStatus,
	}, res.Concepts)
}

// TestConcept_ForeignTokenLowercased: a token from another producer is
// normalised on read and never parked in the unknown bag.
func TestConcept_ForeignTokenLowercased(t *testing.T) {
	res, err := vtodo.ParseVCalendar(strings.NewReader(
		conflictingTodo("task_01h455vb4pex5vsknk084sn02q@tlc.local", "Mission", false)))
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"task_01h455vb4pex5vsknk084sn02q@tlc.local": vtodo.ConceptMission,
	}, res.Concepts)
	require.Nil(t, res.Tasks[0].Meta, "the token is typed, not an unknown extension")
}

// TestConcept_NoTokenLeavesConceptsNil: a calendar from before the
// property existed reports no declarations rather than an empty map.
func TestConcept_NoTokenLeavesConceptsNil(t *testing.T) {
	res, err := vtodo.ParseVCalendar(strings.NewReader(calWith("DTSTAMP:20260502T143000Z")))
	require.NoError(t, err)
	require.Nil(t, res.Concepts)
}

// TestConcept_RoundTripStable: the token is re-derived on export, never
// carried through Meta, so a second cycle is byte-identical and the
// x_tlc bag stays empty.
func TestConcept_RoundTripStable(t *testing.T) {
	first := mustSerialize(t, fullCalendar(t))
	res, err := vtodo.ParseVCalendar(strings.NewReader(first))
	require.NoError(t, err)
	for _, m := range []map[string]interface{}{res.Tasks[0].Meta, res.Tracks[0].Meta, res.Logs[0].Meta} {
		require.NotContains(t, m, vtodo.MetaXTLCKey)
	}
	cal, err := vtodo.BuildVCalendar(res.Tasks, res.Tracks, res.Logs, vtodo.WithIncludeLogs(true))
	require.NoError(t, err)
	second := mustSerialize(t, cal)
	require.Equal(t, first, second)
	require.Equal(t, 3, strings.Count(second, "\r\n"+vtodo.XPropConcept+":"), second)
}
