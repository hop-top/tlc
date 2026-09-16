package vtodo_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

// customStatuses is a vocabulary that shares NO status name with the
// built-in four beyond TODO, so any mapping keyed on the built-in names
// necessarily loses IN_REVIEW and SHIPPED.
func customStatuses() []config.StatusDefinition {
	return []config.StatusDefinition{
		{Name: "BACKLOG", Label: "Backlog", Role: config.RoleInitial},
		{Name: "IN_REVIEW", Label: "In Review", Role: config.RoleActive},
		{Name: "SHIPPED", Label: "Shipped", IsTerminal: true, Role: config.RoleCompleted},
		{Name: "DROPPED", Label: "Dropped", IsTerminal: true, Role: config.RoleSkipped},
	}
}

func vocabTask(status core.TaskStatus) *core.Task {
	return &core.Task{
		ID:        core.NewTaskID(),
		Title:     "vocab task",
		Status:    status,
		CreatedAt: time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 3, 1, 11, 0, 0, 0, time.UTC),
	}
}

// TestCustomStatusVocab_WireStatusFollowsRole proves the RFC 5545 STATUS
// value is derived from the configured role, not from a built-in status
// name. A vocabulary declaring IN_REVIEW with role "active" must export
// IN-PROCESS, never the NEEDS-ACTION default.
func TestCustomStatusVocab_WireStatusFollowsRole(t *testing.T) {
	cases := []struct {
		status core.TaskStatus
		wire   string
	}{
		{"BACKLOG", "STATUS:NEEDS-ACTION"},
		{"IN_REVIEW", "STATUS:IN-PROCESS"},
		{"SHIPPED", "STATUS:COMPLETED"},
		{"DROPPED", "STATUS:CANCELLED"}, //nolint:misspell // RFC 5545 spelling
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			cal, err := vtodo.BuildVCalendar(
				[]*core.Task{vocabTask(tc.status)}, nil, nil,
				vtodo.WithStatusDefinitions(customStatuses()),
			)
			require.NoError(t, err)
			out := mustSerialize(t, cal)
			require.Contains(t, out, tc.wire)
		})
	}
}

// TestCustomStatusVocab_NameRoundTrips proves the original status NAME
// survives export + import. Role-only mapping would collapse IN_REVIEW
// to the role-default active status on the way back.
func TestCustomStatusVocab_NameRoundTrips(t *testing.T) {
	for _, status := range []core.TaskStatus{"BACKLOG", "IN_REVIEW", "SHIPPED", "DROPPED"} {
		t.Run(string(status), func(t *testing.T) {
			opt := vtodo.WithStatusDefinitions(customStatuses())
			res := roundTrip(t, []*core.Task{vocabTask(status)}, nil, nil, opt)
			require.Len(t, res.Tasks, 1)
			require.Equal(t, status, res.Tasks[0].Status)
		})
	}
}

// TestCustomStatusVocab_XPropEmitted pins the X-property itself: without
// it the name cannot round-trip, so its absence is the defect.
func TestCustomStatusVocab_XPropEmitted(t *testing.T) {
	cal, err := vtodo.BuildVCalendar(
		[]*core.Task{vocabTask("IN_REVIEW")}, nil, nil,
		vtodo.WithStatusDefinitions(customStatuses()),
	)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.Contains(t, out, vtodo.XPropStatus+":IN_REVIEW")
}

// TestStatusDecode_UnknownNameFallsBackToRole covers the interop path: a
// calendar carrying an X-TLC-STATUS name the CURRENT vocabulary does not
// declare must fall back to the role default for the wire STATUS rather
// than importing a status the config rejects.
func TestStatusDecode_UnknownNameFallsBackToRole(t *testing.T) {
	ics := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//tlc//vtodo//EN",
		"BEGIN:VTODO",
		"UID:foreign-1@example.com",
		"SUMMARY:from another project",
		"STATUS:IN-PROCESS",
		vtodo.XPropStatus + ":SOME_OTHER_VOCAB",
		"END:VTODO",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	res, err := vtodo.ParseVCalendar(
		strings.NewReader(ics),
		vtodo.WithStatusDefinitions(customStatuses()),
	)
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	require.Equal(t, core.TaskStatus("IN_REVIEW"), res.Tasks[0].Status)
}

