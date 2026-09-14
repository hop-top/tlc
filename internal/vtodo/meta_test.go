package vtodo_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

// wrapICS builds a minimal calendar envelope around the supplied
// component bodies so tests can feed hand-written wire input through the
// decoder.
func wrapICS(bodies ...string) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//probe//EN\r\n" +
		strings.Join(bodies, "") + "END:VCALENDAR\r\n"
}

// TestMeta_TaskRoundTrip proves arbitrary Meta values survive
// export→import. JSON numbers decode as float64 per encoding/json's
// interface{} contract, so the expectation is stated in those terms
// rather than asserting the Go int that went in.
func TestMeta_TaskRoundTrip(t *testing.T) {
	in := sampleTask()
	in.Meta = map[string]interface{}{
		"priority_source": "rule",
		"priority_rule":   "all",
		"eva":             []interface{}{"hint-a", "hint-b"},
		"retries":         float64(3),
		"urgent":          true,
		"nested": map[string]interface{}{
			"depth": float64(2),
			"label": "inner",
		},
	}

	res := roundTrip(t, []*core.Task{in}, nil, nil)
	require.Len(t, res.Tasks, 1)
	require.Equal(t, in.Meta, res.Tasks[0].Meta)
}

// TestMeta_TaskMetaOnWire pins the chosen shape: one X-TLC-META property
// carrying JSON, not a flat X-TLC-META-<KEY> family.
func TestMeta_TaskMetaOnWire(t *testing.T) {
	task := sampleTask()
	task.Meta = map[string]interface{}{"priority_source": "rule"}

	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)

	require.Contains(t, out, `X-TLC-META:{"priority_source":"rule"}`)
	require.NotContains(t, out, "X-TLC-META-PRIORITY-SOURCE")
}

// TestMeta_DerivedKeysNotDuplicated guards round-trip stability: keys
// that decode rebuilds from UID or RELATED-TO must not also ride inside
// X-TLC-META, or the second encode would differ from the first.
func TestMeta_DerivedKeysNotDuplicated(t *testing.T) {
	task := sampleTask()
	task.Meta = map[string]interface{}{
		"blocked_by":   []string{"task_01h455vb4pex5vsknk084sn0az"},
		"external_uid": "foreign@example.com",
	}

	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)

	require.NotContains(t, out, "X-TLC-META:", "derived-only Meta must emit no X-TLC-META")
	require.Contains(t, out, "RELTYPE=DEPENDS-ON")
}

// TestMeta_UnknownXTLCPreserved is the core regression: an X-TLC-*
// property the decoder has no typed field for must be preserved, not
// silently dropped.
func TestMeta_UnknownXTLCPreserved(t *testing.T) {
	ics := wrapICS(
		"BEGIN:VTODO\r\n" +
			"UID:task_01h455vb4pex5vsknk084sn02q@tlc.local\r\n" +
			"SUMMARY:Replace JWT signer\r\n" +
			"STATUS:NEEDS-ACTION\r\n" +
			"X-TLC-EFFORT:M\r\n" +
			"X-TLC-FUTURE-FIELD:from-a-newer-tlc\r\n" +
			"X-TLC-ANOTHER:second\r\n" +
			"END:VTODO\r\n",
	)

	res, err := vtodo.ParseVCalendar(strings.NewReader(ics))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	got := res.Tasks[0]

	// Known property still lands on its typed field.
	require.Equal(t, core.EffortM, got.Effort)

	bag, ok := got.Meta[vtodo.MetaXTLCKey].(map[string]interface{})
	require.True(t, ok, "unknown X-TLC-* dropped; Meta = %#v", got.Meta)
	require.Equal(t, "from-a-newer-tlc", bag["X-TLC-FUTURE-FIELD"])
	require.Equal(t, "second", bag["X-TLC-ANOTHER"])

	// Known names must never leak into the unknown bag.
	require.NotContains(t, bag, "X-TLC-EFFORT")
}

