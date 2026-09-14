package vtodo_test

import (
	"bytes"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	vstar "hop.top/vstar"
	"hop.top/vstar/codec/rfc5545"
	"hop.top/vstar/hashing"
	"hop.top/vstar/validate"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

const gateTaskPath = "VCALENDAR.VTODO[uid=task_01h455vb4pex5vsknk084sn02q@tlc.local]"

// requireValidExport is the gate every round trip and builder test
// passes through: no blocking diagnostic. Warnings and allowed errors
// are logged, so `-v` output shows what the policy tolerated, and
// returned so a test can pin them.
func requireValidExport(t *testing.T, cal vstar.Calendar) vtodo.Report {
	t.Helper()
	r := vtodo.ValidateExport(cal)
	for _, d := range r.Warnings {
		t.Logf("validate warning %s %s: %s", d.Code, d.Path, d.Message)
	}
	for _, d := range r.Allowed {
		t.Logf("validate allowed %s %s: %s", d.Code, d.Path, d.Message)
	}
	require.NoError(t, r.Err())
	return r
}

// codes returns the sorted diagnostic codes, nil when there are none.
func codes(diags []validate.Diagnostic) []string {
	if len(diags) == 0 {
		return nil
	}
	out := make([]string, 0, len(diags))
	for _, d := range diags {
		out = append(out, d.Code)
	}
	sort.Strings(out)
	return out
}

// keys returns sorted "CODE PATH" pairs, nil when there are none.
func keys(diags []validate.Diagnostic) []string {
	if len(diags) == 0 {
		return nil
	}
	out := make([]string, 0, len(diags))
	for _, d := range diags {
		out = append(out, d.Code+" "+d.Path)
	}
	sort.Strings(out)
	return out
}

// TestValidateGate_AllowedErrorCodesPinned pins the allow-list to
// exactly the deviations docs/VSTAR-CONFORMANCE.md records. Adding a
// code here without a deviation entry is the change this test exists
// to make visible.
func TestValidateGate_AllowedErrorCodesPinned(t *testing.T) {
	require.Equal(
		t,
		map[string]struct{}{validate.CodeVTODOMissingDue: {}},
		vtodo.AllowedErrorCodes,
	)
}

// TestValidateGate_EveryBuilderPath runs the gate over one build per
// builder path and pins which allowed diagnostics each one carries
// today, so a new error or warning on any path is a visible change,
// not a silently tolerated one.
func TestValidateGate_EveryBuilderPath(t *testing.T) {
	task := func(mut func(*core.Task)) *core.Task {
		tk := sampleTask()
		mut(tk)
		return tk
	}
	track := func(mut func(*core.Track)) *core.Track {
		tr := hashTrack()
		mut(tr)
		return tr
	}
	due := fixedTime.Add(72 * time.Hour)
	vs040 := []string{validate.CodeVTODOMissingDue}

	cases := []struct {
		name        string
		tasks       []*core.Task
		tracks      []*core.Track
		logs        []*core.LogEntry
		opts        []vtodo.Option
		wantAllowed []string
	}{
		{
			name:  "task: dated, open, reminder, RRULE",
			tasks: []*core.Task{sampleTask()},
		},
		{
			name: "task: date-only DUE",
			tasks: []*core.Task{task(func(tk *core.Task) {
				tk.Meta = map[string]interface{}{vtodo.MetaDueDateOnly: true}
			})},
		},
		{
			name: "task: several reminders",
			tasks: []*core.Task{task(func(tk *core.Task) {
				tk.Meta = map[string]interface{}{vtodo.MetaReminders: []string{
					"2026-05-03T04:00:00Z", "2026-05-03T06:00:00Z",
				}}
			})},
		},
		{
			name:  "task: auto reminder suppressed",
			tasks: []*core.Task{task(func(tk *core.Task) { tk.NoAutoRemind = true })},
		},
		{
			name:        "task: undated, open",
			tasks:       []*core.Task{task(func(tk *core.Task) { tk.DueAt = nil })},
			wantAllowed: vs040,
		},
		{
			name: "task: undated, completed role",
			tasks: []*core.Task{task(func(tk *core.Task) {
				tk.DueAt = nil
				tk.Status = core.StatusDone
			})},
		},
		{
			name: "task: undated, skipped role",
			tasks: []*core.Task{task(func(tk *core.Task) {
				tk.DueAt = nil
				tk.Status = core.StatusSkipped
			})},
			wantAllowed: vs040,
		},
		{
			name:  "task: email assignee as ATTENDEE",
			tasks: []*core.Task{task(func(tk *core.Task) { tk.AssignedTo = ptr("alice@example.com") })},
		},
		{
			name: "task: blockers and Meta",
			tasks: []*core.Task{task(func(tk *core.Task) {
				tk.Meta = map[string]interface{}{
					"blocked_by": []string{"task_01h455vb4pex5vsknk084sn0az"},
					"estimate":   3,
				}
			})},
		},
		{
			name:  "task: member of a track",
			tasks: []*core.Task{task(func(tk *core.Task) { tk.TrackID = ptr(hashTrack().ID) })},
		},
		{
			name:        "track: no members, no DUE",
			tracks:      []*core.Track{hashTrack()},
			wantAllowed: vs040,
		},
		{
			name:   "track: DUE",
			tracks: []*core.Track{track(func(tr *core.Track) { tr.DueAt = &due })},
		},
		{
			name:        "track: members",
			tasks:       []*core.Task{task(func(tk *core.Task) { tk.TrackID = ptr(hashTrack().ID) })},
			tracks:      []*core.Track{hashTrack()},
			wantAllowed: vs040,
		},
		{
			// STATUS=COMPLETED now pairs with COMPLETED, the other half
			// of what spec 05 §5 accepts, so no VS040 even without DUE.
			name:   "track: completed, no DUE",
			tracks: []*core.Track{track(func(tr *core.Track) { tr.Status = core.TrackStatusCompleted })},
		},
		{
			name: "journal",
			logs: []*core.LogEntry{{
				TaskID:    sampleTask().ID,
				Timestamp: fixedTime,
				By:        "alice",
				Action:    "CLAIMED",
				Note:      "starting work",
				Meta:      map[string]interface{}{"source": "cli"},
			}},
			opts: []vtodo.Option{vtodo.WithIncludeLogs(true)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cal, err := vtodo.BuildVCalendar(tc.tasks, tc.tracks, tc.logs, tc.opts...)
			require.NoError(t, err)
			r := requireValidExport(t, cal)
			require.Equal(t, tc.wantAllowed, codes(r.Allowed))
			require.Empty(t, r.Warnings, "%v", keys(r.Warnings))
		})
	}
}

// TestValidateGate_UndatedOpenTaskIsAllowedVS040 is the allow-list's
// reason to exist: an open task without a due date is the normal case
// for tlc, and it exports as a VTODO with neither DUE nor COMPLETED.
// The gate must let it through as VS040 and report nothing else.
func TestValidateGate_UndatedOpenTaskIsAllowedVS040(t *testing.T) {
	tk := sampleTask()
	tk.DueAt = nil
	cal, err := vtodo.BuildVCalendar([]*core.Task{tk}, nil, nil)
	require.NoError(t, err)

	r := vtodo.ValidateExport(cal)
	require.NoError(t, r.Err())
	require.Empty(t, r.Blocking, "%v", keys(r.Blocking))
	require.Equal(t, []string{validate.CodeVTODOMissingDue + " " + gateTaskPath}, keys(r.Allowed))
}

// TestValidateGate_TrackVS040NamesTheTrack proves that in a
// track-with-members build the allowed VS040 is the undated track's,
// and the dated member is clean.
func TestValidateGate_TrackVS040NamesTheTrack(t *testing.T) {
	track := hashTrack()
	member := sampleTask()
	member.TrackID = ptr(track.ID)
	cal, err := vtodo.BuildVCalendar([]*core.Task{member}, []*core.Track{track}, nil)
	require.NoError(t, err)

	r := requireValidExport(t, cal)
	require.Equal(
		t,
		[]string{validate.CodeVTODOMissingDue + " VCALENDAR.VTODO[uid=" + fidelityTrackUID + "]"},
		keys(r.Allowed),
	)
}

// TestValidateGate_WalksSubComponents proves the Sub walk is
// load-bearing: validate.Validate never looks inside a VTODO, so
// without tlc's own recursion a VALARM stripped of its UID would pass.
func TestValidateGate_WalksSubComponents(t *testing.T) {
	build := func(t *testing.T) vstar.Calendar {
		t.Helper()
		cal, err := vtodo.BuildVCalendar([]*core.Task{sampleTask()}, nil, nil)
		require.NoError(t, err)
		require.Len(t, cal.Components, 1)
		// Sub[0] is the RemindAt alarm the cases below mutate; Sub[1]
		// is the auto reminder.
		require.Len(t, cal.Components[0].Sub, 2)
		return cal
	}

	t.Run("VALARM without UID blocks as VS001", func(t *testing.T) {
		cal := build(t)
		cal.Components[0].Sub[0].Remove("UID")

		r := vtodo.ValidateExport(cal)
		require.Error(t, r.Err())
		require.Contains(t, codes(r.Blocking), validate.CodeMissingUID)
		// A UID-less nested component takes the positional locator
		// validate.ValidateComponent assigns it, anchored under the parent.
		require.Contains(t, keys(r.Blocking),
			validate.CodeMissingUID+" "+gateTaskPath+".VALARM[#0].UID")
	})

	t.Run("VALARM without DTSTAMP blocks as VS002", func(t *testing.T) {
		cal := build(t)
		cal.Components[0].Sub[0].Remove("DTSTAMP")

		r := vtodo.ValidateExport(cal)
		require.Contains(t, keys(r.Blocking),
			validate.CodeMissingDTSTAMP+" "+gateTaskPath+".VALARM[uid=task_01h455vb4pex5vsknk084sn02q-alarm@tlc.local].DTSTAMP")
	})

	t.Run("tampered VALARM blocks as VS010 on the alarm and its parent", func(t *testing.T) {
		cal := build(t)
		cal.Components[0].Sub[0].Set(vstar.Property{
			Name:   "TRIGGER",
			Params: []vstar.Param{{Name: "VALUE", Value: "DATE-TIME"}},
			Value:  "20260503T033000Z",
		})

		r := vtodo.ValidateExport(cal)
		require.Equal(t, []string{
			validate.CodeBadXVSTARHash + " " + gateTaskPath + ".VALARM[uid=task_01h455vb4pex5vsknk084sn02q-alarm@tlc.local].X-VSTAR-HASH",
			validate.CodeBadXVSTARHash + " " + gateTaskPath + ".X-VSTAR-HASH",
		}, keys(r.Blocking), "the parent's hash covers its Sub, so both break")
	})
}

// TestValidateGate_WarningsDoNotBlock pins the warning policy: a
// SeverityWarning is reported, never fatal. An RRULE feature outside
// the vstar evaluator's scope (VS050) is the case tlc can actually
// produce, since it emits a user's RRULE verbatim.
func TestValidateGate_WarningsDoNotBlock(t *testing.T) {
	tk := sampleTask()
	tk.RRule = "FREQ=MINUTELY;INTERVAL=5"
	cal, err := vtodo.BuildVCalendar([]*core.Task{tk}, nil, nil)
	require.NoError(t, err)

	r := requireValidExport(t, cal)
	require.Empty(t, r.Blocking)
	require.Equal(t, []string{validate.CodeRRuleUnsupported + " " + gateTaskPath + ".RRULE"}, keys(r.Warnings))
}

// TestValidateGate_ErrNamesEveryBlockingDiagnostic proves an error
// outside the allow-list blocks, that the returned error names each one
// by code and path, and that an allowed VS040 alongside them stays out
// of the error.
func TestValidateGate_ErrNamesEveryBlockingDiagnostic(t *testing.T) {
	tk := sampleTask()
	tk.DueAt = nil
	cal, err := vtodo.BuildVCalendar([]*core.Task{tk}, nil, nil)
	require.NoError(t, err)
	require.Len(t, cal.Components[0].Sub, 1)
	cal.Components[0].Remove(hashing.XVSTARHashProperty)
	cal.Components[0].Sub[0].Remove("DTSTAMP")

	r := vtodo.ValidateExport(cal)
	err = r.Err()
	require.Error(t, err)
	msg := err.Error()
	require.True(t, strings.HasPrefix(msg, "vtodo: export fails V* validation (3 blocking)"), msg)
	require.Contains(t, msg, "\n  "+validate.CodeMissingXVSTARHash+" "+gateTaskPath+".X-VSTAR-HASH: ")
	require.Contains(t, msg, "\n  "+validate.CodeMissingDTSTAMP+" "+gateTaskPath+".VALARM[uid=task_01h455vb4pex5vsknk084sn02q-alarm@tlc.local].DTSTAMP: ")
	require.Contains(t, msg, "\n  "+validate.CodeBadXVSTARHash+" "+gateTaskPath+".VALARM[uid=task_01h455vb4pex5vsknk084sn02q-alarm@tlc.local].X-VSTAR-HASH: ")
	require.NotContains(t, msg, validate.CodeVTODOMissingDue, "an allowed code never blocks")
	require.Equal(t, []string{validate.CodeVTODOMissingDue}, codes(r.Allowed))
}

// TestFixture_ValidatesUnderGate runs the gate over the committed golden
// documents and pins exactly which allowed diagnostics each carries:
// the undated track in track-with-tasks.ics, nothing else.
func TestFixture_ValidatesUnderGate(t *testing.T) {
	cases := []struct {
		name        string
		wantAllowed []string
	}{
		{name: "single-task.ics"},
		{
			name:        "track-with-tasks.ics",
			wantAllowed: []string{validate.CodeVTODOMissingDue + " VCALENDAR.VTODO[uid=" + fidelityTrackUID + "]"},
		},
		{name: "recurring-rrule.ics"},
		{name: "with-dependencies.ics"},
		{name: "with-logs.ics"},
		{name: "recipe-run.ics"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cal, err := rfc5545.Parse(bytes.NewReader(loadFixture(t, tc.name)))
			require.NoError(t, err)
			r := requireValidExport(t, cal)
			require.Equal(t, tc.wantAllowed, keys(r.Allowed))
			require.Empty(t, r.Warnings, "%v", keys(r.Warnings))
		})
	}
}