// TestStatusDecode_NoXPropFallsBackToRole covers calendars written by
// foreign tools that carry only the RFC STATUS.
func TestStatusDecode_NoXPropFallsBackToRole(t *testing.T) {
	ics := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//tlc//vtodo//EN",
		"BEGIN:VTODO",
		"UID:foreign-2@example.com",
		"SUMMARY:plain ical",
		"STATUS:COMPLETED",
		"END:VTODO",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	res, err := vtodo.ParseVCalendar(
		strings.NewReader(ics),
		vtodo.WithStatusDefinitions(customStatuses()),
	)
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	require.Equal(t, core.TaskStatus("SHIPPED"), res.Tasks[0].Status)
}

// TestTrackStatus_CompletedVsArchivedDistinct proves the two track
// statuses that share a wire STATUS stay distinguishable across a round
// trip. Both export COMPLETED for interop; only X-TLC-TRACK-STATUS tells
// them apart.
func TestTrackStatus_CompletedVsArchivedDistinct(t *testing.T) {
	completed := &core.Track{
		ID: core.NewTrackID(), Title: "done track",
		Status:    core.TrackStatusCompleted,
		CreatedAt: time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC),
	}
	archived := &core.Track{
		ID: core.NewTrackID(), Title: "archived track",
		Status:    core.TrackStatusArchived,
		CreatedAt: time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC),
	}

	res := roundTrip(t, nil, []*core.Track{completed, archived}, nil)
	require.Len(t, res.Tracks, 2)

	byID := map[string]*core.Track{}
	for _, tr := range res.Tracks {
		byID[tr.ID] = tr
	}
	require.Equal(t, core.TrackStatusCompleted, byID[completed.ID].Status)
	require.Equal(t, core.TrackStatusArchived, byID[archived.ID].Status)
}

// TestTrackStatus_WireStaysInteroperable pins that the archived track
// still carries an RFC-legal STATUS for foreign consumers.
func TestTrackStatus_WireStaysInteroperable(t *testing.T) {
	archived := &core.Track{
		ID: core.NewTrackID(), Title: "archived track",
		Status:    core.TrackStatusArchived,
		CreatedAt: time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC),
	}
	cal, err := vtodo.BuildVCalendar(nil, []*core.Track{archived}, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.Contains(t, out, "STATUS:COMPLETED")
	require.Contains(t, out, vtodo.XPropTrackStatus+":archived")
}

// TestTaskArchived_RoundTrips proves Task.Archived survives export and
// import; it was previously never encoded at all.
func TestTaskArchived_RoundTrips(t *testing.T) {
	archived := vocabTask(core.StatusDone)
	archived.Archived = true
	live := vocabTask(core.StatusDone)

	res := roundTrip(t, []*core.Task{archived, live}, nil, nil)
	require.Len(t, res.Tasks, 2)

	byID := map[string]*core.Task{}
	for _, tk := range res.Tasks {
		byID[tk.ID] = tk
	}
	require.True(t, byID[archived.ID].Archived)
	require.False(t, byID[live.ID].Archived)
}

// TestTaskArchived_OmittedWhenFalse keeps the wire quiet for the common
// case rather than stamping every VTODO with X-TLC-ARCHIVED:FALSE.
func TestTaskArchived_OmittedWhenFalse(t *testing.T) {
	cal, err := vtodo.BuildVCalendar([]*core.Task{vocabTask(core.StatusTodo)}, nil, nil)
	require.NoError(t, err)
	require.NotContains(t, mustSerialize(t, cal), vtodo.XPropArchived)
}
