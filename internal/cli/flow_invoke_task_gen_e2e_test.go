package cli

// End-to-end tests for `flow invoke` task generation from templates.
//
// Verifies that invoking a flow with task_template steps creates tasks
// in the DB with correct fields, input variable substitution, and flow
// metadata. Covers US-078 acceptance scenarios.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

// TestFlowInvoke_TaskGeneration_HappyPath verifies that invoking a flow
// with 3 task-template steps and --var sprint_name=S42 creates exactly
// 3 tasks with substituted titles/descriptions and correct flow metadata.
//
// Story: 078 — Flow Invoke Generates Tasks (scenarios 1, 2, 3)
// Fixture: examples/flows/task-generation.yaml
func TestFlowInvoke_TaskGeneration_HappyPath(t *testing.T) {
	withTestLock(func() {
		tmpDir, _ := setupProjectScopedTestDir(t, "tlc-flow-taskgen-", "test/project")

		flowPath := filepath.Join(tmpDir, "task-generation.yaml")
		flowYAML := `flow_id: "flow:task-generation:1.0"
name: "Sprint Task Generation"
version: "1.0"
entry_step: "define-scope"
config:
  inputs:
    sprint_name:
      description: "Sprint identifier"
      required: true
steps:
  define-scope:
    step_id: "define-scope"
    type: "task"
    title: "Define sprint scope for {{sprint_name}}"
    task_template:
      title: "Define scope for {{sprint_name}}"
      description: "Identify scope for sprint {{sprint_name}}."
      requirements:
        capabilities: ["planning"]
  implement-features:
    step_id: "implement-features"
    type: "task"
    title: "Implement features for {{sprint_name}}"
    depends_on: ["define-scope"]
    task_template:
      title: "Implement {{sprint_name}} features"
      description: "Build features for {{sprint_name}}."
      requirements:
        capabilities: ["coding"]
  validate-delivery:
    step_id: "validate-delivery"
    type: "task"
    title: "Validate delivery for {{sprint_name}}"
    depends_on: ["implement-features"]
    task_template:
      title: "Validate {{sprint_name}} delivery"
      description: "Run tests for {{sprint_name}}."
      requirements:
        capabilities: ["testing"]
`
		if err := os.WriteFile(flowPath, []byte(flowYAML), 0o644); err != nil {
			t.Fatalf("WriteFile flow yaml: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(FlowCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"flow", "invoke", flowPath, "--var", "sprint_name=S42"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("flow invoke: %v\noutput:\n%s", err, buf.String())
		}

		out := buf.String()
		if !contains(out, "invoked successfully") {
			t.Fatalf("expected success message; got:\n%s", out)
		}

		// Verify 3 tasks created.
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		tasks, err := s.ListTasks(context.Background(), core.Query{Limit: 20})
		if err != nil {
			t.Fatalf("ListTasks: %v", err)
		}
		if len(tasks) != 3 {
			t.Fatalf("expected 3 tasks, got %d", len(tasks))
		}

		// Verify substitution: no {{sprint_name}} literals remain.
		for _, task := range tasks {
			if strings.Contains(task.Title, "{{") {
				t.Errorf("task %s title has unsubstituted placeholder: %q",
					task.ID, task.Title)
			}
			if strings.Contains(task.Description, "{{") {
				t.Errorf("task %s description has unsubstituted placeholder: %q",
					task.ID, task.Description)
			}
			if !strings.Contains(task.Title, "S42") {
				t.Errorf("task %s title missing 'S42': %q", task.ID, task.Title)
			}
		}

		// Verify flow metadata on each task.
		for _, task := range tasks {
			if task.Meta == nil {
				t.Errorf("task %s has nil meta", task.ID)
				continue
			}
			flowID, _ := task.Meta["flow_id"].(string)
			if flowID != "flow:task-generation:1.0" {
				t.Errorf("task %s meta.flow_id = %q, want flow:task-generation:1.0",
					task.ID, flowID)
			}
			runID, _ := task.Meta["flow_run_id"].(string)
			if runID == "" {
				t.Errorf("task %s meta.flow_run_id is empty", task.ID)
			}
			stepID, _ := task.Meta["step_id"].(string)
			if stepID == "" {
				t.Errorf("task %s meta.step_id is empty", task.ID)
			}
		}
	})
}

// TestFlowInvoke_TaskGeneration_MissingInputFails verifies that omitting
// a required input produces an actionable error naming the missing var.
//
// Story: 078 — Flow Invoke Generates Tasks (scenario 4)
func TestFlowInvoke_TaskGeneration_MissingInputFails(t *testing.T) {
	withTestLock(func() {
		tmpDir, _ := setupProjectScopedTestDir(t, "tlc-flow-taskgen-miss-", "test/project")

		flowPath := filepath.Join(tmpDir, "task-generation.yaml")
		flowYAML := `flow_id: "flow:task-generation:1.0"
name: "Sprint Task Generation"
version: "1.0"
entry_step: "do"
config:
  inputs:
    sprint_name:
      description: "Sprint identifier"
      required: true
steps:
  do:
    step_id: "do"
    type: "task"
    title: "Do {{sprint_name}}"
    task_template:
      title: "Do {{sprint_name}}"
      description: "Work on {{sprint_name}}"
`
		if err := os.WriteFile(flowPath, []byte(flowYAML), 0o644); err != nil {
			t.Fatalf("WriteFile flow yaml: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(FlowCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"flow", "invoke", flowPath})

		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected error for missing required input; output:\n%s", buf.String())
		}
		if !strings.Contains(err.Error(), "sprint_name") {
			t.Errorf("error should mention missing 'sprint_name'; got: %v", err)
		}
	})
}
