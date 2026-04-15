package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFlowGate_E2E_Pass verifies end-to-end: task → gated-step (pass) →
// final-step. When the EVA gateway returns pass, all three steps succeed
// and the flow run completes with status succeeded.
//
// Story: 077 — Gate Step with EVA Contract (scenario 1)
// Fixture: examples/flows/gate-checkpoint.yaml (pass)
func TestFlowGate_E2E_Pass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"eva_status": "pass",
			"attempts":   1,
		})
	}))
	defer srv.Close()

	repo := NewMockRepository()
	logRepo := NewMockLogRepository()
	ctx := context.Background()

	// Pre-create tasks for each step.
	if err := repo.CreateTask(ctx, &Task{ID: "T-gate-produce", Title: "Produce", Status: StatusTodo}); err != nil {
		t.Fatalf("CreateTask T-gate-produce: %v", err)
	}
	if err := repo.CreateTask(ctx, &Task{ID: "T-gate-check", Title: "Gate check", Status: StatusTodo}); err != nil {
		t.Fatalf("CreateTask T-gate-check: %v", err)
	}
	if err := repo.CreateTask(ctx, &Task{ID: "T-gate-final", Title: "Final", Status: StatusTodo}); err != nil {
		t.Fatalf("CreateTask T-gate-final: %v", err)
	}

	flow := &Flow{
		ID:        "flow:gate-checkpoint:1.0",
		Name:      "Gate Checkpoint",
		EntryStep: "produce-output",
		Steps: map[string]Step{
			"produce-output": {
				ID:      "produce-output",
				Type:    StepTypeTask,
				Title:   "Produce output",
				TaskRef: "T-gate-produce",
			},
			"gated-step": {
				ID:        "gated-step",
				Type:      StepTypeTask,
				Title:     "Quality gate checkpoint",
				TaskRef:   "T-gate-check",
				DependsOn: []string{"produce-output"},
				Gate: &StepGate{
					Contract: "quality-check",
					EvaURL:   srv.URL,
				},
			},
			"final-step": {
				ID:        "final-step",
				Type:      StepTypeTask,
				Title:     "Post-gate work",
				TaskRef:   "T-gate-final",
				DependsOn: []string{"gated-step"},
			},
		},
	}

	executor := NewFlowExecutor(repo, logRepo)
	run, _, err := executor.Execute(ctx, flow, "test-engineer")
	if err != nil {
		t.Fatalf("expected flow to succeed, got: %v", err)
	}
	if run.Status != FlowStatusSucceeded {
		t.Errorf("expected flow status succeeded, got %s", run.Status)
	}

	// All tasks should be DONE.
	for _, tid := range []string{"T-gate-produce", "T-gate-check", "T-gate-final"} {
		task, err := repo.GetTask(ctx, tid)
		if err != nil {
			t.Fatalf("GetTask %s: %v", tid, err)
		}
		if task == nil {
			t.Fatalf("task not found: %s", tid)
		}
		if task.Status != StatusDone {
			t.Errorf("task %s: expected DONE, got %s", tid, task.Status)
		}
	}

	// Verify audit trail includes gate-related step logs.
	logs, _ := logRepo.ListLogs(ctx, LogQuery{})
	var stepEndCount int
	for _, l := range logs {
		if l.Action == "STEP_END" {
			stepEndCount++
		}
	}
	if stepEndCount < 3 {
		t.Errorf("expected at least 3 STEP_END logs, got %d", stepEndCount)
	}
}

// TestFlowGate_E2E_Fail verifies end-to-end: task → gated-step (fail) →
// final-step never reached. When the EVA gateway returns contract_violation,
// the gated step fails, downstream steps are blocked, and the flow run
// completes with status failed.
//
// Story: 077 — Gate Step with EVA Contract (scenario 2)
// Fixture: examples/flows/gate-checkpoint.yaml (fail)
func TestFlowGate_E2E_Fail(t *testing.T) {
	reason := "missing required quality field"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"eva_status": "contract_violation",
			"attempts":   1,
			"violations": []map[string]any{
				{
					"evaluator": "contains",
					"score":     0.0,
					"reason":    reason,
				},
			},
		})
	}))
	defer srv.Close()

	repo := NewMockRepository()
	logRepo := NewMockLogRepository()
	ctx := context.Background()

	if err := repo.CreateTask(ctx, &Task{ID: "T-gf-produce", Title: "Produce", Status: StatusTodo}); err != nil {
		t.Fatalf("CreateTask T-gf-produce: %v", err)
	}
	if err := repo.CreateTask(ctx, &Task{ID: "T-gf-check", Title: "Gate check", Status: StatusTodo}); err != nil {
		t.Fatalf("CreateTask T-gf-check: %v", err)
	}
	if err := repo.CreateTask(ctx, &Task{ID: "T-gf-final", Title: "Final", Status: StatusTodo}); err != nil {
		t.Fatalf("CreateTask T-gf-final: %v", err)
	}

	flow := &Flow{
		ID:        "flow:gate-checkpoint:1.0",
		Name:      "Gate Checkpoint",
		EntryStep: "produce-output",
		Steps: map[string]Step{
			"produce-output": {
				ID:      "produce-output",
				Type:    StepTypeTask,
				Title:   "Produce output",
				TaskRef: "T-gf-produce",
			},
			"gated-step": {
				ID:        "gated-step",
				Type:      StepTypeTask,
				Title:     "Quality gate checkpoint",
				TaskRef:   "T-gf-check",
				DependsOn: []string{"produce-output"},
				Gate: &StepGate{
					Contract: "quality-check",
					EvaURL:   srv.URL,
				},
			},
			"final-step": {
				ID:        "final-step",
				Type:      StepTypeTask,
				Title:     "Post-gate work",
				TaskRef:   "T-gf-final",
				DependsOn: []string{"gated-step"},
			},
		},
	}

	executor := NewFlowExecutor(repo, logRepo)
	run, _, err := executor.Execute(ctx, flow, "test-engineer")

	// Flow must fail.
	if err == nil {
		t.Fatal("expected flow to fail due to gate violation")
	}
	if run.Status != FlowStatusFailed {
		t.Errorf("expected flow status failed, got %s", run.Status)
	}

	// Error should mention contract violation and reason.
	if !strings.Contains(err.Error(), "contract violation") {
		t.Errorf("error should mention contract violation: %v", err)
	}
	if !strings.Contains(err.Error(), reason) {
		t.Errorf("error should include violation reason: %v", err)
	}

	// Produce step should be DONE (it ran before the gate).
	produce, err := repo.GetTask(ctx, "T-gf-produce")
	if err != nil {
		t.Fatalf("GetTask T-gf-produce: %v", err)
	}
	if produce == nil {
		t.Fatal("task not found: T-gf-produce")
	}
	if produce.Status != StatusDone {
		t.Errorf("produce task: expected DONE, got %s", produce.Status)
	}

	// Final step should NOT be DONE (blocked by failed gate).
	final, err := repo.GetTask(ctx, "T-gf-final")
	if err != nil {
		t.Fatalf("GetTask T-gf-final: %v", err)
	}
	if final == nil {
		t.Fatal("task not found: T-gf-final")
	}
	if final.Status == StatusDone {
		t.Error("final task should not have completed after gate failure")
	}

	// Verify audit log includes gate rejection.
	logs, _ := logRepo.ListLogs(ctx, LogQuery{})
	var gateRejected bool
	for _, l := range logs {
		if strings.Contains(l.Note, "EVA gate rejected") {
			gateRejected = true
			break
		}
	}
	if !gateRejected {
		t.Error("expected audit log entry mentioning EVA gate rejection")
	}
}
