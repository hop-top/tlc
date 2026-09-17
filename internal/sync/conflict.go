package sync

import (
	"fmt"
	"strings"
	"time"

	"hop.top/vstar/diff"

	"hop.top/tlc/internal/core"
)

type ConflictStrategy string

const (
	StrategyRemoteWins ConflictStrategy = "remote-wins"
	StrategyLocalWins  ConflictStrategy = "local-wins"
	StrategyManual     ConflictStrategy = "manual"
	StrategyLastWrite  ConflictStrategy = "last-write-wins"
)

// clockSlack absorbs the precision gap between a remote system's
// modification timestamp (whole seconds on most APIs) and the local
// LastSyncAt (nanoseconds). It gates the remote side, and the local
// side's timestamp fallback, exactly as it always has.
const clockSlack = time.Second

// Conflict records a task whose local and remote copies both moved
// since the last sync and no longer agree on content.
type Conflict struct {
	TaskID      string
	LocalTask   *core.Task
	RemoteTask  *core.Task
	Description string
	// Diff is the property-level difference between the two content
	// views, local on the left. Diff.String() renders it for a manual
	// resolution prompt. Empty when the views could not be built, in
	// which case Description says why.
	Diff diff.ComponentDiff
}

// DetectConflict compares a local task with its remote version and
// returns a Conflict when both moved since the last sync and their
// exported content differs.
//
// "Moved" is decided per side. Local: the task's content hash differs
// from the one recorded at the last sync (see MarkSynced); with no hash
// recorded, UpdatedAt more than clockSlack after LastSyncAt. Remote: its
// UpdatedAt, the remote system's own modification instant, more than
// clockSlack after LastSyncAt. No remote baseline hash exists to compare
// against, and the remote's representation drops whatever it cannot
// store, so the local baseline would never match it.
//
// "Differs" is diff.Component over the two content views: property
// order, identity, timestamps and the wire hash play no part. Two sides
// that changed to the same content are not a conflict.
func DetectConflict(local *core.Task, remote *core.Task) *Conflict {
	if local.OriginSystem == nil || *local.OriginSystem == "" {
		return nil
	}
	if local.LastSyncAt == nil {
		// Never synced: no baseline to have diverged from.
		return nil
	}
	if !localChangedSinceSync(local) || !remote.UpdatedAt.After(local.LastSyncAt.Add(clockSlack)) {
		return nil
	}

	lv, err := contentView(local)
	if err != nil {
		return unresolvedConflict(local, remote, err)
	}
	rv, err := contentView(remote)
	if err != nil {
		return unresolvedConflict(local, remote, err)
	}
	if diff.Component(lv, rv) {
		return nil
	}
	d := diff.OfComponent(lv, rv)
	return &Conflict{
		TaskID:     local.ID,
		LocalTask:  local,
		RemoteTask: remote,
		Description: fmt.Sprintf("Both local and remote were modified since last sync (%s); differs in %s",
			local.LastSyncAt.Format(time.RFC3339), strings.Join(changedNames(d), ", ")),
		Diff: d,
	}
}

// localChangedSinceSync is NeedsPush's rule with this detector's own
// slack in the fallback: hash against the recorded baseline when there
// is one, else UpdatedAt more than clockSlack after LastSyncAt.
func localChangedSinceSync(local *core.Task) bool {
	if recorded, ok := local.LastSyncHash(); ok {
		if current, err := ContentHash(local); err == nil {
			return current != recorded
		}
	}
	return local.UpdatedAt.After(local.LastSyncAt.Add(clockSlack))
}

// unresolvedConflict is the conservative answer when a content view
// cannot be built: both sides moved and equality is unknown, so the
// caller gets a conflict to resolve rather than a silent overwrite.
func unresolvedConflict(local, remote *core.Task, err error) *Conflict {
	return &Conflict{
		TaskID:     local.ID,
		LocalTask:  local,
		RemoteTask: remote,
		Description: fmt.Sprintf("Both local and remote were modified since last sync (%s); content could not be compared: %v",
			local.LastSyncAt.Format(time.RFC3339), err),
	}
}

// changedNames lists, in diff order, the property names that differ at
// the top level followed by the path of every sub-component that
// differs. Repeated names (multi-valued properties) appear once.
func changedNames(d diff.ComponentDiff) []string {
	seen := make(map[string]bool, len(d.Properties))
	names := make([]string, 0, len(d.Properties)+len(d.SubDiffs))
	for _, p := range d.Properties {
		name := strings.ToUpper(p.Property.Name)
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	for _, sd := range d.SubDiffs {
		if !sd.Empty() {
			names = append(names, sd.Path)
		}
	}
	return names
}

// ResolveConflict resolves a conflict based on the provided strategy.
// Returns the resolved task and whether it should be updated locally.
//
// last-write-wins is the one strategy that still reads timestamps: the
// side with the later UpdatedAt wins, remote on a tie. That is the
// strategy's definition, recency, not a change heuristic; DetectConflict
// has already established by content that the two sides differ.
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
