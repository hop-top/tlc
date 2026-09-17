package vtodo_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

const fidelityTrackUID = "track_01h455vbqkfsn02nk084ksn02q@tlc.local"

// memberOf returns a reminder-less copy of sampleTask in the given
// status, linked to track.
func memberOf(track *core.Track, id string, status core.TaskStatus) *core.Task {
	tk := sampleTask()
	tk.ID = id
	tk.Status = status
	tk.TrackID = ptr(track.ID)
	tk.RemindAt = nil
	return tk
}

// TestTrack_PercentCompleteFromMembers proves a track's PERCENT-COMPLETE
// is the Progress column: terminal members (DONE or SKIPPED) over all
// members, rounded to the nearest integer.
func TestTrack_PercentCompleteFromMembers(t *testing.T) {
	track := hashTrack()
	percent := func(tasks []*core.Task) (string, bool) {
		cal, err := vtodo.BuildVCalendar(tasks, []*core.Track{track}, nil)
		require.NoError(t, err)
		c, ok := cal.Find(fidelityTrackUID)
		require.True(t, ok)
		p, ok := c.Get("PERCENT-COMPLETE")
		return p.Value, ok
	}

	got, ok := percent([]*core.Task{
		memberOf(track, "task_01h455vb4pex5vsknk084sn0aw", core.StatusDone),
		memberOf(track, "task_01h455vb4pex5vsknk084sn0ax", core.StatusSkipped),
		memberOf(track, "task_01h455vb4pex5vsknk084sn0az", core.StatusTodo),
		memberOf(track, "task_01h455vb4pex5vsknk084sn0aa", core.StatusInProgress),
	})
	require.True(t, ok)
	require.Equal(t, "50", got)

	got, ok = percent([]*core.Task{
		memberOf(track, "task_01h455vb4pex5vsknk084sn0aw", core.StatusDone),
		memberOf(track, "task_01h455vb4pex5vsknk084sn0ax", core.StatusTodo),
		memberOf(track, "task_01h455vb4pex5vsknk084sn0az", core.StatusTodo),
	})
	require.True(t, ok)
	require.Equal(t, "33", got, "rounded, not truncated to a multiple of ten")

	got, ok = percent([]*core.Task{
		memberOf(track, "task_01h455vb4pex5vsknk084sn0aw", core.StatusTodo),
	})
	require.True(t, ok)
	require.Equal(t, "0", got, "no member done is 0, distinct from no members")

	_, ok = percent(nil)
	require.False(t, ok, "a track with no members has no progress to report")
}

// TestTrack_DueRoundTrips proves Track.DueAt exports as DUE and comes
// back; tasks always did this, tracks never did.
func TestTrack_DueRoundTrips(t *testing.T) {
	track := hashTrack()
	due := fixedTime.Add(72 * time.Hour)
	track.DueAt = &due

	cal, err := vtodo.BuildVCalendar(nil, []*core.Track{track}, nil)
	require.NoError(t, err)
	require.Contains(t, mustSerialize(t, cal), "DUE:20260505T143000Z")

	res := roundTrip(t, nil, []*core.Track{track}, nil)
	require.Len(t, res.Tracks, 1)
	require.NotNil(t, res.Tracks[0].DueAt)
	require.Equal(t, due, res.Tracks[0].DueAt.UTC())
}

// TestTrack_DueOmittedWhenUnset proves an unset DueAt writes no DUE.
func TestTrack_DueOmittedWhenUnset(t *testing.T) {
	cal, err := vtodo.BuildVCalendar(nil, []*core.Track{hashTrack()}, nil)
	require.NoError(t, err)
	c, ok := cal.Find(fidelityTrackUID)
	require.True(t, ok)
	_, ok = c.Get("DUE")
	require.False(t, ok)

	res := roundTrip(t, nil, []*core.Track{hashTrack()}, nil)
	require.Nil(t, res.Tracks[0].DueAt)
}

// TestTrack_SeqRoundTrips proves Track.Seq travels as X-TLC-TRACK-SEQ,
// decodes back into the typed field, and -- because the property is
// registered as known -- is NOT also parked in the x_tlc bag, which
// would make the next export emit it twice.
func TestTrack_SeqRoundTrips(t *testing.T) {
	track := hashTrack()
	track.Seq = 7

	res := roundTrip(t, nil, []*core.Track{track}, nil)
	require.Len(t, res.Tracks, 1)
	require.Equal(t, int64(7), res.Tracks[0].Seq)
	require.Nil(t, res.Tracks[0].Meta[vtodo.MetaXTLCKey],
		"a typed X-prop must not also land in the unknown-property bag")

	cal, err := vtodo.BuildVCalendar(nil, res.Tracks, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.Equal(t, 1, strings.Count(out, "X-TLC-TRACK-SEQ:"), out)
	require.Contains(t, out, "X-TLC-TRACK-SEQ:7")
}

// TestTrack_SeqOmittedWhenZero proves an unassigned Seq writes nothing.
func TestTrack_SeqOmittedWhenZero(t *testing.T) {
	cal, err := vtodo.BuildVCalendar(nil, []*core.Track{hashTrack()}, nil)
	require.NoError(t, err)
	require.NotContains(t, mustSerialize(t, cal), "X-TLC-TRACK-SEQ")
}

// TestMeta_PriorityProvenanceNotInJSON proves priority_source and
// priority_rule travel ONLY as their typed X-properties: they were
// also being written into the X-TLC-META JSON, and on import the JSON
// copy won. The round trip still restores both, through the typed
// path.
func TestMeta_PriorityProvenanceNotInJSON(t *testing.T) {
	in := sampleTask()
	in.Meta = map[string]interface{}{
		core.MetaPrioritySource: core.PrioritySourceDerived,
		core.MetaPriorityRule:   "due-soon",
		"owner":                 "alice",
	}

	cal, err := vtodo.BuildVCalendar([]*core.Task{in}, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.Contains(t, out, "X-TLC-PRIORITY-SOURCE:"+string(core.PrioritySourceDerived))
	require.Contains(t, out, "X-TLC-PRIORITY-RULE:due-soon")
	require.Contains(t, out, `X-TLC-META:{"owner":"alice"}`)
	require.NotContains(t, out, `"priority_source"`)
	require.NotContains(t, out, `"priority_rule"`)

	res := roundTrip(t, []*core.Task{in}, nil, nil)
	require.Equal(t, in.Meta, res.Tasks[0].Meta)
}
