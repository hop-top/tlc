package core_test

import (
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

func TestComputeTrackState_Healthy(t *testing.T) {
	track := &core.Track{Status: core.TrackStatusActive}
	tasks := []*core.Task{
		{Status: core.StatusInProgress, UpdatedAt: time.Now().UTC()},
	}
	flags := core.ComputeTrackState(track, tasks, 24*time.Hour)
	assertFlags(t, flags, core.TrackStateHealthy)
}

func TestComputeTrackState_Unlinked(t *testing.T) {
	track := &core.Track{Status: core.TrackStatusPending}
	flags := core.ComputeTrackState(track, nil, 24*time.Hour)
	assertFlags(t, flags, core.TrackStateUnlinked)
}

func TestComputeTrackState_Stale(t *testing.T) {
	track := &core.Track{Status: core.TrackStatusActive}
	tasks := []*core.Task{
		{Status: core.StatusInProgress, UpdatedAt: time.Now().UTC().Add(-48 * time.Hour)},
	}
	flags := core.ComputeTrackState(track, tasks, 24*time.Hour)
	assertFlags(t, flags, core.TrackStateStale)
}

func TestComputeTrackState_StaleIgnoredForCompleted(t *testing.T) {
	track := &core.Track{Status: core.TrackStatusCompleted}
	tasks := []*core.Task{
		{Status: core.StatusDone, UpdatedAt: time.Now().UTC().Add(-48 * time.Hour)},
	}
	flags := core.ComputeTrackState(track, tasks, 24*time.Hour)
	// Completed tracks are not flagged as stale.
	assertFlags(t, flags, core.TrackStateHealthy)
}

func TestComputeTrackState_Blocked(t *testing.T) {
	track := &core.Track{Status: core.TrackStatusActive}
	tasks := []*core.Task{
		blockedTask("T-0001", core.StatusTodo, "T-0099"),
		blockedTask("T-0002", core.StatusInProgress, "T-0100"),
		// Terminal task — doesn't count.
		{ID: "T-0003", Status: core.StatusDone, UpdatedAt: time.Now().UTC()},
	}
	flags := core.ComputeTrackState(track, tasks, 24*time.Hour)
	assertFlags(t, flags, core.TrackStateBlocked)
}

func TestComputeTrackState_NotBlockedWhenSomeUnblocked(t *testing.T) {
	track := &core.Track{Status: core.TrackStatusActive}
	tasks := []*core.Task{
		blockedTask("T-0001", core.StatusTodo, "T-0099"),
		{ID: "T-0002", Status: core.StatusInProgress, UpdatedAt: time.Now().UTC()},
	}
	flags := core.ComputeTrackState(track, tasks, 24*time.Hour)
	assertFlags(t, flags, core.TrackStateHealthy)
}

func TestComputeTrackState_MultipleFlags(t *testing.T) {
	track := &core.Track{Status: core.TrackStatusActive}
	tasks := []*core.Task{
		blockedTask("T-0001", core.StatusTodo, "T-0099"),
	}
	// Make task old enough to be stale too.
	tasks[0].UpdatedAt = time.Now().UTC().Add(-48 * time.Hour)

	flags := core.ComputeTrackState(track, tasks, 24*time.Hour)
	if len(flags) != 2 {
		t.Fatalf("expected 2 flags, got %v", flags)
	}
	flagSet := make(map[core.TrackStateFlag]bool)
	for _, f := range flags {
		flagSet[f] = true
	}
	if !flagSet[core.TrackStateStale] || !flagSet[core.TrackStateBlocked] {
		t.Fatalf("expected stale+blocked, got %v", flags)
	}
}

func TestComputeTrackState_PendingCanBeStale(t *testing.T) {
	track := &core.Track{Status: core.TrackStatusPending}
	tasks := []*core.Task{
		{Status: core.StatusTodo, UpdatedAt: time.Now().UTC().Add(-48 * time.Hour)},
	}
	flags := core.ComputeTrackState(track, tasks, 24*time.Hour)
	assertFlags(t, flags, core.TrackStateStale)
}

// --- helpers ---

func blockedTask(id string, status core.TaskStatus, blockedBy string) *core.Task {
	t := &core.Task{
		ID:        id,
		Status:    status,
		UpdatedAt: time.Now().UTC(),
		Meta:      map[string]interface{}{"blocked_by": []string{blockedBy}},
	}
	return t
}

func assertFlags(t *testing.T, got []core.TrackStateFlag, want ...core.TrackStateFlag) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected flags %v, got %v", want, got)
	}
	for i, f := range got {
		if f != want[i] {
			t.Fatalf("flag[%d]: expected %s, got %s", i, want[i], f)
		}
	}
}
