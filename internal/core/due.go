package core

import (
	"fmt"
	"time"

	"hop.top/vstar/rrule"
)

// ReminderHorizon bounds how far ahead NextReminder looks for the
// next occurrence of a recurring rule. A rule with neither UNTIL nor
// COUNT is infinite, so some window is required to expand it at all;
// one year is chosen because the reminder view is a near-term nudge
// list (`tlc task remind` renders relative times) and an occurrence
// more than a year out is not an actionable "next". A recurring task
// whose next fire lies beyond the horizon therefore reports no
// upcoming reminder rather than a date a year or more away.
const ReminderHorizon = 365 * 24 * time.Hour

// IsOverdue reports whether the task has passed its due date.
func (t *Task) IsOverdue() bool {
	if t.DueAt == nil {
		return false
	}
	return time.Now().After(*t.DueAt)
}

// DueIn returns how long until the task is due, or nil if no due date.
func (t *Task) DueIn() *time.Duration {
	if t.DueAt == nil {
		return nil
	}
	d := time.Until(*t.DueAt)
	return &d
}

// AutoRemindAt returns 12h before due. Nil if no due date or
// NoAutoRemind is set.
func (t *Task) AutoRemindAt() *time.Time {
	if t.DueAt == nil || t.NoAutoRemind {
		return nil
	}
	r := t.DueAt.Add(-12 * time.Hour)
	return &r
}

// NextReminder returns the earliest upcoming reminder time.
// Considers RemindAt, AutoRemindAt, and RRule recurrence.
//
// A nil result with a nil error means no reminder is due within
// ReminderHorizon. A non-nil error means the RRULE could not be
// evaluated — it fails to parse, or it never fires (an unsatisfiable
// BY-* combination such as FREQ=YEARLY;BYMONTH=2;BYMONTHDAY=30, which
// surfaces as a wrapped rrule.ErrIterationCap). Callers must not
// collapse the error into "no reminder": the two used to be
// indistinguishable, and a task carrying a broken rule silently
// vanished from the reminder list.
func (t *Task) NextReminder() (*time.Time, error) {
	now := time.Now()
	var earliest *time.Time

	if t.RemindAt != nil && t.RemindAt.After(now) {
		earliest = t.RemindAt
	}

	if ar := t.AutoRemindAt(); ar != nil && ar.After(now) {
		if earliest == nil || ar.Before(*earliest) {
			earliest = ar
		}
	}

	if t.RRule != "" {
		next, err := t.nextRRuleFire(now)
		if err != nil {
			return nil, err
		}
		if next != nil && (earliest == nil || next.Before(*earliest)) {
			earliest = next
		}
	}

	return earliest, nil
}

// nextRRuleFire expands the task's RRULE from its anchor (CreatedAt)
// over [now, now+ReminderHorizon) and returns the first occurrence
// strictly after now and before DueAt, or nil when the window holds
// none. Expanding from the anchor preserves the cadence across
// reminder ticks — a task created at xx:17 with FREQ=HOURLY fires at
// xx:17 every hour, not at "1h from whenever NextReminder happens to
// be called". UNTIL/COUNT inside the rule still apply.
func (t *Task) nextRRuleFire(now time.Time) (*time.Time, error) {
	r, err := parseRRule(t.RRule)
	if err != nil {
		return nil, err
	}
	anchor := t.CreatedAt
	if anchor.IsZero() {
		anchor = now
	}
	occs, err := rrule.Between(r, anchor, now, now.Add(ReminderHorizon))
	if err != nil {
		return nil, fmt.Errorf("RRULE %q: %w", t.RRule, err)
	}
	for _, occ := range occs {
		if !occ.After(now) {
			continue
		}
		if t.DueAt != nil && !occ.Before(*t.DueAt) {
			return nil, nil
		}
		return &occ, nil
	}
	return nil, nil
}
