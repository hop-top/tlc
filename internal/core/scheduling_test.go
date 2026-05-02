package core_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"hop.top/tlc/internal/core"
)

func TestApplySchedulingDefaults(t *testing.T) {
	now := time.Now().UTC()

	t.Run("applies due and rrule when empty", func(t *testing.T) {
		task := &core.Task{
			Priority:  "P0",
			CreatedAt: now,
		}
		defaults := &core.ScheduleDefaults{
			Due:   24 * time.Hour,
			RRule: "FREQ=HOURLY;INTERVAL=2",
		}
		core.ApplySchedulingDefaults(task, defaults)
		assert.NotNil(t, task.DueAt)
		assert.Equal(t, now.Add(24*time.Hour).Unix(), task.DueAt.Unix())
		assert.Equal(t, "FREQ=HOURLY;INTERVAL=2", task.RRule)
	})

	t.Run("does not override explicit due", func(t *testing.T) {
		explicit := now.Add(48 * time.Hour)
		task := &core.Task{
			Priority:  "P0",
			CreatedAt: now,
			DueAt:     &explicit,
		}
		defaults := &core.ScheduleDefaults{Due: 24 * time.Hour}
		core.ApplySchedulingDefaults(task, defaults)
		assert.Equal(t, explicit.Unix(), task.DueAt.Unix())
	})

	t.Run("nil defaults is no-op", func(t *testing.T) {
		task := &core.Task{CreatedAt: now}
		core.ApplySchedulingDefaults(task, nil)
		assert.Nil(t, task.DueAt)
		assert.Empty(t, task.RRule)
	})
}

func TestCheckAgeNudges(t *testing.T) {
	now := time.Now().UTC()

	t.Run("triggers on stale IN_PROGRESS", func(t *testing.T) {
		task := &core.Task{
			ID:        "T-0001",
			Status:    core.StatusInProgress,
			UpdatedAt: now.Add(-72 * time.Hour),
		}
		rules := []core.AgeNudgeConfig{
			{Status: "IN_PROGRESS", Threshold: 48 * time.Hour, Action: "remind"},
		}
		n := core.CheckAgeNudges(task, rules)
		assert.NotNil(t, n)
		assert.Equal(t, "remind", n.Action)
	})

	t.Run("skips tasks with explicit due", func(t *testing.T) {
		due := now.Add(24 * time.Hour)
		task := &core.Task{
			ID:        "T-0002",
			Status:    core.StatusInProgress,
			UpdatedAt: now.Add(-72 * time.Hour),
			DueAt:     &due,
		}
		rules := []core.AgeNudgeConfig{
			{Status: "IN_PROGRESS", Threshold: 48 * time.Hour, Action: "remind"},
		}
		assert.Nil(t, core.CheckAgeNudges(task, rules))
	})

	t.Run("skips terminal tasks", func(t *testing.T) {
		task := &core.Task{
			ID:        "T-0003",
			Status:    core.StatusDone,
			UpdatedAt: now.Add(-72 * time.Hour),
		}
		rules := []core.AgeNudgeConfig{
			{Threshold: 48 * time.Hour, Action: "remind"},
		}
		assert.Nil(t, core.CheckAgeNudges(task, rules))
	})

	t.Run("status filter", func(t *testing.T) {
		task := &core.Task{
			ID:        "T-0004",
			Status:    core.StatusTodo,
			UpdatedAt: now.Add(-72 * time.Hour),
		}
		rules := []core.AgeNudgeConfig{
			{Status: "IN_PROGRESS", Threshold: 48 * time.Hour, Action: "remind"},
		}
		assert.Nil(t, core.CheckAgeNudges(task, rules))
	})

	t.Run("empty status matches any", func(t *testing.T) {
		task := &core.Task{
			ID:        "T-0005",
			Status:    core.StatusTodo,
			UpdatedAt: now.Add(-72 * time.Hour),
		}
		rules := []core.AgeNudgeConfig{
			{Threshold: 48 * time.Hour, Action: "remind"},
		}
		n := core.CheckAgeNudges(task, rules)
		assert.NotNil(t, n)
	})
}
