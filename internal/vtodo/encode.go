package vtodo

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	vstar "hop.top/vstar"
	"hop.top/vstar/codec/rfc5545"

	"hop.top/tlc/internal/core"
)

// RFC 5545 / RFC 9253 RELTYPE parameter values. RFC 9253 introduces
// DEPENDS-ON; vstar treats RELTYPE values as opaque strings, so we
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

// utcStampLayout is the RFC 5545 §3.3.5 form #2 (UTC) DATE-TIME
// representation used for absolute timestamps (CREATED, DTSTAMP,
// LAST-MODIFIED, DUE, TRIGGER VALUE=DATE-TIME, …).
const utcStampLayout = "20060102T150405Z"

// BuildVCalendar serialises tasks, tracks and (optionally) log entries
// into a vstar.Calendar with VTODO and VJOURNAL components. Pair with
// Serialize to produce an .ics string.
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
) (vstar.Calendar, error) {
	o := resolve(opts)

	cal := vstar.Calendar{ProdID: o.productID}

	// Index tasks per track for CHILD-link emission.
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
		cal.Append(buildTrackComponent(tr, tasksByTrack[tr.ID], o.uidDomain))
	}

	// Tasks → VTODO.
	for _, t := range tasks {
		if t == nil {
			continue
		}
		cal.Append(buildTaskComponent(t, o.uidDomain))
	}

	// LogEntries → VJOURNAL (gated).
	if o.includeLogs {
		for _, le := range logs {
			if le == nil {
				continue
			}
			cal.Append(buildLogComponent(le, o.uidDomain))
		}
	}

	return cal, nil
}

// Serialize encodes cal as RFC 5545 wire format. Returns the UTF-8
// string ready to write to a file or stream.
//
// Post-processes the codec output to inject CALSCALE:GREGORIAN +
// METHOD:PUBLISH after PRODID. vstar.Calendar has no Props field for
// arbitrary calendar-level properties (vstar T-0135); inject manually
// until upstream lands. Both lines are RFC 5545 §3.6 standard
// calendar-level properties; CALSCALE defaults to GREGORIAN, METHOD
// defaults to PUBLISH for tlc's export use case.
func Serialize(cal vstar.Calendar) (string, error) {
	var buf bytes.Buffer
	if err := rfc5545.Encode(&buf, cal); err != nil {
		return "", fmt.Errorf("vtodo encode: %w", err)
	}
	return injectCalendarProps(buf.String()), nil
}

