package sync

import (
	"fmt"
	"time"

	"github.com/google/oss-tlc-cli/internal/core"
)

type ConflictStrategy string

const (
	StrategyRemoteWins   ConflictStrategy = "remote-wins"
	StrategyLocalWins    ConflictStrategy = "local-wins"
	StrategyManual       ConflictStrategy = "manual"
	StrategyLastWrite    ConflictStrategy = "last-write-wins"
)

type Conflict struct {
	TaskID      string
	LocalTask   *core.Task
	RemoteTask  *core.Task
	Description string
}

// DetectConflict compares a local task with its remote version and returns a Conflict if both were modified since last sync.
func DetectConflict(local *core.Task, remote *core.Task) *Conflict {
	if local.OriginSystem == nil || *local.OriginSystem == "" {
		return nil
	}

	if local.LastSyncAt == nil {
		// If it's never been synced, we don't have a baseline to detect conflict.
		return nil
	}

	// Local is modified if UpdatedAt > LastSyncAt
	// We use a small buffer (1s) to account for database vs API precision differences
	localModified := local.UpdatedAt.After(local.LastSyncAt.Add(time.Second))
	
	// Remote is modified if its UpdatedAt > LastSyncAt
	remoteModified := remote.UpdatedAt.After(local.LastSyncAt.Add(time.Second))

	if localModified && remoteModified {
		// Check if they are actually different in meaningful ways to avoid false positives
		if local.Status == remote.Status && local.Title == remote.Title && local.Description == remote.Description {
			return nil
		}

		return &Conflict{
			TaskID:      local.ID,
			LocalTask:   local,
			RemoteTask:  remote,
			Description: fmt.Sprintf("Both local and remote were modified since last sync (%s)", local.LastSyncAt.Format(time.RFC3339)),
		}
	}

	return nil
}

// ResolveConflict resolves a conflict based on the provided strategy.
// Returns the resolved task and whether it should be updated locally.
func ResolveConflict(conflict *Conflict, strategy ConflictStrategy) (*core.Task, bool) {
	switch strategy {
	case StrategyLocalWins:
		// Keep local version, but we might want to mark it as needing push
		return conflict.LocalTask, false
	case StrategyRemoteWins:
		// Overwrite with remote version
		return conflict.RemoteTask, true
	case StrategyLastWrite:
		if conflict.LocalTask.UpdatedAt.After(conflict.RemoteTask.UpdatedAt) {
			return conflict.LocalTask, false
		}
		return conflict.RemoteTask, true
	default:
		// Default to remote wins for safety if strategy is unknown or manual (handled elsewhere)
		return conflict.RemoteTask, true
	}
}
