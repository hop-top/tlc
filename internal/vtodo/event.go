package vtodo

import (
	"fmt"
	"sort"
	"strings"
	"time"

	vstar "hop.top/vstar"
	"hop.top/vstar/helpers"

	"hop.top/tlc/internal/core"
)

// X-properties of the playthrough VEVENT and of the assignment →
// playthrough edge.
const (
	// XPropRun, on a task VTODO, is the UID of the playthrough VEVENT
	// the task was materialised into (Task.RunID). It is an X-property
	// rather than a RELATED-TO because no RELTYPE says "member of a
	// run": PARENT is taken by the mission edge, CHILD and SIBLING are
	// hierarchy, DEPENDS-ON (RFC 9253) is ordering, and RFC 5545 readers
	// degrade an unknown RELTYPE to PARENT, which would silently re-parent
	// the task. A RELTYPE for run membership is an upstream ask.
	XPropRun = "X-TLC-RUN"
	// XPropRecipeID, XPropRecipeVersion and XPropRecipeHash carry the
	// recipe a playthrough materialised: which one, at which declared
	// version, at which content hash. Version and hash are omitted when
	// the run recorded none.
	XPropRecipeID      = "X-TLC-RECIPE-ID"
	XPropRecipeVersion = "X-TLC-RECIPE-VERSION"
	XPropRecipeHash    = "X-TLC-RECIPE-HASH"
)

// turnOpeners and turnClosers are the log actions that bound a claim
// window. A claim is opened by the compare-and-set that stamps
// ClaimedAt (CLAIMED) or by a takeover of a stale one (RECLAIMED), and
// closed wherever the executor or a human clears it. The list is the
// decision document's; a configured status name is not a bound, the
// executor logging the constants above whatever the vocabulary.
var (
	turnOpeners = actionSet(core.ActionClaimed, core.ActionReclaimed)
	turnClosers = actionSet(
		core.ActionDone, core.ActionReleased, core.ActionSkipped,
		core.ActionApproved, core.ActionRejected, core.ActionRetry,
		core.ActionBlocked,
	)
)

// claimWindow is one turn: a bounded execution window on an assignment.
// A zero end is an open turn, the one Task.ClaimedAt holds.
type claimWindow struct {
	start, end time.Time
	// by is the player who held the claim: the opening entry's actor,
	// or the task's assignee when the window comes from ClaimedAt.
	by string
}

// claimWindows reconstructs a task's turns from its log, in time
// order. An opener starts a window; a closer ends the open one; an
// opener while a window is open (a reclaim of a stale claim) ends that
// window at the reclaim instant and starts the next. A closer with no
// open window is ignored. The last window is left open when nothing
// closed it. Entries without a timestamp cannot bound anything and are
// skipped.
func claimWindows(logs []*core.LogEntry) []claimWindow {
	sorted := make([]*core.LogEntry, 0, len(logs))
	for _, le := range logs {
		if le != nil && !le.Timestamp.IsZero() {
			sorted = append(sorted, le)
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})
	var out []claimWindow
	open := -1
	for _, le := range sorted {
		action := strings.ToUpper(strings.TrimSpace(le.Action))
		switch {
		case turnOpeners[action]:
			if open >= 0 {
				out[open].end = le.Timestamp
			}
			out = append(out, claimWindow{start: le.Timestamp, by: le.By})
			open = len(out) - 1
		case turnClosers[action]:
			if open >= 0 {
				out[open].end = le.Timestamp
				open = -1
			}
		}
	}
	return out
}

// turnsFor picks a task's turns. The log is the richer source: it
// holds closed turns as well as the open one, so when the exported log
// opens at least one window the task row is not consulted. Otherwise
// ClaimedAt, which holds only the open turn, is the fallback. The two
// are never combined, so a claim never appears twice.
func turnsFor(t *core.Task, logs []*core.LogEntry) []claimWindow {
	if ws := claimWindows(logs); len(ws) > 0 {
		return ws
	}
	if t.ClaimedAt != nil && !t.ClaimedAt.IsZero() {
		by := ""
		if t.AssignedTo != nil {
			by = *t.AssignedTo
		}
		return []claimWindow{{start: *t.ClaimedAt, by: by}}
	}
	return nil
}

