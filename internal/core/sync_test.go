package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTask_NeedsPush(t *testing.T) {
	origin := "github"
	now := time.Now().UTC()

	tests := []struct {
		name         string
		originSystem *string
		updatedAt    time.Time
		lastSyncAt   *time.Time
		expected     bool
	}{
		{
			name:         "No origin system",
			originSystem: nil,
			updatedAt:    now,
			lastSyncAt:   nil,
			expected:     false,
		},
		{
			name:         "Origin system but never synced",
			originSystem: &origin,
			updatedAt:    now,
			lastSyncAt:   nil,
			expected:     true,
		},
		{
			name:         "Synced and not modified",
			originSystem: &origin,
			updatedAt:    now,
			lastSyncAt:   &now,
			expected:     false,
		},
		{
			name:         "Synced and then modified",
			originSystem: &origin,
			updatedAt:    now.Add(time.Minute),
			lastSyncAt:   &now,
			expected:     true,
		},
		{
			name:         "Synced after modification",
			originSystem: &origin,
			updatedAt:    now,
			lastSyncAt:   func() *time.Time { t := now.Add(time.Minute); return &t }(),
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{
				OriginSystem: tt.originSystem,
				UpdatedAt:    tt.updatedAt,
				LastSyncAt:   tt.lastSyncAt,
			}
			assert.Equal(t, tt.expected, task.NeedsPush())
		})
	}
}

func TestTask_NeedsPush_EdgeCases(t *testing.T) {
	github := "github"
	jira := "jira"
	now := time.Now().UTC()

	t.Run("Empty origin system string", func(t *testing.T) {
		empty := ""
		task := &Task{
			OriginSystem: &empty,
			UpdatedAt:    now,
			LastSyncAt:   nil,
		}
		assert.False(t, task.NeedsPush(), "should not need push with empty origin system")
	})

	t.Run("Different origin systems", func(t *testing.T) {
		task := &Task{
			OriginSystem: &jira,
			UpdatedAt:    now.Add(time.Minute),
			LastSyncAt:   &now,
		}
		assert.True(t, task.NeedsPush(), "should need push for any origin system with modifications")
	})

	t.Run("Updated exactly at sync time", func(t *testing.T) {
		syncTime := now
		task := &Task{
			OriginSystem: &github,
			UpdatedAt:    syncTime,
			LastSyncAt:   &syncTime,
		}
		assert.False(t, task.NeedsPush(), "should not need push when updated at sync time")
	})

	t.Run("Updated one millisecond after sync", func(t *testing.T) {
		syncTime := now
		task := &Task{
			OriginSystem: &github,
			UpdatedAt:    syncTime.Add(2 * time.Millisecond),
			LastSyncAt:   &syncTime,
		}
		assert.True(t, task.NeedsPush(), "should need push when updated after sync")
	})

	t.Run("Last sync is in the future", func(t *testing.T) {
		futureSync := now.Add(1 * time.Hour)
		task := &Task{
			OriginSystem: &github,
			UpdatedAt:    now,
			LastSyncAt:   &futureSync,
		}
		assert.False(t, task.NeedsPush(), "should not need push when last sync is in future")
	})

	t.Run("Multiple modifications with sync", func(t *testing.T) {
		secondSync := now.Add(-30 * time.Minute)
		update2 := now.Add(-10 * time.Minute)

		task := &Task{
			OriginSystem: &github,
			UpdatedAt:    update2,
			LastSyncAt:   &secondSync,
		}
		assert.True(t, task.NeedsPush(), "should need push when last update is after last sync")
	})

	t.Run("Updated long before sync", func(t *testing.T) {
		oldUpdate := now.Add(-24 * time.Hour)
		recentSync := now.Add(-1 * time.Hour)

		task := &Task{
			OriginSystem: &github,
			UpdatedAt:    oldUpdate,
			LastSyncAt:   &recentSync,
		}
		assert.False(t, task.NeedsPush(), "should not need push when updated before last sync")
	})
}
