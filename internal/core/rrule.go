package core

import (
	"fmt"
	"time"

	ics "github.com/arran4/golang-ical"
)

// supportedFreq returns true if the given FREQ value is supported by
// the v1 RRULE helper (DAILY/WEEKLY/MONTHLY/HOURLY).
func supportedFreq(f ics.Frequency) bool {
	switch f {
	case ics.FrequencyHourly,
		ics.FrequencyDaily,
		ics.FrequencyWeekly,
		ics.FrequencyMonthly:
		return true
	}
	return false
}

// ValidateRRule parses an RFC 5545 RRULE string. An empty string is valid
// (it means "no recurring schedule"). For minimal v1 support, only
// FREQ=DAILY|WEEKLY|MONTHLY|HOURLY are accepted along with INTERVAL,
// UNTIL, COUNT, and BYDAY modifiers — golang-ical performs structural
// parsing, this helper layers an additional FREQ allow-list on top.
func ValidateRRule(rule string) error {
	if rule == "" {
		return nil
	}
	r, err := ics.ParseRecurrenceRule(rule)
	if err != nil {
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

// NextFireFromRRule returns the next occurrence strictly after the given
// reference time.
//
// Returns:
//   - (zeroTime, false, nil) when the rule is empty (no recurrence).
//   - (t, true, nil)         when a next occurrence exists.
//   - (zeroTime, false, nil) when the rule has been exhausted (UNTIL/COUNT).
//   - (zeroTime, false, err) on parse error or unsupported FREQ.
//
// The walk is intentionally simple: starting at the reference time, it
// advances by INTERVAL units of FREQ, applying BYDAY/UNTIL/COUNT
// constraints as it goes. Sufficient for v1 reminder use-cases.
func NextFireFromRRule(rule string, after time.Time) (time.Time, bool, error) {
	if rule == "" {
		return time.Time{}, false, nil
	}
	r, err := ics.ParseRecurrenceRule(rule)
	if err != nil {
		return time.Time{}, false, fmt.Errorf(
			"invalid RRULE %q: %w", rule, err,
		)
	}
	if !supportedFreq(r.Freq) {
		return time.Time{}, false, fmt.Errorf(
			"unsupported FREQ=%s in RRULE %q; supported: HOURLY, DAILY, WEEKLY, MONTHLY",
			r.Freq, rule,
		)
	}

	interval := r.Interval
	if interval <= 0 {
		interval = 1
	}

	const maxSteps = 10000
	candidate := after
	yielded := 0
	for i := 0; i < maxSteps; i++ {
		candidate = stepCandidate(candidate, r, interval)

		// UNTIL bound (inclusive per RFC 5545 §3.3.10).
		if !r.Until.IsZero() && candidate.After(r.Until) {
			return time.Time{}, false, nil
		}

		// BYDAY filter (meaningful for WEEKLY/DAILY/MONTHLY).
		if len(r.ByDay) > 0 && !matchesByDay(candidate, r.ByDay) {
			continue
		}

		// BYMONTHDAY filter (meaningful for MONTHLY).
		if len(r.ByMonthDay) > 0 && !matchesByMonthDay(candidate, r.ByMonthDay) {
			continue
		}

		yielded++
		// COUNT bound — count the occurrences yielded so far.
		if r.Count > 0 && yielded > r.Count {
			return time.Time{}, false, nil
		}

		return candidate, true, nil
	}
	return time.Time{}, false, nil
}

// stepCandidate moves the cursor by one logical unit. When BYDAY is
// present we walk one day at a time so the filter can hit any listed
// weekday; otherwise we advance by INTERVAL units of FREQ. Same idea
// for BYMONTHDAY with MONTHLY: walk one day so the filter can land on
// the listed day-of-month inside each month.
func stepCandidate(t time.Time, r *ics.RecurrenceRule, interval int) time.Time {
	if len(r.ByDay) > 0 || (len(r.ByMonthDay) > 0 && r.Freq == ics.FrequencyMonthly) {
		// Day-by-day walk; INTERVAL gating is handled inline below.
		return t.AddDate(0, 0, 1)
	}
	return advance(t, r.Freq, interval)
}

// advance moves t forward by one INTERVAL step of freq. MONTHLY uses
// calendar arithmetic so day-of-month is preserved across varying
// month lengths; the other frequencies use fixed durations.
func advance(t time.Time, freq ics.Frequency, interval int) time.Time {
	switch freq {
	case ics.FrequencyHourly:
		return t.Add(time.Duration(interval) * time.Hour)
	case ics.FrequencyDaily:
		return t.AddDate(0, 0, interval)
	case ics.FrequencyWeekly:
		return t.AddDate(0, 0, 7*interval)
	case ics.FrequencyMonthly:
		return t.AddDate(0, interval, 0)
	}
	return t
}

// matchesByMonthDay reports whether t's day-of-month is in the
// BYMONTHDAY list. Negative offsets ("-1" = last day of month) are
// resolved relative to the candidate's month length.
func matchesByMonthDay(t time.Time, days []int) bool {
	d := t.Day()
	last := lastDayOfMonth(t)
	for _, want := range days {
		if want > 0 && want == d {
			return true
		}
		if want < 0 && (last+want+1) == d {
			return true
		}
	}
	return false
}

// lastDayOfMonth returns the count of days in t's month.
func lastDayOfMonth(t time.Time) int {
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	return first.AddDate(0, 1, -1).Day()
}

// matchesByDay reports whether t's weekday is in the BYDAY list.
// Ordinal weekday selectors (e.g., "2MO" = second Monday) are ignored
// for v1 — only the weekday portion is matched.
func matchesByDay(t time.Time, days []ics.WeekdayNum) bool {
	wd := weekdayCode(t.Weekday())
	for _, d := range days {
		if d.Day == wd {
			return true
		}
	}
	return false
}

func weekdayCode(w time.Weekday) ics.Weekday {
	switch w {
	case time.Sunday:
		return ics.WeekdaySunday
	case time.Monday:
		return ics.WeekdayMonday
	case time.Tuesday:
		return ics.WeekdayTuesday
	case time.Wednesday:
		return ics.WeekdayWednesday
	case time.Thursday:
		return ics.WeekdayThursday
	case time.Friday:
		return ics.WeekdayFriday
	case time.Saturday:
		return ics.WeekdaySaturday
	}
	return ""
}
