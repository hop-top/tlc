package vtodo

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	vstar "hop.top/vstar"
	"hop.top/vstar/duration"
	"hop.top/vstar/helpers"

	"hop.top/tlc/internal/core"
)

// MetaDueDateOnly is the Meta key that marks DueAt as a calendar date
// rather than an instant: the task (or track) is due ON that day, and
// the wire form is DUE;VALUE=DATE:YYYYMMDD (RFC 5545 §3.3.4).
//
// The model has one DueAt *time.Time, so a date-only DUE is stored as
// midnight UTC of that day; that is what every consumer of DueAt (the
// overdue check, the auto reminder) already treats a midnight instant
// as. The key is what keeps the distinction spec-vstar 03 rule 11
// insists on: on export a flagged task writes VALUE=DATE again, never
// a DATE-TIME promoted to midnight. A Meta key rather than a column
// because Meta already round-trips through every store, and because
// the flag is a fact about the wire form, not about scheduling.
//
// Derived (see derivedMetaKeys): the VALUE=DATE parameter carries it.
const MetaDueDateOnly = "due_date_only"

// MetaReminders is the Meta key holding a task's reminders beyond the
// one in Task.RemindAt, as a list of RFC 3339 UTC instants, sorted.
//
// A VTODO may carry any number of VALARMs (Apple Reminders and
// Thunderbird write several) and the model has one RemindAt. On
// import every VALARM that resolves to an instant is kept: the
// earliest one still in the future becomes RemindAt (the reminder tlc
// will fire next), every other one lands here so re-export writes it
// back as its own VALARM instead of dropping it. When none is in the
// future the earliest is RemindAt, so a calendar of past reminders
// still decodes to the same model tlc wrote.
//
// Derived (see derivedMetaKeys): the VALARMs carry it.
const MetaReminders = "reminders"

// autoRemindOffset is the relative trigger tlc emits for
// Task.AutoRemindAt: 12 hours before DUE, RELATED=END. The offset is
// the one internal/core derives; TestAutoRemind_OffsetMatchesCore
// pins the two together.
var autoRemindOffset = duration.Duration{Negative: true, Hours: 12}

// alarmAction is the ACTION every tlc VALARM carries.
const alarmAction = "DISPLAY"

// isAutoRemindTrigger reports whether tr is the wire form of the
// derived auto reminder: relative, anchored to the end (DUE on a
// VTODO), the same offset AutoRemindAt applies. Compared by signed
// length so an authored -P0DT12H reads as the same reminder; a
// relative trigger from DTSTART, or an absolute instant that merely
// happens to fall 12 hours before DUE, is a reminder someone set and
// is kept.
func isAutoRemindTrigger(tr duration.Trigger) bool {
	return tr.Relative &&
		tr.Related == duration.RelatedEnd &&
		tr.Duration.Signed() == autoRemindOffset.Signed()
}

// alarmInstant resolves the instant at which alarm fires against its
// parent VTODO. The library resolves a relative trigger against the
// DATE-TIME anchor its RELATED parameter selects (DTSTART for START,
// DUE for END on a VTODO); when that anchor is a date-only value the
// library reports ErrNoAnchor, its Resolve refusing to promote a
// DATE, and the offset is applied here to the date's midnight UTC,
// the same reading DueAt gets. Returns ok=false for a VALARM without a
// TRIGGER, a malformed one, or one whose anchor the parent lacks.
func alarmInstant(tr duration.Trigger, parent vstar.Component) (time.Time, bool) {
	at, err := tr.Resolve(parent, vstar.Calendar{})
	if err == nil {
		return at, true
	}
	if !errors.Is(err, duration.ErrNoAnchor) {
		return time.Time{}, false
	}
	var d vstar.Date
	var ok bool
	if tr.Related == duration.RelatedEnd {
		d, ok = parent.DUEDate()
	} else {
		d, ok = parent.DTSTARTDate()
	}
	if !ok {
		return time.Time{}, false
	}
	return tr.Duration.AddTo(d.Time()), true
}

