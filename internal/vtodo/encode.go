package vtodo

import (
	"fmt"
	"sort"
	"strings"

	ics "github.com/arran4/golang-ical"

	"hop.top/tlc/internal/core"
)

// RFC 5545 / RFC 9253 RELTYPE parameter values. RFC 9253 introduces
// DEPENDS-ON; the lib treats RELTYPE values as opaque strings (see
// docs/superpowers/specs/2026-05-02-golang-ical-decision.md), so we
// hold our own constants.
const (
	RelTypeParent    = "PARENT"
	RelTypeChild     = "CHILD"
	RelTypeDependsOn = "DEPENDS-ON"
)

// X-property names used to round-trip tlc-specific fields that have no
// native iCalendar equivalent.
const (
	XPropEffort    = "X-TLC-EFFORT"
	XPropAssignee  = "X-TLC-ASSIGNEE"
	XPropProjectID = "X-TLC-PROJECT-ID"
	XPropTaskSeq   = "X-TLC-TASK-SEQ"
	XPropTrackSlug = "X-TLC-TRACK-SLUG"
	XPropTrackType = "X-TLC-TRACK-TYPE"
	XPropTrackKind = "X-TLC-IS-TRACK"
	XPropLogAction = "X-TLC-LOG-ACTION"
	XPropLogBy     = "X-TLC-LOG-BY"
	XPropLogTaskID = "X-TLC-LOG-TASK"
)

// reltypeParam is a tiny PropertyParameter that emits RELTYPE=<value>.
// Equivalent to the lib's KeyValues helpers but specialised so we don't
// have to repeat the boilerplate.
type reltypeParam string

func (r reltypeParam) KeyValue(_ ...interface{}) (string, []string) {
	return "RELTYPE", []string{string(r)}
}

// BuildVCalendar serialises tasks, tracks and (optionally) log entries
// into a VCALENDAR with VTODO and VJOURNAL components. The returned
// *ics.Calendar is ready to be Serialize()d to an .ics string.
//
// Hierarchy: each Task with a non-nil TrackID emits a RELATED-TO
// RELTYPE=PARENT pointing at the track's UID. Each Track emits a
// matching RELATED-TO RELTYPE=CHILD per member task.
//
// Dependencies: if Task.Meta["blocked_by"] holds a []string of task IDs
// (or anything coerceable to one), each blocker emits a RELATED-TO
// RELTYPE=DEPENDS-ON.
//
// Logs: gated by WithIncludeLogs(true). Each LogEntry becomes a
// VJOURNAL with a RELATED-TO pointing at its task's UID.
func BuildVCalendar(
	tasks []*core.Task,
	tracks []*core.Track,
	logs []*core.LogEntry,
	opts ...Option,
) (*ics.Calendar, error) {
	o := resolve(opts)

	cal := ics.NewCalendar()
	cal.SetVersion("2.0")
	cal.SetProductId(o.productID)
	cal.SetCalscale("GREGORIAN")
	cal.SetMethod(ics.MethodPublish)

	// Index tracks by ID for child-task lookup and validate IDs.
	trackByID := make(map[string]*core.Track, len(tracks))
	for _, tr := range tracks {
		if tr == nil {
			continue
		}
		trackByID[tr.ID] = tr
	}

	// Index tasks per track (deterministic order — sort by ID).
	tasksByTrack := make(map[string][]*core.Task)
	for _, t := range tasks {
		if t == nil || t.TrackID == nil {
			continue
		}
		tasksByTrack[*t.TrackID] = append(tasksByTrack[*t.TrackID], t)
	}
	for k := range tasksByTrack {
		sort.SliceStable(tasksByTrack[k], func(i, j int) bool {
			return tasksByTrack[k][i].ID < tasksByTrack[k][j].ID
		})
	}

	// Tracks → VTODO with CHILD links.
	for _, tr := range tracks {
		if tr == nil {
			continue
		}
		todo := cal.AddTodo(uidFor(tr.ID, o.uidDomain))
		writeTrack(todo, tr, tasksByTrack[tr.ID], o.uidDomain)
	}

	// Tasks → VTODO.
	for _, t := range tasks {
		if t == nil {
			continue
		}
		todo := cal.AddTodo(uidFor(t.ID, o.uidDomain))
		writeTask(todo, t, o.uidDomain)
	}

	// LogEntries → VJOURNAL (gated).
	if o.includeLogs {
		for _, le := range logs {
			if le == nil {
				continue
			}
			j := cal.AddJournal(logUID(le, o.uidDomain))
			writeLogEntry(j, le, o.uidDomain)
		}
	}

	return cal, nil
}

