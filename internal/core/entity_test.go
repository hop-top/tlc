package core

import "testing"

func TestTaskGetID(t *testing.T) {
	tests := []struct {
		name      string
		projectID *string
		id        string
		want      string
	}{
		{"no project", nil, "T-0001", "T-0001"},
		{"empty project", strPtr(""), "T-0001", "T-0001"},
		{"with project", strPtr("hop-top/tlc"), "T-0001", "hop-top/tlc:T-0001"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{ID: tt.id, ProjectID: tt.projectID}
			if got := task.GetID(); got != tt.want {
				t.Errorf("Task.GetID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTrackGetID(t *testing.T) {
	tests := []struct {
		name      string
		projectID *string
		id        string
		want      string
	}{
		{"no project", nil, "kit-domain", "kit-domain"},
		{"with project", strPtr("hop-top/tlc"), "kit-domain", "hop-top/tlc:kit-domain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			track := &Track{ID: tt.id, ProjectID: tt.projectID}
			if got := track.GetID(); got != tt.want {
				t.Errorf("Track.GetID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFlowRunGetID(t *testing.T) {
	run := &FlowRun{ID: "run-abc-123"}
	if got := run.GetID(); got != "run-abc-123" {
		t.Errorf("FlowRun.GetID() = %q, want %q", got, "run-abc-123")
	}
}

func TestCompoundKey(t *testing.T) {
	tests := []struct {
		projectID string
		entityID  string
		want      string
	}{
		{"", "T-0001", "T-0001"},
		{"proj", "T-0001", "proj:T-0001"},
	}
	for _, tt := range tests {
		if got := CompoundKey(tt.projectID, tt.entityID); got != tt.want {
			t.Errorf("CompoundKey(%q, %q) = %q, want %q",
				tt.projectID, tt.entityID, got, tt.want)
		}
	}
}

// strPtr is declared in plan_tasks_project_scope_test.go