// decodeReminders reads every VALARM under todo into the task: the
// instants of all resolvable triggers except the derived auto
// reminder, deduplicated and sorted, split between RemindAt and
// Meta[MetaReminders] as MetaReminders describes. now decides which
// reminder is the next one.
func decodeReminders(t *core.Task, todo vstar.Component, now time.Time) {
	var instants []time.Time
	for _, alarm := range todo.Sub {
		if alarm.Type != vstar.CompAlarm {
			continue
		}
		tr, err := duration.AlarmTrigger(alarm)
		if err != nil {
			continue
		}
		if isAutoRemindTrigger(tr) {
			// AutoRemindAt recomputes this one from DUE; storing it
			// as well would fire it twice and double it on re-export.
			continue
		}
		at, ok := alarmInstant(tr, todo)
		if !ok {
			continue
		}
		instants = append(instants, at.UTC())
	}
	instants = sortedUniqueInstants(instants)
	if len(instants) == 0 {
		return
	}
	next := 0
	for i, at := range instants {
		if at.After(now) {
			next = i
			break
		}
	}
	remind := instants[next]
	t.RemindAt = &remind
	rest := append(append([]time.Time{}, instants[:next]...), instants[next+1:]...)
	if len(rest) == 0 {
		return
	}
	list := make([]string, 0, len(rest))
	for _, at := range rest {
		list = append(list, at.Format(time.RFC3339))
	}
	setMeta(t, MetaReminders, list)
}

// absoluteReminders is the set of instants a task's own reminders
// fire at: RemindAt and Meta[MetaReminders], deduplicated and sorted.
// The auto reminder is not among them; it is relative to DUE and has
// its own builder.
func absoluteReminders(t *core.Task) []time.Time {
	var out []time.Time
	if t.RemindAt != nil && !t.RemindAt.IsZero() {
		out = append(out, t.RemindAt.UTC())
	}
	for _, s := range metaStringList(t.Meta, MetaReminders) {
		if at, err := time.Parse(time.RFC3339, strings.TrimSpace(s)); err == nil {
			out = append(out, at.UTC())
		}
	}
	return sortedUniqueInstants(out)
}

// addReminderAlarms appends the task's VALARMs to c: one absolute
// alarm per instant in absoluteReminders, in time order, then the
// relative auto reminder when the task derives one. stamp is the
// parent's DTSTAMP, which every alarm takes.
func addReminderAlarms(c *vstar.Component, t *core.Task, stamp time.Time) error {
	for _, at := range absoluteReminders(t) {
		uid := alarmUID(c.UID())
		if t.RemindAt == nil || !at.Equal(t.RemindAt.UTC()) {
			uid = alarmSeriesUID(c.UID(), at)
		}
		alarm, err := buildAlarmComponent(uid, at, t.Title, stamp)
		if err != nil {
			return err
		}
		c.Sub = append(c.Sub, alarm)
	}
	if t.AutoRemindAt() != nil {
		alarm, err := buildAutoRemindComponent(c.UID(), t.Title, stamp)
		if err != nil {
			return err
		}
		c.Sub = append(c.Sub, alarm)
	}
	return nil
}

// buildAlarmComponent emits a VALARM carrying an absolute trigger,
// through helpers.NewAbsoluteAlarm: the property is written as
// TRIGGER;VALUE=DATE-TIME:<UTC form #2> with no RELATED, the shape
// spec-vstar 03 "TRIGGER conventions" asks emitters for. stamp is the
// parent's DTSTAMP: a reminder has no life of its own, it changes when
// its task does.
func buildAlarmComponent(uid string, remindAt time.Time, summary string, stamp time.Time) (vstar.Component, error) {
	c, err := helpers.NewAbsoluteAlarm(uid, alarmAction, remindAt.UTC())
	if err != nil {
		return vstar.Component{}, fmt.Errorf("vtodo: alarm %q: %w", uid, err)
	}
	finishAlarm(&c, summary, stamp)
	return c, nil
}

// buildAutoRemindComponent emits the derived reminder as a relative
// VALARM, TRIGGER;RELATED=END:-PT12H, through helpers.NewRelativeAlarm.
// "Twelve hours before it is due" is what AutoRemindAt means, and the
// relative form says so to every reader without pinning an instant
// that moves whenever DUE does; the decoder recognises the shape and
// lets AutoRemindAt derive it again rather than storing it.
func buildAutoRemindComponent(parentUID, summary string, stamp time.Time) (vstar.Component, error) {
	c, err := helpers.NewRelativeAlarm(autoRemindUID(parentUID), alarmAction, autoRemindOffset, duration.RelatedEnd)
	if err != nil {
		return vstar.Component{}, fmt.Errorf("vtodo: auto reminder for %q: %w", parentUID, err)
	}
	finishAlarm(&c, summary, stamp)
	return c, nil
}

