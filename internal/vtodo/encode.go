package vtodo

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	vstar "hop.top/vstar"
	"hop.top/vstar/codec/rfc5545"
	"hop.top/vstar/helpers"

	"hop.top/tlc/internal/config"
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
	statusDefs := o.statusDefinitions()

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
		cal.Append(buildTrackComponent(tr, tasksByTrack[tr.ID], o.uidDomain, o.exportTime))
	}

	// Tasks → VTODO.
	for _, t := range tasks {
		if t == nil {
			continue
		}
		cal.Append(buildTaskComponent(t, o.uidDomain, o.priorities, statusDefs, o.exportTime))
	}

	// LogEntries → VJOURNAL (gated).
	if o.includeLogs {
		for _, le := range logs {
			if le == nil {
				continue
			}
			cal.Append(buildLogComponent(le, o.uidDomain, o.exportTime))
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

func buildTaskComponent(
	t *core.Task,
	domain string,
	defs []config.PriorityDefinition,
	statusDefs []config.StatusDefinition,
	exportAt time.Time,
) vstar.Component {
	c := vstar.Component{Type: vstar.CompTodo}
	c.Add(vstar.Property{Name: "UID", Value: uidFor(t.ID, domain)})

	if t.Title != "" {
		c.Add(vstar.Property{Name: "SUMMARY", Value: t.Title})
	}
	if t.Description != "" {
		c.Add(vstar.Property{Name: "DESCRIPTION", Value: t.Description})
	}
	c.Add(vstar.Property{Name: "STATUS", Value: statusToWire(t.Status, statusDefs)})
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
	// DTSTAMP is when this calendar instance was created (RFC 5545
	// §3.8.7.2) — i.e. export time, not entity creation time.
	c.Add(vstar.Property{Name: "DTSTAMP", Value: vstar.FormatTime(exportAt)})
	if !t.UpdatedAt.IsZero() {
		c.Add(vstar.Property{Name: "LAST-MODIFIED", Value: vstar.FormatTime(t.UpdatedAt)})
	}
	if t.DueAt != nil && !t.DueAt.IsZero() {
		c.SetDUE(*t.DueAt)
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
		helpers.AddRelatedTo(&c, uidFor(*t.TrackID, domain), RelTypeParent)
	}
	for _, blocker := range blockedByList(t.Meta) {
		helpers.AddRelatedTo(&c, uidFor(blocker, domain), RelTypeDependsOn)
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
	if t.Archived {
		c.Add(vstar.Property{Name: XPropArchived, Value: "TRUE"})
	}
	addMeta(&c, t.Meta)
	addUnknownTLCProps(&c, t.Meta)
	// CATEGORIES last, and via the helper rather than one Add per tag:
	// RFC 5545 §3.8.1.2 models categories as ONE property holding a
	// comma-separated value list, which is what SetCategories emits
	// (deduped, first-seen order preserved, case-sensitive).
	//
	// Position matters. Every helpers mutator refreshes X-VSTAR-HASH as
	// its last step, and that hash covers the component AS IT STANDS.
	// Called mid-build it would pin a digest of a half-populated VTODO
	// -- self-inconsistent the moment CREATED or DUE lands after it.
	// Called here it covers the finished component, so the emitted hash
	// verifies.
	helpers.SetCategories(&c, t.Tags)
	return c
}

func buildTrackComponent(tr *core.Track, members []*core.Task, domain string, exportAt time.Time) vstar.Component {
	c := vstar.Component{Type: vstar.CompTodo}
	c.Add(vstar.Property{Name: "UID", Value: uidFor(tr.ID, domain)})

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

	if !tr.CreatedAt.IsZero() {
		c.Add(vstar.Property{Name: "CREATED", Value: vstar.FormatTime(tr.CreatedAt)})
	}
	c.Add(vstar.Property{Name: "DTSTAMP", Value: vstar.FormatTime(exportAt)})
	if !tr.UpdatedAt.IsZero() {
		c.Add(vstar.Property{Name: "LAST-MODIFIED", Value: vstar.FormatTime(tr.UpdatedAt)})
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
		helpers.AddRelatedTo(&c, uidFor(member.ID, domain), RelTypeChild)
	}
	addMeta(&c, tr.Meta)
	addUnknownTLCProps(&c, tr.Meta)
	return c
}

func buildLogComponent(le *core.LogEntry, domain string, exportAt time.Time) vstar.Component {
	c := vstar.Component{Type: vstar.CompJournal}
	c.Add(vstar.Property{Name: "UID", Value: logUID(le, domain)})

	if le.Note != "" {
		c.Add(vstar.Property{Name: "SUMMARY", Value: firstLine(le.Note)})
		c.Add(vstar.Property{Name: "DESCRIPTION", Value: le.Note})
	} else if le.Action != "" {
		c.Add(vstar.Property{Name: "SUMMARY", Value: le.Action})
	}
	c.Add(vstar.Property{Name: "DTSTAMP", Value: vstar.FormatTime(exportAt)})
	if !le.Timestamp.IsZero() {
		c.Add(vstar.Property{Name: "CREATED", Value: vstar.FormatTime(le.Timestamp)})
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
		Value:  vstar.FormatTime(remindAt),
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
