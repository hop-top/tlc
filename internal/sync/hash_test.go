package sync

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	vstar "hop.top/vstar"
	"hop.top/vstar/canonical"
	"hop.top/vstar/diff"
	"hop.top/vstar/hashing"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

var (
	syncedAt   = time.Date(2026, 5, 2, 14, 30, 0, 0, time.UTC)
	originName = "github"
)

func strp(s string) *string { return &s }

// syncedTask is a task whose content was last pushed at syncedAt, with
// the content hash of that instant recorded, the state every task is in
// after MarkSynced.
func syncedTask(t *testing.T) *core.Task {
	t.Helper()
	remind := syncedAt.Add(-time.Hour)
	task := &core.Task{
		ID:           "task_01h455vb4pex5vsknk084sn02q",
		Seq:          7,
		Title:        "Replace JWT signer",
		Description:  "Swap HS256 for RS256.",
		Status:       core.StatusInProgress,
		AssignedTo:   strp("alice"),
		Tags:         []string{"security", "auth"},
		Reference:    "https://github.com/acme/api/issues/42",
		Effort:       core.Effort("M"),
		Priority:     core.Priority("high"),
		CreatedAt:    syncedAt.Add(-48 * time.Hour),
		UpdatedAt:    syncedAt.Add(-time.Minute),
		OriginSystem: &originName,
		ProjectID:    strp("proj_alpha"),
		RemindAt:     &remind,
		Meta: map[string]interface{}{
			"origin_system": "github",
			"origin_id":     "42",
			"blocked_by":    []string{"task_01h455vb4pex5vsknk084sn0az"},
		},
	}
	require.NoError(t, MarkSynced(task, syncedAt))
	return task
}

// TestNeedsPush_TouchOnlyIsNotAChange is the case the timestamp
// heuristic got wrong: a save that bumps UpdatedAt without changing
// exported content is not a pending push.
func TestNeedsPush_TouchOnlyIsNotAChange(t *testing.T) {
	task := syncedTask(t)
	task.UpdatedAt = syncedAt.Add(2 * time.Hour)
	require.False(t, task.NeedsPush())

	// Fields the exporter never sees are touches too.
	task.Attempts = 3
	claimed := syncedAt.Add(3 * time.Hour)
	task.ClaimedAt = &claimed
	task.StaleFiredAt = &claimed
	require.False(t, task.NeedsPush())
}

// TestNeedsPush_ContentChangeIsAChange proves each exported field moves
// the hash, UpdatedAt notwithstanding.
func TestNeedsPush_ContentChangeIsAChange(t *testing.T) {
	cases := map[string]func(*core.Task){
		"title":       func(x *core.Task) { x.Title = "Replace JWT signer, again" },
		"description": func(x *core.Task) { x.Description = "Swap HS256 for EdDSA." },
		"status":      func(x *core.Task) { x.Status = core.StatusDone },
		"tags":        func(x *core.Task) { x.Tags = append(x.Tags, "urgent") },
		"assignee":    func(x *core.Task) { x.AssignedTo = strp("bob") },
		"priority":    func(x *core.Task) { x.Priority = core.Priority("low") },
		"reminder":    func(x *core.Task) { r := syncedAt.Add(time.Hour); x.RemindAt = &r },
		"blocked_by":  func(x *core.Task) { x.Meta["blocked_by"] = []string{} },
		"meta":        func(x *core.Task) { x.Meta["milestone"] = "v2" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			task := syncedTask(t)
			mutate(task)
			// UpdatedAt deliberately left BEFORE LastSyncAt: the hash
			// decides, not the clock.
			require.True(t, task.UpdatedAt.Before(*task.LastSyncAt))
			require.True(t, task.NeedsPush())
		})
	}
}

// TestNeedsPush_FirstSyncFallsBackToTimestamps pins the fallback: with
// LastSyncAt set but no hash recorded, the one-millisecond timestamp
// rule stands.
func TestNeedsPush_FirstSyncFallsBackToTimestamps(t *testing.T) {
	task := syncedTask(t)
	delete(task.Meta, core.MetaLastSyncHash)
	_, ok := task.LastSyncHash()
	require.False(t, ok)

	task.UpdatedAt = syncedAt.Add(2 * time.Millisecond)
	require.True(t, task.NeedsPush(), "touch after sync, no baseline: timestamp rule says push")

	task.UpdatedAt = syncedAt
	require.False(t, task.NeedsPush())
}

