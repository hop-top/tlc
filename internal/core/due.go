package core

import "time"

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
func (t *Task) NextReminder() *time.Time {
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
		// Walk from the task's anchor (CreatedAt) so the cadence is
		// preserved across reminder ticks — a task created at xx:17
		// with FREQ=HOURLY fires at xx:17 every hour, not at "1h
		// from whenever NextReminder happens to be called".
		//
		// We step the rule forward until we land strictly after
		// `now`. UNTIL/COUNT constraints inside the rule still apply.
		anchor := t.CreatedAt
		if anchor.IsZero() {
			anchor = now
		}
		cursor := anchor
		const maxSteps = 10000
		for i := 0; i < maxSteps; i++ {
			next, ok, _ := NextFireFromRRule(t.RRule, cursor)
			if !ok {
				break
			}
			if next.After(now) {
				candidate := next
				if t.DueAt == nil || candidate.Before(*t.DueAt) {
					if earliest == nil || candidate.Before(*earliest) {
						earliest = &candidate
					}
				}
				break
			}
			cursor = next
		}
	}

	return earliest
}
