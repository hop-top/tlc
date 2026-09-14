package vtodo

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	vstar "hop.top/vstar"
	"hop.top/vstar/codec/rfc5545"
	"hop.top/vstar/helpers"

	"hop.top/tlc/internal/config"
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
	o := resolve(opts)
	statusDefs := o.statusDefinitions()

	cal, err := rfc5545.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parse calendar: %w", err)
	}

	res := &ParseResult{}

	// Build a map UID-body → entity ID for cross-component linkage and
	// VJOURNAL TaskID resolution. Two passes because VJOURNAL parsing
	// needs the task index.
	uidToTaskID := make(map[string]string)
	uidToTrackID := make(map[string]string)

	todos := cal.Filter(vstar.CompTodo)
	for _, todo := range todos {
		if isTrackComponent(todo) {
			tr, uidBodyVal, perr := decodeTrack(todo)
			if perr != nil {
				return nil, perr
			}
			res.Tracks = append(res.Tracks, tr)
			if uidBodyVal != "" {
				uidToTrackID[uidBodyVal] = tr.ID
			}
		} else {
			t, uidBodyVal, perr := decodeTask(todo, statusDefs)
			if perr != nil {
				return nil, perr
			}
			res.Tasks = append(res.Tasks, t)
			if uidBodyVal != "" {
				uidToTaskID[uidBodyVal] = t.ID
			}
		}
	}

	// Resolve PARENT links to track IDs and DEPENDS-ON to task IDs.
	for _, todo := range todos {
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
		// helpers.RelatedTo applies the RFC 5545 §3.2.15 default of
		// PARENT when the RELTYPE param is absent, and matches the
		// param name case-insensitively. Unknown RELTYPEs are ignored.
		for _, rel := range helpers.RelatedTo(todo) {
			body := uidBody(rel.UID)
			switch strings.ToUpper(rel.RelType) {
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
				// Normalise rather than assert: Meta may already carry
				// blocked_by in any supported shape (notably the
				// []interface{} produced by JSON decoding), and a bare
				// []string assertion would silently drop it.
				existing := core.NormalizeBlockedBy(task.Meta["blocked_by"])
				task.Meta["blocked_by"] = append(existing, blocker)
			}
		}
	}

	// VJOURNAL → LogEntry.
	for _, j := range cal.Filter(vstar.CompJournal) {
		le := decodeJournal(j, uidToTaskID)
		res.Logs = append(res.Logs, le)
	}

	return res, nil
}

func isTrackComponent(todo vstar.Component) bool {
	if p, ok := todo.Get(XPropTrackKind); ok && strings.EqualFold(p.Value, "TRUE") {
		return true
	}
	// Fall-back: if UID body looks like a track typeid, treat as track.
	if core.IsTrackID(uidBody(getUID(todo))) {
		return true
	}
	return false
}

