package vtodo

import (
	"fmt"
	"time"

	vstar "hop.top/vstar"
	"hop.top/vstar/helpers"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// Per-section builders behind buildTaskComponent. Each appends one
// group of properties in the order the wire has always carried them;
// buildTaskComponent fixes the order of the groups, and the fixtures
// under tests/fixtures/vtodo pin the result.

// addTaskStatus writes the RFC 5545 status, then the tlc status name.
func addTaskStatus(c *vstar.Component, t *core.Task, statusDefs []config.StatusDefinition, doneAt time.Time) {
	if statusRole(t.Status, statusDefs) == config.RoleCompleted {
		// Complete writes STATUS, COMPLETED and PERCENT-COMPLETE=100 as
		// one unit, so a foreign reader sees a consistent "done" triple
		// rather than a bare STATUS. COMPLETED is the instant of the last
		// completing transition when the log has one, else the last
		// modification, the closest fact the model holds.
		if doneAt.IsZero() {
			doneAt = t.UpdatedAt
		}
		helpers.Complete(c, doneAt)
	} else {
		c.Add(vstar.Property{Name: "STATUS", Value: statusToWire(t.Status, statusDefs)})
	}
	if t.Status != "" {
		c.Add(vstar.Property{Name: XPropStatus, Value: string(t.Status)})
	}
}

// addTaskPriority writes the numeric PRIORITY, the priority name and
// its recorded provenance.
func addTaskPriority(c *vstar.Component, t *core.Task, defs []config.PriorityDefinition) {
	if p := priorityToICS(t.Priority, defs); p != 0 {
		c.Add(vstar.Property{Name: "PRIORITY", Value: fmt.Sprintf("%d", p)})
	}
	if t.Priority != "" {
		// Name alongside the number: the number is a lossy projection
		// onto nine slots, the name is the fact.
		c.Add(vstar.Property{Name: XPropPriority, Value: string(t.Priority)})
	}
	if v := metaString(t.Meta, core.MetaPrioritySource); v != "" {
		c.Add(vstar.Property{Name: XPropPrioritySource, Value: v})
	}
	if v := metaString(t.Meta, core.MetaPriorityRule); v != "" {
		c.Add(vstar.Property{Name: XPropPriorityRule, Value: v})
	}
}

// addTaskDates writes CREATED and LAST-MODIFIED for the instants the
// task carries.
func addTaskDates(c *vstar.Component, t *core.Task) {
	if !t.CreatedAt.IsZero() {
		c.Add(vstar.Property{Name: propCreated, Value: vstar.FormatTime(t.CreatedAt)})
	}
	if !t.UpdatedAt.IsZero() {
		c.Add(vstar.Property{Name: "LAST-MODIFIED", Value: vstar.FormatTime(t.UpdatedAt)})
	}
}

// addTaskRelations writes the containment edge to the track, then one
// DEPENDS-ON edge per blocker.
func addTaskRelations(c *vstar.Component, t *core.Task, domain string) {
	if t.TrackID != nil && *t.TrackID != "" {
		addParentRelation(c, uidFor(*t.TrackID, domain))
	}
	for _, blocker := range blockedByList(t.Meta) {
		helpers.AddRelatedTo(c, uidFor(blocker, domain), vstar.RelDependsOn)
	}
}

// addTaskTLCFields writes the typed fields with no iCalendar
// equivalent: effort, player, project, sequence, archived flag and the
// run edge.
func addTaskTLCFields(c *vstar.Component, t *core.Task, domain string) {
	if t.Effort != "" {
		c.Add(vstar.Property{Name: XPropEffort, Value: string(t.Effort)})
	}
	if t.AssignedTo != nil {
		addAssignee(c, *t.AssignedTo)
	}
	if t.ProjectID != nil && *t.ProjectID != "" {
		c.Add(vstar.Property{Name: XPropProjectID, Value: *t.ProjectID})
	}
	if t.Seq > 0 {
		c.Add(vstar.Property{Name: XPropTaskSeq, Value: fmt.Sprintf("%d", t.Seq)})
	}
	if t.Archived {
		c.Add(vstar.Property{Name: XPropArchived, Value: xPropTrue})
	}
	if t.RunID != "" {
		// The assignment → playthrough edge; see XPropRun for why it is
		// not a RELATED-TO.
		c.Add(vstar.Property{Name: XPropRun, Value: uidFor(t.RunID, domain)})
	}
}
