package vtodo

import (
	"bytes"
	"fmt"
	"math"
	"sort"
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
	// XPropTrackSeq carries Track.Seq, the per-project sequence behind
	// the "L-NNNN" display alias, as XPropTaskSeq does for tasks.
	XPropTrackSeq  = "X-TLC-TRACK-SEQ"
	XPropLogAction = "X-TLC-LOG-ACTION"
	XPropLogBy     = "X-TLC-LOG-BY"
	XPropLogTaskID = "X-TLC-LOG-TASK"
	// XPropPriority carries the tlc priority NAME verbatim. The numeric
	// PRIORITY property is lossy for any vocabulary that is not exactly
	// nine entries long, and meaningless for a custom vocabulary whose
	// names a foreign reader has never heard of; this is what makes the
	// round-trip exact.
	XPropPriority = "X-TLC-PRIORITY"
	// XPropPrioritySource and XPropPriorityRule export the priority
	// provenance recorded in Task.Meta, so a re-imported task still
	// knows whether a human or a derivation rule set its priority.
	XPropPrioritySource = "X-TLC-PRIORITY-SOURCE"
	XPropPriorityRule   = "X-TLC-PRIORITY-RULE"

	// XPropStatus carries the tlc status NAME alongside the RFC 5545
	// STATUS property. STATUS has four legal values; a tlc status
	// vocabulary is configurable and may declare more (or rename all of
	// them), so STATUS alone is lossy. The X-property is what makes the
	// round trip exact; STATUS stays for foreign consumers.
	XPropStatus = "X-TLC-STATUS"

	// XPropTrackStatus does the same for tracks, where "completed" and
	// "archived" both map to STATUS:COMPLETED and would otherwise be
	// indistinguishable on import.
	XPropTrackStatus = "X-TLC-TRACK-STATUS"

	// XPropArchived carries Task.Archived, which has no iCalendar
	// equivalent at all. Emitted only when true.
	XPropArchived = "X-TLC-ARCHIVED"

	// XPropConcept declares which V* agentic concept a component is
	// (spec-vstar 02 "Core mapping", conformance criterion 4). One
	// token per component, from the Concept* constants: a VTODO says
	// whether it is a mission or an assignment, a VJOURNAL what kind of
	// journal it is. VALARM never carries it (single-concept). Written
	// right after the constructor-seeded properties so the declaration
	// leads the block.
	//
	// Not CATEGORIES: that property carries Task.Tags, and the codec
	// escapes every comma in a TEXT value, so a token appended there
	// would be glued to the user's labels.
	XPropConcept = "X-TLC-CONCEPT"
)

// icsPriorityMin and icsPriorityMax bound the DEFINED half of the
// RFC 5545 §3.8.1.9 PRIORITY range. 0 is reserved for "undefined" and is
// never produced by the spread.
const (
	icsPriorityMin = 1
	icsPriorityMax = 9
)

