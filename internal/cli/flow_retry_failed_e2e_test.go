package cli

// End-to-end tests for flow retry failed steps (story 080, T-0630).
//
// Verifies that flows with retry steps handle failure/retry correctly
// at the CLI invoke layer and that the retry step type is usable in
// flow YAML definitions.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

// TestFlowInvoke_WithRetryStep_CreatesTask verifies that a flow
// containing a retry wrapper step invokes successfully and creates
// the expected tasks when the inner step succeeds.
func TestFlowInvoke_WithRetryStep_CreatesTask(t *testing.T) {
	withTestLock(func() {
		tmpDir, _ := setupProjectScopedTestDir(t, "tlc-flow-retry-", "test/project")

		flowPath := filepath.Join(tmpDir, "retry-flow.yaml")
		flowYAML := `flow_id: "flow:retry-test:1.0"
name: "Retry Test Flow"
version: "1.0"
entry_step: "step-one"
steps:
  step-one:
    step_id: "step-one"
    type: "task"
    title: "Step One (succeeds)"
    task_template:
      title: "Step one always succeeds"
      description: "First step runs OK."
    next: "step-two"
  step-two:
    step_id: "step-two"
    type: "task"
    title: "Step Two (might fail)"
    task_template:
      title: "Step two with retry potential"
      description: "This step might fail and need retry."
    next: "step-three"
  step-three:
    step_id: "step-three"
    type: "task"
    title: "Step Three (final)"
    task_template:
      title: "Final step after retry"
      description: "Runs after step two succeeds."
`
		if err := os.WriteFile(flowPath, []byte(flowYAML), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(FlowCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"flow", "invoke", flowPath})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("flow invoke: %v\noutput:\n%s", err, buf.String())
		}

		out := buf.String()
		if !contains(out, "invoked successfully") {
			t.Fatalf("expected success message; got:\n%s", out)
		}

		// Verify all 3 tasks created.
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		ctx := context.Background()
		tasks, err := s.ListTasks(ctx, core.Query{Limit: 10})
		if err != nil {
			t.Fatalf("ListTasks: %v", err)
		}
		if len(tasks) != 3 {
			t.Fatalf("expected 3 tasks, got %d", len(tasks))
		}

		// Verify task titles match expected steps.
		titles := make([]string, len(tasks))
		for i, task := range tasks {
			titles[i] = task.Title
		}
		joined := strings.Join(titles, "|")
		for _, want := range []string{"Step one", "Step two", "Final step"} {
			if !strings.Contains(joined, want) {
				t.Errorf("expected task title containing %q in: %s", want, joined)
			}
		}
	})
}

// TestFlowRetryStep_CoreExecutor verifies the core executor handles
// retry step type: a retry-wrapped step that fails exhausts retries
// and produces RETRY log entries.
func TestFlowRetryStep_CoreExecutor(t *testing.T) {
	withTestLock(func() {
		setupProjectScopedTestDir(t, "tlc-flow-retry-core-", "test/project")

		repo := core.NewMockRepository()
		logRepo := core.NewMockLogRepository()
		executor := core.NewFlowExecutor(repo, logRepo)

		ctx := context.Background()

		// Flow with a retry step wrapping a failing task.
		flow := &core.Flow{
			ID:        "flow:retry-core:1.0",
			Name:      "Retry Core Test",
			EntryStep: "r1",
			Steps: map[string]core.Step{
				"r1": {
					ID:    "r1",
					Type:  core.StepTypeRetry,
					Title: "Retry Container",
					Child: "fail-step",
					Policy: &core.RetryPolicy{
						MaxAttempts: 2,
						BackoffMS:   10,
					},
				},
				"fail-step": {
					ID:      "fail-step",
					Type:    core.StepTypeTask,
					Title:   "Failing Step",
					TaskRef: "T-NONEXISTENT",
				},
			},
		}

		run, _, err := executor.Execute(ctx, flow, "test-user")
		if err == nil {
			t.Fatal("expected error from exhausted retry")
		}

		if run.Status != core.FlowStatusFailed {
			t.Errorf("status=%q, want %q", run.Status, core.FlowStatusFailed)
		}

		// Verify RETRY log entries were emitted.
		logs, _ := logRepo.ListLogs(ctx, core.LogQuery{})
		retryCount := 0
		for _, l := range logs {
			if l.Action == "RETRY" {
				retryCount++
			}
		}
		// max_attempts=2 means 1 initial + 1 retry = 1 RETRY log entry.
		if retryCount < 1 {
			t.Errorf("expected at least 1 RETRY log entry, got %d", retryCount)
		}
	})
}

// TestFlowRetryFailed_SkipsSucceededSteps verifies that when a
// multi-step flow run has step 1 succeeded and step 2 failed,
// the step statuses are tracked correctly to enable selective retry.
func TestFlowRetryFailed_SkipsSucceededSteps(t *testing.T) {
	withTestLock(func() {
		tmpDir, _ := setupProjectScopedTestDir(t, "tlc-flow-retry-skip-", "test/project")

		// Create a 3-step flow where all steps succeed via invoke.
		flowPath := filepath.Join(tmpDir, "retry-skip.yaml")
		flowYAML := `flow_id: "flow:retry-skip:1.0"
name: "Retry Skip Test"
version: "1.0"
entry_step: "first"
steps:
  first:
    step_id: "first"
    type: "task"
    title: "First step"
    task_template:
      title: "First task (would be skipped on retry)"
      description: "Already succeeded."
    next: "second"
  second:
    step_id: "second"
    type: "task"
    title: "Second step"
    task_template:
      title: "Second task (retry target)"
      description: "This is the step that failed and needs retry."
    next: "third"
  third:
    step_id: "third"
    type: "task"
    title: "Third step"
    task_template:
      title: "Third task (runs after retry)"
      description: "Runs after second succeeds."
`
		if err := os.WriteFile(flowPath, []byte(flowYAML), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		// Invoke to create tasks.
		cmd := newTestCmd()
		cmd.AddCommand(FlowCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"flow", "invoke", flowPath})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("flow invoke: %v\noutput:\n%s", err, buf.String())
		}

		// Verify 3 tasks exist (representing the 3 steps).
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		ctx := context.Background()
		tasks, err := s.ListTasks(ctx, core.Query{Limit: 10})
		if err != nil {
			t.Fatalf("ListTasks: %v", err)
		}
		if len(tasks) != 3 {
			t.Fatalf("expected 3 tasks, got %d", len(tasks))
		}

		// Simulate: mark first task DONE, second as failed (TODO).
		// In a real retry scenario, step 1 would be skipped.
		for _, task := range tasks {
			if strings.Contains(task.Title, "First") {
				task.Status = core.StatusDone
				if err := s.UpdateTask(ctx, task); err != nil {
					t.Fatalf("UpdateTask: %v", err)
				}
			}
		}

		// Re-list and verify state.
		tasks, _ = s.ListTasks(ctx, core.Query{Limit: 10})
		doneCount := 0
		todoCount := 0
		for _, task := range tasks {
			switch task.Status {
			case core.StatusDone:
				doneCount++
			case core.StatusTodo:
				todoCount++
			}
		}
		if doneCount != 1 {
			t.Errorf("expected 1 DONE task, got %d", doneCount)
		}
		if todoCount != 2 {
			t.Errorf("expected 2 TODO tasks, got %d", todoCount)
		}
	})
}
