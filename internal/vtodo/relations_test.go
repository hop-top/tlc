package vtodo_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

const (
	relTaskA  = "task_01h455vb4pex5vsknk084sn0aa"
	relTaskB  = "task_01h455vb4pex5vsknk084sn0ab"
	relTaskC  = "task_01h455vb4pex5vsknk084sn0ac"
	relTrackA = "track_01h455vbqkfsn02nk084ksn02q"
)

// relTask builds a minimal task suitable for relation assertions.
func relTask(id, title string) *core.Task {
	return &core.Task{
		ID:        id,
		Title:     title,
		Status:    core.StatusTodo,
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	}
}

// TestRelations_ParentRoundTrip covers the PARENT edge: a task carrying
// a TrackID must emit a bare RELATED-TO (RFC 5545 §3.2.15 defaults
// RELTYPE to PARENT, and spec 02 requires the parameter be omitted for
// that value) and recover the same TrackID.
func TestRelations_ParentRoundTrip(t *testing.T) {
	trackID := relTrackA
	task := relTask(relTaskA, "child task")
	task.TrackID = &trackID
	track := &core.Track{
		ID:        trackID,
		Slug:      "auth-rewrite",
		Title:     "Auth rewrite",
		Type:      core.TrackTypeFeature,
		Status:    core.TrackStatusActive,
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	}

	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, []*core.Track{track}, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.Contains(t, out, "\r\nRELATED-TO:"+trackID+"@tlc.local\r\n")
	require.NotContains(t, out, "RELTYPE=PARENT", "spec 02: RELTYPE is omitted when its value is PARENT")

	res, err := vtodo.ParseVCalendar(strings.NewReader(out))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	require.NotNil(t, res.Tasks[0].TrackID)
	require.Equal(t, trackID, *res.Tasks[0].TrackID)
}

