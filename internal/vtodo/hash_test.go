package vtodo_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	vstar "hop.top/vstar"
	"hop.top/vstar/hashing"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

// requireVerifies asserts c carries an X-VSTAR-HASH equal to the digest
// of its finished content.
func requireVerifies(t *testing.T, c vstar.Component, label string) {
	t.Helper()
	ok, want, got := hashing.VerifyXVSTAR(c)
	require.True(t, ok, "%s: X-VSTAR-HASH stored %q, computed %q", label, got, want)
}

func hashTrack() *core.Track {
	return &core.Track{
		ID:        "track_01h455vbqkfsn02nk084ksn02q",
		Slug:      "auth-rewrite",
		Title:     "Auth rewrite",
		Type:      core.TrackTypeFeature,
		Status:    core.TrackStatusActive,
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	}
}

// fullCalendar builds one of everything: a track with a member and Meta,
// a task with a reminder, tags, Meta, a PARENT link and a blocker, and a
// journal with Meta.
func fullCalendar(t *testing.T) vstar.Calendar {
	t.Helper()
	track := hashTrack()
	track.Meta = map[string]any{"owner": "alice"}
	task := sampleTask()
	task.TrackID = ptr(track.ID)
	task.Meta = map[string]interface{}{
		"blocked_by": []string{"task_01h455vb4pex5vsknk084sn0az"},
		"estimate":   3,
	}
	logs := []*core.LogEntry{{
		TaskID:    task.ID,
		Timestamp: fixedTime,
		By:        "alice",
		Action:    "CLAIMED",
		Note:      "starting work",
		Meta:      map[string]interface{}{"source": "cli"},
	}}
	cal, err := vtodo.BuildVCalendar(
		[]*core.Task{task}, []*core.Track{track}, logs,
		vtodo.WithIncludeLogs(true),
	)
	require.NoError(t, err)
	// track, task, the CLAIMED journal and the open turn it opens.
	require.Len(t, cal.Components, 4)
	requireValidExport(t, cal)
	return cal
}

// TestHash_EveryBuilderVerifies proves every component a build emits --
// track VTODO, task VTODO, its VALARM, and the VJOURNAL -- carries an
// X-VSTAR-HASH that verifies over the finished component.
func TestHash_EveryBuilderVerifies(t *testing.T) {
	cal := fullCalendar(t)
	alarms := 0
	for _, c := range cal.Components {
		requireVerifies(t, c, string(c.Type)+" "+c.UID())
		for _, sub := range c.Sub {
			require.Equal(t, vstar.CompAlarm, sub.Type)
			requireVerifies(t, sub, string(sub.Type)+" "+sub.UID())
			alarms++
		}
	}
	require.Equal(t, 1, alarms, "the task's reminder must be present as a VALARM")
}

// TestHash_TrackWithoutMembersVerifies covers the track that used to
// carry no hash at all: with no CHILD link, no helpers mutator ever ran.
func TestHash_TrackWithoutMembersVerifies(t *testing.T) {
	cal, err := vtodo.BuildVCalendar(nil, []*core.Track{hashTrack()}, nil)
	require.NoError(t, err)
	todos := cal.Filter(vstar.CompTodo)
	require.Len(t, todos, 1)
	_, ok := hashing.GetXVSTAR(todos[0])
	require.True(t, ok, "track VTODO must carry X-VSTAR-HASH")
	requireVerifies(t, todos[0], "track")
}

// TestHash_TrackWithMetaVerifies covers the track that used to carry a
// STALE hash: X-TLC-META lands after the last CHILD link, so a hash
// refreshed by AddRelatedTo never saw it.
func TestHash_TrackWithMetaVerifies(t *testing.T) {
	track := hashTrack()
	track.Meta = map[string]any{"owner": "alice"}
	member := sampleTask()
	member.TrackID = ptr(track.ID)

	cal, err := vtodo.BuildVCalendar([]*core.Task{member}, []*core.Track{track}, nil)
	require.NoError(t, err)
	c, ok := cal.Find("track_01h455vbqkfsn02nk084ksn02q@tlc.local")
	require.True(t, ok)
	_, ok = c.Get(vtodo.XPropMeta)
	require.True(t, ok, "the track must carry X-TLC-META for this case to mean anything")
	requireVerifies(t, c, "track with Meta")
}

// TestHash_IsLastProperty proves the finalize step leaves X-VSTAR-HASH
// as the last property of every component, so on the wire it closes
// the block it certifies rather than sitting wherever the first helpers
// mutator happened to write it.
func TestHash_IsLastProperty(t *testing.T) {
	cal := fullCalendar(t)
	var check func(c vstar.Component)
	check = func(c vstar.Component) {
		require.NotEmpty(t, c.Props)
		last := c.Props[len(c.Props)-1]
		require.Equal(t, hashing.XVSTARHashProperty, last.Name, "%s %s", c.Type, c.UID())
		for _, sub := range c.Sub {
			check(sub)
		}
	}
	for _, c := range cal.Components {
		check(c)
	}
}

