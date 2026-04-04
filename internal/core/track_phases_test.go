package core_test

import (
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

func TestComputeTrackProgress_Empty(t *testing.T) {
	p := core.ComputeTrackProgress(nil)
	if p.TotalTasks != 0 || p.CompletedTasks != 0 || p.TotalPhases != 0 {
		t.Fatalf("expected zeros, got %+v", p)
	}
}

func TestComputeTrackProgress_NoPhases(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-1", Status: core.StatusDone, UpdatedAt: time.Now().UTC()},
		{ID: "T-2", Status: core.StatusInProgress, UpdatedAt: time.Now().UTC()},
	}
	p := core.ComputeTrackProgress(tasks)
	if p.TotalTasks != 2 {
		t.Fatalf("expected 2 total, got %d", p.TotalTasks)
	}
	if p.CompletedTasks != 1 {
		t.Fatalf("expected 1 completed, got %d", p.CompletedTasks)
	}
	if p.TotalPhases != 0 {
		t.Fatalf("expected 0 phases, got %d", p.TotalPhases)
	}
	if len(p.Unphased) != 2 {
		t.Fatalf("expected 2 unphased, got %d", len(p.Unphased))
	}
}

func TestComputeTrackProgress_WithPhases(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-1", Status: core.StatusDone, Tags: []string{"phase:1"}, UpdatedAt: time.Now().UTC()},
		{ID: "T-2", Status: core.StatusDone, Tags: []string{"phase:1"}, UpdatedAt: time.Now().UTC()},
		{ID: "T-3", Status: core.StatusInProgress, Tags: []string{"phase:2"}, UpdatedAt: time.Now().UTC()},
		{ID: "T-4", Status: core.StatusTodo, Tags: []string{"phase:2"}, UpdatedAt: time.Now().UTC()},
		{ID: "T-5", Status: core.StatusTodo, Tags: []string{"phase:3"}, UpdatedAt: time.Now().UTC()},
	}
	p := core.ComputeTrackProgress(tasks)

	if p.TotalTasks != 5 {
		t.Fatalf("expected 5 total, got %d", p.TotalTasks)
	}
	if p.CompletedTasks != 2 {
		t.Fatalf("expected 2 completed, got %d", p.CompletedTasks)
	}
	if p.TotalPhases != 3 {
		t.Fatalf("expected 3 phases, got %d", p.TotalPhases)
	}
	if p.CurrentPhase != 2 {
		t.Fatalf("expected current phase 2, got %d", p.CurrentPhase)
	}

	// Phase 1: 2/2 complete.
	if p.Phases[0].Total != 2 || p.Phases[0].Completed != 2 {
		t.Fatalf("phase 1: expected 2/2, got %d/%d", p.Phases[0].Completed, p.Phases[0].Total)
	}
	// Phase 2: 0/2 complete.
	if p.Phases[1].Total != 2 || p.Phases[1].Completed != 0 {
		t.Fatalf("phase 2: expected 0/2, got %d/%d", p.Phases[1].Completed, p.Phases[1].Total)
	}
}

func TestComputeTrackProgress_SkippedCountsAsCompleted(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-1", Status: core.StatusSkipped, Tags: []string{"phase:1"}, UpdatedAt: time.Now().UTC()},
		{ID: "T-2", Status: core.StatusDone, Tags: []string{"phase:1"}, UpdatedAt: time.Now().UTC()},
	}
	p := core.ComputeTrackProgress(tasks)
	if p.CompletedTasks != 2 {
		t.Fatalf("expected 2 completed (DONE+SKIPPED), got %d", p.CompletedTasks)
	}
	if p.Phases[0].Completed != 2 {
		t.Fatalf("phase 1: expected 2 completed, got %d", p.Phases[0].Completed)
	}
}

func TestComputeTrackProgress_AllComplete(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-1", Status: core.StatusDone, Tags: []string{"phase:1"}, UpdatedAt: time.Now().UTC()},
		{ID: "T-2", Status: core.StatusDone, Tags: []string{"phase:2"}, UpdatedAt: time.Now().UTC()},
	}
	p := core.ComputeTrackProgress(tasks)
	if p.CurrentPhase != 0 {
		t.Fatalf("expected current phase 0 (all done), got %d", p.CurrentPhase)
	}
}

func TestComputeTrackProgress_MixedPhasedAndUnphased(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-1", Status: core.StatusDone, Tags: []string{"phase:1"}, UpdatedAt: time.Now().UTC()},
		{ID: "T-2", Status: core.StatusTodo, UpdatedAt: time.Now().UTC()},
	}
	p := core.ComputeTrackProgress(tasks)
	if p.TotalPhases != 1 {
		t.Fatalf("expected 1 phase, got %d", p.TotalPhases)
	}
	if len(p.Unphased) != 1 {
		t.Fatalf("expected 1 unphased, got %d", len(p.Unphased))
	}
}

func TestComputeTrackProgress_GapInPhases(t *testing.T) {
	// Tasks in phase 1 and phase 3 (phase 2 has no tasks).
	tasks := []*core.Task{
		{ID: "T-1", Status: core.StatusDone, Tags: []string{"phase:1"}, UpdatedAt: time.Now().UTC()},
		{ID: "T-2", Status: core.StatusTodo, Tags: []string{"phase:3"}, UpdatedAt: time.Now().UTC()},
	}
	p := core.ComputeTrackProgress(tasks)
	if p.TotalPhases != 3 {
		t.Fatalf("expected 3 phases, got %d", p.TotalPhases)
	}
	if len(p.Phases) != 3 {
		t.Fatalf("expected 3 phase entries, got %d", len(p.Phases))
	}
	// Phase 2 should exist but be empty.
	if p.Phases[1].Total != 0 {
		t.Fatalf("phase 2: expected 0 total, got %d", p.Phases[1].Total)
	}
	// Current phase should be 3 (first incomplete).
	if p.CurrentPhase != 3 {
		t.Fatalf("expected current phase 3, got %d", p.CurrentPhase)
	}
}

func TestFormatProgress_WithPhases(t *testing.T) {
	p := core.TrackProgress{
		TotalTasks:     8,
		CompletedTasks: 5,
		CurrentPhase:   2,
		TotalPhases:    3,
	}
	got := core.FormatProgress(p)
	want := "5/8 (P2/3)"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestFormatProgress_NoPhases(t *testing.T) {
	p := core.TrackProgress{
		TotalTasks:     4,
		CompletedTasks: 2,
		TotalPhases:    0,
	}
	got := core.FormatProgress(p)
	want := "2/4"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestFormatProgress_AllDone(t *testing.T) {
	p := core.TrackProgress{
		TotalTasks:     3,
		CompletedTasks: 3,
		CurrentPhase:   0,
		TotalPhases:    2,
	}
	got := core.FormatProgress(p)
	want := "3/3 (P0/2)"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