// TestRelations_NoChildBackReference pins spec 02's direction rule: an
// edge is encoded once, on the contained component, and the track
// carries no RELTYPE=CHILD back-reference. Membership still round-trips,
// derived from the members' PARENT edges alone.
func TestRelations_NoChildBackReference(t *testing.T) {
	trackID := relTrackA
	members := []*core.Task{
		relTask(relTaskA, "member one"),
		relTask(relTaskB, "member two"),
	}
	for _, m := range members {
		m.TrackID = &trackID
	}
	track := &core.Track{
		ID:        trackID,
		Slug:      "auth-rewrite",
		Title:     "Auth rewrite",
		Type:      core.TrackTypeFeature,
		Status:    core.TrackStatusActive,
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	}

	cal, err := vtodo.BuildVCalendar(members, []*core.Track{track}, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.NotContains(t, out, "RELTYPE=CHILD")
	tr, ok := cal.Find(trackID + "@tlc.local")
	require.True(t, ok)
	require.Empty(t, tr.GetAll("RELATED-TO"), "a track carries no edge of its own")

	res, err := vtodo.ParseVCalendar(strings.NewReader(out))
	require.NoError(t, err)
	require.Len(t, res.Tracks, 1)
	require.Len(t, res.Tasks, 2)
	for _, task := range res.Tasks {
		require.NotNil(t, task.TrackID, "task %s lost its track", task.ID)
		require.Equal(t, trackID, *task.TrackID)
	}
}

// TestRelations_DependsOnRoundTrip covers RELTYPE=DEPENDS-ON for multiple
// blockers, in order.
func TestRelations_DependsOnRoundTrip(t *testing.T) {
	blocked := relTask(relTaskC, "blocked task")
	blocked.Meta = map[string]interface{}{
		"blocked_by": []string{relTaskA, relTaskB},
	}
	tasks := []*core.Task{
		relTask(relTaskA, "blocker one"),
		relTask(relTaskB, "blocker two"),
		blocked,
	}

	cal, err := vtodo.BuildVCalendar(tasks, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.Contains(t, out, "RELATED-TO;RELTYPE=DEPENDS-ON:"+relTaskA+"@tlc.local")
	require.Contains(t, out, "RELATED-TO;RELTYPE=DEPENDS-ON:"+relTaskB+"@tlc.local")

	res, err := vtodo.ParseVCalendar(strings.NewReader(out))
	require.NoError(t, err)
	got := findTaskByID(t, res, relTaskC)
	require.Equal(t, []string{relTaskA, relTaskB}, got.Meta["blocked_by"])
}

// TestRelations_BlockedByInterfaceSliceEncodes pins the encoder's
// acceptance of the []interface{} shape produced by JSON decoding — the
// shape core.NormalizeBlockedBy now handles for every call site.
func TestRelations_BlockedByInterfaceSliceEncodes(t *testing.T) {
	blocked := relTask(relTaskC, "blocked task")
	blocked.Meta = map[string]interface{}{
		"blocked_by": []interface{}{relTaskA, relTaskB},
	}
	tasks := []*core.Task{
		relTask(relTaskA, "blocker one"),
		relTask(relTaskB, "blocker two"),
		blocked,
	}

	cal, err := vtodo.BuildVCalendar(tasks, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)

	res, err := vtodo.ParseVCalendar(strings.NewReader(out))
	require.NoError(t, err)
	got := findTaskByID(t, res, relTaskC)
	require.Equal(t, []string{relTaskA, relTaskB}, got.Meta["blocked_by"])
}

// TestRelations_BlockedByCommaStringEncodes pins the string shape, which
// the plugin mappers write (gitlab-sync, bitbucket-sync) and which the
// previous local coercion treated as a single opaque ID.
func TestRelations_BlockedByCommaStringEncodes(t *testing.T) {
	blocked := relTask(relTaskC, "blocked task")
	blocked.Meta = map[string]interface{}{
		"blocked_by": relTaskA + "," + relTaskB,
	}
	tasks := []*core.Task{
		relTask(relTaskA, "blocker one"),
		relTask(relTaskB, "blocker two"),
		blocked,
	}

	cal, err := vtodo.BuildVCalendar(tasks, nil, nil)
	require.NoError(t, err)
	out := mustSerialize(t, cal)
	require.Contains(t, out, "RELATED-TO;RELTYPE=DEPENDS-ON:"+relTaskA+"@tlc.local")
	require.Contains(t, out, "RELATED-TO;RELTYPE=DEPENDS-ON:"+relTaskB+"@tlc.local")
}

// TestRelations_DecodeMergesBlockers guards the merge path in the
// DEPENDS-ON branch: each edge must accumulate onto the previous ones
// rather than replace them.
//
// Note on the shape hazard this replaced: the branch previously read the
// accumulator with a bare `existing, _ := task.Meta["blocked_by"].
// ([]string)`. That assertion yields nil for the []interface{} shape
// produced by JSON decoding — the exact shape the encoder explicitly
// accepts — which would discard every prior blocker. It is unreachable
// through ParseVCalendar today (the decoder seeds Meta itself and only
// ever writes []string), so this test passes either way; routing the
// read through core.NormalizeBlockedBy removes the hazard rather than
// fixing a live defect. See TestRelations_DecodeDedupesBlockers for the
// assertion that does discriminate between the two implementations.
func TestRelations_DecodeMergesBlockers(t *testing.T) {
	// Two DEPENDS-ON edges on the same component exercise the merge.
	ics := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//tlc//vtodo//EN",
		"BEGIN:VTODO",
		"UID:" + relTaskA + "@tlc.local",
		"SUMMARY:blocker one",
		"END:VTODO",
		"BEGIN:VTODO",
		"UID:" + relTaskB + "@tlc.local",
		"SUMMARY:blocker two",
		"END:VTODO",
		"BEGIN:VTODO",
		"UID:" + relTaskC + "@tlc.local",
		"SUMMARY:blocked task",
		"RELATED-TO;RELTYPE=DEPENDS-ON:" + relTaskA + "@tlc.local",
		"RELATED-TO;RELTYPE=DEPENDS-ON:" + relTaskB + "@tlc.local",
		"END:VTODO",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	res, err := vtodo.ParseVCalendar(strings.NewReader(ics))
	require.NoError(t, err)
	got := findTaskByID(t, res, relTaskC)
	require.Equal(t, []string{relTaskA, relTaskB}, got.Meta["blocked_by"],
		"second DEPENDS-ON edge must merge with the first, not replace it")
}

// TestRelations_DecodeDedupesBlockers pins the coercion the DEPENDS-ON
// branch now shares with core: a repeated blocker collapses to one
// entry. This is the assertion that discriminates between
// core.NormalizeBlockedBy and the raw []string assertion it replaced —
// the raw read appends blindly and yields a duplicate.
func TestRelations_DecodeDedupesBlockers(t *testing.T) {
	ics := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//tlc//vtodo//EN",
		"BEGIN:VTODO",
		"UID:" + relTaskA + "@tlc.local",
		"SUMMARY:blocker one",
		"END:VTODO",
		"BEGIN:VTODO",
		"UID:" + relTaskC + "@tlc.local",
		"SUMMARY:blocked task",
		"RELATED-TO;RELTYPE=DEPENDS-ON:" + relTaskA + "@tlc.local",
		"RELATED-TO;RELTYPE=DEPENDS-ON:" + relTaskA + "@tlc.local",
		"END:VTODO",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	res, err := vtodo.ParseVCalendar(strings.NewReader(ics))
	require.NoError(t, err)
	got := findTaskByID(t, res, relTaskC)
	require.Equal(t, []string{relTaskA}, got.Meta["blocked_by"],
		"a repeated DEPENDS-ON edge must not produce a duplicate blocker")
}

// TestRelations_MissingRelTypeDefaultsToParent documents the RFC 5545
// §3.2.15 default that helpers.RelatedTo applies: a RELATED-TO with no
// RELTYPE param is a PARENT link. tlc previously matched such a property
// against neither PARENT nor DEPENDS-ON and dropped the edge entirely.
func TestRelations_MissingRelTypeDefaultsToParent(t *testing.T) {
	ics := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//tlc//vtodo//EN",
		"BEGIN:VTODO",
		"UID:" + relTrackA + "@tlc.local",
		"X-TLC-IS-TRACK:TRUE",
		"SUMMARY:Auth rewrite",
		"X-TLC-TRACK-SLUG:auth-rewrite",
		"END:VTODO",
		"BEGIN:VTODO",
		"UID:" + relTaskA + "@tlc.local",
		"SUMMARY:child task",
		"RELATED-TO:" + relTrackA + "@tlc.local",
		"END:VTODO",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	res, err := vtodo.ParseVCalendar(strings.NewReader(ics))
	require.NoError(t, err)
	got := findTaskByID(t, res, relTaskA)
	require.NotNil(t, got.TrackID, "absent RELTYPE must be read as PARENT")
	require.Equal(t, relTrackA, *got.TrackID)
}

// TestRelations_RelTypeIsCaseInsensitive pins the case-insensitive
// RELTYPE value handling. RFC 5545 §3.2 parameter values for enumerated
// params are not case sensitive; other producers emit mixed case.
func TestRelations_RelTypeIsCaseInsensitive(t *testing.T) {
	ics := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//tlc//vtodo//EN",
		"BEGIN:VTODO",
		"UID:" + relTaskA + "@tlc.local",
		"SUMMARY:blocker one",
		"END:VTODO",
		"BEGIN:VTODO",
		"UID:" + relTaskC + "@tlc.local",
		"SUMMARY:blocked task",
		"RELATED-TO;reltype=depends-on:" + relTaskA + "@tlc.local",
		"END:VTODO",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	res, err := vtodo.ParseVCalendar(strings.NewReader(ics))
	require.NoError(t, err)
	got := findTaskByID(t, res, relTaskC)
	require.Equal(t, []string{relTaskA}, got.Meta["blocked_by"])
}

// TestRelations_UnknownRelTypeIgnored keeps the existing contract: a
// RELTYPE tlc does not model contributes no TrackID and no blocker.
func TestRelations_UnknownRelTypeIgnored(t *testing.T) {
	ics := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//tlc//vtodo//EN",
		"BEGIN:VTODO",
		"UID:" + relTaskA + "@tlc.local",
		"SUMMARY:sibling task",
		"END:VTODO",
		"BEGIN:VTODO",
		"UID:" + relTaskC + "@tlc.local",
		"SUMMARY:subject task",
		"RELATED-TO;RELTYPE=SIBLING:" + relTaskA + "@tlc.local",
		"END:VTODO",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	res, err := vtodo.ParseVCalendar(strings.NewReader(ics))
	require.NoError(t, err)
	got := findTaskByID(t, res, relTaskC)
	require.Nil(t, got.TrackID)
	require.Nil(t, got.Meta["blocked_by"])
}

// TestRelations_ReencodeStable asserts that the X-VSTAR-HASH property
// helpers.AddRelatedTo stamps on any component carrying a relation does
// not destabilize the encode→decode→encode cycle.
func TestRelations_ReencodeStable(t *testing.T) {
	data := loadFixture(t, "with-dependencies.ics")
	res, err := vtodo.ParseVCalendar(strings.NewReader(string(data)))
	require.NoError(t, err)

	cal, err := vtodo.BuildVCalendar(res.Tasks, res.Tracks, res.Logs)
	require.NoError(t, err)
	first := mustSerialize(t, cal)
	require.Contains(t, first, "X-VSTAR-HASH:sha256:")

	res2, err := vtodo.ParseVCalendar(strings.NewReader(first))
	require.NoError(t, err)
	cal2, err := vtodo.BuildVCalendar(res2.Tasks, res2.Tracks, res2.Logs)
	require.NoError(t, err)
	require.Equal(t, first, mustSerialize(t, cal2))
}

func findTaskByID(t *testing.T, res *vtodo.ParseResult, id string) *core.Task {
	t.Helper()
	for _, task := range res.Tasks {
		if task.ID == id {
			return task
		}
	}
	t.Fatalf("task %s not found in parse result", id)
	return nil
}
