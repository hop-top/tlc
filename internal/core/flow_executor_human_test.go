// Tests for FlowExecutor's type:human step wiring.
// Story 025 / T-0199.
//
// Author: jadb
package core

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func newHumanFlow() *Flow {
	return &Flow{
		ID:        "flow:test:human",
		Name:      "human test",
		EntryStep: "approve",
		Steps: map[string]Step{
			"approve": {
				ID:    "approve",
				Type:  StepTypeHuman,
				Title: "Approve send",
			},
		},
	}
}

func newRunningRun(t *testing.T, repo Repository, flowID string) *FlowRun {
	t.Helper()
	run := &FlowRun{
		ID:     "run:human-test",
		FlowID: flowID,
		Status: FlowStatusRunning,
	}
	if err := repo.CreateFlowRun(context.Background(), run); err != nil {
		t.Fatalf("create flow run: %v", err)
	}
	return run
}

func TestExecuteHumanStep_FailsWithoutStore(t *testing.T) {
	repo := NewMockRepository()
	logRepo := NewMockLogRepository()
	exec := NewFlowExecutor(repo, logRepo)

	flow := newHumanFlow()
	run := newRunningRun(t, repo, flow.ID)
	step := flow.Steps["approve"]

	_, err := exec.executeHumanStep(context.Background(), run, step)
	if err == nil {
		t.Fatal("want error when no approval store configured")
	}
	if !strings.Contains(err.Error(), "no approval store") {
		t.Errorf("got %q, want 'no approval store'", err.Error())
	}
}

func TestExecuteHumanStep_ApproveResolvesGate(t *testing.T) {
	repo := NewMockRepository()
	logRepo := NewMockLogRepository()
	store := NewFileApprovalStore(t.TempDir())
	exec := NewFlowExecutor(repo, logRepo).
		WithApprovalStore(store).
		WithApprovalPoll(10 * time.Millisecond)

	flow := newHumanFlow()
	run := newRunningRun(t, repo, flow.ID)
	step := flow.Steps["approve"]

	// Approve in a goroutine after a short delay.
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = store.Approve(context.Background(), run.ID, step.ID, "user:jad", "")
	}()

	output, err := exec.executeHumanStep(context.Background(), run, step)
	if err != nil {
		t.Fatalf("executeHumanStep: %v", err)
	}
	if got, _ := output["status"].(string); got != "approved" {
		t.Errorf("status = %v, want approved", output["status"])
	}
	if got, _ := output["by"].(string); got != "user:jad" {
		t.Errorf("by = %v, want user:jad", output["by"])
	}
}

func TestExecuteHumanStep_RejectFailsRun(t *testing.T) {
	repo := NewMockRepository()
	logRepo := NewMockLogRepository()
	store := NewFileApprovalStore(t.TempDir())
	exec := NewFlowExecutor(repo, logRepo).
		WithApprovalStore(store).
		WithApprovalPoll(10 * time.Millisecond)

	flow := newHumanFlow()
	run := newRunningRun(t, repo, flow.ID)
	step := flow.Steps["approve"]

	go func() {
		time.Sleep(30 * time.Millisecond)
		_ = store.Reject(context.Background(), run.ID, step.ID, "user:jad", "looks risky")
	}()

	_, err := exec.executeHumanStep(context.Background(), run, step)
	if err == nil {
		t.Fatal("want rejection error")
	}
	if !strings.Contains(err.Error(), "rejected") || !strings.Contains(err.Error(), "looks risky") {
		t.Errorf("err = %q, want 'rejected' + reason", err.Error())
	}
}

func TestExecuteHumanStep_CancelFailsRun(t *testing.T) {
	repo := NewMockRepository()
	logRepo := NewMockLogRepository()
	store := NewFileApprovalStore(t.TempDir())
	exec := NewFlowExecutor(repo, logRepo).
		WithApprovalStore(store).
		WithApprovalPoll(10 * time.Millisecond)

	flow := newHumanFlow()
	run := newRunningRun(t, repo, flow.ID)
	step := flow.Steps["approve"]

	go func() {
		time.Sleep(30 * time.Millisecond)
		_ = store.CancelRun(context.Background(), run.ID, "user:jad")
	}()

	_, err := exec.executeHumanStep(context.Background(), run, step)
	if err == nil {
		t.Fatal("want cancel error")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Errorf("err = %q, want 'canceled'", err.Error())
	}
}