// BuildVCalendar serialises tasks, tracks and (optionally) log entries
// into a vstar.Calendar with VTODO and VJOURNAL components. Pair with
// Serialize to produce an .ics string.
//
// Hierarchy: each Task with a non-nil TrackID emits a bare RELATED-TO
// pointing at the track's UID (RFC 5545 §3.2.15 defaults RELTYPE to
// PARENT; spec 02 requires the parameter be omitted for that value).
// The edge is encoded once, in that direction: a Track carries no
// CHILD back-reference, and membership is read from the tasks.
//
// Dependencies: if Task.Meta["blocked_by"] holds a []string of task IDs
// (or anything coerceable to one), each blocker emits a RELATED-TO
// RELTYPE=DEPENDS-ON.
//
// Logs: gated by WithIncludeLogs(true). Each LogEntry becomes a
// VJOURNAL with a RELATED-TO pointing at its task's UID. A status
// transition whose task is in the export is emitted as a spec 02
// supersession entry (UID journal:status:<task uid>:<t>,
// CATEGORIES:status-supersession, X-VSTAR-EFFECTIVE-STATUS), the
// append-only ledger shape; every other entry keeps the plain shape
// (UID log-<task>-<action>-<t>@<domain>). See buildLogComponent.
//
// DTSTAMP: every component's DTSTAMP is the instant its entity was last
// modified (UpdatedAt, a log entry's own Timestamp; a VALARM takes its
// parent's), NOT the export clock RFC 5545 §3.8.7.2 describes. The
// canonical form spec-vstar hashes includes DTSTAMP, so an export-time
// stamp would give unchanged content a new X-VSTAR-HASH on every
// export and defeat the hash as a change detector. Documented as a
// deviation in docs/VSTAR-CONFORMANCE.md. The export clock
// (WithExportTime) only backs entities that carry no timestamp at all.
//
// Turns: each exported task's claim windows become VEVENTs
// (X-TLC-CONCEPT:turn) after the journals. When logs are exported the
// windows come from CLAIMED/RECLAIMED … DONE/RELEASED/… pairs in the
// task's log, closed and open alike; otherwise, or when the log opens
// no window, Task.ClaimedAt yields the one open turn. See turnsFor.
//
// Playthroughs: each run passed through WithRecipeRuns becomes a VEVENT
// (X-TLC-CONCEPT:playthrough) after the turns, with RELATED-TO the
// track VTODO when the run has one. A task materialised by a run
// points at it with X-TLC-RUN.
//
// Hashing: every builder's last step is finalize, so each VTODO,
// VJOURNAL, VEVENT and VALARM leaves here with an X-VSTAR-HASH that
// verifies over the finished component.
func BuildVCalendar(
	tasks []*core.Task,
	tracks []*core.Track,
	logs []*core.LogEntry,
	opts ...Option,
) (vstar.Calendar, error) {
	o := resolve(opts)
	statusDefs := o.statusDefinitions()

	// NewCalendar is PRODID-only at this vstar version; CALSCALE and
	// METHOD are injected by Serialize.
	cal := helpers.NewCalendar(o.productID)

	// Index tasks per track for PERCENT-COMPLETE.
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

	// The log is consulted for COMPLETED whether or not it is exported.
	doneAt := completedAtIndex(logs, statusDefs)

	// Tracks → VTODO.
	for _, tr := range tracks {
		if tr == nil {
			continue
		}
		c, err := buildTrackComponent(tr, tasksByTrack[tr.ID], o.uidDomain, o.exportTime)
		if err != nil {
			return vstar.Calendar{}, err
		}
		cal.Append(c)
	}

	// Tasks → VTODO. The finished components are kept by task ID: a
	// supersession journal is built against the component it
	// supersedes, and supersession.Supersedes verifies that target's
	// hash, so it has to be the one that went into the calendar.
	targets := make(map[string]vstar.Component, len(tasks))
	for _, t := range tasks {
		if t == nil {
			continue
		}
		c, err := buildTaskComponent(t, o.uidDomain, o.priorities, statusDefs, o.exportTime, doneAt[t.ID])
		if err != nil {
			return vstar.Calendar{}, err
		}
		cal.Append(c)
		targets[t.ID] = c
	}

	// LogEntries → VJOURNAL (gated). The exported log is also indexed
	// per task for the turns below; an unexported log bounds nothing a
	// reader could see.
	logsByTask := make(map[string][]*core.LogEntry)
	if o.includeLogs {
		for _, le := range logs {
			if le == nil {
				continue
			}
			c, err := buildLogComponent(le, o.uidDomain, statusDefs, o.exportTime, targets)
			if err != nil {
				return vstar.Calendar{}, err
			}
			cal.Append(c)
			logsByTask[le.TaskID] = append(logsByTask[le.TaskID], le)
		}
	}

	// Turns → VEVENT, per exported task.
	for _, t := range tasks {
		if t == nil {
			continue
		}
		for _, w := range turnsFor(t, logsByTask[t.ID]) {
			c, err := buildTurnComponent(t, w, o.uidDomain, o.exportTime)
			if err != nil {
				return vstar.Calendar{}, err
			}
			cal.Append(c)
		}
	}

	// Playthroughs → VEVENT.
	for _, run := range o.recipeRuns {
		if run == nil {
			continue
		}
		c, err := buildPlaythroughComponent(run, o.uidDomain, o.exportTime)
		if err != nil {
			return vstar.Calendar{}, err
		}
		cal.Append(c)
	}

	return cal, nil
}

