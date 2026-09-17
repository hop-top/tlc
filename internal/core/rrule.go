package core

import (
	"errors"
	"fmt"
	"time"

	"hop.top/vstar/rrule"
)

// ValidateRRule parses an RFC 5545 RRULE string. An empty string is
// valid (no recurring schedule). Every FREQ the library evaluates is
// accepted — HOURLY, DAILY, WEEKLY, MONTHLY and YEARLY; SECONDLY and
// MINUTELY are outside the library's scope and come back as
// rrule.ErrUnsupportedRRule. tlc used to layer its own FREQ allow-list
// on top that also rejected YEARLY; with a bounded expansion API a
// yearly rule is no more expensive to evaluate than a daily one, so
// the gate is gone and vtodo import keeps yearly rules too.
func ValidateRRule(rule string) error {
	if rule == "" {
		return nil
	}
	if _, err := parseRRule(rule); err != nil {
		return err
	}
	return nil
}

// parseRRule wraps rrule.ParseRRule with the rule text in the message.
// Library sentinels stay reachable through errors.Is.
func parseRRule(rule string) (rrule.Rule, error) {
	r, err := rrule.ParseRRule(rule)
	if err != nil {
		if errors.Is(err, rrule.ErrUnsupportedRRule) {
			return rrule.Rule{}, fmt.Errorf("unsupported feature in RRULE %q: %w", rule, err)
		}
		return rrule.Rule{}, fmt.Errorf("invalid RRULE %q: %w", rule, err)
	}
	return r, nil
}

// NextFireFromRRule returns the next occurrence strictly after `after`,
// computed relative to `dtstart` (the rule's anchor — typically the
// task's CreatedAt).
//
// Returns:
//   - (zeroTime, false, nil) when the rule is empty (no recurrence).
//   - (t, true, nil)         when a next occurrence exists.
//   - (zeroTime, false, nil) when the rule has been exhausted (UNTIL/COUNT).
//   - (zeroTime, false, err) on parse error, unsupported feature, or
//     when the evaluator gave up on a rule that never fires (an
//     unsatisfiable BY-* combination); the last case wraps
//     rrule.ErrIterationCap and callers MUST NOT read it as "series
//     ended".
//
// Callers stepping a recurrence forward MUST pass a stable dtstart
// across calls. Passing `after` as both args restarts COUNT/UNTIL
// every invocation — bounded rules then never terminate.
func NextFireFromRRule(rule string, dtstart, after time.Time) (time.Time, bool, error) {
	if rule == "" {
		return time.Time{}, false, nil
	}
	r, err := parseRRule(rule)
	if err != nil {
		return time.Time{}, false, err
	}
	t, ok, err := rrule.NextOccurrence(r, dtstart, after)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("RRULE %q: %w", rule, err)
	}
	return t, ok, nil
}
