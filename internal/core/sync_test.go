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