// Serialize encodes cal as RFC 5545 wire format. Returns the UTF-8
// string ready to write to a file or stream.
//
// Post-processes the codec output to inject CALSCALE:GREGORIAN +
// METHOD:PUBLISH after PRODID. vstar.Calendar has no Props field for
// arbitrary calendar-level properties; inject manually until upstream
// grows one. Both lines are RFC 5545 §3.6 standard
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
// Workaround for the missing calendar-level property bag — drop when
// vstar.Calendar gains Props.
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
// wire string by looking the status up in the configured vocabulary and
// reading its ROLE.
//
// Role, not name, because the status vocabulary is configurable: a
// project declaring IN_REVIEW (role "active") and SHIPPED (role
// "completed") has no status this package could recognise by name, and a
// name switch would export NEEDS-ACTION for both. The four roles line up
// one-for-one with the four RFC 5545 VTODO STATUS values, so role is
// exactly the fact the wire format wants.
//
// Falls back to NEEDS-ACTION for a status the vocabulary does not
// declare, which is the RFC's own "nothing has happened yet" value and
// the least wrong guess available.
func statusToWire(s core.TaskStatus, defs []config.StatusDefinition) string {
	return roleToWire(statusRole(s, defs))
}

// statusRole resolves a status name to its configured role. Comparison is
// case-insensitive: config files are hand-written and the wire is not the
// place to punish a lowercase "done".
func statusRole(s core.TaskStatus, defs []config.StatusDefinition) string {
	for _, def := range defs {
		if strings.EqualFold(def.Name, string(s)) {
			return def.Role
		}
	}
	return ""
}

// roleToWire maps a config role constant to its RFC 5545 STATUS value.
func roleToWire(role string) string {
	switch role {
	case config.RoleActive:
		return string(vstar.TodoInProcess)
	case config.RoleCompleted:
		return string(vstar.TodoCompleted)
	case config.RoleSkipped:
		return string(vstar.TodoCancelled)
	default:
		return string(vstar.TodoNeedsAction)
	}
}

// trackStatusToWire maps a TrackStatus to an RFC 5545 STATUS value.
// Tracks carry their own fixed vocabulary (no config knob), but
// "completed" and "archived" collapse onto COMPLETED, which is why the
// encoder also emits XPropTrackStatus.
func trackStatusToWire(s core.TrackStatus) string {
	switch s {
	case core.TrackStatusCompleted, core.TrackStatusArchived:
		return string(vstar.TodoCompleted)
	case core.TrackStatusAbandoned:
		return string(vstar.TodoCancelled)
	case core.TrackStatusActive:
		return string(vstar.TodoInProcess)
	default:
		return string(vstar.TodoNeedsAction)
	}
}

// priorityToICS maps a tlc Priority onto the iCalendar PRIORITY integer
// (RFC 5545 §3.8.1.9: 0 = undefined, 1 = highest urgency, 9 = lowest).
//
// The mapping is driven by the priority's RANK within the CONFIGURED
// vocabulary, not by its name. tlc's priority vocabulary is
// user-declared (`task.priorities`) and declaration order is rank order;
// a hardcoded P0/P1/P2/P3 table silently dropped the PRIORITY property
// for every project that renamed its priorities.
//
// FORMULA - the vocabulary of N names is spread linearly across the nine
// defined slots by giving each rank a band of width 9/N and taking that
// band's LOWER edge:
//
//	ics = 1 + floor(rank * 9 / N)
//
// so rank 0 always lands on 1 (most urgent) and rank N-1 lands as near 9
// as an integer band allows. This particular spread is chosen over an
// endpoint-anchored one (1 + rank*8/(N-1), which would give 1/4/6/9)
// because for the built-in four-name vocabulary it reproduces exactly
// the 1/3/5/7 tlc has always emitted, so existing .ics consumers see no
// change, and because it is the exact inverse of the decoder's
// nearest-rank fallback.
//
// For N > 9 two adjacent ranks can share a slot - unavoidable, the
// numeric range only has nine values - which is precisely why the name
// is also written to X-TLC-PRIORITY and preferred on decode.
//
// Empty priority returns 0 ("undefined"), which the caller skips: no
// priority set is a different fact from the least urgent priority.
func priorityToICS(p core.Priority, defs []config.PriorityDefinition) int {
	if p == "" {
		return 0
	}
	n := len(defs)
	if n == 0 {
		return 0
	}
	rank := -1
	for i, d := range defs {
		if d.Name == string(p) {
			rank = i
			break
		}
	}
	if rank < 0 {
		// Not in this vocabulary: no defensible rank, so no numeric
		// claim. The name still survives via X-TLC-PRIORITY.
		return 0
	}
	ics := icsPriorityMin + rank*(icsPriorityMax+1-icsPriorityMin)/n
	if ics > icsPriorityMax {
		ics = icsPriorityMax
	}
	return ics
}