// finishAlarm applies what every tlc VALARM shares after its
// constructor: the parent's DTSTAMP, the DESCRIPTION a DISPLAY alarm
// needs (RFC 5545 §3.6.6; the task title), and the hash last.
func finishAlarm(c *vstar.Component, summary string, stamp time.Time) {
	setDTSTAMP(c, stamp)
	if summary != "" {
		c.Add(vstar.Property{Name: "DESCRIPTION", Value: summary})
	}
	finalize(c)
}

// alarmUID derives the UID of a task's primary reminder (RemindAt)
// from its parent's: the parent UID with "-alarm" inserted before the
// domain separator, so task_<id>@tlc.local reminds through
// task_<id>-alarm@tlc.local. A foreign parent UID without '@' takes
// the suffix at its end. Deterministic across exports and free of
// stored state; alarmSeriesUID and autoRemindUID extend the scheme to
// the other reminders a task can carry.
func alarmUID(parentUID string) string {
	return alarmSuffixUID(parentUID, "-alarm")
}

// alarmSeriesUID names a further absolute reminder by its instant:
// task_<id>-alarm-<UTC form #2>@<domain>. Instants are unique within
// a task (absoluteReminders deduplicates), so the UID is.
func alarmSeriesUID(parentUID string, at time.Time) string {
	return alarmSuffixUID(parentUID, "-alarm-"+vstar.FormatTime(at.UTC()))
}

// autoRemindUID names the derived reminder: task_<id>-alarm-auto@<domain>.
func autoRemindUID(parentUID string) string {
	return alarmSuffixUID(parentUID, "-alarm-auto")
}

func alarmSuffixUID(parentUID, suffix string) string {
	if i := strings.LastIndexByte(parentUID, '@'); i >= 0 {
		return parentUID[:i] + suffix + parentUID[i:]
	}
	return parentUID + suffix
}

// sortedUniqueInstants sorts ascending and drops repeats, comparing
// by instant so two spellings of one moment collapse.
func sortedUniqueInstants(in []time.Time) []time.Time {
	if len(in) == 0 {
		return nil
	}
	out := append([]time.Time{}, in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	uniq := out[:1]
	for _, at := range out[1:] {
		if !at.Equal(uniq[len(uniq)-1]) {
			uniq = append(uniq, at)
		}
	}
	return uniq
}

// dueDateOnly reports whether meta flags DueAt as a calendar date.
// The value is written as a bool; a string spelling survives a store
// that widened it.
func dueDateOnly(meta map[string]interface{}) bool {
	if meta == nil {
		return false
	}
	switch v := meta[MetaDueDateOnly].(type) {
	case bool:
		return v
	case string:
		return isTrueValue(v)
	}
	return false
}

// newTodoWithDue seeds a VTODO through helpers.NewTodo and writes DUE
// in the form the entity asks for: DUE;VALUE=DATE:YYYYMMDD through
// SetDUEDate when dateOnly, the UTC DATE-TIME NewTodo writes
// otherwise, nothing for a nil due.
func newTodoWithDue(uid string, due *time.Time, dateOnly bool) (vstar.Component, error) {
	if due == nil || due.IsZero() {
		return helpers.NewTodo(uid, time.Time{})
	}
	if !dateOnly {
		return helpers.NewTodo(uid, *due)
	}
	c, err := helpers.NewTodo(uid, time.Time{})
	if err != nil {
		return vstar.Component{}, err
	}
	c.SetDUEDate(vstar.DateOf(due.UTC()))
	return c, nil
}

// decodeDue reads DUE from a VTODO: a VALUE=DATE value through the
// library's date accessor, as midnight UTC of that day plus the
// date-only flag; a DATE-TIME through the instant accessor. An
// untagged eight-octet value is a malformed DATE-TIME to both and
// decodes to nothing, which is the library's posture and spec-vstar
// 03 rule 11's: a DATE is never promoted, and a DATE-TIME is never
// guessed.
func decodeDue(todo vstar.Component) (*time.Time, bool) {
	if todo.IsDateOnly("DUE") {
		d, ok := todo.DUEDate()
		if !ok {
			return nil, false
		}
		at := d.Time()
		return &at, true
	}
	if at, ok := todo.DUE(vstar.Calendar{}); ok {
		return &at, false
	}
	return nil, false
}

// metaStringList reads a list-valued Meta entry in either shape a
// store hands back: []string as written, or the []interface{} JSON
// decoding produces. Non-string items are skipped.
func metaStringList(meta map[string]interface{}, key string) []string {
	if meta == nil {
		return nil
	}
	switch v := meta[key].(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
