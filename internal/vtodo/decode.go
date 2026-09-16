package vtodo

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	vstar "hop.top/vstar"
	"hop.top/vstar/codec/rfc5545"
	"hop.top/vstar/hashing"
	"hop.top/vstar/helpers"
	"hop.top/vstar/supersession"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// ParseResult is the structured output of ParseVCalendar.
type ParseResult struct {
	Tasks  []*core.Task
	Tracks []*core.Track
	Logs   []*core.LogEntry
	// Warnings lists integrity findings that did not stop the import:
	// one entry per component (nested VALARMs included) whose
	// X-VSTAR-HASH is absent or does not match its content. Empty for
	// a calendar tlc wrote and nobody altered.
	Warnings []string
	// Concepts maps a component's wire UID to the agentic concept it
	// declared through X-TLC-CONCEPT (lowercased), for every component
	// that carried one, VEVENT and sub-components included. Keyed by
	// UID rather than entity ID because the declaration is a fact
	// about the wire component: log entries have no ID of their own,
	// and a VEVENT maps to no entity at all. Nil when nothing in the
	// calendar declared a concept. The token is never stored on the
	// entities themselves; the encoder re-derives it from TrackID and
	// Action on export.
	Concepts map[string]string
}

// ParseVCalendar reads an iCalendar stream and returns the tasks,
// tracks, and log entries it contained.
//
// VTODO classification, first match wins:
//   - X-TLC-CONCEPT=assignment → Task (an assignment is a scoped unit
//     inside a mission, and tlc has no track inside a track)
//   - X-TLC-IS-TRACK=TRUE marker → Track
//   - UID body is a track TypeID → Track
//   - otherwise → Task
//
// X-TLC-CONCEPT=mission is not decisive on its own: a track and a
// standalone task are both missions, so the marker and the UID prefix
// still pick the Go type.
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
//
// VEVENT: an open turn (X-TLC-CONCEPT:turn, no DTEND) sets ClaimedAt on
// the task its PARENT edge names; nothing else is decoded from events.
func ParseVCalendar(r io.Reader, opts ...Option) (*ParseResult, error) {
	o := resolve(opts)
	statusDefs := o.statusDefinitions()

	cal, err := rfc5545.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parse calendar: %w", err)
	}

	res := &ParseResult{Warnings: verifyHashes(cal), Concepts: collectConcepts(cal)}

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
			t, uidBodyVal, perr := decodeTask(todo, cal.Components, o.uidDomain, o.priorities, statusDefs)
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
		// PARENT when the RELTYPE param is absent, matches the param
		// name case-insensitively and folds a registered value to its
		// canonical spelling, so the typed constants compare directly.
		// Unknown RELTYPEs are ignored.
		for _, rel := range helpers.RelatedTo(todo) {
			body := uidBody(rel.UID, o.uidDomain)
			switch rel.RelType {
			case vstar.RelParent:
				if id, ok := uidToTrackID[body]; ok {
					tid := id
					task.TrackID = &tid
				} else if core.IsTrackID(body) {
					tid := body
					task.TrackID = &tid
				}
			case vstar.RelDependsOn:
				blocker := body
				if id, ok := uidToTaskID[body]; ok {
					blocker = id
				}
				if task.Meta == nil {
					task.Meta = map[string]interface{}{}
				}
				// Normalise rather than assert. Meta may already carry
				// blocked_by in any supported shape (notably the
				// []interface{} produced by JSON decoding), and a bare
				// []string assertion would silently drop it. Re-running
				// the coercion over the appended slice also collapses a
				// repeated DEPENDS-ON edge to a single blocker.
				merged := append(
					core.NormalizeBlockedBy(task.Meta["blocked_by"]),
					blocker,
				)
				task.Meta["blocked_by"] = core.NormalizeBlockedBy(merged)
			}
		}
	}

	// VJOURNAL → LogEntry.
	for _, j := range cal.Filter(vstar.CompJournal) {
		le := decodeJournal(j, uidToTaskID, o.uidDomain, statusDefs)
		res.Logs = append(res.Logs, le)
	}

	// An open turn VEVENT is the wire form of Task.ClaimedAt: the task
	// row itself carries no claim property. Only a VEVENT that declares
	// itself a turn counts; a foreign event related to a task is not a
	// claim. VEVENTs are otherwise not decoded: a turn is a log-derived
	// window and a playthrough has no entity the decoder mints.
	for _, ev := range cal.Filter(vstar.CompEvent) {
		if declaredConcept(ev) != ConceptTurn {
			continue
		}
		if _, closed := ev.Get("DTEND"); closed {
			continue
		}
		start, ok := propTime(ev, "DTSTART")
		if !ok {
			continue
		}
		for _, rel := range helpers.RelatedTo(ev) {
			if rel.RelType != vstar.RelParent {
				continue
			}
			id, ok := uidToTaskID[uidBody(rel.UID, o.uidDomain)]
			if !ok {
				continue
			}
			for _, t := range res.Tasks {
				if t.ID == id && (t.ClaimedAt == nil || start.After(*t.ClaimedAt)) {
					at := start
					t.ClaimedAt = &at
				}
			}
		}
	}

	return res, nil
}