func TestExecuteHumanStep_TimeoutOnApprove(t *testing.T) {
	repo := NewMockRepository()
	logRepo := NewMockLogRepository()
	store := NewFileApprovalStore(t.TempDir())
	exec := NewFlowExecutor(repo, logRepo).
		WithApprovalStore(store).
		WithApprovalPoll(10 * time.Millisecond)

	flow := &Flow{
		ID:        "flow:test:human-timeout",
		Name:      "human timeout",
		EntryStep: "approve",
		Steps: map[string]Step{
			"approve": {
				ID:    "approve",
				Type:  StepTypeHuman,
				Title: "Auto-approve",
				Human: &HumanStepConfig{
					Timeout:   "50ms",
					OnTimeout: HumanTimeoutApprove,
				},
			},
		},
	}
	run := newRunningRun(t, repo, flow.ID)
	step := flow.Steps["approve"]

	output, err := exec.executeHumanStep(context.Background(), run, step)
	if err != nil {
		t.Fatalf("executeHumanStep: %v", err)
	}
	if got, _ := output["status"].(string); got != "approved" {
		t.Errorf("status = %v, want approved (via timeout)", output["status"])
	}
	if got, _ := output["by"].(string); got != "timeout" {
		t.Errorf("by = %v, want timeout", output["by"])
	}
}

func TestExecuteHumanStep_TimeoutDefaultFails(t *testing.T) {
	repo := NewMockRepository()
	logRepo := NewMockLogRepository()
	store := NewFileApprovalStore(t.TempDir())
	exec := NewFlowExecutor(repo, logRepo).
		WithApprovalStore(store).
		WithApprovalPoll(10 * time.Millisecond)

	flow := &Flow{
		ID:        "flow:test:human-deadline",
		Name:      "human deadline",
		EntryStep: "approve",
		Steps: map[string]Step{
			"approve": {
				ID:    "approve",
				Type:  StepTypeHuman,
				Title: "No auto-approve",
				Human: &HumanStepConfig{
					Timeout: "30ms",
					// OnTimeout unset → no auto-approve, deadline fails the step.
				},
			},
		},
	}
	run := newRunningRun(t, repo, flow.ID)
	step := flow.Steps["approve"]

	_, err := exec.executeHumanStep(context.Background(), run, step)
	if err == nil {
		t.Fatal("want deadline-exceeded error when no on_timeout policy")
	}
	if !strings.Contains(err.Error(), "deadline") {
		t.Errorf("err = %q, want 'deadline'", err.Error())
	}
}

// End-to-end via runSequential: human step blocks the scheduler;
// approving via the store in another goroutine resumes it.
func TestRunSequential_HumanStep_ApproveResumes(t *testing.T) {
	repo := NewMockRepository()
	logRepo := NewMockLogRepository()
	store := NewFileApprovalStore(t.TempDir())
	exec := NewFlowExecutor(repo, logRepo).
		WithApprovalStore(store).
		WithApprovalPoll(10 * time.Millisecond)

	flow := newHumanFlow()
	run := newRunningRun(t, repo, flow.ID)

	statuses := map[string]StepStatus{"approve": StepStatusPending}

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = store.Approve(context.Background(), run.ID, "approve", "user:jad", "")
	}()

	stepOutputs := make(map[string]map[string]any)
	mu := &sync.Mutex{}
	isChild := make(map[string]bool)
	if err := exec.runSequential(context.Background(), flow, run, statuses, stepOutputs, mu, isChild, "test"); err != nil {
		t.Fatalf("runSequential: %v", err)
	}
	if statuses["approve"] != StepStatusSucceeded {
		t.Errorf("step status = %s, want succeeded", statuses["approve"])
	}
}
