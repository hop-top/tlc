package core

import (
	"context"
	"strings"
	"testing"
)

func TestParseFlow_YAML(t *testing.T) {
	yamlInput := `
flow_id: "flow:qa:0.1"
name: "QA Pipeline"
version: "0.1"
entry_step: "lint"
steps:
  lint:
    step_id: "lint"
    type: "task"
    title: "Lint"
    task_ref: "task:lint"
`
	flow, err := ParseFlow(strings.NewReader(yamlInput), "test.yaml")
	if err != nil {
		t.Fatalf("failed to parse YAML: %v", err)
	}

	if flow.ID != "flow:qa:0.1" {
		t.Errorf("expected ID flow:qa:0.1, got %s", flow.ID)
	}
	if flow.EntryStep != "lint" {
		t.Errorf("expected entry_step lint, got %s", flow.EntryStep)
	}
	if len(flow.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(flow.Steps))
	}
}

func TestParseFlow_JSON(t *testing.T) {
	jsonInput := `{
		"flow_id": "flow:test:1.0",
		"name": "Test Flow",
		"version": "1.0",
		"entry_step": "start",
		"steps": {
			"start": {
				"step_id": "start",
				"type": "task",
				"title": "Start",
				"task_ref": "T-1"
			}
		}
	}`
	flow, err := ParseFlow(strings.NewReader(jsonInput), "test.json")
	if err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if flow.ID != "flow:test:1.0" {
		t.Errorf("expected ID flow:test:1.0, got %s", flow.ID)
	}
}

func TestFlowExecutor_Sequential(t *testing.T) {
	repo := NewMockRepository()
	logRepo := NewMockLogRepository()
	executor := NewFlowExecutor(repo, logRepo)

	ctx := context.Background()

	// Setup tasks
	repo.CreateTask(ctx, &Task{ID: "T-1", Title: "Task 1", Status: StatusTodo})
	repo.CreateTask(ctx, &Task{ID: "T-2", Title: "Task 2", Status: StatusTodo})

	// Setup flow: T-1 -> T-2
	flow := &Flow{
		ID:        "flow:test:0.1",
		Name:      "Test Flow",
		EntryStep: "step1",
		Steps: map[string]Step{
			"step1": {
				ID:      "step1",
				Type:    StepTypeTask,
				Title:   "Step 1",
				TaskRef: "T-1",
			},
			"step2": {
				ID:        "step2",
				Type:      StepTypeTask,
				Title:     "Step 2",
				TaskRef:   "T-2",
				DependsOn: []string{"step1"},
			},
		},
	}

	run, err := executor.Execute(ctx, flow, "test-user")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if run.Status != FlowStatusSucceeded {
		t.Errorf("expected flow succeeded, got %s", run.Status)
	}

	// Verify tasks are DONE
	t1, _ := repo.GetTask(ctx, "T-1")
	if t1.Status != StatusDone {
		t.Errorf("task T-1 status: expected DONE, got %s", t1.Status)
	}
	t2, _ := repo.GetTask(ctx, "T-2")
	if t2.Status != StatusDone {
		t.Errorf("task T-2 status: expected DONE, got %s", t2.Status)
	}

	// Verify logs
	logs, _ := logRepo.ListLogs(ctx, LogQuery{})
	// EXPECT: FLOW_START, STEP_START(1), CLAIMED(T1), DONE(T1), STEP_END(1), STEP_START(2), CLAIMED(T2), DONE(T2), STEP_END(2), FLOW_END
	if len(logs) < 10 {
		t.Errorf("expected at least 10 log entries, got %d", len(logs))
	}
}

func TestFlowExecutor_Parallel(t *testing.T) {
	repo := NewMockRepository()
	logRepo := NewMockLogRepository()
	executor := NewFlowExecutor(repo, logRepo)

	ctx := context.Background()

	// Setup tasks
	repo.CreateTask(ctx, &Task{ID: "T-1", Title: "Task 1", Status: StatusTodo})
	repo.CreateTask(ctx, &Task{ID: "T-2", Title: "Task 2", Status: StatusTodo})

	// Setup flow: parallel(T-1, T-2)
	flow := &Flow{
		ID:        "flow:test:parallel",
		Name:      "Parallel Test",
		EntryStep: "p1",
		Steps: map[string]Step{
			"p1": {
				ID:       "p1",
				Type:     StepTypeParallel,
				Title:    "Parallel Container",
				Children: []string{"s1", "s2"},
			},
			"s1": {
				ID:      "s1",
				Type:    StepTypeTask,
				Title:   "Step 1",
				TaskRef: "T-1",
			},
			"s2": {
				ID:      "s2",
				Type:    StepTypeTask,
				Title:   "Step 2",
				TaskRef: "T-2",
			},
		},
	}

	run, err := executor.Execute(ctx, flow, "test-user")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if run.Status != FlowStatusSucceeded {
		t.Errorf("expected flow succeeded, got %s", run.Status)
	}

	// Verify tasks
	t1, _ := repo.GetTask(ctx, "T-1")
	if t1.Status != StatusDone {
		t.Errorf("task T-1 expected DONE")
	}
	t2, _ := repo.GetTask(ctx, "T-2")
	if t2.Status != StatusDone {
		t.Errorf("task T-2 expected DONE")
	}
}

