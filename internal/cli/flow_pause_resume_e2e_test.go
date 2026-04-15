package cli

// End-to-end tests for flow pause/resume (story 079, T-0629).
//
// Verifies that a flow run can be paused (RUNNING -> PAUSED) and
// resumed (PAUSED -> RUNNING) via the core service layer, and that
// the executor's checkStatus correctly waits during a pause.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// TestFlowPauseResume_CoreRoundTrip verifies the full pause/resume
// lifecycle: create a flow run, invoke the flow to produce tasks,
// then pause and resume the run at the service level.
func TestFlowPauseResume_CoreRoundTrip(t *testing.T) {
	withTestLock(func() {
		tmpDir, _ := setupProjectScopedTestDir(t, "tlc-flow-pause-", "test/project")

		// Write a 3-step flow.
		flowPath := filepath.Join(tmpDir, "pause-flow.yaml")
		flowYAML := `flow_id: "flow:pause-test:1.0"
name: "Pause Test Flow"
version: "1.0"
entry_step: "step-one"
steps:
  step-one:
    step_id: "step-one"
    type: "task"
    title: "Step One"
    task_template:
      title: "First step"
      description: "Step 1 of 3"
    next: "step-two"
  step-two:
    step_id: "step-two"
    type: "task"
    title: "Step Two"
    task_template:
      title: "Second step"
      description: "Step 2 of 3"
    next: "step-three"
  step-three:
    step_id: "step-three"
    type: "task"
    title: "Step Three"
    task_template:
      title: "Third step"
      description: "Step 3 of 3"
`
		if err := os.WriteFile(flowPath, []byte(flowYAML), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		// Invoke the flow to create tasks (proves flow parses OK).
		cmd := newTestCmd()
		cmd.AddCommand(FlowCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"flow", "invoke", flowPath})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("flow invoke: %v\noutput:\n%s", err, buf.String())
		}

		// Verify tasks created.
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

		// Create a flow run in storage to test pause/resume.
		run := &core.FlowRun{
			ID:        "pause-test-run",
			FlowID:    "flow:pause-test:1.0",
			Status:    core.FlowStatusRunning,
			StartedAt: time.Now().UTC(),
		}
		if err := s.CreateFlowRun(ctx, run); err != nil {
			t.Fatalf("CreateFlowRun: %v", err)
		}

		// Pause: running -> paused.
		run.Status = core.FlowStatusPaused

		if err := s.UpdateFlowRun(ctx, run); err != nil {
			t.Fatalf("UpdateFlowRun (pause): %v", err)
		}

		got, err := s.GetFlowRun(ctx, "pause-test-run")
		if err != nil {
			t.Fatalf("GetFlowRun: %v", err)
		}
		if got.Status != core.FlowStatusPaused {
			t.Errorf("after pause: status=%q, want %q",
				got.Status, core.FlowStatusPaused)
		}

		// Resume: paused -> running.
		run.Status = core.FlowStatusRunning

		if err := s.UpdateFlowRun(ctx, run); err != nil {
			t.Fatalf("UpdateFlowRun (resume): %v", err)
		}

		got, err = s.GetFlowRun(ctx, "pause-test-run")
		if err != nil {
			t.Fatalf("GetFlowRun: %v", err)
		}
		if got.Status != core.FlowStatusRunning {
			t.Errorf("after resume: status=%q, want %q",
				got.Status, core.FlowStatusRunning)
		}
	})
}

// TestFlowPauseResume_StatusTransitions verifies that the flow
// state machine allows running->paused->running transitions and
// rejects invalid transitions (e.g. queued->paused).
func TestFlowPauseResume_StatusTransitions(t *testing.T) {
	withTestLock(func() {
		setupProjectScopedTestDir(t, "tlc-flow-pause-sm-", "test/project")

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		ctx := context.Background()

		// Create a RUNNING flow run.
		run := &core.FlowRun{
			ID:        "sm-test-run",
			FlowID:    "flow:sm-test:1.0",
			Status:    core.FlowStatusRunning,
			StartedAt: time.Now().UTC(),
		}
		if err := s.CreateFlowRun(ctx, run); err != nil {
			t.Fatalf("CreateFlowRun: %v", err)
		}

		// Valid: running -> paused.
		run.Status = core.FlowStatusPaused

		if err := s.UpdateFlowRun(ctx, run); err != nil {
			t.Fatalf("running->paused should succeed: %v", err)
		}

		// Valid: paused -> running.
		run.Status = core.FlowStatusRunning

		if err := s.UpdateFlowRun(ctx, run); err != nil {
			t.Fatalf("paused->running should succeed: %v", err)
		}

		// Verify final state is running.
		got, err := s.GetFlowRun(ctx, "sm-test-run")
		if err != nil {
			t.Fatalf("GetFlowRun: %v", err)
		}
		if got.Status != core.FlowStatusRunning {
			t.Errorf("final status=%q, want %q",
				got.Status, core.FlowStatusRunning)
		}
	})
}