// TestMarkSynced_RecordsBaseline proves the recorded hash is the
// task's own content hash and that recording it does not feed back
// into the hash (the key is scrubbed before hashing).
func TestMarkSynced_RecordsBaseline(t *testing.T) {
	task := syncedTask(t)
	before, err := ContentHash(task)
	require.NoError(t, err)
	recorded, ok := task.LastSyncHash()
	require.True(t, ok)
	require.Equal(t, before, recorded)
	require.True(t, strings.HasPrefix(recorded, "sha256:"))
	require.Equal(t, syncedAt, *task.LastSyncAt)

	require.NoError(t, MarkSynced(task, syncedAt.Add(time.Hour)))
	again, _ := task.LastSyncHash()
	require.Equal(t, recorded, again, "re-marking unchanged content must not move the baseline")
}

// TestContentHash_IgnoresIdentity proves two tasks that export the same
// content hash the same whatever their ID, sequence, project, creation
// instant or last modification, which is what lets a remote copy (fresh
// ID, no sequence) be compared with its local twin.
func TestContentHash_IgnoresIdentity(t *testing.T) {
	a := syncedTask(t)
	b := syncedTask(t)
	b.ID = ""
	b.Seq = 0
	b.ProjectID = nil
	b.CreatedAt = time.Time{}
	b.UpdatedAt = time.Time{}
	b.LastSyncAt = nil
	delete(b.Meta, core.MetaLastSyncHash)

	ha, err := ContentHash(a)
	require.NoError(t, err)
	hb, err := ContentHash(b)
	require.NoError(t, err)
	require.Equal(t, ha, hb)
}

// TestContentHash_CompletedTaskTouchOnly covers COMPLETED, which the
// exporter derives from UpdatedAt when no log is supplied: a done task
// touched after sync is still unchanged.
func TestContentHash_CompletedTaskTouchOnly(t *testing.T) {
	task := syncedTask(t)
	task.Status = core.StatusDone
	require.NoError(t, MarkSynced(task, syncedAt))
	task.UpdatedAt = syncedAt.Add(time.Hour)
	require.False(t, task.NeedsPush())
}

// TestContentView_StripsVolatileAtEveryLevel proves the view carries no
// identity, timestamp or hash property on the VTODO or its VALARM, and
// still carries the content that matters.
func TestContentView_StripsVolatileAtEveryLevel(t *testing.T) {
	view, err := contentView(syncedTask(t))
	require.NoError(t, err)
	var walk func(c vstar.Component, label string)
	walk = func(c vstar.Component, label string) {
		for name := range volatileProps {
			_, present := c.Get(name)
			require.False(t, present, "%s still carries %s", label, name)
		}
		for _, s := range c.Sub {
			walk(s, label+" > "+string(s.Type))
		}
	}
	walk(view, string(view.Type))
	require.Len(t, view.Sub, 1, "the reminder must survive as a VALARM")
	for _, name := range []string{"SUMMARY", "DESCRIPTION", "STATUS", "CATEGORIES", "RELATED-TO", vtodo.XPropEffort, vtodo.XPropMeta} {
		_, present := view.Get(name)
		require.True(t, present, "view lost %s", name)
	}
	meta, _ := view.Get(vtodo.XPropMeta)
	require.NotContains(t, meta.Value, core.MetaLastSyncHash)
}

