package vtodo

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"

	"hop.top/tlc/internal/core"
)

// ParseResult is the structured output of ParseVCalendar.
type ParseResult struct {
	Tasks  []*core.Task
	Tracks []*core.Track
	Logs   []*core.LogEntry
}

// ParseVCalendar reads an iCalendar stream and returns the tasks,
// tracks, and log entries it contained.
//
// VTODO classification:
//   - X-TLC-IS-TRACK=TRUE marker → Track
//   - otherwise → Task
//
// UID handling: a UID of the form `<typeid>@<anything>` where typeid
// matches core.IsTaskID/IsTrackID is reused as the entity ID. Otherwise
// the decoder mints a fresh ID and stashes the original UID in
// Meta["external_uid"].
//
// VJOURNAL: each becomes a LogEntry. The TaskID is taken from the first
// RELATED-TO property whose value (after stripping the @domain) maps to
// a known Task UID; if the link can't be resolved the LogEntry is still
// emitted with TaskID set to the raw UID body.
func ParseVCalendar(r io.Reader, opts ...Option) (*ParseResult, error) {
	_ = resolve(opts) // currently no decode-time options consume from o

	cal, err := ics.ParseCalendar(r)
	if err != nil {
		return nil, fmt.Errorf("parse calendar: %w", err)
	}

	res := &ParseResult{}

	// Build a map UID-body → entity ID for cross-component linkage and
	// VJOURNAL TaskID resolution. We do this in two passes because
	// VJOURNAL parsing needs the task index.
	uidToTaskID := make(map[string]string)
	uidToTrackID := make(map[string]string)

	for _, todo := range cal.Todos() {
		if isTrackComponent(todo) {
			tr, uidBody, perr := decodeTrack(todo)
			if perr != nil {
				return nil, perr
			}
			res.Tracks = append(res.Tracks, tr)
			if uidBody != "" {
				uidToTrackID[uidBody] = tr.ID
			}
		} else {
			t, uidBody, perr := decodeTask(todo)
			if perr != nil {
				return nil, perr
			}
			res.Tasks = append(res.Tasks, t)
			if uidBody != "" {
				uidToTaskID[uidBody] = t.ID
			}
		}
	}

	// Resolve PARENT links to track IDs and DEPENDS-ON to task IDs.
	for _, todo := range cal.Todos() {
		if isTrackComponent(todo) {
			continue
		}
		uid := getUID(todo)
		taskID := uidToTaskID[uidBody(uid)]
		if taskID == "" {
			continue
		}
		var task *core.Task
		for _, t := range res.Tasks {
			if t.ID == taskID {
				task = t
				break
			}
		}
		if task == nil {
			continue
		}
		for _, rel := range todo.GetProperties(ics.ComponentPropertyRelatedTo) {
			reltype := paramFirst(rel.ICalParameters, "RELTYPE")
			body := uidBody(rel.Value)
			switch reltype {
			case RelTypeParent:
				if id, ok := uidToTrackID[body]; ok {
					tid := id
					task.TrackID = &tid
				} else if core.IsTrackID(body) {
					tid := body
					task.TrackID = &tid
				}
			case RelTypeDependsOn:
				blocker := body
				if id, ok := uidToTaskID[body]; ok {
					blocker = id
				}
				if task.Meta == nil {
					task.Meta = map[string]interface{}{}
				}
				existing, _ := task.Meta["blocked_by"].([]string)
				task.Meta["blocked_by"] = append(existing, blocker)
			}
		}
	}

	// VJOURNAL → LogEntry.
	for _, j := range cal.Journals() {
		le := decodeJournal(j, uidToTaskID)
		res.Logs = append(res.Logs, le)
	}

	return res, nil
}

func isTrackComponent(todo *ics.VTodo) bool {
	p := todo.GetProperty(ics.ComponentProperty(XPropTrackKind))
	if p != nil && strings.EqualFold(p.Value, "TRUE") {
		return true
	}
	// Fall-back: if UID body looks like a track typeid, treat as track.
	uid := getUID(todo)
	if core.IsTrackID(uidBody(uid)) {
		return true
	}
	return false
}

