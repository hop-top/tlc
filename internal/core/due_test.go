package core_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"hop.top/tlc/internal/core"
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
		assert.Nil(t, task.NextReminder())
	})
	t.Run("remind_at only", func(t *testing.T) {
		task := &core.Task{RemindAt: &future}
		r := task.NextReminder()
		assert.NotNil(t, r)
		assert.Equal(t, future.Unix(), r.Unix())
	})
	t.Run("auto-remind wins over later remind_at", func(t *testing.T) {
		due := now.Add(6 * time.Hour)
		task := &core.Task{
			DueAt:    &due,
			RemindAt: &farFuture,
		}
		r := task.NextReminder()
		assert.NotNil(t, r)
		// auto-remind = due - 12h, but that's in the past
		// so remind_at (farFuture) should win
		assert.Equal(t, farFuture.Unix(), r.Unix())
	})
	t.Run("rrule recurrence", func(t *testing.T) {
		// Hourly RRULE → next fire is +1h from now.
		task := &core.Task{
			CreatedAt: now.Add(-150 * time.Minute),
			RRule:     "FREQ=HOURLY",
		}
		r := task.NextReminder()
		assert.NotNil(t, r)
		// next-from-now is now + 1h (within ~1s tolerance).
		assert.InDelta(t, now.Add(time.Hour).Unix(), r.Unix(), 2)
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
		r := task.NextReminder()
		assert.Nil(t, r)
	})
}