// setDTSTAMP pins DTSTAMP to at. Every helpers constructor stamps
// DTSTAMP with the wall clock, which would make two exports of one
// unchanged calendar differ; the builder decides the instant instead.
func setDTSTAMP(c *vstar.Component, at time.Time) {
	c.Set(vstar.Property{Name: "DTSTAMP", Value: vstar.FormatTime(at)})
}

// dtstampFor picks a component's DTSTAMP: the first non-zero candidate
// (callers pass the entity's last modification first, then its
// creation), falling back to the export clock for an entity that
// carries no timestamp at all. See the DTSTAMP note on BuildVCalendar.
func dtstampFor(exportAt time.Time, candidates ...time.Time) time.Time {
	for _, t := range candidates {
		if !t.IsZero() {
			return t
		}
	}
	return exportAt
}

// finalize is the single hashing step of every builder and its last
// statement. The helpers mutators used mid-build (AddRelatedTo,
// Complete, SetCategories) each refresh X-VSTAR-HASH over the component
// AS IT STANDS, so whatever they wrote is stale by the time the builder
// returns; only a digest taken over the finished component, Sub
// included, verifies. The property is removed first so the fresh value
// lands last on the wire rather than at the slot the first mutator
// claimed.
func finalize(c *vstar.Component) {
	c.Remove(hashing.XVSTARHashProperty)
	hashing.SetXVSTAR(c)
}

// addParentRelation appends the containment edge from c to the
// component uid names, as a bare RELATED-TO. RFC 5545 §3.2.15 defaults
// RELTYPE to PARENT, and spec 02 ("Relationship types") requires the
// parameter be omitted for that value: RELATED-TO:x and
// RELATED-TO;RELTYPE=PARENT:x name the same edge but hash differently.
// helpers.AddRelatedTo spells out whatever RelType it is given,
// vstar.RelParent included, so the omission is the empty value.
func addParentRelation(c *vstar.Component, uid string) {
	helpers.AddRelatedTo(c, uid, "")
}

// completedAtIndex maps each task ID to the instant of its most recent
// transition into a completed-role status, read from the log. A
// transition entry's Action is the target status NAME (Task.Transition
// writes it that way), so the configured vocabulary's roles decide what
// counts as completion rather than a hard-coded "DONE". Tasks with no
// such entry are absent from the map and fall back to UpdatedAt.
func completedAtIndex(logs []*core.LogEntry, defs []config.StatusDefinition) map[string]time.Time {
	idx := make(map[string]time.Time)
	for _, le := range logs {
		if le == nil || le.TaskID == "" || le.Timestamp.IsZero() {
			continue
		}
		if statusRole(core.TaskStatus(le.Action), defs) != config.RoleCompleted {
			continue
		}
		if prev, ok := idx[le.TaskID]; !ok || le.Timestamp.After(prev) {
			idx[le.TaskID] = le.Timestamp
		}
	}
	return idx
}

