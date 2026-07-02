package core_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"hop.top/tlc/internal/core"
)

func TestValidateRRule(t *testing.T) {
	t.Run("empty is valid", func(t *testing.T) {
		assert.NoError(t, core.ValidateRRule(""))
	})
	t.Run("daily ok", func(t *testing.T) {
		assert.NoError(t, core.ValidateRRule("FREQ=DAILY"))
	})
	t.Run("weekly with byday ok", func(t *testing.T) {
		assert.NoError(t, core.ValidateRRule("FREQ=WEEKLY;BYDAY=MO,WE,FR"))
	})
	t.Run("hourly with interval ok", func(t *testing.T) {
		assert.NoError(t, core.ValidateRRule("FREQ=HOURLY;INTERVAL=2"))
	})
	t.Run("monthly with until ok", func(t *testing.T) {
		assert.NoError(t,
			core.ValidateRRule("FREQ=MONTHLY;UNTIL=20260601T000000Z"))
	})
	t.Run("yearly is unsupported", func(t *testing.T) {
		err := core.ValidateRRule("FREQ=YEARLY")
		assert.Error(t, err)
	})
	t.Run("garbage is invalid", func(t *testing.T) {
		err := core.ValidateRRule("not-an-rrule")
		assert.Error(t, err)
	})
	t.Run("missing freq is invalid", func(t *testing.T) {
		err := core.ValidateRRule("INTERVAL=2")
		assert.Error(t, err)
	})
}

func TestNextFireFromRRule_EmptyRule(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	next, ok, err := core.NextFireFromRRule("", now, now)
	assert.NoError(t, err)
	assert.False(t, ok)
	assert.True(t, next.IsZero())
}

func TestNextFireFromRRule_Daily(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	next, ok, err := core.NextFireFromRRule("FREQ=DAILY", now, now)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, now.AddDate(0, 0, 1), next)
}

func TestNextFireFromRRule_DailyInterval(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	next, ok, err := core.NextFireFromRRule("FREQ=DAILY;INTERVAL=3", now, now)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, now.AddDate(0, 0, 3), next)
}

func TestNextFireFromRRule_HourlyInterval(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	next, ok, err := core.NextFireFromRRule("FREQ=HOURLY;INTERVAL=2", now, now)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, now.Add(2*time.Hour), next)
}

func TestNextFireFromRRule_WeeklyByDay(t *testing.T) {
	// Friday 2026-05-01 12:00 UTC → next BYDAY=MO,WE,FR after.
	// Walk forward day-by-day, filtering by weekday: Sat (skip),
	// Sun (skip), Mon (match) → 2026-05-04 12:00 UTC.
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC) // Friday
	next, ok, err := core.NextFireFromRRule(
		"FREQ=WEEKLY;BYDAY=MO,WE,FR", now, now,
	)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC), next)
}

// TestNextFireFromRRule_WeeklyByDayCoversAllListedDays asserts that
// successive calls walk the full BYDAY set, not just the start weekday.
// Reproduces the bug where WEEKLY+BYDAY only ever fired on the start day.
func TestNextFireFromRRule_WeeklyByDayCoversAllListedDays(t *testing.T) {
	// Start on Sunday so the first three matches are Mon/Wed/Fri.
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC) // Sunday

	// 1st call: Mon 2026-05-04
	t1, ok1, err1 := core.NextFireFromRRule(
		"FREQ=WEEKLY;BYDAY=MO,WE,FR", now, now,
	)
	assert.NoError(t, err1)
	assert.True(t, ok1)
	assert.Equal(t, time.Monday, t1.Weekday())

	// 2nd call: from Monday, next is Wednesday. Anchor stays at `now`
	// (DTSTART), cursor advances per call.
	t2, ok2, err2 := core.NextFireFromRRule(
		"FREQ=WEEKLY;BYDAY=MO,WE,FR", now, t1,
	)
	assert.NoError(t, err2)
	assert.True(t, ok2)
	assert.Equal(t, time.Wednesday, t2.Weekday())

	// 3rd call: from Wednesday, next is Friday.
	t3, ok3, err3 := core.NextFireFromRRule(
		"FREQ=WEEKLY;BYDAY=MO,WE,FR", now, t2,
	)
	assert.NoError(t, err3)
	assert.True(t, ok3)
	assert.Equal(t, time.Friday, t3.Weekday())
}

