package core

import "testing"

func TestTaskGetID(t *testing.T) {
	task := &Task{ID: "T-0001"}
	if got := task.GetID(); got != "T-0001" {
		t.Errorf("Task.GetID() = %q, want %q", got, "T-0001")
	}
}

func TestTrackGetID(t *testing.T) {
	track := &Track{ID: "kit-domain"}
	if got := track.GetID(); got != "kit-domain" {
		t.Errorf("Track.GetID() = %q, want %q", got, "kit-domain")
	}
}

// strPtr is declared in plan_tasks_project_scope_test.go