func buildTaskComponent(
	t *core.Task,
	domain string,
	defs []config.PriorityDefinition,
	statusDefs []config.StatusDefinition,
	exportAt time.Time,
	doneAt time.Time,
) (vstar.Component, error) {
	// NewTodo seeds UID, DTSTAMP and DUE (omitted for a nil due; a
	// date-only due is written as VALUE=DATE, see newTodoWithDue) and
	// refuses an empty UID, which spec-vstar 02 requires on every
	// component.
	c, err := newTodoWithDue(uidFor(t.ID, domain), t.DueAt, dueDateOnly(t.Meta))
	if err != nil {
		return vstar.Component{}, fmt.Errorf("vtodo: task %q: %w", t.ID, err)
	}
	stamp := dtstampFor(exportAt, t.UpdatedAt, t.CreatedAt)
	setDTSTAMP(&c, stamp)
	c.Add(vstar.Property{Name: XPropConcept, Value: taskConcept(t)})

	if t.Title != "" {
		c.Add(vstar.Property{Name: "SUMMARY", Value: t.Title})
	}
	if t.Description != "" {
		c.Add(vstar.Property{Name: "DESCRIPTION", Value: t.Description})
	}
	if statusRole(t.Status, statusDefs) == config.RoleCompleted {
		// Complete writes STATUS, COMPLETED and PERCENT-COMPLETE=100 as
		// one unit, so a foreign reader sees a consistent "done" triple
		// rather than a bare STATUS. COMPLETED is the instant of the last
		// completing transition when the log has one, else the last
		// modification, the closest fact the model holds.
		if doneAt.IsZero() {
			doneAt = t.UpdatedAt
		}
		helpers.Complete(&c, doneAt)
	} else {
		c.Add(vstar.Property{Name: "STATUS", Value: statusToWire(t.Status, statusDefs)})
	}
	if t.Status != "" {
		c.Add(vstar.Property{Name: XPropStatus, Value: string(t.Status)})
	}
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
	if !t.CreatedAt.IsZero() {
		c.Add(vstar.Property{Name: "CREATED", Value: vstar.FormatTime(t.CreatedAt)})
	}
	if !t.UpdatedAt.IsZero() {
		c.Add(vstar.Property{Name: "LAST-MODIFIED", Value: vstar.FormatTime(t.UpdatedAt)})
	}
	if t.RRule != "" {
		c.Add(vstar.Property{Name: "RRULE", Value: t.RRule})
	}
	if t.Reference != "" {
		c.Add(vstar.Property{Name: "URL", Value: t.Reference})
	}
	// Reminders: RemindAt and Meta["reminders"] as absolute VALARMs,
	// the derived auto reminder as a relative one; see reminders.go.
	if err := addReminderAlarms(&c, t, stamp); err != nil {
		return vstar.Component{}, err
	}
	if t.TrackID != nil && *t.TrackID != "" {
		addParentRelation(&c, uidFor(*t.TrackID, domain))
	}
	for _, blocker := range blockedByList(t.Meta) {
		helpers.AddRelatedTo(&c, uidFor(blocker, domain), vstar.RelDependsOn)
	}
	if t.Effort != "" {
		c.Add(vstar.Property{Name: XPropEffort, Value: string(t.Effort)})
	}
	if t.AssignedTo != nil {
		addAssignee(&c, *t.AssignedTo)
	}
	if t.ProjectID != nil && *t.ProjectID != "" {
		c.Add(vstar.Property{Name: XPropProjectID, Value: *t.ProjectID})
	}
	if t.Seq > 0 {
		c.Add(vstar.Property{Name: XPropTaskSeq, Value: fmt.Sprintf("%d", t.Seq)})
	}
	if t.Archived {
		c.Add(vstar.Property{Name: XPropArchived, Value: "TRUE"})
	}
	if t.RunID != "" {
		// The assignment → playthrough edge; see XPropRun for why it is
		// not a RELATED-TO.
		c.Add(vstar.Property{Name: XPropRun, Value: uidFor(t.RunID, domain)})
	}
	addCategories(&c, t.Tags)
	addMeta(&c, t.Meta)
	addUnknownTLCProps(&c, t.Meta)
	finalize(&c)
	return c, nil
}