func TestFlowExecutor_Retry(t *testing.T) {
	repo := NewMockRepository()
	logRepo := NewMockLogRepository()
	executor := NewFlowExecutor(repo, logRepo)

	ctx := context.Background()

	// Task that fails first time then succeeds?
	// Actually executeTaskStep is deterministic success in my current mock.
	// I need a way to make it fail.
	
	// Setup flow: retry(T-1)
	flow := &Flow{
		ID:        "flow:test:retry",
		Name:      "Retry Test",
		EntryStep: "r1",
		Steps: map[string]Step{
			"r1": {
				ID:    "r1",
				Type:  StepTypeRetry,
				Title: "Retry Container",
				Child: "s1",
				Policy: &RetryPolicy{
					MaxAttempts: 3,
					BackoffMS:   10,
				},
			},
			"s1": {
				ID:      "s1",
				Type:    StepTypeTask,
				Title:   "Failing Step",
				TaskRef: "T-FAIL",
			},
		},
	}

	// s1 will fail because T-FAIL not in repo
	run, err := executor.Execute(ctx, flow, "test-user")
	if err == nil {
		t.Fatal("expected error from exhausted retry")
	}

	if run.Status != FlowStatusFailed {
		t.Errorf("expected flow failed, got %s", run.Status)
	}

	// Verify logs for RETRY actions
	logs, _ := logRepo.ListLogs(ctx, LogQuery{})
	retryCount := 0
	for _, l := range logs {
		if l.Action == "RETRY" {
			retryCount++
		}
	}
	if retryCount != 2 {
		t.Errorf("expected 2 RETRY log entries, got %d", retryCount)
	}
}

func TestFlowExecutor_Cancel(t *testing.T) {
	repo := NewMockRepository()
	logRepo := NewMockLogRepository()
	executor := NewFlowExecutor(repo, logRepo)

	ctx := context.Background()

	repo.CreateTask(ctx, &Task{ID: "T-1", Title: "Task 1", Status: StatusTodo})

	// Better yet, just test checkStatus directly.
	run := &FlowRun{ID: "test-run", Status: FlowStatusRunning}
	repo.CreateFlowRun(ctx, run)
	
	err := executor.checkStatus(ctx, run, "test")
	if err != nil {
		t.Errorf("checkStatus: expected nil err for Running, got %v", err)
	}

	run.Status = FlowStatusCanceled
	repo.UpdateFlowRun(ctx, run)
	err = executor.checkStatus(ctx, run, "test")
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Errorf("checkStatus: expected canceled error, got %v", err)
	}
}





func TestParseFlow_Validation(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "missing flow_id",
			input:   "name: Test",
			wantErr: "flow_id is required",
		},
		{
			name:    "missing entry_step",
			input:   "flow_id: test",
			wantErr: "entry_step is required",
		},
		{
			name:    "missing steps",
			input:   "flow_id: test\nentry_step: start",
			wantErr: "flow must contain at least one step",
		},
		{
			name:    "entry_step not found",
			input:   "flow_id: test\nentry_step: start\nsteps:\n  other: { step_id: other }",
			wantErr: "entry_step start not found in steps",
		},
		{
			name: "cycle detected",
			input: `
flow_id: test
entry_step: A
steps:
  A: { step_id: A, depends_on: [B] }
  B: { step_id: B, depends_on: [A] }
`,
			wantErr: "cycle detected",
		},
		{
			name: "missing dependency",
			input: `
flow_id: test
entry_step: A
steps:
  A: { step_id: A, depends_on: [C] }
`,
			wantErr: "depends on non-existent step C",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFlow(strings.NewReader(tt.input), "test.yaml")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ParseFlow() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}


