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

func TestFlowRunGetID(t *testing.T) {
	run := &FlowRun{ID: "run-abc-123"}
	if got := run.GetID(); got != "run-abc-123" {
		t.Errorf("FlowRun.GetID() = %q, want %q", got, "run-abc-123")
	}
}

// strPtr is declared in plan_tasks_project_scope_test.go
