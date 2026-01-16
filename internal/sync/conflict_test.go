package sync

import (
	"testing"
	"time"

	"github.com/google/oss-tlc-cli/internal/core"
	"github.com/stretchr/testify/assert"
)

func TestDetectConflict(t *testing.T) {
	origin := "github"
	lastSync := time.Now().Add(-1 * time.Hour).Truncate(time.Second)
	
	tests := []struct {
		name          string
		localUpdate   time.Time
		remoteUpdate  time.Time
		localTitle    string
		remoteTitle   string
		expectConflict bool
	}{
		{
			name:          "No changes",
			localUpdate:   lastSync,
			remoteUpdate:  lastSync,
			localTitle:    "Same",
			remoteTitle:   "Same",
			expectConflict: false,
		},
		{
			name:          "Only local changed",
			localUpdate:   lastSync.Add(10 * time.Minute),
			remoteUpdate:  lastSync,
			localTitle:    "Changed",
			remoteTitle:   "Same",
			expectConflict: false,
		},
		{
			name:          "Only remote changed",
			localUpdate:   lastSync,
			remoteUpdate:  lastSync.Add(10 * time.Minute),
			localTitle:    "Same",
			remoteTitle:   "Changed",
			expectConflict: false,
		},
		{
			name:          "Both changed - Conflict",
			localUpdate:   lastSync.Add(10 * time.Minute),
			remoteUpdate:  lastSync.Add(20 * time.Minute),
			localTitle:    "Local Change",
			remoteTitle:   "Remote Change",
			expectConflict: true,
		},
		{
			name:          "Both changed to same content - No conflict",
			localUpdate:   lastSync.Add(10 * time.Minute),
			remoteUpdate:  lastSync.Add(20 * time.Minute),
			localTitle:    "Same Change",
			remoteTitle:   "Same Change",
			expectConflict: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local := &core.Task{
				ID:           "T-1",
				Title:        tt.localTitle,
				OriginSystem: &origin,
				LastSyncAt:   &lastSync,
				UpdatedAt:    tt.localUpdate,
			}
			remote := &core.Task{
				ID:        "GH-1",
				Title:     tt.remoteTitle,
				UpdatedAt: tt.remoteUpdate,
			}

			conflict := DetectConflict(local, remote)
			if tt.expectConflict {
				assert.NotNil(t, conflict)
			} else {
				assert.Nil(t, conflict)
			}
		})
	}
}

func TestResolveConflict(t *testing.T) {
	local := &core.Task{ID: "T-1", Title: "Local", UpdatedAt: time.Now()}
	remote := &core.Task{ID: "GH-1", Title: "Remote", UpdatedAt: time.Now().Add(time.Minute)}
	conflict := &Conflict{
		TaskID:     "T-1",
		LocalTask:  local,
		RemoteTask: remote,
	}

	t.Run("RemoteWins", func(t *testing.T) {
		resolved, updated := ResolveConflict(conflict, StrategyRemoteWins)
		assert.Equal(t, "Remote", resolved.Title)
		assert.True(t, updated)
	})

	t.Run("LocalWins", func(t *testing.T) {
		resolved, updated := ResolveConflict(conflict, StrategyLocalWins)
		assert.Equal(t, "Local", resolved.Title)
		assert.False(t, updated)
	})

	t.Run("LastWriteWins - Remote newer", func(t *testing.T) {
		resolved, updated := ResolveConflict(conflict, StrategyLastWrite)
		assert.Equal(t, "Remote", resolved.Title)
		assert.True(t, updated)
	})
}
