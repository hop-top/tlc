package vtodo

import (
	"strconv"
	"strings"
	"time"

	vstar "hop.top/vstar"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// Per-property decode steps behind decodeTask, decodeTrack and
// decodeJournal. Each reads one group of wire properties into the
// entity and owns the fields it writes, so the steps compose in any
// order except where a comment says otherwise.

// indexUID records body → id for cross-component linkage. A component
// with no UID has no body and is not indexable.
func indexUID(idx map[string]string, body, id string) {
	if body != "" {
		idx[body] = id
	}
}

// decodeIdentity resolves a component's UID to an entity ID: the UID
// body itself when isOwn accepts it, a freshly minted ID otherwise.
// body is returned for indexing either way; external is the original
// UID when it was foreign (the caller parks it in Meta["external_uid"])
// and "" when the ID was reused or the component carried no UID.
func decodeIdentity(todo vstar.Component, domain string, isOwn func(string) bool, mint func() string) (id, body, external string) {
	uid := getUID(todo)
	body = uidBody(uid, domain)
	if isOwn(body) {
		return body, body, ""
	}
	return mint(), body, uid
}

// entityTimestamps reads CREATED and LAST-MODIFIED, the latter falling
// back to DTSTAMP when absent. Zero for a property the component lacks.
func entityTimestamps(c vstar.Component) (created, updated time.Time) {
	if ts, ok := propTime(c, propCreated); ok {
		created = ts
	}
	if ts, ok := propTime(c, "LAST-MODIFIED"); ok {
		updated = ts
	}
	if updated.IsZero() {
		// Fallback to DTSTAMP when LAST-MODIFIED is absent.
		if ts, ok := propTime(c, "DTSTAMP"); ok {
			updated = ts
		}
	}
	return created, updated
}

// decodeTaskPriority resolves the priority and its recorded provenance.
func decodeTaskPriority(t *core.Task, todo vstar.Component, defs []config.PriorityDefinition) {
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
}

// decodeTaskSchedule reads DUE (with its date-only flag) and RRULE. An
// RRULE core cannot validate is dropped rather than stored.
func decodeTaskSchedule(t *core.Task, todo vstar.Component) {
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
}

// decodeTaskAssignee prefers X-TLC-ASSIGNEE and falls back to the first
// ATTENDEE, stripping a mailto: prefix.
func decodeTaskAssignee(t *core.Task, todo vstar.Component) {
	if p, ok := todo.Get(XPropAssignee); ok {
		val := p.Value
		t.AssignedTo = &val
		return
	}
	p, ok := todo.Get("ATTENDEE")
	if !ok {
		return
	}
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

// decodeTaskTLCFields reads the typed X-TLC-* properties that have no
// iCalendar equivalent: effort, project, sequence, archived flag and
// the run edge.
func decodeTaskTLCFields(t *core.Task, todo vstar.Component, domain string) {
	if p, ok := todo.Get(XPropEffort); ok {
		eff := core.Effort(strings.TrimSpace(p.Value))
		if core.ValidEffort(eff) {
			t.Effort = eff
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
}

// decodeTrackDue reads DUE into the track, flagging a date-only value
// in Meta as decodeTaskSchedule does for a task.
func decodeTrackDue(tr *core.Track, todo vstar.Component) {
	due, dateOnly := decodeDue(todo)
	if due == nil {
		return
	}
	tr.DueAt = due
	if dateOnly {
		if tr.Meta == nil {
			tr.Meta = map[string]any{}
		}
		tr.Meta[MetaDueDateOnly] = true
	}
}

// decodeJournalStatus keeps a supersession journal's effective status
// in Meta. X-VSTAR-* properties are not tlc's to adopt (see applyMeta),
// with this one exception: the effective status is the ledger's fact
// about the task, and dropping it would re-export a foreign ledger
// entry as a plain journal. It is kept only when the entry's own
// action cannot reproduce it, so tlc's own output decodes to the Meta
// it was exported from.
func decodeJournalStatus(le *core.LogEntry, j vstar.Component, statusDefs []config.StatusDefinition) {
	eff, ok := supersessionStatus(j)
	if !ok {
		return
	}
	if isStatusTransition(le.Action, statusDefs) &&
		strings.EqualFold(eff, derivedEffectiveStatus(le.Action, statusDefs)) {
		return
	}
	if le.Meta == nil {
		le.Meta = map[string]interface{}{}
	}
	le.Meta[MetaEffectiveStatusKey] = eff
}

// journalTimestamp reads a log entry's own instant. CREATED carries it;
// tlc writes DTSTAMP with the same value, but a foreign producer's
// DTSTAMP is whatever its clock said, so it is only a fallback for
// calendars that emit no CREATED.
func journalTimestamp(j vstar.Component) time.Time {
	if ts, ok := propTime(j, propCreated); ok {
		return ts
	}
	if ts, ok := propTime(j, "DTSTAMP"); ok {
		return ts
	}
	return time.Time{}
}

// journalTaskFromRelations resolves a journal's task from its
// RELATED-TO properties, in wire order: the first whose UID body names
// a decoded task, else the first that is itself a task TypeID. "" when
// neither.
func journalTaskFromRelations(j vstar.Component, uidToTaskID map[string]string, domain string) string {
	for _, rel := range j.GetAll("RELATED-TO") {
		body := uidBody(rel.Value, domain)
		if id, ok := uidToTaskID[body]; ok {
			return id
		}
		if core.IsTaskID(body) {
			return body
		}
	}
	return ""
}