// verifyHashes checks X-VSTAR-HASH on every component of cal, nested
// sub-components included, and returns one warning per component that
// fails: a missing hash (the producer is not V*-conformant) or a
// mismatch (the component changed after it was hashed, or its bytes
// were altered in transit). Warnings rather than errors: the entities
// are still importable, and the caller decides how loudly to say so.
func verifyHashes(cal vstar.Calendar) []string {
	var out []string
	var walk func(c vstar.Component, parent string)
	walk = func(c vstar.Component, parent string) {
		label := string(c.Type)
		if uid := c.UID(); uid != "" {
			label += " " + uid
		}
		if parent != "" {
			label = parent + " > " + label
		}
		ok, want, got := hashing.VerifyXVSTAR(c)
		switch {
		case ok:
		case got == "":
			out = append(out, label+": no X-VSTAR-HASH")
		default:
			out = append(out, fmt.Sprintf("%s: X-VSTAR-HASH mismatch: stored %s, computed %s", label, got, want))
		}
		for _, sub := range c.Sub {
			walk(sub, label)
		}
	}
	for _, c := range cal.Components {
		walk(c, "")
	}
	return out
}

// isTrackComponent picks the Go type of a VTODO. The concept token is
// consulted first, the track marker second, the UID prefix last, so a
// declaration a producer made on purpose beats a marker and a marker
// beats a naming convention.
//
// Only `assignment` decides: it names a unit inside a mission, which in
// tlc is always a task, whatever marker or UID prefix the component
// also carries. `mission` covers tracks and standalone tasks alike and
// so falls through to the marker; any other token (or none, the case
// for every calendar written before the property existed) does too.
func isTrackComponent(todo vstar.Component, domain string) bool {
	if declaredConcept(todo) == ConceptAssignment {
		return false
	}
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
	ledger []vstar.Component,
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
	t.Status = decodeTaskStatus(todo, ledger, statusDefs)
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
	if due, dateOnly := decodeDue(todo); due != nil {
		t.DueAt = due
		if dateOnly {
			setMeta(t, MetaDueDateOnly, true)
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
	if p, ok := todo.Get(XPropRun); ok {
		// Only a run TypeID under our domain is ours to reference; a
		// foreign playthrough UID names a run this store has no row for.
		if body := uidBody(strings.TrimSpace(p.Value), domain); core.IsRecipeRunID(body) {
			t.RunID = body
		}
	}
	// After DUE: a relative trigger anchored to END resolves against
	// it, and the derived auto reminder is told apart by its shape.
	decodeReminders(t, todo, time.Now())

	// X-TLC-META payload plus any unrecognized X-TLC-* property, so a
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
	if p, ok := todo.Get(XPropTrackSeq); ok {
		if n, err := strconv.ParseInt(strings.TrimSpace(p.Value), 10, 64); err == nil {
			tr.Seq = n
		}
	}
	if due, dateOnly := decodeDue(todo); due != nil {
		tr.DueAt = due
		if dateOnly {
			if tr.Meta == nil {
				tr.Meta = map[string]any{}
			}
			tr.Meta[MetaDueDateOnly] = true
		}
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

func decodeJournal(
	j vstar.Component,
	uidToTaskID map[string]string,
	domain string,
	statusDefs []config.StatusDefinition,
) *core.LogEntry {
	le := &core.LogEntry{}

	if p, ok := j.Get(XPropLogAction); ok {
		le.Action = p.Value
	}
	// X-VSTAR-* properties are not tlc's to adopt (see applyMeta), with
	// one exception: the effective status of a supersession journal is
	// the ledger's fact about the task, and dropping it would re-export
	// a foreign ledger entry as a plain journal. It is kept only when
	// the entry's own action cannot reproduce it, so tlc's own output
	// decodes to the Meta it was exported from.
	if eff, ok := supersessionStatus(j); ok {
		if !isStatusTransition(le.Action, statusDefs) ||
			!strings.EqualFold(eff, derivedEffectiveStatus(le.Action, statusDefs)) {
			if le.Meta == nil {
				le.Meta = map[string]interface{}{}
			}
			le.Meta[MetaEffectiveStatusKey] = eff
		}
	}
	if p, ok := j.Get(XPropLogBy); ok {
		le.By = p.Value
	}
	if p, ok := j.Get("DESCRIPTION"); ok {
		le.Note = p.Value
	} else if p, ok := j.Get("SUMMARY"); ok {
		le.Note = p.Value
	}
	// CREATED carries the log entry's own instant. tlc writes DTSTAMP
	// with the same value, but a foreign producer's DTSTAMP is whatever
	// its clock said, so it is only a fallback for calendars that emit
	// no CREATED.
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

// decodeTaskStatus recovers a tlc status from a VTODO, walking a
// three-step ladder:
//
//  1. The exact name in XPropStatus, when the CURRENT vocabulary declares
//     it. tlc writes the property with the task's current status, so a
//     VTODO carrying it is a mutated-in-place snapshot and the ledger
//     behind it is history, not a correction.
//  2. The ledger. A VTODO without a usable XPropStatus is foreign, or
//     from a vocabulary this config no longer has; for it the spec 02
//     discipline applies: the original component is never mutated and
//     the latest supersession journal pointing at it holds the truth.
//     supersession.Superseded picks that entry (latest DTSTAMP wins);
//     its value is used when it is one of the four RFC 5545 VTODO
//     STATUS values, the vocabulary tlc writes and the one the spec
//     example shows. Any other vocabulary is opaque here and falls
//     through.
//  3. The role the VTODO's own STATUS value implies.
//
// The X-property is only trusted when the CURRENT vocabulary declares
// that name. A calendar exported from another project (or from this one
// before a vocabulary change) can name a status this config would reject,
// and importing it would seed the store with a value every later
// validation refuses. Falling back to the role default keeps the import
// inside the configured vocabulary while preserving the coarse meaning.
func decodeTaskStatus(todo vstar.Component, ledger []vstar.Component, defs []config.StatusDefinition) core.TaskStatus {
	if p, ok := todo.Get(XPropStatus); ok {
		name := strings.TrimSpace(p.Value)
		for _, def := range defs {
			if strings.EqualFold(def.Name, name) {
				return core.TaskStatus(def.Name)
			}
		}
	}
	wire := ""
	if eff, ok := supersession.Superseded(todo, ledger); ok && isTodoStatusWire(eff) {
		wire = strings.TrimSpace(eff)
	} else if p, ok := todo.Get("STATUS"); ok {
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
// 1 + floor(rank*9/N), and the rank minimizing |encoded - n| wins. Ties
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
// unrecognized the numeric value still gives a defensible nearest-rank
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

// propTime reads a DATE-TIME property from c through vstar.ParseTime:
// RFC 5545 §3.3.5 form #2 only. The properties read this way
// (CREATED, LAST-MODIFIED, DTSTAMP, DTSTART of a turn) are DATE-TIME
// by definition; a VALUE=DATE among them is non-conforming input and
// decodes to nothing rather than to a midnight the producer never
// wrote. DUE, the one property tlc reads in both forms, goes through
// decodeDue.
func propTime(c vstar.Component, name string) (time.Time, bool) {
	p, ok := c.Get(name)
	if !ok {
		return time.Time{}, false
	}
	return vstar.ParseTime(p.Value)
}

// decodeCategories reads a task's tags from the CATEGORIES property
// (RFC 5545 §3.8.1.2), tolerating BOTH wire shapes.
//
// The shape tlc writes is one property PER tag (see addCategories). The
// other legal shape is a single property carrying a comma-separated
// list, which foreign producers and one interim tlc release write;
// helpers.Categories reads that one: it splits on comma, trims, and
// drops empty tokens.
//
// helpers.Categories cannot read the repeated shape: it resolves the
// property with Component.Get, which returns only the FIRST match, so
// `CATEGORIES:security` + `CATEGORIES:auth` would silently decode to
// just ["security"] -- tags dropped with no error, the kind of loss a
// user only notices later as a filter returning nothing. So when GetAll
// finds more than one property, each is parsed and the results are
// concatenated. Each property's value is still split on comma, which
// is what keeps a mixed-shape file readable.
//
// Order is preserved in both shapes: tlc tags are ordered and callers
// compare the slice. Duplicates are dropped across the whole set --
// repeated properties may legitimately repeat a tag -- keeping
// first-seen order, matching addCategories on the write side so a
// decode/encode round trip is stable.
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
// is case-sensitive, matching addCategories -- CATEGORIES are
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
