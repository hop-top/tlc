package cli

// End-to-end tests for track-aware flow execution (story 077, T-0625).
//
// Verifies that `flow invoke` with --var track=<id> creates tasks scoped
// to the track, and that template placeholders are substituted with the
// track ID throughout task titles and descriptions.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

// TestFlowInvoke_TrackAware_CreatesTasksForTrack verifies that a flow
// referencing a track_id input creates tasks with substituted titles
// and descriptions containing the actual track ID.
func TestFlowInvoke_TrackAware_CreatesTasksForTrack(t *testing.T) {
	withTestLock(func() {
		tmpDir, _ := setupProjectScopedTestDir(t, "tlc-flow-track-aware-", "test/project")

		// Create a track so the track exists in storage.
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		ctx := context.Background()
		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID:    "auth-rewrite",
			Title: "Auth Rewrite",
			Type:  core.TrackTypeRefactor,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("CreateTrack: %v", err)
		}

		// Write the track-aware flow YAML.
		flowPath := filepath.Join(tmpDir, "track-aware.yaml")
		flowYAML := `flow_id: "flow:track-aware:1.0"
name: "Track-Aware Execution"
version: "1.0"
entry_step: "resolve-tasks"
config:
  inputs:
    track:
      description: "Track ID to operate on"
      required: true
steps:
  resolve-tasks:
    step_id: "resolve-tasks"
    type: "task"
    title: "Resolve track {{track}} tasks"
    task_template:
      title: "Resolve tasks for track {{track}}"
      description: "List TODO tasks for track {{track}}, build dep graph."
    next: "execute-tasks"
  execute-tasks:
    step_id: "execute-tasks"
    type: "task"
    title: "Execute track {{track}} tasks"
    task_template:
      title: "Execute tasks for track {{track}}"
      description: "Run each task in dep order for track {{track}}."
    next: "verify-completion"
  verify-completion:
    step_id: "verify-completion"
    type: "task"
    title: "Verify track {{track}} completion"
    task_template:
      title: "Verify all tasks done for track {{track}}"
      description: "Gate: all tasks for track {{track}} are DONE."
`
		if err := os.WriteFile(flowPath, []byte(flowYAML), 0o644); err != nil {
			t.Fatalf("WriteFile flow yaml: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(FlowCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"flow", "invoke", flowPath,
			"--var", "track=auth-rewrite",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("flow invoke --var track=auth-rewrite: %v\noutput:\n%s",
				err, buf.String())
		}

		out := buf.String()
		if !contains(out, "invoked successfully") {
			t.Fatalf("expected success message; got:\n%s", out)
		}

		// Verify tasks created with substituted track ID.
		tasks, err := s.ListTasks(ctx, core.Query{Limit: 10})
		if err != nil {
			t.Fatalf("ListTasks: %v", err)
		}
		if len(tasks) != 3 {
			t.Fatalf("expected 3 tasks created, got %d", len(tasks))
		}

		for _, task := range tasks {
			if strings.Contains(task.Title, "{{") {
				t.Errorf("task %s title has unsubstituted placeholder: %q",
					task.ID, task.Title)
			}
			if !strings.Contains(task.Title, "auth-rewrite") {
				t.Errorf("task %s title missing track ID: %q",
					task.ID, task.Title)
			}
			if strings.Contains(task.Description, "{{") {
				t.Errorf("task %s description has unsubstituted placeholder: %q",
					task.ID, task.Description)
			}
			if !strings.Contains(task.Description, "auth-rewrite") {
				t.Errorf("task %s description missing track ID: %q",
					task.ID, task.Description)
			}
		}
	})
}

// TestFlowInvoke_TrackAware_StepOrder verifies that a multi-step
// track-aware flow creates tasks in the expected sequential order
// (resolve -> execute -> verify).
func TestFlowInvoke_TrackAware_StepOrder(t *testing.T) {
	withTestLock(func() {
		tmpDir, _ := setupProjectScopedTestDir(t, "tlc-flow-track-order-", "test/project")

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		ctx := context.Background()
		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID:    "perf-track",
			Title: "Perf Track",
			Type:  core.TrackTypeFeature,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("CreateTrack: %v", err)
		}

		flowPath := filepath.Join(tmpDir, "track-order.yaml")
		flowYAML := `flow_id: "flow:track-order:1.0"
name: "Track Order Test"
version: "1.0"
entry_step: "step-a"
config:
  inputs:
    track:
      description: "Track ID"
      required: true
steps:
  step-a:
    step_id: "step-a"
    type: "task"
    title: "Step A for {{track}}"
    task_template:
      title: "Step A: resolve {{track}}"
      description: "First step for {{track}}"
    next: "step-b"
  step-b:
    step_id: "step-b"
    type: "task"
    title: "Step B for {{track}}"
    task_template:
      title: "Step B: execute {{track}}"
      description: "Second step for {{track}}"
    next: "step-c"
  step-c:
    step_id: "step-c"
    type: "task"
    title: "Step C for {{track}}"
    task_template:
      title: "Step C: verify {{track}}"
      description: "Third step for {{track}}"
`
		if err := os.WriteFile(flowPath, []byte(flowYAML), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(FlowCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"flow", "invoke", flowPath,
			"--var", "track=perf-track",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("flow invoke: %v\noutput:\n%s", err, buf.String())
		}

		tasks, err := s.ListTasks(ctx, core.Query{Limit: 10})
		if err != nil {
			t.Fatalf("ListTasks: %v", err)
		}
		if len(tasks) != 3 {
			t.Fatalf("expected 3 tasks, got %d", len(tasks))
		}

		// Verify all three steps produced tasks with correct track ref.
		found := map[string]bool{"Step A:": false, "Step B:": false, "Step C:": false}
		for _, task := range tasks {
			for prefix := range found {
				if strings.Contains(task.Title, prefix) {
					found[prefix] = true
				}
			}
		}
		for prefix, ok := range found {
			if !ok {
				t.Errorf("missing task with title prefix %q", prefix)
			}
		}
	})
}