// TestMeta_UnknownXTLCReEmitted closes the loop: what was preserved on
// import must go back out on export.
func TestMeta_UnknownXTLCReEmitted(t *testing.T) {
	ics := wrapICS(
		"BEGIN:VTODO\r\n" +
			"UID:task_01h455vb4pex5vsknk084sn02q@tlc.local\r\n" +
			"SUMMARY:Replace JWT signer\r\n" +
			"STATUS:NEEDS-ACTION\r\n" +
			"X-TLC-FUTURE-FIELD:from-a-newer-tlc\r\n" +
			"END:VTODO\r\n",
	)

	res, err := vtodo.ParseVCalendar(strings.NewReader(ics))
	require.NoError(t, err)

	cal, err := vtodo.BuildVCalendar(res.Tasks, res.Tracks, res.Logs)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.Contains(t, out, "X-TLC-FUTURE-FIELD:from-a-newer-tlc")

	// And a second cycle is stable.
	res2, err := vtodo.ParseVCalendar(strings.NewReader(out))
	require.NoError(t, err)
	require.Equal(t, res.Tasks[0].Meta, res2.Tasks[0].Meta)
}

// TestMeta_NonTLCExtensionIgnored documents the policy for extensions
// owned by other systems: tlc classifies them via ext.SystemName and
// deliberately does NOT adopt them into its own Meta namespace.
func TestMeta_NonTLCExtensionIgnored(t *testing.T) {
	ics := wrapICS(
		"BEGIN:VTODO\r\n" +
			"UID:task_01h455vb4pex5vsknk084sn02q@tlc.local\r\n" +
			"SUMMARY:Replace JWT signer\r\n" +
			"STATUS:NEEDS-ACTION\r\n" +
			"X-APPLE-SORT-ORDER:12\r\n" +
			"X-VSTAR-HASH:sha256:abc\r\n" +
			"X-EXP-THING:maybe\r\n" +
			"END:VTODO\r\n",
	)

	res, err := vtodo.ParseVCalendar(strings.NewReader(ics))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)

	// No foreign extension may appear anywhere in tlc Meta.
	require.Nil(t, res.Tasks[0].Meta,
		"foreign extensions must not populate Meta; got %#v", res.Tasks[0].Meta)

	// And nothing foreign is echoed back out.
	cal, err := vtodo.BuildVCalendar(res.Tasks, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.NotContains(t, out, "X-APPLE-SORT-ORDER")
	require.NotContains(t, out, "X-VSTAR-HASH")
	require.NotContains(t, out, "X-EXP-THING")
}

// TestMeta_ForeignNameInBagNotEmitted guards the export side of the same
// policy: even if a non-TLC name is somehow parked in the bag, encode
// refuses to emit it under tlc's namespace.
func TestMeta_ForeignNameInBagNotEmitted(t *testing.T) {
	task := sampleTask()
	task.Meta = map[string]interface{}{
		vtodo.MetaXTLCKey: map[string]interface{}{
			"X-APPLE-SORT-ORDER": "12",
			"X-TLC-FUTURE-FIELD": "kept",
			"NOT-AN-EXTENSION":   "nope",
		},
	}

	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)

	require.Contains(t, out, "X-TLC-FUTURE-FIELD:kept")
	require.NotContains(t, out, "X-APPLE-SORT-ORDER")
	require.NotContains(t, out, "NOT-AN-EXTENSION")
}