// buildTurnComponent emits one turn as a VEVENT: DTSTART is the claim
// instant, DTEND the release when the turn is closed (helpers.NewEvent
// omits DTEND for the zero time), RELATED-TO;RELTYPE=PARENT the
// assignment, and the player as the task builder writes an assignee.
// No STATUS: the pinned vstar has no VEVENT status enum, and an open
// turn is already told apart from a closed one by the absence of
// DTEND. DTSTAMP is the turn's last modification: its end when closed,
// its start while open.
func buildTurnComponent(t *core.Task, w claimWindow, domain string, exportAt time.Time) (vstar.Component, error) {
	c, err := helpers.NewEvent(turnUID(t.ID, w.start, domain), w.start, w.end)
	if err != nil {
		return vstar.Component{}, fmt.Errorf("vtodo: turn on task %q: %w", t.ID, err)
	}
	setDTSTAMP(&c, dtstampFor(exportAt, w.end, w.start))
	c.Add(vstar.Property{Name: XPropConcept, Value: ConceptTurn})
	if t.Title != "" {
		c.Add(vstar.Property{Name: "SUMMARY", Value: t.Title})
	}
	helpers.AddRelatedTo(&c, uidFor(t.ID, domain), RelTypeParent)
	addAssignee(&c, w.by)
	finalize(&c)
	return c, nil
}

// turnUID derives a turn's UID from the task and the claim instant:
// turn-<task id>-<DTSTART>@<domain>. A task holds one claim at a time,
// so the pair is unique, and it is the same whether the window came
// from the log or from ClaimedAt.
func turnUID(taskID string, start time.Time, domain string) string {
	if taskID == "" {
		return ""
	}
	if domain == "" {
		domain = DefaultUIDDomain
	}
	return fmt.Sprintf("turn-%s-%s@%s", taskID, vstar.FormatTime(start), domain)
}

// buildPlaythroughComponent emits a recipe run as a VEVENT: DTSTART
// is the materialisation instant and there is no DTEND, a run having
// no end timestamp (RFC 5545 §3.6.1 permits it). The recipe identity
// travels as X-properties, the mission edge as RELATED-TO;RELTYPE=PARENT
// to the track VTODO when the run has a track; a trackless run is a
// path through the world and carries no edge. DTSTAMP is the same
// instant: a run row never changes after it is written.
func buildPlaythroughComponent(run *core.RecipeRun, domain string, exportAt time.Time) (vstar.Component, error) {
	start := dtstampFor(exportAt, run.CreatedAt)
	c, err := helpers.NewEvent(uidFor(run.ID, domain), start, time.Time{})
	if err != nil {
		return vstar.Component{}, fmt.Errorf("vtodo: recipe run %q: %w", run.ID, err)
	}
	setDTSTAMP(&c, start)
	c.Add(vstar.Property{Name: XPropConcept, Value: ConceptPlaythrough})
	if run.RecipeID != "" {
		c.Add(vstar.Property{Name: "SUMMARY", Value: run.RecipeID})
		c.Add(vstar.Property{Name: XPropRecipeID, Value: run.RecipeID})
	}
	if run.Version != "" {
		c.Add(vstar.Property{Name: XPropRecipeVersion, Value: run.Version})
	}
	if run.Hash != "" {
		c.Add(vstar.Property{Name: XPropRecipeHash, Value: run.Hash})
	}
	if run.TrackID != "" {
		helpers.AddRelatedTo(&c, uidFor(run.TrackID, domain), RelTypeParent)
	}
	if run.ProjectID != "" {
		c.Add(vstar.Property{Name: XPropProjectID, Value: run.ProjectID})
	}
	finalize(&c)
	return c, nil
}
