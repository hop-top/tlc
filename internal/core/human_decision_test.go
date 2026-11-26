package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

func humanGate(id string) *Task {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	return &Task{
		ID: id, Title: "sign-off", Status: StatusTodo, Kind: TaskKindHuman,
		RunID: "run_1", StepID: "gate", CreatedAt: now, UpdatedAt: now,
	}
}

func decisionLogs(repo *MockRepository, taskID string) []*LogEntry {
	var out []*LogEntry
	for _, l := range repo.logs {
		if l.TaskID == taskID {
			out = append(out, l)
		}
	}
	return out
}

func TestDecideHumanTask_ApproveRecordsDecision(t *testing.T) {
	repo := NewMockRepository()
	task := humanGate("task_g1")
	_ = repo.CreateTask(context.Background(), task)

	err := DecideHumanTask(context.Background(), repo, DefaultWorkflow(), task,
		HumanDecisionInput{Decision: HumanApproved, By: "lead", Note: "lgtm"})
	if err != nil {
		t.Fatalf("DecideHumanTask: %v", err)
	}

	if task.Status != StatusDone {
		t.Errorf("status = %q; want DONE", task.Status)
	}
	want := map[string]any{"decision": "approved", "by": "lead", "note": "lgtm"}
	for k, v := range want {
		if task.Result[k] != v {
			t.Errorf("result[%s] = %v; want %v (result %v)", k, task.Result[k], v, task.Result)
		}
	}
	logs := decisionLogs(repo, task.ID)
	if len(logs) != 1 || logs[0].Action != ActionApproved || logs[0].By != "lead" ||
		!strings.Contains(logs[0].Note, "approved: lgtm") {
		t.Errorf("log = %+v; want one APPROVED entry by lead noting approved: lgtm", logs)
	}
}

func TestDecideHumanTask_RejectBlocksAndRecordsDecision(t *testing.T) {
	repo := NewMockRepository()
	task := humanGate("task_g2")
	_ = repo.CreateTask(context.Background(), task)

	err := DecideHumanTask(context.Background(), repo, DefaultWorkflow(), task,
		HumanDecisionInput{Decision: HumanRejected, By: "lead", Reason: "tests missing"})
	if err != nil {
		t.Fatalf("DecideHumanTask: %v", err)
	}

	if task.Status != StatusTodo {
		t.Errorf("status = %q; reject must leave the task where it is", task.Status)
	}
	if task.BlockedReason == nil || *task.BlockedReason != "rejected: tests missing" {
		t.Errorf("blocked reason = %v; want %q", task.BlockedReason, "rejected: tests missing")
	}
	want := map[string]any{"decision": "rejected", "by": "lead", "reason": "tests missing"}
	for k, v := range want {
		if task.Result[k] != v {
			t.Errorf("result[%s] = %v; want %v (result %v)", k, task.Result[k], v, task.Result)
		}
	}
	logs := decisionLogs(repo, task.ID)
	if len(logs) != 1 || logs[0].Action != ActionRejected || logs[0].By != "lead" ||
		logs[0].Note != "rejected: tests missing" {
		t.Errorf("log = %+v; want one REJECTED entry by lead", logs)
	}
}

// The recorded decision is what a downstream when: reads.
func TestDecideHumanTask_DecisionReadableByWhen(t *testing.T) {
	repo := NewMockRepository()
	task := humanGate("task_g3")
	_ = repo.CreateTask(context.Background(), task)
	if err := DecideHumanTask(context.Background(), repo, DefaultWorkflow(), task,
		HumanDecisionInput{Decision: HumanApproved, By: "lead"}); err != nil {
		t.Fatalf("DecideHumanTask: %v", err)
	}

	env := CondEnv{Results: map[string]map[string]any{"gate": task.Result}, Vars: map[string]any{}}
	for expr, want := range map[string]bool{
		"results.gate.decision == 'approved'": true,
		"results.gate.decision == 'rejected'": false,
	} {
		got, err := EvalConditionEnv(expr, env)
		if err != nil || got != want {
			t.Errorf("%s = %v, %v; want %v", expr, got, err, want)
		}
	}
}

func TestDecideHumanTask_RefusesNonHumanTask(t *testing.T) {
	repo := NewMockRepository()
	task := humanGate("task_g4")
	task.Kind = TaskKindExec
	_ = repo.CreateTask(context.Background(), task)

	err := DecideHumanTask(context.Background(), repo, DefaultWorkflow(), task,
		HumanDecisionInput{Decision: HumanApproved, By: "lead"})
	if err == nil {
		t.Fatal("decided an exec task; want error")
	}
}

func TestDecideHumanTask_UnknownDecision(t *testing.T) {
	repo := NewMockRepository()
	task := humanGate("task_g5")
	_ = repo.CreateTask(context.Background(), task)

	err := DecideHumanTask(context.Background(), repo, DefaultWorkflow(), task,
		HumanDecisionInput{Decision: "maybe", By: "lead"})
	if err == nil {
		t.Fatal("accepted decision \"maybe\"; want error")
	}
}
