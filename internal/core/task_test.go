package core

import "testing"

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		name    string
		current TaskStatus
		next    TaskStatus
		wantErr bool
	}{
		{"TODO to IN_PROGRESS", StatusTodo, StatusInProgress, false},
		{"TODO to SKIPPED", StatusTodo, StatusSkipped, false},
		{"TODO to DONE", StatusTodo, StatusDone, true}, // Must go through IN_PROGRESS
		{"IN_PROGRESS to DONE", StatusInProgress, StatusDone, false},
		{"IN_PROGRESS to TODO", StatusInProgress, StatusTodo, false},
		{"DONE to TODO", StatusDone, StatusTodo, true},
		{"SKIPPED to IN_PROGRESS", StatusSkipped, StatusInProgress, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTransition(tt.current, tt.next)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTransition() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTask_Transition(t *testing.T) {
	task := &Task{ID: "T-1", Status: StatusTodo}
	
	log, err := task.Transition(StatusInProgress, "user", "starting")
	if err != nil {
		t.Fatalf("Transition failed: %v", err)
	}

	if task.Status != StatusInProgress {
		t.Errorf("expected IN_PROGRESS, got %s", task.Status)
	}
	if log.Action != string(StatusInProgress) {
		t.Errorf("expected log action IN_PROGRESS, got %s", log.Action)
	}
}