func decodeTask(todo vstar.Component, statusDefs []config.StatusDefinition) (*core.Task, string, error) {
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

	if p, ok := todo.Get("SUMMARY"); ok {
		t.Title = unescapeText(p.Value)
	}
	if p, ok := todo.Get("DESCRIPTION"); ok {
		t.Description = unescapeText(p.Value)
	}
	t.Status = decodeTaskStatus(todo, statusDefs)
	if p, ok := todo.Get("PRIORITY"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(p.Value)); err == nil {
			t.Priority = icsToPriority(n)
		}
	}
	for _, p := range todo.GetAll("CATEGORIES") {
		for _, raw := range strings.Split(p.Value, ",") {
			tag := strings.TrimSpace(unescapeText(raw))
			if tag != "" {
				t.Tags = append(t.Tags, tag)
			}
		}
	}
	if p, ok := todo.Get("CREATED"); ok {
		if ts, err := parseICSTime(p.Value); err == nil {
			t.CreatedAt = ts
		}
	}
	if p, ok := todo.Get("LAST-MODIFIED"); ok {
		if ts, err := parseICSTime(p.Value); err == nil {
			t.UpdatedAt = ts
		}
	}
	if t.UpdatedAt.IsZero() {
		// Fallback to DTSTAMP when LAST-MODIFIED is absent.
		if p, ok := todo.Get("DTSTAMP"); ok {
			if ts, err := parseICSTime(p.Value); err == nil {
				t.UpdatedAt = ts
			}
		}
	}
	if p, ok := todo.Get("DUE"); ok {
		if ts, err := parseICSTime(p.Value); err == nil {
			due := ts
			t.DueAt = &due
		}
	}
	if p, ok := todo.Get("RRULE"); ok {
		if err := core.ValidateRRule(p.Value); err == nil {
			t.RRule = p.Value
		}
	}
	if p, ok := todo.Get("URL"); ok {
		t.Reference = p.Value
	}
	if p, ok := todo.Get(XPropEffort); ok {
		eff := core.Effort(strings.TrimSpace(p.Value))
		if core.ValidEffort(eff) {
			t.Effort = eff
		}
	}
	if p, ok := todo.Get(XPropAssignee); ok {
		val := p.Value
		t.AssignedTo = &val
	} else if p, ok := todo.Get("ATTENDEE"); ok {
		// First ATTENDEE wins for v1. Strip mailto: prefix when present;
		// URI scheme is case-insensitive (RFC 3986 §3.1) so MAILTO:,
		// Mailto:, etc. all need to drop.
		email := p.Value
		if len(email) >= len("mailto:") && strings.EqualFold(email[:len("mailto:")], "mailto:") {
			email = email[len("mailto:"):]
		}
		if email != "" {
			t.AssignedTo = &email
		}
	}
	if p, ok := todo.Get(XPropProjectID); ok {
		val := p.Value
		t.ProjectID = &val
	}
	if p, ok := todo.Get(XPropTaskSeq); ok {
		if n, err := strconv.ParseInt(strings.TrimSpace(p.Value), 10, 64); err == nil {
			t.Seq = n
		}
	}
	if p, ok := todo.Get(XPropArchived); ok {
		t.Archived = isTrueValue(p.Value)
	}
	if p, ok := todo.Get(XPropArchived); ok {
		t.Archived = isTrueValue(p.Value)
	}
	for _, alarm := range todo.Sub {
		if alarm.Type != vstar.CompAlarm {
			continue
		}
		trig, ok := alarm.Get("TRIGGER")
		if !ok {
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

func decodeTrack(todo vstar.Component) (*core.Track, string, error) {
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

	if p, ok := todo.Get("SUMMARY"); ok {
		tr.Title = unescapeText(p.Value)
	}
	tr.Status = decodeTrackStatus(todo)
	if p, ok := todo.Get("CREATED"); ok {
		if ts, err := parseICSTime(p.Value); err == nil {
			tr.CreatedAt = ts
		}
	}
	if p, ok := todo.Get("LAST-MODIFIED"); ok {
		if ts, err := parseICSTime(p.Value); err == nil {
			tr.UpdatedAt = ts
		}
	}
	if tr.UpdatedAt.IsZero() {
		if p, ok := todo.Get("DTSTAMP"); ok {
			if ts, err := parseICSTime(p.Value); err == nil {
				tr.UpdatedAt = ts
			}
		}
	}
	if p, ok := todo.Get(XPropTrackSlug); ok {
		tr.Slug = p.Value
	}
	if p, ok := todo.Get(XPropTrackType); ok {
		tr.Type = p.Value
	}
	if p, ok := todo.Get(XPropAssignee); ok {
		val := p.Value
		tr.AssignedTo = &val
	}
	if p, ok := todo.Get(XPropProjectID); ok {
		val := p.Value
		tr.ProjectID = &val
	}

	return tr, body, nil
}

func decodeJournal(j vstar.Component, uidToTaskID map[string]string) *core.LogEntry {
	le := &core.LogEntry{}

	if p, ok := j.Get(XPropLogAction); ok {
		le.Action = p.Value
	}
	if p, ok := j.Get(XPropLogBy); ok {
		le.By = p.Value
	}
	if p, ok := j.Get("DESCRIPTION"); ok {
		le.Note = unescapeText(p.Value)
	} else if p, ok := j.Get("SUMMARY"); ok {
		le.Note = unescapeText(p.Value)
	}
	if p, ok := j.Get("DTSTAMP"); ok {
		if ts, err := parseICSTime(p.Value); err == nil {
			le.Timestamp = ts
		}
	}
	if le.Timestamp.IsZero() {
		if p, ok := j.Get("CREATED"); ok {
			if ts, err := parseICSTime(p.Value); err == nil {
				le.Timestamp = ts
			}
		}
	}

	// Prefer X-TLC-LOG-TASK if present (preserves the raw typeid even
	// when the calendar didn't include a matching VTODO).
	if p, ok := j.Get(XPropLogTaskID); ok {
		le.TaskID = p.Value
	}
	if le.TaskID == "" {
		for _, rel := range j.GetAll("RELATED-TO") {
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

// decodeTaskStatus recovers a tlc status from a VTODO, preferring the
// exact name in XPropStatus and falling back to the role the RFC STATUS
// value implies.
//
// The X-property is only trusted when the CURRENT vocabulary declares
// that name. A calendar exported from another project (or from this one
// before a vocabulary change) can name a status this config would reject,
// and importing it would seed the store with a value every later
// validation refuses. Falling back to the role default keeps the import
// inside the configured vocabulary while preserving the coarse meaning.
func decodeTaskStatus(todo vstar.Component, defs []config.StatusDefinition) core.TaskStatus {
	if p, ok := todo.Get(XPropStatus); ok {
		name := strings.TrimSpace(p.Value)
		for _, def := range defs {
			if strings.EqualFold(def.Name, name) {
				return core.TaskStatus(def.Name)
			}
		}
	}
	wire := ""
	if p, ok := todo.Get("STATUS"); ok {
		wire = strings.TrimSpace(p.Value)
	}
	return roleDefaultStatus(wireToRole(wire), defs)
}

// wireToRole is roleToWire inverted: an RFC 5545 STATUS value back to the
// config role it stands for. Comparison is case-insensitive because the
// value may have come from a foreign producer.
func wireToRole(wire string) string {
	switch {
	case strings.EqualFold(wire, string(vstar.TodoInProcess)):
		return config.RoleActive
	case strings.EqualFold(wire, string(vstar.TodoCompleted)):
		return config.RoleCompleted
	case strings.EqualFold(wire, string(vstar.TodoCancelled)):
		return config.RoleSkipped
	default:
		return config.RoleInitial
	}
}

// roleDefaultStatus picks the vocabulary's status for a role: the FIRST
// declaring it, matching how WorkflowManager resolves a role to a single
// transition target. Falls back to the first initial-role status, then to
// the first declared status, so a decode always lands on something the
// config accepts.
func roleDefaultStatus(role string, defs []config.StatusDefinition) core.TaskStatus {
	for _, def := range defs {
		if def.Role == role {
			return core.TaskStatus(def.Name)
		}
	}
	for _, def := range defs {
		if def.Role == config.RoleInitial {
			return core.TaskStatus(def.Name)
		}
	}
	if len(defs) > 0 {
		return core.TaskStatus(defs[0].Name)
	}
	return core.StatusTodo
}

// decodeTrackStatus prefers XPropTrackStatus so "completed" and
// "archived" — which share STATUS:COMPLETED — survive the round trip, and
// falls back to the RFC STATUS for foreign calendars.
func decodeTrackStatus(todo vstar.Component) core.TrackStatus {
	if p, ok := todo.Get(XPropTrackStatus); ok {
		name := strings.TrimSpace(p.Value)
		for _, st := range core.TrackStatuses() {
			if strings.EqualFold(string(st), name) {
				return st
			}
		}
	}
	wire := ""
	if p, ok := todo.Get("STATUS"); ok {
		wire = strings.TrimSpace(p.Value)
	}
	switch {
	case strings.EqualFold(wire, string(vstar.TodoInProcess)):
		return core.TrackStatusActive
	case strings.EqualFold(wire, string(vstar.TodoCompleted)):
		return core.TrackStatusCompleted
	case strings.EqualFold(wire, string(vstar.TodoCancelled)):
		return core.TrackStatusAbandoned
	default:
		return core.TrackStatusPending
	}
}

// isTrueValue reads the boolean X-properties tlc emits. Only "TRUE" is
// ever written; the wider set is accepted because hand-edited .ics files
// are a real input.
func isTrueValue(s string) bool {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "TRUE", "YES", "1":
		return true
	}
	return false
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

func getUID(todo vstar.Component) string {
	return todo.UID()
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
// codec applies (\\, \;, \,, \n, \N).
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
