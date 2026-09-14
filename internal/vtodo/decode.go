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
			t, uidBodyVal, perr := decodeTask(todo, o.priorities)
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
		for _, rel := range todo.GetAll("RELATED-TO") {
			reltype := paramFirst(rel.Params, "RELTYPE")
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

func decodeTask(todo vstar.Component, defs []config.PriorityDefinition) (*core.Task, string, error) {
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
		t.Title = p.Value
	}
	if p, ok := todo.Get("DESCRIPTION"); ok {
		t.Description = p.Value
	}
	if p, ok := todo.Get("STATUS"); ok {
		t.Status = wireToStatus(p.Value)
	}
	t.Priority = priorityFromComponent(todo, defs)
	if p, ok := todo.Get(XPropPrioritySource); ok {
		if v := strings.TrimSpace(p.Value); v != "" {
			setMeta(t, core.MetaPrioritySource, v)
		}
	}
	if p, ok := todo.Get(XPropPriorityRule); ok {
		if v := strings.TrimSpace(p.Value); v != "" {
			setMeta(t, core.MetaPriorityRule, v)
		}
	}
	t.Tags = decodeCategories(todo)
	if ts, ok := propTime(todo, "CREATED"); ok {
		t.CreatedAt = ts
	}
	if ts, ok := propTime(todo, "LAST-MODIFIED"); ok {
		t.UpdatedAt = ts
	}
	if t.UpdatedAt.IsZero() {
		// Fallback to DTSTAMP when LAST-MODIFIED is absent.
		if ts, ok := propTime(todo, "DTSTAMP"); ok {
			t.UpdatedAt = ts
		}
	}
	if due, ok := todo.DUE(vstar.Calendar{}); ok {
		t.DueAt = &due
	} else if due, ok := propTime(todo, "DUE"); ok {
		// Date-only DUE; see parseDateOnly.
		t.DueAt = &due
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
	for _, alarm := range todo.Sub {
		if alarm.Type != vstar.CompAlarm {
			continue
		}
		trig, ok := alarm.Get("TRIGGER")
		if !ok {
			continue
		}
		if ts, ok := parsePropValue(trig.Value); ok {
			tt := ts
			t.RemindAt = &tt
			break
		}
	}

	// X-TLC-META payload plus any unrecognised X-TLC-* property, so a
	// foreign producer's extensions survive the import. ext's scope
	// walk covers only the VTODO's own Props; VALARM and every other
	// sub-component is folded in explicitly below.
	t.Meta = applyMeta(t.Meta, todo)
	for _, sub := range todo.Sub {
		t.Meta = applyMeta(t.Meta, sub)
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
		tr.Title = p.Value
	}
	if p, ok := todo.Get("STATUS"); ok {
		tr.Status = wireStatusToTrack(p.Value)
	}
	if ts, ok := propTime(todo, "CREATED"); ok {
		tr.CreatedAt = ts
	}
	if ts, ok := propTime(todo, "LAST-MODIFIED"); ok {
		tr.UpdatedAt = ts
	}
	if tr.UpdatedAt.IsZero() {
		if ts, ok := propTime(todo, "DTSTAMP"); ok {
			tr.UpdatedAt = ts
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

	tr.Meta = applyMeta(tr.Meta, todo)
	for _, sub := range todo.Sub {
		tr.Meta = applyMeta(tr.Meta, sub)
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
		le.Note = p.Value
	} else if p, ok := j.Get("SUMMARY"); ok {
		le.Note = p.Value
	}
	// CREATED carries the log entry's own instant; DTSTAMP carries the
	// export instant (RFC 5545 §3.8.7.2) and is the same for every
	// component in a calendar, so it is only a fallback for producers
	// that emit no CREATED.
	if ts, ok := propTime(j, "CREATED"); ok {
		le.Timestamp = ts
	}
	if le.Timestamp.IsZero() {
		if ts, ok := propTime(j, "DTSTAMP"); ok {
			le.Timestamp = ts
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

	// VJOURNAL is a top-level component, so ext's non-recursive walk
	// needs to be invoked on it directly rather than inherited from the
	// VTODO pass above.
	le.Meta = applyMeta(le.Meta, j)
	for _, sub := range j.Sub {
		le.Meta = applyMeta(le.Meta, sub)
	}

	return le
}

// wireToStatus maps an RFC 5545 §3.8.1.11 STATUS wire string to a
// tlc TaskStatus. Comparison is case-sensitive per the RFC.
func wireToStatus(s string) core.TaskStatus {
	switch s {
	case string(vstar.TodoInProcess):
		return core.StatusInProgress
	case string(vstar.TodoCompleted):
		return core.StatusDone
	case string(vstar.TodoCancelled):
		return core.StatusSkipped
	case string(vstar.TodoNeedsAction):
		return core.StatusTodo
	}
	return core.StatusTodo
}

func wireStatusToTrack(s string) core.TrackStatus {
	switch s {
	case string(vstar.TodoInProcess):
		return core.TrackStatusActive
	case string(vstar.TodoCompleted):
		return core.TrackStatusCompleted
	case string(vstar.TodoCancelled):
		return core.TrackStatusAbandoned
	case string(vstar.TodoNeedsAction):
		return core.TrackStatusPending
	}
	return core.TrackStatusPending
}

// icsToPriority maps an RFC 5545 PRIORITY integer back onto the
// CONFIGURED priority vocabulary by nearest rank.
//
// This is the FALLBACK path, used when a component carries no
// X-TLC-PRIORITY - a calendar written by some other tool, or by a tlc
// old enough to predate the X-property. It is inherently lossy: nine
// numeric slots cannot name a vocabulary entry that was never in the
// file, so the best available answer is the rank whose encoded slot sits
// closest to n.
//
// Inverts priorityToICS: for each rank the encoder would have written
// 1 + floor(rank*9/N), and the rank minimising |encoded - n| wins. Ties
// go to the MORE urgent rank (the lower index), because over-reporting
// urgency on an ambiguous import is the recoverable direction.
//
// n <= 0 is RFC 5545's "undefined" and decodes to the empty priority -
// NOT to the least urgent one. "Nobody set a priority" is a different
// fact from "somebody set the lowest", and conflating them would invent
// a triage decision the author never made.
func icsToPriority(n int, defs []config.PriorityDefinition) core.Priority {
	if n <= 0 || len(defs) == 0 {
		return ""
	}
	best, bestDist := 0, -1
	for rank := range defs {
		d := icsPriorityMin + rank*(icsPriorityMax+1-icsPriorityMin)/len(defs)
		if d > icsPriorityMax {
			d = icsPriorityMax
		}
		dist := d - n
		if dist < 0 {
			dist = -dist
		}
		if bestDist < 0 || dist < bestDist {
			best, bestDist = rank, dist
		}
	}
	return core.Priority(defs[best].Name)
}

// priorityFromComponent resolves a task's priority, preferring the
// verbatim X-TLC-PRIORITY name over the lossy numeric PRIORITY.
//
// The name only wins when it NAMES a currently-configured priority. A
// calendar exported under one vocabulary and imported under another
// would otherwise smuggle in a value that fails core.ValidPriority, and
// every downstream consumer (filters, sorts, the enum flags) would then
// be looking at a priority the project does not have. When the name is
// unrecognised the numeric value still gives a defensible nearest-rank
// answer in the CURRENT vocabulary, so that is what we fall back to.
func priorityFromComponent(todo vstar.Component, defs []config.PriorityDefinition) core.Priority {
	if p, ok := todo.Get(XPropPriority); ok {
		name := strings.TrimSpace(p.Value)
		for _, d := range defs {
			if d.Name == name {
				return core.Priority(name)
			}
		}
	}
	if p, ok := todo.Get("PRIORITY"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(p.Value)); err == nil {
			return icsToPriority(n, defs)
		}
	}
	return ""
}

// setMeta writes a Task.Meta entry, allocating the map on first use.
func setMeta(t *core.Task, key string, value interface{}) {
	if t.Meta == nil {
		t.Meta = map[string]interface{}{}
	}
	t.Meta[key] = value
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

func paramFirst(params []vstar.Param, key string) string {
	for _, p := range params {
		if strings.EqualFold(p.Name, key) {
			return p.Value
		}
	}
	return ""
}

// dateOnlyLayout is RFC 5545 §3.3.4 DATE (`YYYYMMDD`), the value form
// carried by a VALUE=DATE property.
const dateOnlyLayout = "20060102"

// parseDateOnly parses an RFC 5545 §3.3.4 DATE value as midnight UTC.
//
// Upstream gap: vstar has no VALUE=DATE support — vstar.ParseTime is
// strict form #2 only and the typed Component accessors reject a
// date-only value. This is the single fallback that remains; every
// other form goes through vstar. Drop it once vstar grows a DATE type.
func parseDateOnly(s string) (time.Time, bool) {
	t, err := time.ParseInLocation(dateOnlyLayout, s, time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// propTime reads a time-bearing property from todo, accepting RFC 5545
// §3.3.5 form #2 (via vstar) and falling back to a bare DATE value.
func propTime(c vstar.Component, name string) (time.Time, bool) {
	p, ok := c.Get(name)
	if !ok {
		return time.Time{}, false
	}
	return parsePropValue(p.Value)
}

// parsePropValue applies the form #2 → DATE ladder to a raw property
// value.
func parsePropValue(v string) (time.Time, bool) {
	if t, ok := vstar.ParseTime(v); ok {
		return t, true
	}
	return parseDateOnly(v)
}

// decodeCategories reads a task's tags from the CATEGORIES property
// (RFC 5545 §3.8.1.2), tolerating BOTH wire shapes.
//
// The idiomatic shape -- and the only one tlc writes now -- is a single
// property carrying a comma-separated list, which helpers.Categories
// reads: it splits on comma, trims, and drops empty tokens.
//
// The other shape is one property PER tag. tlc emitted that for its
// whole history, so it is what every .ics already on disk looks like.
// helpers.Categories cannot read it: it resolves the property with
// Component.Get, which returns only the FIRST match, so a file with
// `CATEGORIES:security` + `CATEGORIES:auth` would silently decode to
// just ["security"] -- tags dropped with no error, the kind of loss a
// user only notices later as a filter returning nothing. So when GetAll
// finds more than one property, each is parsed and the results are
// concatenated.
//
// Order is preserved in both shapes: tlc tags are ordered and callers
// compare the slice. Duplicates are dropped across the whole set --
// repeated properties may legitimately repeat a tag -- keeping
// first-seen order, matching helpers.SetCategories on the write side so
// a decode/encode round trip is stable.
func decodeCategories(todo vstar.Component) []string {
	props := todo.GetAll("CATEGORIES")
	if len(props) == 0 {
		return nil
	}
	if len(props) == 1 {
		return dedupeTags(helpers.Categories(todo))
	}
	var out []string
	for _, p := range props {
		single := vstar.Component{Props: []vstar.Property{p}}
		out = append(out, helpers.Categories(single)...)
	}
	return dedupeTags(out)
}

// dedupeTags drops repeats while preserving first-seen order. Compare
// is case-sensitive, matching helpers.SetCategories -- CATEGORIES are
// user-facing labels, so "Work" and "work" are distinct tags.
func dedupeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if _, dup := seen[tag]; dup {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}