func decodeTask(todo *ics.VTodo) (*core.Task, string, error) {
	t := &core.Task{}

	uid := getUID(todo)
	body := uidBody(uid)
	if core.IsTaskID(body) {
		t.ID = body
	} else {
		t.ID = core.NewTaskID()
		if uid != "" {
			if t.Meta == nil {
				t.Meta = map[string]interface{}{}
			}
			t.Meta["external_uid"] = uid
		}
	}

	if p := todo.GetProperty(ics.ComponentPropertySummary); p != nil {
		t.Title = unescapeText(p.Value)
	}
	if p := todo.GetProperty(ics.ComponentPropertyDescription); p != nil {
		t.Description = unescapeText(p.Value)
	}
	if p := todo.GetProperty(ics.ComponentPropertyStatus); p != nil {
		t.Status = icsToStatus(ics.ObjectStatus(p.Value))
	}
	if p := todo.GetProperty(ics.ComponentPropertyPriority); p != nil {
		if n, err := strconv.Atoi(strings.TrimSpace(p.Value)); err == nil {
			t.Priority = icsToPriority(n)
		}
	}
	for _, p := range todo.GetProperties(ics.ComponentPropertyCategories) {
		for _, raw := range strings.Split(p.Value, ",") {
			tag := strings.TrimSpace(unescapeText(raw))
			if tag != "" {
				t.Tags = append(t.Tags, tag)
			}
		}
	}
	if p := todo.GetProperty(ics.ComponentPropertyCreated); p != nil {
		if ts, err := parseICSTime(p.Value); err == nil {
			t.CreatedAt = ts
		}
	}
	if p := todo.GetProperty(ics.ComponentPropertyLastModified); p != nil {
		if ts, err := parseICSTime(p.Value); err == nil {
			t.UpdatedAt = ts
		}
	}
	if t.UpdatedAt.IsZero() {
		// Fallback to DTSTAMP when LAST-MODIFIED is absent.
		if p := todo.GetProperty(ics.ComponentPropertyDtstamp); p != nil {
			if ts, err := parseICSTime(p.Value); err == nil {
				t.UpdatedAt = ts
			}
		}
	}
	if p := todo.GetProperty(ics.ComponentPropertyDue); p != nil {
		if ts, err := parseICSTime(p.Value); err == nil {
			due := ts
			t.DueAt = &due
		}
	}
	if p := todo.GetProperty(ics.ComponentPropertyRrule); p != nil {
		if err := core.ValidateRRule(p.Value); err == nil {
			t.RRule = p.Value
		}
	}
	if p := todo.GetProperty(ics.ComponentPropertyUrl); p != nil {
		t.Reference = p.Value
	}
	if p := todo.GetProperty(ics.ComponentProperty(XPropEffort)); p != nil {
		eff := core.Effort(strings.TrimSpace(p.Value))
		if core.ValidEffort(eff) {
			t.Effort = eff
		}
	}
	if p := todo.GetProperty(ics.ComponentProperty(XPropAssignee)); p != nil {
		val := p.Value
		t.AssignedTo = &val
	} else if attendees := todo.Attendees(); len(attendees) > 0 {
		// First attendee wins for v1.
		email := attendees[0].Email()
		if email != "" {
			t.AssignedTo = &email
		}
	}
	if p := todo.GetProperty(ics.ComponentProperty(XPropProjectID)); p != nil {
		val := p.Value
		t.ProjectID = &val
	}
	if p := todo.GetProperty(ics.ComponentProperty(XPropTaskSeq)); p != nil {
		if n, err := strconv.ParseInt(strings.TrimSpace(p.Value), 10, 64); err == nil {
			t.Seq = n
		}
	}
	for _, alarm := range todo.Alarms() {
		trig := alarm.GetProperty(ics.ComponentPropertyTrigger)
		if trig == nil {
			continue
		}
		if ts, err := parseICSTime(trig.Value); err == nil {
			tt := ts
			t.RemindAt = &tt
			break
		}
	}

	return t, body, nil
}

func decodeTrack(todo *ics.VTodo) (*core.Track, string, error) {
	tr := &core.Track{}

	uid := getUID(todo)
	body := uidBody(uid)
	if core.IsTrackID(body) {
		tr.ID = body
	} else {
		tr.ID = core.NewTrackID()
		if uid != "" {
			if tr.Meta == nil {
				tr.Meta = map[string]any{}
			}
			tr.Meta["external_uid"] = uid
		}
	}

	if p := todo.GetProperty(ics.ComponentPropertySummary); p != nil {
		tr.Title = unescapeText(p.Value)
	}
	if p := todo.GetProperty(ics.ComponentPropertyStatus); p != nil {
		tr.Status = icsStatusToTrack(ics.ObjectStatus(p.Value))
	}
	if p := todo.GetProperty(ics.ComponentPropertyCreated); p != nil {
		if ts, err := parseICSTime(p.Value); err == nil {
			tr.CreatedAt = ts
		}
	}
	if p := todo.GetProperty(ics.ComponentPropertyLastModified); p != nil {
		if ts, err := parseICSTime(p.Value); err == nil {
			tr.UpdatedAt = ts
		}
	}
	if tr.UpdatedAt.IsZero() {
		if p := todo.GetProperty(ics.ComponentPropertyDtstamp); p != nil {
			if ts, err := parseICSTime(p.Value); err == nil {
				tr.UpdatedAt = ts
			}
		}
	}
	if p := todo.GetProperty(ics.ComponentProperty(XPropTrackSlug)); p != nil {
		tr.Slug = p.Value
	}
	if p := todo.GetProperty(ics.ComponentProperty(XPropTrackType)); p != nil {
		tr.Type = p.Value
	}
	if p := todo.GetProperty(ics.ComponentProperty(XPropAssignee)); p != nil {
		val := p.Value
		tr.AssignedTo = &val
	}
	if p := todo.GetProperty(ics.ComponentProperty(XPropProjectID)); p != nil {
		val := p.Value
		tr.ProjectID = &val
	}

	return tr, body, nil
}