// uidFor renders a tlc TypeID as the iCalendar UID. Foreign IDs (already
// containing '@') pass through unchanged.
func uidFor(typeID, domain string) string {
	if typeID == "" {
		return ""
	}
	if strings.Contains(typeID, "@") {
		return typeID
	}
	if domain == "" {
		domain = DefaultUIDDomain
	}
	return typeID + "@" + domain
}

// statusToICS maps a tlc TaskStatus to an iCalendar ObjectStatus.
// Per RFC 5545 §3.8.1.11 valid VTODO STATUS values are NEEDS-ACTION,
// COMPLETED, IN-PROCESS, CANCELLED.
func statusToICS(s core.TaskStatus) ics.ObjectStatus {
	switch s {
	case core.StatusInProgress:
		return ics.ObjectStatusInProcess
	case core.StatusDone:
		return ics.ObjectStatusCompleted
	case core.StatusSkipped:
		return ics.ObjectStatusCancelled
	default: // includes StatusTodo and empty
		return ics.ObjectStatusNeedsAction
	}
}

// priorityToICS maps a tlc Priority to the iCalendar PRIORITY integer
// (0-9, lower = higher urgency per RFC 5545 §3.8.1.9). Mapping follows
// the spec table: P0→1, P1→3, P2→5, P3→7. Empty priority → 0
// (undefined), which the encoder skips.
func priorityToICS(p core.Priority) int {
	switch p {
	case core.PriorityP0:
		return 1
	case core.PriorityP1:
		return 3
	case core.PriorityP2:
		return 5
	case core.PriorityP3:
		return 7
	}
	return 0
}

func writeTask(todo *ics.VTodo, t *core.Task, domain string) {
	if t.Title != "" {
		todo.SetSummary(t.Title)
	}
	if t.Description != "" {
		todo.SetDescription(t.Description)
	}
	todo.SetStatus(statusToICS(t.Status))
	if p := priorityToICS(t.Priority); p != 0 {
		todo.SetPriority(p)
	}
	for _, tag := range t.Tags {
		if tag == "" {
			continue
		}
		todo.AddCategory(tag)
	}
	if !t.CreatedAt.IsZero() {
		todo.SetCreatedTime(t.CreatedAt)
		todo.SetDtStampTime(t.CreatedAt)
	}
	if !t.UpdatedAt.IsZero() {
		todo.SetModifiedAt(t.UpdatedAt)
	}
	if t.DueAt != nil && !t.DueAt.IsZero() {
		todo.SetDueAt(*t.DueAt)
	}
	if t.RRule != "" {
		todo.AddRrule(t.RRule)
	}
	if t.Reference != "" {
		todo.SetURL(t.Reference)
	}
	if t.RemindAt != nil && !t.RemindAt.IsZero() {
		alarm := todo.AddAlarm()
		alarm.SetAction(ics.ActionDisplay)
		alarm.SetTrigger(t.RemindAt.UTC().Format("20060102T150405Z"))
		alarm.SetProperty(ics.ComponentPropertyDescription, t.Title)
		// VALUE=DATE-TIME signals an absolute trigger (RFC 5545 §3.8.6.3).
		alarm.GetProperty(ics.ComponentPropertyTrigger).ICalParameters["VALUE"] = []string{"DATE-TIME"}
	}
	if t.TrackID != nil && *t.TrackID != "" {
		todo.AddProperty(
			ics.ComponentPropertyRelatedTo,
			uidFor(*t.TrackID, domain),
			reltypeParam(RelTypeParent),
		)
	}
	for _, blocker := range blockedByList(t.Meta) {
		todo.AddProperty(
			ics.ComponentPropertyRelatedTo,
			uidFor(blocker, domain),
			reltypeParam(RelTypeDependsOn),
		)
	}
	if t.Effort != "" {
		todo.SetProperty(ics.ComponentProperty(XPropEffort), string(t.Effort))
	}
	if t.AssignedTo != nil && *t.AssignedTo != "" {
		assignee := *t.AssignedTo
		if isEmail(assignee) {
			todo.AddAttendee(assignee)
		} else {
			todo.SetProperty(ics.ComponentProperty(XPropAssignee), assignee)
		}
	}
	if t.ProjectID != nil && *t.ProjectID != "" {
		todo.SetProperty(ics.ComponentProperty(XPropProjectID), *t.ProjectID)
	}
	if t.Seq > 0 {
		todo.SetProperty(ics.ComponentProperty(XPropTaskSeq), fmt.Sprintf("%d", t.Seq))
	}
}