// TestExport_DeterministicAcrossClocks is the determinism claim behind
// using the hash as an ETag: exporting an unchanged task under two
// different export clocks yields byte-identical canonical bytes, the
// same X-VSTAR-HASH and the same serialized document, because DTSTAMP is
// the task's UpdatedAt, not the clock.
func TestExport_DeterministicAcrossClocks(t *testing.T) {
	task := syncedTask(t)
	build := func(clock time.Time) (vstar.Component, string) {
		cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil, vtodo.WithExportTime(clock))
		require.NoError(t, err)
		require.Len(t, cal.Components, 1)
		out, err := vtodo.Serialize(cal)
		require.NoError(t, err)
		return cal.Components[0], out
	}
	c1, s1 := build(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	c2, s2 := build(time.Date(2031, 12, 31, 23, 59, 59, 0, time.UTC))

	require.True(t, bytes.Equal(canonical.Component(c1), canonical.Component(c2)))
	h1, ok := hashing.GetXVSTAR(c1)
	require.True(t, ok)
	h2, _ := hashing.GetXVSTAR(c2)
	require.Equal(t, h1, h2)
	require.Equal(t, s1, s2, "serialized documents must be byte-identical")

	// Without an export clock at all: two wall-clock exports agree too.
	calA, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.NoError(t, err)
	calB, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil)
	require.NoError(t, err)
	require.True(t, diff.Calendar(calA, calB))
}

// TestExport_TimestamplessTaskIsClockStamped marks the boundary of that
// claim: a task with neither UpdatedAt nor CreatedAt takes DTSTAMP from
// the export clock (documented in VSTAR-CONFORMANCE.md), so its
// X-VSTAR-HASH differs per export. The sync content hash strips DTSTAMP
// and is unaffected.
func TestExport_TimestamplessTaskIsClockStamped(t *testing.T) {
	task := &core.Task{ID: "task_01h455vb4pex5vsknk084sn0zz", Title: "No clock"}
	hashAt := func(clock time.Time) string {
		cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil, vtodo.WithExportTime(clock))
		require.NoError(t, err)
		h, ok := hashing.GetXVSTAR(cal.Components[0])
		require.True(t, ok)
		return h
	}
	require.NotEqual(t,
		hashAt(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		hashAt(time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)),
		"X-VSTAR-HASH of a timestamp-less task is expected to follow the export clock")

	h1, err := ContentHash(task)
	require.NoError(t, err)
	task.UpdatedAt = time.Now().UTC()
	h2, err := ContentHash(task)
	require.NoError(t, err)
	require.Equal(t, h1, h2)
}

// TestDetectConflict_LocalTouchIsNotAConflict: local was saved after the
// sync (UpdatedAt moved) but its content is what was synced, so a remote
// change is a plain pull, not a conflict, however different the remote.
func TestDetectConflict_LocalTouchIsNotAConflict(t *testing.T) {
	local := syncedTask(t)
	local.UpdatedAt = syncedAt.Add(10 * time.Minute)
	remote := syncedTask(t)
	remote.ID = ""
	remote.Title = "Replace JWT signer (remote edit)"
	remote.UpdatedAt = syncedAt.Add(20 * time.Minute)

	require.Nil(t, DetectConflict(local, remote))
}

// TestDetectConflict_HashOutranksClock: local content moved since the
// sync although UpdatedAt did not; the baseline hash still catches it.
func TestDetectConflict_HashOutranksClock(t *testing.T) {
	local := syncedTask(t)
	local.Description = "Swap HS256 for EdDSA."
	require.True(t, local.UpdatedAt.Before(*local.LastSyncAt))
	remote := syncedTask(t)
	remote.Title = "Replace JWT signer (remote edit)"
	remote.UpdatedAt = syncedAt.Add(20 * time.Minute)

	c := DetectConflict(local, remote)
	require.NotNil(t, c)
	require.Contains(t, c.Description, "differs in DESCRIPTION, SUMMARY")
}

// TestDetectConflict_SameContentBothSides: both moved, both landed on the
// same content (the remote copy carries no ID, sequence or project): no
// conflict.
func TestDetectConflict_SameContentBothSides(t *testing.T) {
	local := syncedTask(t)
	local.Title = "Same edit"
	local.UpdatedAt = syncedAt.Add(10 * time.Minute)
	remote := syncedTask(t)
	remote.ID = ""
	remote.Seq = 0
	remote.ProjectID = nil
	remote.Title = "Same edit"
	remote.UpdatedAt = syncedAt.Add(20 * time.Minute)
	delete(remote.Meta, core.MetaLastSyncHash)

	require.Nil(t, DetectConflict(local, remote))
}

