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
// UID handling: a UID of the form `<typeid>@<our-domain>` — the domain
// being the one WithUIDDomain configured, DefaultUIDDomain otherwise —
// where typeid matches core.IsTaskID/IsTrackID is reused as the entity
// ID. So is a bare `<typeid>` carrying no domain at all. Everything
// else is foreign, a well-formed typeid under someone else's domain
// included: the decoder mints a fresh ID and stashes the original UID
// in Meta["external_uid"].
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
		if isTrackComponent(todo, o.uidDomain) {
			tr, uidBodyVal, perr := decodeTrack(todo, o.uidDomain)
			if perr != nil {
				return nil, perr
			}
			res.Tracks = append(res.Tracks, tr)
			if uidBodyVal != "" {
				uidToTrackID[uidBodyVal] = tr.ID
			}
		} else {
			t, uidBodyVal, perr := decodeTask(todo, o.uidDomain, o.priorities, statusDefs)
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
		if isTrackComponent(todo, o.uidDomain) {
			continue
		}
		uid := getUID(todo)
		taskID := uidToTaskID[uidBody(uid, o.uidDomain)]
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
			body := uidBody(rel.UID, o.uidDomain)
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
		le := decodeJournal(j, uidToTaskID, o.uidDomain)
		res.Logs = append(res.Logs, le)
	}

	return res, nil
}

func isTrackComponent(todo vstar.Component, domain string) bool {
	if p, ok := todo.Get(XPropTrackKind); ok && strings.EqualFold(p.Value, "TRUE") {
		return true
	}
	// Fall-back: if UID body looks like a track typeid, treat as track.
	if core.IsTrackID(uidBody(getUID(todo), domain)) {
		return true
	}
	return false
}

func decodeTask(
	todo vstar.Component,
	domain string,
	defs []config.PriorityDefinition,
	statusDefs []config.StatusDefinition,
) (*core.Task, string, error) {
	t := &core.Task{}

	uid := getUID(todo)
	body := uidBody(uid, domain)
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
	t.Status = decodeTaskStatus(todo, statusDefs)
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

func decodeTrack(todo vstar.Component, domain string) (*core.Track, string, error) {
	tr := &core.Track{}

	uid := getUID(todo)
	body := uidBody(uid, domain)
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
	tr.Status = decodeTrackStatus(todo)
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

func decodeJournal(j vstar.Component, uidToTaskID map[string]string, domain string) *core.LogEntry {
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
			body := uidBody(rel.Value, domain)
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

// uidBody strips our own "@domain" suffix from a UID, leaving the
// typeid for matching. A UID carrying any other domain is returned
// whole: the domain is what distinguishes a UID tlc minted from one a
// foreign calendar minted, and discarding it would let
// `task_<26 chars>@someone-else.example` be mistaken for our own
// identity and silently overwrite that row. Left intact, the body
// fails core.IsTaskID and the caller takes the foreign-UID path —
// fresh TypeID, original UID preserved in Meta["external_uid"].
//
// A bare UID with no "@" at all keeps its historical treatment: it is
// its own body, so a domainless typeid still round-trips.
func uidBody(uid, domain string) string {
	if uid == "" {
		return ""
	}
	i := strings.IndexByte(uid, '@')
	if i < 0 {
		return uid
	}
	if strings.EqualFold(uid[i+1:], domain) {
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
