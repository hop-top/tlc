package core

import (
	"errors"
	"fmt"
	"time"

	"hop.top/vstar/rrule"
)

// supportedFreq returns true if the given FREQ is in tlc's allow-list
// for v1 reminder recurrence. vstar/rrule accepts YEARLY too (spec-vstar
// specs/v0.1/03-canonicalization.md, "RRULE parsing scope") but tlc
// rejects it: long-period yearly reminders are out of scope for the v1
// reminder UI.
func supportedFreq(f rrule.Freq) bool {
	switch f {
	case rrule.FreqHourly, rrule.FreqDaily, rrule.FreqWeekly, rrule.FreqMonthly:
		return true
	}
	return false
}

// ValidateRRule parses an RFC 5545 RRULE string. An empty string is
// valid (no recurring schedule). Only FREQ=DAILY|WEEKLY|MONTHLY|HOURLY
// are accepted along with INTERVAL, UNTIL, COUNT, BYDAY, BYMONTHDAY
// modifiers — vstar/rrule performs structural parsing, this helper
// layers an additional FREQ allow-list on top.
func ValidateRRule(rule string) error {
	if rule == "" {
		return nil
	}
	r, err := rrule.ParseRRule(rule)
	if err != nil {
		if errors.Is(err, rrule.ErrUnsupportedRRule) {
			return fmt.Errorf("unsupported feature in RRULE %q: %w", rule, err)
		}
		return fmt.Errorf("invalid RRULE %q: %w", rule, err)
	}
	if !supportedFreq(r.Freq) {
		return fmt.Errorf(
			"unsupported FREQ=%s in RRULE %q; supported: HOURLY, DAILY, WEEKLY, MONTHLY",
			r.Freq, rule,
		)
	}
	return nil
}

// NextFireFromRRule returns the next occurrence strictly after `after`,
// computed relative to `dtstart` (the rule's anchor — typically the
// task's CreatedAt).
//
// Returns:
//   - (zeroTime, false, nil) when the rule is empty (no recurrence).
//   - (t, true, nil)         when a next occurrence exists.
//   - (zeroTime, false, nil) when the rule has been exhausted (UNTIL/COUNT).
//   - (zeroTime, false, err) on parse error or unsupported FREQ.
//
// Callers stepping a recurrence forward (e.g. Task.NextReminder
// advancing cursor → cursor → cursor) MUST pass a stable dtstart
// across calls. Passing `after` as both args restarts COUNT/UNTIL
// every invocation — bounded rules then never terminate.
func NextFireFromRRule(rule string, dtstart, after time.Time) (time.Time, bool, error) {
	if rule == "" {
		return time.Time{}, false, nil
	}
	r, err := rrule.ParseRRule(rule)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("invalid RRULE %q: %w", rule, err)
	}
	if !supportedFreq(r.Freq) {
		return time.Time{}, false, fmt.Errorf(
			"unsupported FREQ=%s in RRULE %q; supported: HOURLY, DAILY, WEEKLY, MONTHLY",
			r.Freq, rule,
		)
	}
	t, ok, err := rrule.NextOccurrence(r, dtstart, after)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("rrule next: %w", err)
	}
	return t, ok, nil
}