func decodeJournal(j *ics.VJournal, uidToTaskID map[string]string) *core.LogEntry {
	le := &core.LogEntry{}

	if p := j.GetProperty(ics.ComponentProperty(XPropLogAction)); p != nil {
		le.Action = p.Value
	}
	if p := j.GetProperty(ics.ComponentProperty(XPropLogBy)); p != nil {
		le.By = p.Value
	}
	if p := j.GetProperty(ics.ComponentPropertyDescription); p != nil {
		le.Note = unescapeText(p.Value)
	} else if p := j.GetProperty(ics.ComponentPropertySummary); p != nil {
		le.Note = unescapeText(p.Value)
	}
	if p := j.GetProperty(ics.ComponentPropertyDtstamp); p != nil {
		if ts, err := parseICSTime(p.Value); err == nil {
			le.Timestamp = ts
		}
	}
	if le.Timestamp.IsZero() {
		if p := j.GetProperty(ics.ComponentPropertyCreated); p != nil {
			if ts, err := parseICSTime(p.Value); err == nil {
				le.Timestamp = ts
			}
		}
	}

	// Prefer X-TLC-LOG-TASK if present (preserves the raw typeid even
	// when the calendar didn't include a matching VTODO).
	if p := j.GetProperty(ics.ComponentProperty(XPropLogTaskID)); p != nil {
		le.TaskID = p.Value
	}
	if le.TaskID == "" {
		for _, rel := range j.GetProperties(ics.ComponentPropertyRelatedTo) {
			body := uidBody(rel.Value)
			if id, ok := uidToTaskID[body]; ok {
				le.TaskID = id
				break
			}
			if core.IsTaskID(body) {
				le.TaskID = body
				break
			}
		}
	}

	return le
}

func icsToStatus(s ics.ObjectStatus) core.TaskStatus {
	switch s {
	case ics.ObjectStatusInProcess:
		return core.StatusInProgress
	case ics.ObjectStatusCompleted:
		return core.StatusDone
	case ics.ObjectStatusCancelled:
		return core.StatusSkipped
	case ics.ObjectStatusNeedsAction:
		return core.StatusTodo
	}
	return core.StatusTodo
}

func icsStatusToTrack(s ics.ObjectStatus) core.TrackStatus {
	switch s {
	case ics.ObjectStatusInProcess:
		return core.TrackStatusActive
	case ics.ObjectStatusCompleted:
		return core.TrackStatusCompleted
	case ics.ObjectStatusCancelled:
		return core.TrackStatusAbandoned
	case ics.ObjectStatusNeedsAction:
		return core.TrackStatusPending
	}
	return core.TrackStatusPending
}

func icsToPriority(n int) core.Priority {
	switch {
	case n <= 0:
		return ""
	case n <= 2:
		return core.PriorityP0
	case n <= 4:
		return core.PriorityP1
	case n <= 6:
		return core.PriorityP2
	default:
		return core.PriorityP3
	}
}

func getUID(todo *ics.VTodo) string {
	p := todo.GetProperty(ics.ComponentPropertyUniqueId)
	if p == nil {
		return ""
	}
	return p.Value
}

// uidBody strips the "@domain" suffix (if any) from a UID, leaving the
// typeid (or foreign body) for matching.
func uidBody(uid string) string {
	if uid == "" {
		return ""
	}
	if i := strings.IndexByte(uid, '@'); i >= 0 {
		return uid[:i]
	}
	return uid
}

func paramFirst(params map[string][]string, key string) string {
	for k, v := range params {
		if strings.EqualFold(k, key) && len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

// parseICSTime parses iCalendar DATE-TIME forms. Only UTC ("Z") and
// floating local forms are handled; v1 emits UTC exclusively.
func parseICSTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time value")
	}
	formats := []string{
		"20060102T150405Z",
		"20060102T150405",
		"20060102",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, s, time.UTC); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised iCalendar time %q", s)
}

// unescapeText reverses the RFC 5545 §3.3.11 TEXT escapes that the
// emitter applies (\\, \;, \,, \n, \N).
func unescapeText(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			next := s[i+1]
			switch next {
			case 'n', 'N':
				b.WriteByte('\n')
				i++
				continue
			case ',', ';', '\\':
				b.WriteByte(next)
				i++
				continue
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}
