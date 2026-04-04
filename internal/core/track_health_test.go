package core

import (
	"math"
	"testing"
)

func TestComputeProjectHealth_NoTracks(t *testing.T) {
	h := ComputeProjectHealth(nil, nil, 3, 50)
	if h.ActiveLoad != 0 {
		t.Errorf("expected 0 active load, got %d", h.ActiveLoad)
	}
	if h.AvgProgress != 0 {
		t.Errorf("expected 0 avg progress, got %f", h.AvgProgress)
	}
}

func TestComputeProjectHealth_OnlyPending(t *testing.T) {
	tracks := []*Track{
		{ID: "t1", Status: TrackStatusPending},
		{ID: "t2", Status: TrackStatusPending},
	}
	h := ComputeProjectHealth(tracks, nil, 3, 50)
	if h.ActiveLoad != 0 {
		t.Errorf("expected 0 active, got %d", h.ActiveLoad)
	}
}

func TestComputeProjectHealth_ActiveTracks(t *testing.T) {
	tracks := []*Track{
		{ID: "t1", Status: TrackStatusActive},
		{ID: "t2", Status: TrackStatusActive},
		{ID: "t3", Status: TrackStatusCompleted},
	}
	tasks := map[string][]*Task{
		"t1": {
			{ID: "task-1", Status: "DONE"},
			{ID: "task-2", Status: "TODO"},
		},
		"t2": {
			{ID: "task-3", Status: "DONE"},
			{ID: "task-4", Status: "DONE"},
			{ID: "task-5", Status: "DONE"},
			{ID: "task-6", Status: "TODO"},
		},
	}

	h := ComputeProjectHealth(tracks, tasks, 3, 50)

	if h.ActiveLoad != 2 {
		t.Errorf("expected 2 active, got %d", h.ActiveLoad)
	}
	// t1: 1/2=50%, t2: 3/4=75%, avg=62.5%
	if math.Abs(h.AvgProgress-62.5) > 0.01 {
		t.Errorf("expected ~62.5%% avg progress, got %f", h.AvgProgress)
	}
	if h.MaxActive != 3 {
		t.Errorf("expected max_active=3, got %d", h.MaxActive)
	}
	if h.MinProgress != 50 {
		t.Errorf("expected min_progress=50, got %d", h.MinProgress)
	}
}

func TestComputeProjectHealth_ActiveNoTasks(t *testing.T) {
	tracks := []*Track{
		{ID: "t1", Status: TrackStatusActive},
	}
	tasks := map[string][]*Task{
		"t1": {},
	}

	h := ComputeProjectHealth(tracks, tasks, 3, 50)
	if h.ActiveLoad != 1 {
		t.Errorf("expected 1 active, got %d", h.ActiveLoad)
	}
	if h.AvgProgress != 0 {
		t.Errorf("expected 0 avg progress for empty task list, got %f",
			h.AvgProgress)
	}
}

func TestComputeProjectHealth_LowProgress(t *testing.T) {
	// 1 active track with 0% progress; minProgress=50 → should trigger low
	// progress path (active < max but avg < min).
	tracks := []*Track{
		{ID: "t1", Status: TrackStatusActive},
	}
	tasks := map[string][]*Task{
		"t1": {{ID: "a", Status: "TODO"}},
	}

	h := ComputeProjectHealth(tracks, tasks, 3, 50)
	if h.ActiveLoad != 1 {
		t.Errorf("expected 1 active, got %d", h.ActiveLoad)
	}
	if h.AvgProgress >= 50 {
		t.Errorf("expected avg progress < 50, got %f", h.AvgProgress)
	}
}

func TestComputeProjectHealth_AboveMinProgress(t *testing.T) {
	// 1 active track at 100% progress; minProgress=50 → healthy.
	tracks := []*Track{
		{ID: "t1", Status: TrackStatusActive},
	}
	tasks := map[string][]*Task{
		"t1": {{ID: "a", Status: "DONE"}},
	}

	h := ComputeProjectHealth(tracks, tasks, 3, 50)
	if h.AvgProgress < 50 {
		t.Errorf("expected avg progress >= 50, got %f", h.AvgProgress)
	}
}

func TestComputeProjectHealth_Overcommitted(t *testing.T) {
	tracks := []*Track{
		{ID: "t1", Status: TrackStatusActive},
		{ID: "t2", Status: TrackStatusActive},
		{ID: "t3", Status: TrackStatusActive},
	}
	tasks := map[string][]*Task{
		"t1": {{ID: "a", Status: "TODO"}},
		"t2": {{ID: "b", Status: "TODO"}},
		"t3": {{ID: "c", Status: "TODO"}},
	}

	h := ComputeProjectHealth(tracks, tasks, 3, 50)
	if h.ActiveLoad < h.MaxActive {
		t.Errorf("expected overcommitted (active=%d >= max=%d)",
			h.ActiveLoad, h.MaxActive)
	}
}
