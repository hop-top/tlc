package cli

// End-to-end tests for `flow invoke --var key=value` input substitution.
//
// Verifies that flow inputs declared in config.inputs can be passed via
// --var on the CLI and that {{var}} placeholders inside task_template
// fields are substituted before tasks are created.
//
// Without this wiring, track-scoped flows like examples/flows/track-plan-review.yaml
// store the literal "{{track_id}}" in task titles/descriptions, which is useless.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// TestFlowInvoke_VarInputs_SubstitutesTemplatePlaceholders verifies that
// `tlc flow invoke <flow> --var track_id=my-track` produces a task whose
// title/description has {{track_id}} replaced with "my-track".
func TestFlowInvoke_VarInputs_SubstitutesTemplatePlaceholders(t *testing.T) {
	withTestLock(func() {
		tmpDir, dbPath := setupProjectScopedTmpDir(t, "tlc-flow-var-inputs-")

		flowPath := filepath.Join(tmpDir, "track-flow.yaml")
		flowYAML := `flow_id: "flow:track-test:1.0"
name: "Track Test Flow"
version: "1.0"
entry_step: "review"
config:
  inputs:
    track_id:
      description: "Track to operate on"
      required: true
steps:
  review:
    step_id: "review"
    type: "task"
    title: "Review {{track_id}}"
    task_template:
      title: "Review track {{track_id}}"
      description: "Run review for track {{track_id}} and write output."
`
		if err := os.WriteFile(flowPath, []byte(flowYAML), 0o644); err != nil {
			t.Fatalf("WriteFile flow yaml: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(FlowCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"flow", "invoke", flowPath, "--var", "track_id=my-track"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("flow invoke --var track_id=my-track: %v\noutput:\n%s", err, buf.String())
		}

		out := buf.String()
		if !contains(out, "invoked successfully") {
			t.Fatalf("expected success message; got:\n%s", out)
		}

		// Verify the persisted task has the substituted title (not the literal {{track_id}}).
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		tasks, err := s.ListTasks(context.Background(), core.Query{Limit: 10})
		if err != nil {
			t.Fatalf("ListTasks: %v", err)
		}
		if len(tasks) != 1 {
			t.Fatalf("expected exactly 1 task created, got %d", len(tasks))
		}
		got := tasks[0]
		if strings.Contains(got.Title, "{{") {
			t.Errorf("task title still contains template placeholder: %q", got.Title)
		}
		if !strings.Contains(got.Title, "my-track") {
			t.Errorf("task title missing substituted value 'my-track': %q", got.Title)
		}
		if strings.Contains(got.Description, "{{") {
			t.Errorf("task description still contains template placeholder: %q", got.Description)
		}
		if !strings.Contains(got.Description, "my-track") {
			t.Errorf("task description missing substituted value 'my-track': %q", got.Description)
		}

		_ = tmpDir
		_ = dbPath
	})
}

// TestFlowInvoke_VarInputs_MissingRequiredFails verifies that omitting a
// required input causes flow invoke to fail with a clear actionable error
// — not crash deep in the executor with a "{{track_id}}" literal somewhere.
func TestFlowInvoke_VarInputs_MissingRequiredFails(t *testing.T) {
	withTestLock(func() {
		tmpDir, _ := setupProjectScopedTmpDir(t, "tlc-flow-var-required-")

		flowPath := filepath.Join(tmpDir, "needs-input.yaml")
		flowYAML := `flow_id: "flow:needs-input:1.0"
name: "Needs Input"
version: "1.0"
entry_step: "do"
config:
  inputs:
    track_id:
      description: "Required track id"
      required: true
steps:
  do:
    step_id: "do"
    type: "task"
    title: "Do {{track_id}}"
    task_template:
      title: "Do {{track_id}}"
      description: "Operate on {{track_id}}"
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
			t.Fatalf("expected error when required input missing; output:\n%s", buf.String())
		}
		if !strings.Contains(err.Error(), "track_id") {
			t.Errorf("expected error to mention missing input 'track_id'; got: %v", err)
		}
	})
}

// setupProjectScopedTmpDir mirrors the project-scoped setup used by
// TestFlowInvoke_ProjectScoped_CreatesTasksWithoutError: chdirs to a fresh
// tmpDir, writes .tlc/config.yaml with project.id, and wires viper so
// DetectProject() returns InProject=true.
//
// Returns (tmpDir, dbPath). Cleanup is registered via t.Cleanup.
func setupProjectScopedTmpDir(t *testing.T, prefix string) (string, string) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", prefix+"*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	tlcDir := filepath.Join(tmpDir, ".tlc")
	if err := os.MkdirAll(tlcDir, 0o755); err != nil {
		t.Fatalf("MkdirAll .tlc: %v", err)
	}
	projectCfgPath := filepath.Join(tlcDir, "config.yaml")
	dbPath := filepath.Join(tmpDir, "test.sqlite")
	projectCfg := "project:\n  id: test/project\nstorage:\n  backend: sqlite\n  db_path: " + dbPath + "\n"
	if err := os.WriteFile(projectCfgPath, []byte(projectCfg), 0o600); err != nil {
		t.Fatalf("WriteFile config.yaml: %v", err)
	}

	viper.Reset()
	core.ResetDetectionCache()
	dbSyncOnce = sync.Once{}
	touchOnce = sync.Once{}
	cfgFile = projectCfgPath
	flowRunVars = nil
	viper.Set("config", projectCfgPath)
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", dbPath)
	resetTaskFlags()
	t.Cleanup(func() {
		cfgFile = ""
		flowRunVars = nil
		viper.Reset()
		core.ResetDetectionCache()
		dbSyncOnce = sync.Once{}
		touchOnce = sync.Once{}
	})

	return tmpDir, dbPath
}