func writeTrack(todo *ics.VTodo, tr *core.Track, members []*core.Task, domain string) {
	if tr.Title != "" {
		todo.SetSummary(tr.Title)
	}
	// Tracks have no NEEDS-ACTION/etc states; STATUS COMPLETED for
	// completed/archived, otherwise NEEDS-ACTION.
	switch tr.Status {
	case core.TrackStatusCompleted, core.TrackStatusArchived:
		todo.SetStatus(ics.ObjectStatusCompleted)
	case core.TrackStatusAbandoned:
		todo.SetStatus(ics.ObjectStatusCancelled)
	case core.TrackStatusActive:
		todo.SetStatus(ics.ObjectStatusInProcess)
	default:
		todo.SetStatus(ics.ObjectStatusNeedsAction)
	}
	if !tr.CreatedAt.IsZero() {
		todo.SetCreatedTime(tr.CreatedAt)
		todo.SetDtStampTime(tr.CreatedAt)
	}
	if !tr.UpdatedAt.IsZero() {
		todo.SetModifiedAt(tr.UpdatedAt)
	}
	if tr.Slug != "" {
		todo.SetProperty(ics.ComponentProperty(XPropTrackSlug), tr.Slug)
	}
	if tr.Type != "" {
		todo.SetProperty(ics.ComponentProperty(XPropTrackType), tr.Type)
	}
	// Marker so decode can distinguish a track-VTODO from a task-VTODO
	// when the UID is foreign (rare but real for cross-system sync).
	todo.SetProperty(ics.ComponentProperty(XPropTrackKind), "TRUE")
	if tr.AssignedTo != nil && *tr.AssignedTo != "" {
		todo.SetProperty(ics.ComponentProperty(XPropAssignee), *tr.AssignedTo)
	}
	if tr.ProjectID != nil && *tr.ProjectID != "" {
		todo.SetProperty(ics.ComponentProperty(XPropProjectID), *tr.ProjectID)
	}
	for _, member := range members {
		todo.AddProperty(
			ics.ComponentPropertyRelatedTo,
			uidFor(member.ID, domain),
			reltypeParam(RelTypeChild),
		)
	}
}

func writeLogEntry(j *ics.VJournal, le *core.LogEntry, domain string) {
	if le.Note != "" {
		j.SetSummary(firstLine(le.Note))
		j.SetDescription(le.Note)
	} else if le.Action != "" {
		j.SetSummary(le.Action)
	}
	if !le.Timestamp.IsZero() {
		j.SetDtStampTime(le.Timestamp)
		j.SetCreatedTime(le.Timestamp)
	}
	if le.TaskID != "" {
		j.AddProperty(
			ics.ComponentPropertyRelatedTo,
			uidFor(le.TaskID, domain),
			reltypeParam(RelTypeParent),
		)
		j.SetProperty(ics.ComponentProperty(XPropLogTaskID), le.TaskID)
	}
	if le.Action != "" {
		j.SetProperty(ics.ComponentProperty(XPropLogAction), le.Action)
	}
	if le.By != "" {
		j.SetProperty(ics.ComponentProperty(XPropLogBy), le.By)
	}
}

// logUID derives a stable UID for a LogEntry. Combines task ID, action,
// and timestamp so two log entries on the same task with the same action
// at different times don't collide.
func logUID(le *core.LogEntry, domain string) string {
	stamp := ""
	if !le.Timestamp.IsZero() {
		stamp = le.Timestamp.UTC().Format("20060102T150405Z")
	}
	base := fmt.Sprintf("log-%s-%s-%s", le.TaskID, le.Action, stamp)
	return base + "@" + domain
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func isEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return false
	}
	if strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	// require dot in domain
	return strings.IndexByte(s[at+1:], '.') > 0
}

// blockedByList extracts a []string of task IDs from a Task.Meta entry
// keyed "blocked_by". Accepts either []string or []interface{} (the
// usual JSON-decoded shape), and silently skips other shapes.
func blockedByList(meta map[string]interface{}) []string {
	if meta == nil {
		return nil
	}
	raw, ok := meta["blocked_by"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if v != "" {
			return []string{v}
		}
	}
	return nil
}