// TestMeta_TypedXPropsStillRoundTrip is the regression guard for all ten
// pre-existing typed X-properties.
func TestMeta_TypedXPropsStillRoundTrip(t *testing.T) {
	trackID := "track_01h455vbqkfsn02nk084ksn02q"
	task := sampleTask()
	task.AssignedTo = ptr("alice")   // non-email → X-TLC-ASSIGNEE
	task.ProjectID = ptr("proj-one") // X-TLC-PROJECT-ID
	task.TrackID = ptr(trackID)      // X-TLC-EFFORT/SEQ also set by sampleTask

	track := &core.Track{
		ID:         trackID,
		Slug:       "auth-rewrite", // X-TLC-TRACK-SLUG
		Title:      "Auth rewrite", // X-TLC-IS-TRACK marker emitted
		Type:       core.TrackTypeFeature,
		Status:     core.TrackStatusActive,
		AssignedTo: ptr("bob"),
		ProjectID:  ptr("proj-one"),
		CreatedAt:  fixedTime,
		UpdatedAt:  fixedTime,
	}
	log := &core.LogEntry{
		TaskID:    task.ID, // X-TLC-LOG-TASK
		Timestamp: fixedTime,
		By:        "alice",   // X-TLC-LOG-BY
		Action:    "CLAIMED", // X-TLC-LOG-ACTION
		Note:      "starting work",
	}

	cal, err := vtodo.BuildVCalendar(
		[]*core.Task{task}, []*core.Track{track}, []*core.LogEntry{log},
		vtodo.WithIncludeLogs(true),
	)
	require.NoError(t, err)
	out := mustSerialize(t, cal)

	for _, want := range []string{
		"X-TLC-EFFORT:M",
		"X-TLC-ASSIGNEE:alice",
		"X-TLC-PROJECT-ID:proj-one",
		"X-TLC-TASK-SEQ:42",
		"X-TLC-TRACK-SLUG:auth-rewrite",
		"X-TLC-TRACK-TYPE:feature",
		"X-TLC-IS-TRACK:TRUE",
		"X-TLC-LOG-ACTION:CLAIMED",
		"X-TLC-LOG-BY:alice",
		"X-TLC-LOG-TASK:" + task.ID,
	} {
		require.Contains(t, out, want)
	}

	res, err := vtodo.ParseVCalendar(strings.NewReader(out),
		vtodo.WithIncludeLogs(true))
	require.NoError(t, err)

	require.Len(t, res.Tasks, 1)
	gotTask := res.Tasks[0]
	require.Equal(t, core.EffortM, gotTask.Effort)
	require.Equal(t, "alice", *gotTask.AssignedTo)
	require.Equal(t, "proj-one", *gotTask.ProjectID)
	require.Equal(t, int64(42), gotTask.Seq)
	require.Equal(t, trackID, *gotTask.TrackID)

	require.Len(t, res.Tracks, 1)
	gotTrack := res.Tracks[0]
	require.Equal(t, "auth-rewrite", gotTrack.Slug)
	require.Equal(t, core.TrackTypeFeature, gotTrack.Type)
	require.Equal(t, "bob", *gotTrack.AssignedTo)
	require.Equal(t, "proj-one", *gotTrack.ProjectID)

	require.Len(t, res.Logs, 1)
	gotLog := res.Logs[0]
	require.Equal(t, "CLAIMED", gotLog.Action)
	require.Equal(t, "alice", gotLog.By)
	require.Equal(t, task.ID, gotLog.TaskID)

	// None of the ten typed names may leak into the unknown bag.
	for _, m := range []map[string]interface{}{
		gotTask.Meta, gotTrack.Meta, gotLog.Meta,
	} {
		require.NotContains(t, m, vtodo.MetaXTLCKey)
	}
}

// TestMeta_TrackRoundTrip covers Track.Meta, previously never encoded.
func TestMeta_TrackRoundTrip(t *testing.T) {
	track := &core.Track{
		ID:        "track_01h455vbqkfsn02nk084ksn02q",
		Slug:      "auth-rewrite",
		Title:     "Auth rewrite",
		Type:      core.TrackTypeFeature,
		Status:    core.TrackStatusActive,
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
		Meta: map[string]any{
			"owner_team": "platform",
			"budget":     float64(1200),
			"flags":      map[string]interface{}{"pilot": true},
		},
	}

	res := roundTrip(t, nil, []*core.Track{track}, nil)
	require.Len(t, res.Tracks, 1)
	require.Equal(t, track.Meta, res.Tracks[0].Meta)
}

// TestMeta_LogEntryRoundTrip covers LogEntry.Meta, previously never
// encoded.
func TestMeta_LogEntryRoundTrip(t *testing.T) {
	task := sampleTask()
	log := &core.LogEntry{
		TaskID:    task.ID,
		Timestamp: fixedTime,
		By:        "alice",
		Action:    "CLAIMED",
		Note:      "starting work",
		Meta: map[string]interface{}{
			"source":   "cli",
			"attempt":  float64(2),
			"verified": false,
		},
	}

	res := roundTrip(t, []*core.Task{task}, nil, []*core.LogEntry{log},
		vtodo.WithIncludeLogs(true))
	require.Len(t, res.Logs, 1)
	require.Equal(t, log.Meta, res.Logs[0].Meta)
}