// injectCalendarProps inserts CALSCALE + METHOD after the PRODID
// content line, skipping any RFC 5545 §3.1 fold continuation lines
// (lines beginning with SPACE or HTAB). Long PRODIDs may fold across
// multiple physical CRLF-terminated lines; CALSCALE/METHOD must land
// AFTER the last continuation, never between them.
//
// Workaround for vstar T-0135 — drop when vstar.Calendar gains Props.
func injectCalendarProps(s string) string {
	const prodIDPrefix = "PRODID:"
	const inject = "CALSCALE:GREGORIAN\r\nMETHOD:PUBLISH\r\n"
	idx := strings.Index(s, prodIDPrefix)
	if idx < 0 {
		return s
	}
	// Walk physical lines (CRLF-terminated) starting at the PRODID
	// line until we find one whose successor does NOT start with
	// SPACE or HTAB — that's the end of the folded property.
	cursor := idx
	for {
		end := strings.Index(s[cursor:], "\r\n")
		if end < 0 {
			return s // malformed; bail
		}
		afterCRLF := cursor + end + len("\r\n")
		// If we're at end-of-string or the next byte is not a fold
		// continuation, we've found the property boundary.
		if afterCRLF >= len(s) {
			return s[:afterCRLF] + inject
		}
		next := s[afterCRLF]
		if next != ' ' && next != '\t' {
			return s[:afterCRLF] + inject + s[afterCRLF:]
		}
		cursor = afterCRLF
	}
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

// statusToWire maps a tlc TaskStatus to an RFC 5545 §3.8.1.11 STATUS
// wire string. Defaults to NEEDS-ACTION for unknown / empty statuses.
func statusToWire(s core.TaskStatus) string {
	switch s {
	case core.StatusInProgress:
		return string(vstar.TodoInProcess)
	case core.StatusDone:
		return string(vstar.TodoCompleted)
	case core.StatusSkipped:
		return string(vstar.TodoCancelled)
	default:
		return string(vstar.TodoNeedsAction)
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

func buildTaskComponent(t *core.Task, domain string) vstar.Component {
	c := vstar.Component{Type: vstar.CompTodo}
	c.Add(vstar.Property{Name: "UID", Value: uidFor(t.ID, domain)})

	if t.Title != "" {
		c.Add(vstar.Property{Name: "SUMMARY", Value: t.Title})
	}
	if t.Description != "" {
		c.Add(vstar.Property{Name: "DESCRIPTION", Value: t.Description})
	}
	c.Add(vstar.Property{Name: "STATUS", Value: statusToWire(t.Status)})
	if p := priorityToICS(t.Priority); p != 0 {
		c.Add(vstar.Property{Name: "PRIORITY", Value: fmt.Sprintf("%d", p)})
	}
	for _, tag := range t.Tags {
		if tag == "" {
			continue
		}
		c.Add(vstar.Property{Name: "CATEGORIES", Value: tag})
	}
	if !t.CreatedAt.IsZero() {
		stamp := t.CreatedAt.UTC().Format(utcStampLayout)
		c.Add(vstar.Property{Name: "CREATED", Value: stamp})
		c.Add(vstar.Property{Name: "DTSTAMP", Value: stamp})
	}
	if !t.UpdatedAt.IsZero() {
		c.Add(vstar.Property{Name: "LAST-MODIFIED", Value: t.UpdatedAt.UTC().Format(utcStampLayout)})
	}
	if t.DueAt != nil && !t.DueAt.IsZero() {
		c.Add(vstar.Property{Name: "DUE", Value: t.DueAt.UTC().Format(utcStampLayout)})
	}
	if t.RRule != "" {
		c.Add(vstar.Property{Name: "RRULE", Value: t.RRule})
	}
	if t.Reference != "" {
		c.Add(vstar.Property{Name: "URL", Value: t.Reference})
	}
	if t.RemindAt != nil && !t.RemindAt.IsZero() {
		c.Sub = append(c.Sub, buildAlarmComponent(*t.RemindAt, t.Title))
	}
	if t.TrackID != nil && *t.TrackID != "" {
		c.Add(vstar.Property{
			Name:   "RELATED-TO",
			Params: []vstar.Param{{Name: "RELTYPE", Value: RelTypeParent}},
			Value:  uidFor(*t.TrackID, domain),
		})
	}
	for _, blocker := range blockedByList(t.Meta) {
		c.Add(vstar.Property{
			Name:   "RELATED-TO",
			Params: []vstar.Param{{Name: "RELTYPE", Value: RelTypeDependsOn}},
			Value:  uidFor(blocker, domain),
		})
	}
	if t.Effort != "" {
		c.Add(vstar.Property{Name: XPropEffort, Value: string(t.Effort)})
	}
	if t.AssignedTo != nil && *t.AssignedTo != "" {
		assignee := *t.AssignedTo
		if isEmail(assignee) {
			c.Add(vstar.Property{Name: "ATTENDEE", Value: "mailto:" + assignee})
		} else {
			c.Add(vstar.Property{Name: XPropAssignee, Value: assignee})
		}
	}
	if t.ProjectID != nil && *t.ProjectID != "" {
		c.Add(vstar.Property{Name: XPropProjectID, Value: *t.ProjectID})
	}
	if t.Seq > 0 {
		c.Add(vstar.Property{Name: XPropTaskSeq, Value: fmt.Sprintf("%d", t.Seq)})
	}
	addMeta(&c, t.Meta)
	addUnknownTLCProps(&c, t.Meta)
	return c
}

func buildTrackComponent(tr *core.Track, members []*core.Task, domain string) vstar.Component {
	c := vstar.Component{Type: vstar.CompTodo}
	c.Add(vstar.Property{Name: "UID", Value: uidFor(tr.ID, domain)})

	if tr.Title != "" {
		c.Add(vstar.Property{Name: "SUMMARY", Value: tr.Title})
	}
	// Tracks have no NEEDS-ACTION/etc states; STATUS COMPLETED for
	// completed/archived, otherwise NEEDS-ACTION.
	var status string
	switch tr.Status {
	case core.TrackStatusCompleted, core.TrackStatusArchived:
		status = string(vstar.TodoCompleted)
	case core.TrackStatusAbandoned:
		status = string(vstar.TodoCancelled)
	case core.TrackStatusActive:
		status = string(vstar.TodoInProcess)
	default:
		status = string(vstar.TodoNeedsAction)
	}
	c.Add(vstar.Property{Name: "STATUS", Value: status})

	if !tr.CreatedAt.IsZero() {
		stamp := tr.CreatedAt.UTC().Format(utcStampLayout)
		c.Add(vstar.Property{Name: "CREATED", Value: stamp})
		c.Add(vstar.Property{Name: "DTSTAMP", Value: stamp})
	}
	if !tr.UpdatedAt.IsZero() {
		c.Add(vstar.Property{Name: "LAST-MODIFIED", Value: tr.UpdatedAt.UTC().Format(utcStampLayout)})
	}
	if tr.Slug != "" {
		c.Add(vstar.Property{Name: XPropTrackSlug, Value: tr.Slug})
	}
	if tr.Type != "" {
		c.Add(vstar.Property{Name: XPropTrackType, Value: tr.Type})
	}
	// Marker so decode can distinguish a track-VTODO from a task-VTODO
	// when the UID is foreign (rare but real for cross-system sync).
	c.Add(vstar.Property{Name: XPropTrackKind, Value: "TRUE"})
	if tr.AssignedTo != nil && *tr.AssignedTo != "" {
		c.Add(vstar.Property{Name: XPropAssignee, Value: *tr.AssignedTo})
	}
	if tr.ProjectID != nil && *tr.ProjectID != "" {
		c.Add(vstar.Property{Name: XPropProjectID, Value: *tr.ProjectID})
	}
	for _, member := range members {
		c.Add(vstar.Property{
			Name:   "RELATED-TO",
			Params: []vstar.Param{{Name: "RELTYPE", Value: RelTypeChild}},
			Value:  uidFor(member.ID, domain),
		})
	}
	addMeta(&c, tr.Meta)
	addUnknownTLCProps(&c, tr.Meta)
	return c
}

func buildLogComponent(le *core.LogEntry, domain string) vstar.Component {
	c := vstar.Component{Type: vstar.CompJournal}
	c.Add(vstar.Property{Name: "UID", Value: logUID(le, domain)})

	if le.Note != "" {
		c.Add(vstar.Property{Name: "SUMMARY", Value: firstLine(le.Note)})
		c.Add(vstar.Property{Name: "DESCRIPTION", Value: le.Note})
	} else if le.Action != "" {
		c.Add(vstar.Property{Name: "SUMMARY", Value: le.Action})
	}
	if !le.Timestamp.IsZero() {
		stamp := le.Timestamp.UTC().Format(utcStampLayout)
		c.Add(vstar.Property{Name: "DTSTAMP", Value: stamp})
		c.Add(vstar.Property{Name: "CREATED", Value: stamp})
	}
	if le.TaskID != "" {
		c.Add(vstar.Property{
			Name:   "RELATED-TO",
			Params: []vstar.Param{{Name: "RELTYPE", Value: RelTypeParent}},
			Value:  uidFor(le.TaskID, domain),
		})
		c.Add(vstar.Property{Name: XPropLogTaskID, Value: le.TaskID})
	}
	if le.Action != "" {
		c.Add(vstar.Property{Name: XPropLogAction, Value: le.Action})
	}
	if le.By != "" {
		c.Add(vstar.Property{Name: XPropLogBy, Value: le.By})
	}
	addMeta(&c, le.Meta)
	addUnknownTLCProps(&c, le.Meta)
	return c
}

// buildAlarmComponent emits a VALARM block carrying an absolute
// DATE-TIME trigger. Used for tlc reminders that fire at a specific
// instant rather than relative to DTSTART.
func buildAlarmComponent(remindAt time.Time, summary string) vstar.Component {
	c := vstar.Component{Type: vstar.CompAlarm}
	c.Add(vstar.Property{Name: "ACTION", Value: "DISPLAY"})
	c.Add(vstar.Property{
		Name:   "TRIGGER",
		Params: []vstar.Param{{Name: "VALUE", Value: "DATE-TIME"}},
		Value:  remindAt.UTC().Format(utcStampLayout),
	})
	if summary != "" {
		c.Add(vstar.Property{Name: "DESCRIPTION", Value: summary})
	}
	return c
}

// logUID derives a stable UID for a LogEntry. Combines task ID, action,
// and timestamp so two log entries on the same task with the same action
// at different times don't collide.
func logUID(le *core.LogEntry, domain string) string {
	stamp := ""
	if !le.Timestamp.IsZero() {
		stamp = le.Timestamp.UTC().Format(utcStampLayout)
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