func TestNextFireFromRRule_DailyByDay(t *testing.T) {
	// Friday 2026-05-01 — DAILY with BYDAY=MO,WE,FR.
	// next = Sat (skip), Sun (skip), Mon (match).
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC) // Friday
	next, ok, err := core.NextFireFromRRule(
		"FREQ=DAILY;BYDAY=MO,WE,FR", now, now,
	)
	assert.NoError(t, err)
	assert.True(t, ok)
	expected := now.AddDate(0, 0, 3) // Monday
	assert.Equal(t, expected, next)
}

// TestNextFireFromRRule_MonthlyByMonthDay asserts BYMONTHDAY actually
// constrains when the rule fires. Bug: the helper accepted MONTHLY+
// BYMONTHDAY=1 as valid but ignored the filter, so it fired on
// whatever day-of-month the reference time had.
func TestNextFireFromRRule_MonthlyByMonthDay(t *testing.T) {
	// Reference 2026-05-15 → next FREQ=MONTHLY;BYMONTHDAY=1 is 2026-06-01.
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	next, ok, err := core.NextFireFromRRule(
		"FREQ=MONTHLY;BYMONTHDAY=1", now, now,
	)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC), next)
}

// TestNextFireFromRRule_MonthlyByMonthDayLastDay covers the negative
// offset shape: BYMONTHDAY=-1 means "last day of month".
func TestNextFireFromRRule_MonthlyByMonthDayLastDay(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	next, ok, err := core.NextFireFromRRule(
		"FREQ=MONTHLY;BYMONTHDAY=-1", now, now,
	)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC), next)
}

func TestNextFireFromRRule_UntilTermination(t *testing.T) {
	// UNTIL is in the past relative to `after` → exhausted.
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	rule := "FREQ=DAILY;UNTIL=20260505T000000Z"
	next, ok, err := core.NextFireFromRRule(rule, now, now)
	assert.NoError(t, err)
	assert.False(t, ok)
	assert.True(t, next.IsZero())
}

func TestNextFireFromRRule_UntilWithinReach(t *testing.T) {
	// UNTIL is after the next fire, so we still get one.
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	rule := "FREQ=DAILY;UNTIL=20260505T000000Z"
	next, ok, err := core.NextFireFromRRule(rule, now, now)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, now.AddDate(0, 0, 1), next)
}

func TestNextFireFromRRule_CountTermination(t *testing.T) {
	// Per RFC 5545 §3.3.10, COUNT includes DTSTART. Calling with
	// dtstart == cursor == now: COUNT=1 ⇒ the single occurrence is
	// DTSTART itself ⇒ no occurrence "strictly after" `now` ⇒
	// ok=false. COUNT=2 ⇒ DTSTART + one fire ⇒ ok=true.
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	_, ok, err := core.NextFireFromRRule("FREQ=DAILY;COUNT=1", now, now)
	assert.NoError(t, err)
	assert.False(t, ok)

	next, ok, err := core.NextFireFromRRule("FREQ=DAILY;COUNT=2", now, now)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, now.AddDate(0, 0, 1), next)
}

// TestNextFireFromRRule_CountAcrossSteppedCalls reproduces the bug
// reviewer flagged on PR #100: when the caller anchors DTSTART once
// and steps the cursor forward (e.g. Task.NextReminder), COUNT must
// terminate the rule. Old impl with dtstart==cursor restarted COUNT
// every call, so bounded rules fired forever.
func TestNextFireFromRRule_CountAcrossSteppedCalls(t *testing.T) {
	dtstart := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	rule := "FREQ=DAILY;COUNT=2"

	// 1st call: cursor = dtstart. DTSTART is occurrence #1 (excluded
	// by "strictly after"); occurrence #2 is dtstart+1d.
	t1, ok, err := core.NextFireFromRRule(rule, dtstart, dtstart)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, dtstart.AddDate(0, 0, 1), t1)

	// 2nd call: cursor = t1. COUNT is exhausted (occurrences #1+#2
	// already consumed); no fire strictly after t1.
	_, ok2, err := core.NextFireFromRRule(rule, dtstart, t1)
	assert.NoError(t, err)
	assert.False(t, ok2, "COUNT=2 must terminate after second occurrence")
}

func TestNextFireFromRRule_Monthly(t *testing.T) {
	now := time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC)
	next, ok, err := core.NextFireFromRRule("FREQ=MONTHLY", now, now)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, now.AddDate(0, 1, 0), next)
}

func TestNextFireFromRRule_Garbage(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	_, ok, err := core.NextFireFromRRule("garbage", now, now)
	assert.Error(t, err)
	assert.False(t, ok)
}

func TestNextFireFromRRule_UnsupportedFreq(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	_, ok, err := core.NextFireFromRRule("FREQ=YEARLY", now, now)
	assert.Error(t, err)
	assert.False(t, ok)
}
