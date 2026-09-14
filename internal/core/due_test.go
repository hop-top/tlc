package core_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hop.top/tlc/internal/core"
	"hop.top/vstar/rrule"
)

func TestIsOverdue(t *testing.T) {
	tests := []struct {
		name     string
		dueAt    *time.Time
		expected bool
	}{
		{"nil due", nil, false},
		{"future", ptr(time.Now().Add(time.Hour)), false},
		{"past", ptr(time.Now().Add(-time.Hour)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &core.Task{DueAt: tt.dueAt}
			assert.Equal(t, tt.expected, task.IsOverdue())
		})
	}
}

func TestDueIn(t *testing.T) {
	t.Run("nil due", func(t *testing.T) {
		task := &core.Task{}
		assert.Nil(t, task.DueIn())
	})
	t.Run("future", func(t *testing.T) {
		task := &core.Task{DueAt: ptr(time.Now().Add(time.Hour))}
		d := task.DueIn()
		assert.NotNil(t, d)
		assert.True(t, *d > 0, "should be positive for future due")
	})
	t.Run("past", func(t *testing.T) {
		task := &core.Task{DueAt: ptr(time.Now().Add(-time.Hour))}
		d := task.DueIn()
		assert.NotNil(t, d)
		assert.True(t, *d < 0, "should be negative for past due")
	})
}

func TestAutoRemindAt(t *testing.T) {
	due := time.Date(2026, 5, 1, 14, 0, 0, 0, time.UTC)

	t.Run("12h before due", func(t *testing.T) {
		task := &core.Task{DueAt: &due}
		ar := task.AutoRemindAt()
		assert.NotNil(t, ar)
		assert.Equal(t, due.Add(-12*time.Hour), *ar)
	})
	t.Run("nil due", func(t *testing.T) {
		task := &core.Task{}
		assert.Nil(t, task.AutoRemindAt())
	})
	t.Run("no-auto-remind suppresses", func(t *testing.T) {
		task := &core.Task{DueAt: &due, NoAutoRemind: true}
		assert.Nil(t, task.AutoRemindAt())
	})
}

func TestNextReminder(t *testing.T) {
	now := time.Now()
	future := now.Add(24 * time.Hour)
	farFuture := now.Add(48 * time.Hour)

	t.Run("no reminders", func(t *testing.T) {
		task := &core.Task{}
		r, err := task.NextReminder()
		require.NoError(t, err)
		assert.Nil(t, r)
	})
	t.Run("remind_at only", func(t *testing.T) {
		task := &core.Task{RemindAt: &future}
		r, err := task.NextReminder()
		require.NoError(t, err)
		require.NotNil(t, r)
		assert.Equal(t, future.Unix(), r.Unix())
	})
	t.Run("auto-remind wins over later remind_at", func(t *testing.T) {
		due := now.Add(6 * time.Hour)
		task := &core.Task{
			DueAt:    &due,
			RemindAt: &farFuture,
		}
		r, err := task.NextReminder()
		require.NoError(t, err)
		require.NotNil(t, r)
		// auto-remind = due - 12h, but that's in the past
		// so remind_at (farFuture) should win
		assert.Equal(t, farFuture.Unix(), r.Unix())
	})
	t.Run("rrule recurrence preserves anchor cadence", func(t *testing.T) {
		// Anchor (CreatedAt) = now - 2h30m. FREQ=HOURLY fires at
		// anchor+1h, anchor+2h, anchor+3h ... so the cadence is
		// at minute-30 within each hour. Next strictly-future
		// fire after `now` is now + 30m, NOT now + 1h.
		task := &core.Task{
			CreatedAt: now.Add(-150 * time.Minute),
			RRule:     "FREQ=HOURLY",
		}
		r, err := task.NextReminder()
		require.NoError(t, err)
		require.NotNil(t, r)
		assert.InDelta(t, now.Add(30*time.Minute).Unix(), r.Unix(), 2)
	})
	t.Run("rrule weekly", func(t *testing.T) {
		// Anchor two days ago, FREQ=WEEKLY → next fire five days out.
		task := &core.Task{
			CreatedAt: now.Add(-2 * 24 * time.Hour),
			RRule:     "FREQ=WEEKLY",
		}
		r, err := task.NextReminder()
		require.NoError(t, err)
		require.NotNil(t, r)
		assert.InDelta(t, now.Add(5*24*time.Hour).Unix(), r.Unix(), 2)
	})
	t.Run("rrule yearly", func(t *testing.T) {
		// Anchor six months back, FREQ=YEARLY → next fire six
		// months out, inside the horizon.
		anchor := now.AddDate(0, -6, 0)
		task := &core.Task{
			CreatedAt: anchor,
			RRule:     "FREQ=YEARLY",
		}
		r, err := task.NextReminder()
		require.NoError(t, err)
		require.NotNil(t, r)
		assert.Equal(t, anchor.AddDate(1, 0, 0).Unix(), r.Unix())
	})
	t.Run("rrule stops at due", func(t *testing.T) {
		// Next fire = now + 2h, but due is now + 10min, so
		// the rrule-derived candidate is suppressed and
		// auto-remind is in the past → no reminder.
		dueAt := now.Add(10 * time.Minute)
		task := &core.Task{
			CreatedAt: now.Add(-30 * time.Minute),
			DueAt:     &dueAt,
			RRule:     "FREQ=HOURLY;INTERVAL=2",
		}
		r, err := task.NextReminder()
		require.NoError(t, err)
		assert.Nil(t, r)
	})
	t.Run("rrule beyond horizon is no reminder", func(t *testing.T) {
		// Anchor one day back, FREQ=YEARLY → next fire in a year
		// less a day... unless the rule's INTERVAL pushes it past
		// the horizon: INTERVAL=2 fires two years after the anchor.
		task := &core.Task{
			CreatedAt: now.Add(-24 * time.Hour),
			RRule:     "FREQ=YEARLY;INTERVAL=2",
		}
		r, err := task.NextReminder()
		require.NoError(t, err)
		assert.Nil(t, r)
	})
	t.Run("rrule exhausted is no reminder", func(t *testing.T) {
		// UNTIL already passed: the series ended, nothing to
		// report, and that is not an error.
		task := &core.Task{
			CreatedAt: now.AddDate(0, 0, -10),
			RRule:     "FREQ=DAILY;UNTIL=20200101T000000Z",
		}
		r, err := task.NextReminder()
		require.NoError(t, err)
		assert.Nil(t, r)
	})
	t.Run("unsatisfiable rrule is an error, not silence", func(t *testing.T) {
		// February 30th never comes. The evaluator walks its
		// iteration budget and gives up; that MUST reach the caller
		// as ErrIterationCap rather than collapse into "no
		// reminder", which used to burn ten thousand steps and then
		// return nil as if the series had simply ended.
		task := &core.Task{
			CreatedAt: now.Add(-time.Hour),
			RRule:     "FREQ=YEARLY;BYMONTH=2;BYMONTHDAY=30",
		}
		r, err := task.NextReminder()
		require.Error(t, err)
		assert.True(t, errors.Is(err, rrule.ErrIterationCap), "got %v", err)
		assert.Nil(t, r)
	})
	t.Run("invalid rrule is an error", func(t *testing.T) {
		task := &core.Task{
			CreatedAt: now.Add(-time.Hour),
			RRule:     "not-an-rrule",
		}
		r, err := task.NextReminder()
		require.Error(t, err)
		assert.Nil(t, r)
	})
}
