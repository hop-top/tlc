package sync

import (
	"testing"
	"time"

	"github.com/IdeaCraftersLabs/oss-tlc-cli/internal/core"
	"github.com/stretchr/testify/assert"
)

func TestDetectConflict(t *testing.T) {
	origin := "github"
	lastSync := time.Now().Add(-1 * time.Hour).Truncate(time.Second)

	tests := []struct {
		name           string
		localUpdate    time.Time
		remoteUpdate   time.Time
		localTitle     string
		remoteTitle    string
		expectConflict bool
	}{
		{
			name:           "No changes",
			localUpdate:    lastSync,
			remoteUpdate:   lastSync,
			localTitle:     "Same",
			remoteTitle:    "Same",
			expectConflict: false,
		},
		{
			name:           "Only local changed",
			localUpdate:    lastSync.Add(10 * time.Minute),
			remoteUpdate:   lastSync,
			localTitle:     "Changed",
			remoteTitle:    "Same",
			expectConflict: false,
		},
		{
			name:           "Only remote changed",
			localUpdate:    lastSync,
			remoteUpdate:   lastSync.Add(10 * time.Minute),
			localTitle:     "Same",
			remoteTitle:    "Changed",
			expectConflict: false,
		},
		{
			name:           "Both changed - Conflict",
			localUpdate:    lastSync.Add(10 * time.Minute),
			remoteUpdate:   lastSync.Add(20 * time.Minute),
			localTitle:     "Local Change",
			remoteTitle:    "Remote Change",
			expectConflict: true,
		},
		{
			name:           "Both changed to same content - No conflict",
			localUpdate:    lastSync.Add(10 * time.Minute),
			remoteUpdate:   lastSync.Add(20 * time.Minute),
			localTitle:     "Same Change",
			remoteTitle:    "Same Change",
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

func TestDetectConflict_EdgeCases(t *testing.T) {
	origin := "github"
	lastSync := time.Now().Add(-1 * time.Hour).Truncate(time.Second)

	t.Run("No origin system", func(t *testing.T) {
		local := &core.Task{
			ID:         "T-1",
			Title:      "Task",
			LastSyncAt: &lastSync,
			UpdatedAt:  lastSync.Add(10 * time.Minute),
		}
		remote := &core.Task{
			ID:        "GH-1",
			Title:     "Remote",
			UpdatedAt: lastSync.Add(20 * time.Minute),
		}

		conflict := DetectConflict(local, remote)
		assert.Nil(t, conflict, "should not detect conflict without origin system")
	})

	t.Run("Never synced", func(t *testing.T) {
		local := &core.Task{
			ID:           "T-1",
			Title:        "Task",
			OriginSystem: &origin,
			LastSyncAt:   nil,
			UpdatedAt:    time.Now(),
		}
		remote := &core.Task{
			ID:        "GH-1",
			Title:     "Remote",
			UpdatedAt: time.Now(),
		}

		conflict := DetectConflict(local, remote)
		assert.Nil(t, conflict, "should not detect conflict without last sync time")
	})

	t.Run("Empty origin system", func(t *testing.T) {
		emptyOrigin := ""
		local := &core.Task{
			ID:           "T-1",
			Title:        "Task",
			OriginSystem: &emptyOrigin,
			LastSyncAt:   &lastSync,
			UpdatedAt:    lastSync.Add(10 * time.Minute),
		}
		remote := &core.Task{
			ID:        "GH-1",
			Title:     "Remote",
			UpdatedAt: lastSync.Add(20 * time.Minute),
		}

		conflict := DetectConflict(local, remote)
		assert.Nil(t, conflict, "should not detect conflict with empty origin system")
	})

	t.Run("Both modified with same status and title but different description", func(t *testing.T) {
		local := &core.Task{
			ID:           "T-1",
			Title:        "Task",
			Description:  "Local description",
			Status:       core.StatusTodo,
			OriginSystem: &origin,
			LastSyncAt:   &lastSync,
			UpdatedAt:    lastSync.Add(10 * time.Minute),
		}
		remote := &core.Task{
			ID:          "GH-1",
			Title:       "Task",
			Description: "Remote description",
			Status:      core.StatusTodo,
			UpdatedAt:   lastSync.Add(20 * time.Minute),
		}

		conflict := DetectConflict(local, remote)
		assert.NotNil(t, conflict, "should detect conflict when descriptions differ")
	})

	t.Run("Both modified with same title and description but different status", func(t *testing.T) {
		local := &core.Task{
			ID:           "T-1",
			Title:        "Task",
			Description:  "Same description",
			Status:       core.StatusTodo,
			OriginSystem: &origin,
			LastSyncAt:   &lastSync,
			UpdatedAt:    lastSync.Add(10 * time.Minute),
		}
		remote := &core.Task{
			ID:          "GH-1",
			Title:       "Task",
			Description: "Same description",
			Status:      core.StatusInProgress,
			UpdatedAt:   lastSync.Add(20 * time.Minute),
		}

		conflict := DetectConflict(local, remote)
		assert.NotNil(t, conflict, "should detect conflict when statuses differ")
	})

	t.Run("Timestamp precision - exact same time", func(t *testing.T) {
		sameTime := lastSync.Add(10 * time.Minute)
		local := &core.Task{
			ID:           "T-1",
			Title:        "Local",
			OriginSystem: &origin,
			LastSyncAt:   &lastSync,
			UpdatedAt:    sameTime,
		}
		remote := &core.Task{
			ID:        "GH-1",
			Title:     "Remote",
			UpdatedAt: sameTime,
		}

		conflict := DetectConflict(local, remote)
		assert.NotNil(t, conflict, "should detect conflict when both modified at same time with different content")
	})

	t.Run("One second buffer - local at threshold", func(t *testing.T) {
		thresholdTime := lastSync.Add(time.Second)
		local := &core.Task{
			ID:           "T-1",
			Title:        "Local",
			OriginSystem: &origin,
			LastSyncAt:   &lastSync,
			UpdatedAt:    thresholdTime,
		}
		remote := &core.Task{
			ID:        "GH-1",
			Title:     "Remote",
			UpdatedAt: lastSync.Add(2 * time.Second),
		}

		conflict := DetectConflict(local, remote)
		assert.Nil(t, conflict, "should not detect conflict when local is at 1 second threshold")
	})

	t.Run("One second buffer - remote at threshold", func(t *testing.T) {
		thresholdTime := lastSync.Add(time.Second)
		local := &core.Task{
			ID:           "T-1",
			Title:        "Local",
			OriginSystem: &origin,
			LastSyncAt:   &lastSync,
			UpdatedAt:    lastSync.Add(2 * time.Second),
		}
		remote := &core.Task{
			ID:        "GH-1",
			Title:     "Remote",
			UpdatedAt: thresholdTime,
		}

		conflict := DetectConflict(local, remote)
		assert.Nil(t, conflict, "should not detect conflict when remote is at 1 second threshold")
	})
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

	t.Run("LastWriteWins - Local newer", func(t *testing.T) {
		newerLocal := &core.Task{ID: "T-1", Title: "Local", UpdatedAt: time.Now().Add(2 * time.Minute)}
		remote := &core.Task{ID: "GH-1", Title: "Remote", UpdatedAt: time.Now()}
		conflict := &Conflict{
			TaskID:     "T-1",
			LocalTask:  newerLocal,
			RemoteTask: remote,
		}

		resolved, updated := ResolveConflict(conflict, StrategyLastWrite)
		assert.Equal(t, "Local", resolved.Title)
		assert.False(t, updated)
	})

	t.Run("LastWriteWins - Same timestamp", func(t *testing.T) {
		sameTime := time.Now()
		local := &core.Task{ID: "T-1", Title: "Local", UpdatedAt: sameTime}
		remote := &core.Task{ID: "GH-1", Title: "Remote", UpdatedAt: sameTime}
		conflict := &Conflict{
			TaskID:     "T-1",
			LocalTask:  local,
			RemoteTask: remote,
		}

		resolved, updated := ResolveConflict(conflict, StrategyLastWrite)
		assert.Equal(t, "Remote", resolved.Title, "should prefer remote when timestamps are equal")
		assert.True(t, updated)
	})

	t.Run("Unknown strategy defaults to remote", func(t *testing.T) {
		unknownStrategy := ConflictStrategy("unknown-strategy")
		resolved, updated := ResolveConflict(conflict, unknownStrategy)
		assert.Equal(t, "Remote", resolved.Title, "should default to remote wins for unknown strategy")
		assert.True(t, updated)
	})

	t.Run("Manual strategy defaults to remote", func(t *testing.T) {
		resolved, updated := ResolveConflict(conflict, StrategyManual)
		assert.Equal(t, "Remote", resolved.Title, "should default to remote wins for manual strategy")
		assert.True(t, updated)
	})
}

func TestResolveConflict_TaskFields(t *testing.T) {

	t.Run("RemoteWins preserves all fields", func(t *testing.T) {
		local := &core.Task{
			ID:          "T-1",
			Title:       "Local",
			Description: "Local desc",
			Status:      core.StatusTodo,
			Tags:        []string{"local"},
			UpdatedAt:   time.Now(),
		}
		remote := &core.Task{
			ID:          "GH-1",
			Title:       "Remote",
			Description: "Remote desc",
			Status:      core.StatusInProgress,
			Tags:        []string{"remote"},
			UpdatedAt:   time.Now().Add(time.Minute),
		}

		conflict := &Conflict{
			TaskID:     "T-1",
			LocalTask:  local,
			RemoteTask: remote,
		}

		resolved, updated := ResolveConflict(conflict, StrategyRemoteWins)
		assert.Equal(t, "Remote", resolved.Title)
		assert.Equal(t, "Remote desc", resolved.Description)
		assert.Equal(t, core.StatusInProgress, resolved.Status)
		assert.Equal(t, []string{"remote"}, resolved.Tags)
		assert.True(t, updated)
	})

	t.Run("LocalWins preserves all fields", func(t *testing.T) {
		local := &core.Task{
			ID:          "T-1",
			Title:       "Local",
			Description: "Local desc",
			Status:      core.StatusTodo,
			Tags:        []string{"local"},
			UpdatedAt:   time.Now(),
		}
		remote := &core.Task{
			ID:          "GH-1",
			Title:       "Remote",
			Description: "Remote desc",
			Status:      core.StatusInProgress,
			Tags:        []string{"remote"},
			UpdatedAt:   time.Now().Add(time.Minute),
		}

		conflict := &Conflict{
			TaskID:     "T-1",
			LocalTask:  local,
			RemoteTask: remote,
		}

		resolved, updated := ResolveConflict(conflict, StrategyLocalWins)
		assert.Equal(t, "Local", resolved.Title)
		assert.Equal(t, "Local desc", resolved.Description)
		assert.Equal(t, core.StatusTodo, resolved.Status)
		assert.Equal(t, []string{"local"}, resolved.Tags)
		assert.False(t, updated)
	})

	t.Run("LastWriteWins with complex task comparison", func(t *testing.T) {
		local := &core.Task{
			ID:          "T-1",
			Title:       "Local",
			Description: "Local desc",
			Status:      core.StatusTodo,
			Tags:        []string{"local"},
			UpdatedAt:   time.Now().Add(2 * time.Minute),
		}
		remote := &core.Task{
			ID:          "GH-1",
			Title:       "Remote",
			Description: "Remote desc",
			Status:      core.StatusInProgress,
			Tags:        []string{"remote"},
			UpdatedAt:   time.Now(),
		}

		conflict := &Conflict{
			TaskID:     "T-1",
			LocalTask:  local,
			RemoteTask: remote,
		}

		resolved, updated := ResolveConflict(conflict, StrategyLastWrite)
		assert.Equal(t, "Local", resolved.Title)
		assert.Equal(t, "Local desc", resolved.Description)
		assert.Equal(t, core.StatusTodo, resolved.Status)
		assert.Equal(t, []string{"local"}, resolved.Tags)
		assert.False(t, updated)
	})
}