// TestHash_TracksContent is the sanity check on the other side: a
// content change moves the hash.
func TestHash_TracksContent(t *testing.T) {
	a := sampleTask()
	b := sampleTask()
	b.Title = "Replace JWT signer, again"
	calA, err := vtodo.BuildVCalendar([]*core.Task{a}, nil, nil)
	require.NoError(t, err)
	calB, err := vtodo.BuildVCalendar([]*core.Task{b}, nil, nil)
	require.NoError(t, err)
	ha, _ := hashing.GetXVSTAR(calA.Components[0])
	hb, _ := hashing.GetXVSTAR(calB.Components[0])
	require.NotEqual(t, ha, hb)
}

// TestParse_NoWarningsForOwnOutput proves a calendar tlc wrote verifies
// on the way back in, VALARM included.
func TestParse_NoWarningsForOwnOutput(t *testing.T) {
	out := mustSerialize(t, fullCalendar(t))
	res, err := vtodo.ParseVCalendar(strings.NewReader(out))
	require.NoError(t, err)
	require.Empty(t, res.Warnings)
}

// TestParse_WarnsOnMissingHash proves a component without X-VSTAR-HASH
// is imported but reported.
func TestParse_WarnsOnMissingHash(t *testing.T) {
	res, err := vtodo.ParseVCalendar(strings.NewReader(calWith()))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1, "the task is still imported")
	require.Equal(t,
		[]string{"VTODO task_01h455vb4pex5vsknk084sn02q@tlc.local: no X-VSTAR-HASH"},
		res.Warnings,
	)
}

// TestParse_WarnsOnTamperedComponent proves a component altered after
// hashing is imported as altered but reported with both digests.
func TestParse_WarnsOnTamperedComponent(t *testing.T) {
	cal, err := vtodo.BuildVCalendar([]*core.Task{sampleTask()}, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	stored, _ := hashing.GetXVSTAR(cal.Components[0])
	tampered := strings.Replace(out, "SUMMARY:Replace JWT signer", "SUMMARY:Replace JWT signer NOW", 1)
	require.NotEqual(t, out, tampered)

	res, err := vtodo.ParseVCalendar(strings.NewReader(tampered))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	require.Equal(t, "Replace JWT signer NOW", res.Tasks[0].Title)
	require.Len(t, res.Warnings, 1, "%v", res.Warnings)
	require.Contains(t, res.Warnings[0], "VTODO task_01h455vb4pex5vsknk084sn02q@tlc.local: X-VSTAR-HASH mismatch")
	require.Contains(t, res.Warnings[0], "stored "+stored)
}

// TestParse_WarnsOnTamperedAlarm proves sub-components are verified too.
// A parent's hash covers its VALARM, so altering the alarm fails both,
// and the alarm's warning names its parent.
func TestParse_WarnsOnTamperedAlarm(t *testing.T) {
	cal, err := vtodo.BuildVCalendar([]*core.Task{sampleTask()}, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	tampered := strings.Replace(out,
		"TRIGGER;VALUE=DATE-TIME:20260503T023000Z",
		"TRIGGER;VALUE=DATE-TIME:20260503T033000Z", 1)
	require.NotEqual(t, out, tampered)

	res, err := vtodo.ParseVCalendar(strings.NewReader(tampered))
	require.NoError(t, err)
	require.Len(t, res.Warnings, 2, "%v", res.Warnings)
	require.Contains(t, res.Warnings[0], "VTODO task_01h455vb4pex5vsknk084sn02q@tlc.local: X-VSTAR-HASH mismatch")
	require.Contains(t, res.Warnings[1],
		"VTODO task_01h455vb4pex5vsknk084sn02q@tlc.local > VALARM task_01h455vb4pex5vsknk084sn02q-alarm@tlc.local: X-VSTAR-HASH mismatch")
}

// TestFixture_HashesVerify proves the committed golden documents (the
// artifacts docs/VSTAR-CONFORMANCE.md points at) verify component by
// component.
func TestFixture_HashesVerify(t *testing.T) {
	for _, name := range []string{
		"single-task.ics",
		"track-with-tasks.ics",
		"recurring-rrule.ics",
		"with-dependencies.ics",
		"with-logs.ics",
		"recipe-run.ics",
	} {
		t.Run(name, func(t *testing.T) {
			data := loadFixture(t, name)
			res, err := vtodo.ParseVCalendar(strings.NewReader(string(data)))
			require.NoError(t, err)
			require.Empty(t, res.Warnings)
		})
	}
}