// TestDetectConflict_CarriesPropertyLevelDiff proves a conflict names
// what differs, property by property, local on the left, for manual
// resolution to show.
func TestDetectConflict_CarriesPropertyLevelDiff(t *testing.T) {
	local := syncedTask(t)
	local.Description = "Swap HS256 for EdDSA."
	local.Tags = []string{"security"}
	local.UpdatedAt = syncedAt.Add(10 * time.Minute)
	remote := syncedTask(t)
	remote.ID = ""
	remote.Description = "Swap HS256 for RS256, rotate keys."
	remote.Tags = []string{"security", "auth", "rotation"}
	remote.UpdatedAt = syncedAt.Add(20 * time.Minute)

	c := DetectConflict(local, remote)
	require.NotNil(t, c)
	require.False(t, c.Diff.Empty())

	byOp := map[diff.DiffOp][]string{}
	for _, p := range c.Diff.Properties {
		byOp[p.Op] = append(byOp[p.Op], p.Property.Name+":"+p.Property.Value)
	}
	require.Equal(t, []string{"DESCRIPTION:Swap HS256 for RS256, rotate keys."}, byOp[diff.OpChanged])
	require.ElementsMatch(t, []string{"CATEGORIES:auth", "CATEGORIES:rotation"}, byOp[diff.OpAdded])
	require.Empty(t, byOp[diff.OpRemoved])

	rendered := c.Diff.String()
	require.Contains(t, rendered, "~ DESCRIPTION: Swap HS256 for EdDSA. -> Swap HS256 for RS256, rotate keys.")
	require.Contains(t, rendered, "+ CATEGORIES:rotation")
	require.Contains(t, c.Description, "differs in CATEGORIES, DESCRIPTION")
}

// TestDetectConflict_DiffIgnoresIdentityAndClock: a conflict on one
// field reports that one field, never the UID, timestamps, sequence or
// wire hash that legitimately differ between the two copies.
func TestDetectConflict_DiffIgnoresIdentityAndClock(t *testing.T) {
	local := syncedTask(t)
	local.RemindAt = nil // the VALARM carries the title as DESCRIPTION; keep this to one property
	local.Title = "Local title"
	local.UpdatedAt = syncedAt.Add(10 * time.Minute)
	remote := syncedTask(t)
	remote.ID = "GH-42"
	remote.RemindAt = nil
	remote.Seq = 0
	remote.ProjectID = nil
	remote.CreatedAt = syncedAt.Add(-72 * time.Hour)
	remote.Title = "Remote title"
	remote.UpdatedAt = syncedAt.Add(20 * time.Minute)
	delete(remote.Meta, core.MetaLastSyncHash)

	c := DetectConflict(local, remote)
	require.NotNil(t, c)
	require.Len(t, c.Diff.Properties, 1)
	require.Equal(t, "SUMMARY", c.Diff.Properties[0].Property.Name)
	require.Equal(t, diff.OpChanged, c.Diff.Properties[0].Op)
	require.Equal(t, "Local title", c.Diff.Properties[0].Old.Value)
	require.Equal(t, "Remote title", c.Diff.Properties[0].Property.Value)
	require.Empty(t, c.Diff.SubDiffs)
}

// TestDetectConflict_ReminderChangeSurfacesInSubDiff: a VALARM-level
// difference is reported under the sub-component path.
func TestDetectConflict_ReminderChangeSurfacesInSubDiff(t *testing.T) {
	local := syncedTask(t)
	local.Title = "Local title"
	local.UpdatedAt = syncedAt.Add(10 * time.Minute)
	remote := syncedTask(t)
	remote.Title = "Remote title"
	later := syncedAt.Add(48 * time.Hour)
	remote.RemindAt = &later
	remote.UpdatedAt = syncedAt.Add(20 * time.Minute)

	c := DetectConflict(local, remote)
	require.NotNil(t, c)
	require.Len(t, c.Diff.SubDiffs, 1)
	require.Equal(t, "VALARM[#0]", c.Diff.SubDiffs[0].Path)
	require.Contains(t, c.Description, "VALARM[#0]")
}