func buildTrackComponent(tr *core.Track, members []*core.Task, domain string, exportAt time.Time) (vstar.Component, error) {
	c, err := newTodoWithDue(uidFor(tr.ID, domain), tr.DueAt, dueDateOnly(tr.Meta))
	if err != nil {
		return vstar.Component{}, fmt.Errorf("vtodo: track %q: %w", tr.ID, err)
	}
	stamp := dtstampFor(exportAt, tr.UpdatedAt, tr.CreatedAt)
	setDTSTAMP(&c, stamp)
	// A track is the goal its tasks are scoped units of. The concept
	// says what it IS; XPropTrackKind below still says which Go type it
	// decodes to, because a standalone task is a mission too.
	c.Add(vstar.Property{Name: XPropConcept, Value: ConceptMission})

	if tr.Title != "" {
		c.Add(vstar.Property{Name: "SUMMARY", Value: tr.Title})
	}
	// STATUS stays for interop, but the track vocabulary does not fit in
	// it: completed and archived both read as COMPLETED. XPropTrackStatus
	// carries the real value so decode can tell them apart.
	c.Add(vstar.Property{Name: "STATUS", Value: trackStatusToWire(tr.Status)})
	if tr.Status != "" {
		c.Add(vstar.Property{Name: XPropTrackStatus, Value: string(tr.Status)})
	}
	if trackStatusToWire(tr.Status) == string(vstar.TodoCompleted) {
		// Spec 05 §5 pairs STATUS=COMPLETED with COMPLETED (an undated
		// VTODO is otherwise VS040), as helpers.Complete does for a task.
		// A track has no completing log entry, so the instant is its
		// last modification. PERCENT-COMPLETE stays the Progress column
		// rather than being forced to 100: an archived track can carry
		// open members.
		c.SetCOMPLETED(stamp)
	}
	// PERCENT-COMPLETE is the Progress column: terminal members over
	// all members, the same computation `tlc track` shows. Derived, so
	// it is never decoded; omitted for a track with no members, where
	// progress is undefined rather than zero.
	if progress := core.ComputeTrackProgress(members); progress.TotalTasks > 0 {
		pct := math.Round(float64(progress.CompletedTasks) / float64(progress.TotalTasks) * 100)
		c.Add(vstar.Property{Name: "PERCENT-COMPLETE", Value: fmt.Sprintf("%d", int(pct))})
	}

	if !tr.CreatedAt.IsZero() {
		c.Add(vstar.Property{Name: "CREATED", Value: vstar.FormatTime(tr.CreatedAt)})
	}
	if !tr.UpdatedAt.IsZero() {
		c.Add(vstar.Property{Name: "LAST-MODIFIED", Value: vstar.FormatTime(tr.UpdatedAt)})
	}
	if tr.Slug != "" {
		c.Add(vstar.Property{Name: XPropTrackSlug, Value: tr.Slug})
	}
	if tr.Type != "" {
		c.Add(vstar.Property{Name: XPropTrackType, Value: tr.Type})
	}
	if tr.Seq > 0 {
		c.Add(vstar.Property{Name: XPropTrackSeq, Value: fmt.Sprintf("%d", tr.Seq)})
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
	addMeta(&c, tr.Meta)
	addUnknownTLCProps(&c, tr.Meta)
	finalize(&c)
	return c, nil
}

// buildLogComponent emits one VJOURNAL per log entry, in one of two
// shapes:
//
//   - A status transition (isSupersessionEntry) whose task is among
//     targets is a spec 02 supersession entry, built by
//     supersession.Supersedes against the finished task component:
//     UID journal:status:<task uid>:<t>, DTSTAMP=t, a bare RELATED-TO
//     (RFC 5545 §3.2.15 defaults RELTYPE to PARENT, the shape the spec
//     example uses), CATEGORIES:status-supersession and
//     X-VSTAR-EFFECTIVE-STATUS. Supersedes verifies the target's
//     X-VSTAR-HASH and refuses a mutated one.
//   - Everything else keeps the plain shape: UID
//     log-<task>-<action>-<t>@<domain>, the same bare RELATED-TO.
//     That includes a status transition whose task is NOT in the
//     export: a supersession entry with no target in the same
//     calendar is an orphan (spec 05 §4, VS031), unprojectable by
//     definition, so it travels as history instead.
//
// Both shapes carry the same tlc properties (X-TLC-CONCEPT, SUMMARY,
// DESCRIPTION, CREATED, X-TLC-LOG-*, X-TLC-META) so a reader that
// ignores the ledger discipline sees one kind of journal. Supersedes
// hashes last, but the properties added after it leave that digest
// stale and mid-block, so finalize still closes the builder.
func buildLogComponent(
	le *core.LogEntry,
	domain string,
	statusDefs []config.StatusDefinition,
	exportAt time.Time,
	targets map[string]vstar.Component,
) (vstar.Component, error) {
	// A log entry is immutable, so its own instant is its last
	// modification.
	at := dtstampFor(exportAt, le.Timestamp)

	var c vstar.Component
	target, hasTarget := targets[le.TaskID]
	if hasTarget && isSupersessionEntry(le, statusDefs) {
		j, err := supersession.Supersedes(target, effectiveStatus(le, statusDefs), at)
		if err != nil {
			return vstar.Component{}, fmt.Errorf("vtodo: supersession journal for task %q: %w", le.TaskID, err)
		}
		c = j
	} else {
		// Zero DTSTART: NewJournal omits the property for the zero time.
		// A tlc journal's own instant travels as CREATED, the property
		// both the decoder and the sync spec read; carrying it a second
		// time as DTSTART would only widen the wire.
		j, err := helpers.NewJournal(logUID(le, domain), time.Time{})
		if err != nil {
			return vstar.Component{}, fmt.Errorf("vtodo: log entry for task %q: %w", le.TaskID, err)
		}
		setDTSTAMP(&j, at)
		c = j
	}
	// The sub-type is a function of the action and the exporting
	// project's status vocabulary, the same dependency STATUS has. A
	// preserved foreign supersession entry may carry no action at all;
	// it is a status journal by construction.
	concept := journalConcept(le.Action, statusDefs)
	if strings.TrimSpace(le.Action) == "" && isSupersessionEntry(le, statusDefs) {
		concept = ConceptStatus
	}
	c.Add(vstar.Property{Name: XPropConcept, Value: concept})

	if le.Note != "" {
		c.Add(vstar.Property{Name: "SUMMARY", Value: firstLine(le.Note)})
		c.Add(vstar.Property{Name: "DESCRIPTION", Value: le.Note})
	} else if le.Action != "" {
		c.Add(vstar.Property{Name: "SUMMARY", Value: le.Action})
	}
	if !le.Timestamp.IsZero() {
		c.Add(vstar.Property{Name: "CREATED", Value: vstar.FormatTime(le.Timestamp)})
	}
	if le.TaskID != "" {
		// Supersedes already wrote the edge for a ledger entry; the
		// plain shape writes the same one.
		if _, ok := c.Get("RELATED-TO"); !ok {
			addParentRelation(&c, uidFor(le.TaskID, domain))
		}
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
	finalize(&c)
	return c, nil
}

// addAssignee writes a player: ATTENDEE:mailto: when the name is an
// email address, X-TLC-ASSIGNEE otherwise, nothing for an empty name.
// Shared by the task and turn builders; a track always uses
// X-TLC-ASSIGNEE and does not go through here.
func addAssignee(c *vstar.Component, name string) {
	if name == "" {
		return
	}
	if isEmail(name) {
		c.Add(vstar.Property{Name: "ATTENDEE", Value: "mailto:" + name})
		return
	}
	c.Add(vstar.Property{Name: XPropAssignee, Value: name})
}

// addCategories writes one CATEGORIES property per tag.
//
// RFC 5545 §3.8.1.2 also allows a single property carrying a
// comma-separated list, and helpers.SetCategories emits that shape --
// but the codec escapes EVERY comma in a TEXT value, so the joined
// form reaches the wire as `CATEGORIES:security\,auth`, which any RFC
// 5545 reader parses as ONE category named "security,auth". One
// property per tag is equally legal and needs no separator, so foreign
// readers see the tags tlc meant. The decoder accepts both shapes.
//
// Tags are trimmed, empties dropped and repeats removed keeping
// first-seen order; comparison is case-sensitive (labels, not tokens),
// the same rules the decoder applies, so a round trip is stable.
func addCategories(c *vstar.Component, tags []string) {
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, dup := seen[tag]; dup {
			continue
		}
		seen[tag] = struct{}{}
		c.Add(vstar.Property{Name: "CATEGORIES", Value: tag})
	}
}

// logUID derives a stable UID for a LogEntry. Combines task ID, action,
// and timestamp so two log entries on the same task with the same action
// at different times don't collide.
func logUID(le *core.LogEntry, domain string) string {
	stamp := ""
	if !le.Timestamp.IsZero() {
		stamp = vstar.FormatTime(le.Timestamp)
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

// metaString reads a string-valued Task.Meta entry, returning "" when
// the key is absent or holds a non-string.
func metaString(meta map[string]interface{}, key string) string {
	if meta == nil {
		return ""
	}
	s, _ := meta[key].(string)
	return s
}

// blockedByList extracts the task IDs stored under Task.Meta
// ["blocked_by"], normalised through the single shared coercion in
// core so encode, decode, and the core API agree on the accepted
// shapes (string, []string, []interface{}).
func blockedByList(meta map[string]interface{}) []string {
	if meta == nil {
		return nil
	}
	return core.NormalizeBlockedBy(meta["blocked_by"])
}