// TestMeta_UnknownXTLCOnTrackAndJournal exercises the same preservation
// path on the two component kinds that are not task VTODOs.
func TestMeta_UnknownXTLCOnTrackAndJournal(t *testing.T) {
	ics := wrapICS(
		"BEGIN:VTODO\r\n"+
			"UID:track_01h455vbqkfsn02nk084ksn02q@tlc.local\r\n"+
			"SUMMARY:Auth rewrite\r\n"+
			"STATUS:IN-PROCESS\r\n"+
			"X-TLC-IS-TRACK:TRUE\r\n"+
			"X-TLC-TRACK-BUDGET:1200\r\n"+
			"END:VTODO\r\n",
		"BEGIN:VJOURNAL\r\n"+
			"UID:log-1@tlc.local\r\n"+
			"SUMMARY:note\r\n"+
			"X-TLC-LOG-ACTION:CLAIMED\r\n"+
			"X-TLC-LOG-CHANNEL:slack\r\n"+
			"END:VJOURNAL\r\n",
	)

	res, err := vtodo.ParseVCalendar(strings.NewReader(ics))
	require.NoError(t, err)

	require.Len(t, res.Tracks, 1)
	trackBag, ok := res.Tracks[0].Meta[vtodo.MetaXTLCKey].(map[string]interface{})
	require.True(t, ok, "track lost unknown X-TLC-*; Meta = %#v", res.Tracks[0].Meta)
	require.Equal(t, "1200", trackBag["X-TLC-TRACK-BUDGET"])

	require.Len(t, res.Logs, 1)
	logBag, ok := res.Logs[0].Meta[vtodo.MetaXTLCKey].(map[string]interface{})
	require.True(t, ok, "log lost unknown X-TLC-*; Meta = %#v", res.Logs[0].Meta)
	require.Equal(t, "slack", logBag["X-TLC-LOG-CHANNEL"])
	require.Equal(t, "CLAIMED", res.Logs[0].Action)
}

// TestMeta_UnknownXTLCOnVAlarm proves the explicit sub-component walk
// works: ext.ExtensionsByScope does not recurse into Component.Sub, so a
// VALARM's extensions would be invisible without it.
func TestMeta_UnknownXTLCOnVAlarm(t *testing.T) {
	ics := wrapICS(
		"BEGIN:VTODO\r\n" +
			"UID:task_01h455vb4pex5vsknk084sn02q@tlc.local\r\n" +
			"SUMMARY:Replace JWT signer\r\n" +
			"STATUS:NEEDS-ACTION\r\n" +
			"BEGIN:VALARM\r\n" +
			"ACTION:DISPLAY\r\n" +
			"TRIGGER;VALUE=DATE-TIME:20260503T023000Z\r\n" +
			"X-TLC-ALARM-CHANNEL:push\r\n" +
			"END:VALARM\r\n" +
			"END:VTODO\r\n",
	)

	res, err := vtodo.ParseVCalendar(strings.NewReader(ics))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	got := res.Tasks[0]
	require.NotNil(t, got.RemindAt)

	bag, ok := got.Meta[vtodo.MetaXTLCKey].(map[string]interface{})
	require.True(t, ok, "VALARM extension dropped; Meta = %#v", got.Meta)
	require.Equal(t, "push", bag["X-TLC-ALARM-CHANNEL"])
}

// TestMeta_NonStringValuesDoNotPanic feeds shapes that JSON cannot
// marshal alongside ones it can; export must survive and keep the
// marshalable keys.
func TestMeta_NonStringValuesDoNotPanic(t *testing.T) {
	task := sampleTask()
	task.Meta = map[string]interface{}{
		"good":   "kept",
		"chan":   make(chan int),
		"fn":     func() {},
		"number": float64(7),
	}

	require.NotPanics(t, func() {
		cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
		require.NoError(t, err)
		out := mustSerialize(t, cal)
		require.Contains(t, out, `"good":"kept"`)
		require.Contains(t, out, `"number":7`)
		require.NotContains(t, out, "chan")
		require.NotContains(t, out, `"fn"`)
	})
}

// TestMeta_EmptyMetaEmitsNothing keeps entities without Meta byte-identical
// to the pre-change wire form.
func TestMeta_EmptyMetaEmitsNothing(t *testing.T) {
	for name, meta := range map[string]map[string]interface{}{
		"nil":   nil,
		"empty": {},
	} {
		t.Run(name, func(t *testing.T) {
			task := sampleTask()
			task.Meta = meta
			cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
			require.NoError(t, err)
			require.NotContains(t, mustSerialize(t, cal), "X-TLC-META")
		})
	}
}
