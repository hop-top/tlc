package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// TestTaskListEmptyJSONIsArray guards the empty-result JSON contract:
// a well-formed query matching zero rows must marshal to `[]`, never
// `null`. `null` breaks downstream iteration (`jq '.[]'` errors with
// "Cannot iterate over null").
func TestTaskListEmptyJSONIsArray(t *testing.T) {
	for _, format := range []string{formatJSON, formatYAML} {
		t.Run(format, func(t *testing.T) {
			_, cleanup := setupTestDir(t)
			defer cleanup()
			s, _ := getStorageRaw()
			defer s.Close()

			viper.Set("output.format", format)
			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"task", "list"})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("task list --format %s failed: %v", format, err)
			}

			got := strings.TrimSpace(buf.String())
			if format == formatJSON {
				if got != "[]" {
					t.Fatalf("empty task list must render [], got: %q", got)
				}
				var decoded []any
				if err := json.Unmarshal([]byte(got), &decoded); err != nil {
					t.Fatalf("output is not a JSON array: %v", err)
				}
			} else if got != "[]" {
				t.Fatalf("empty task list must render [], got: %q", got)
			}
		})
	}
}

// TestTaskShowEmptyLogsJSONIsArray guards the same contract on the
// nested `logs` field of `task show --format json` for a task that has
// no audit entries.
func TestTaskShowEmptyLogsJSONIsArray(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	if err := s.CreateTask(ctx, &core.Task{
		ID: "T-0001", Title: "No logs", Status: core.StatusTodo,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	logs, err := s.GetLogs(ctx, "T-0001", "asc")
	if err != nil {
		t.Fatalf("get logs: %v", err)
	}
	if len(logs) != 0 {
		t.Skipf("task creation wrote %d audit entries; empty-logs case not reachable", len(logs))
	}

	viper.Set("output.format", formatJSON)
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "show", "T-0001"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task show --format json failed: %v", err)
	}

	var decoded struct {
		Logs *[]any `json:"logs"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v (%s)", err, buf.String())
	}
	if decoded.Logs == nil {
		t.Fatalf("logs must render [] not null, got: %s", buf.String())
	}
}

// TestLogListEmptyJSONIsArray guards the contract on `tlc log` output.
func TestLogListEmptyJSONIsArray(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	viper.Set("output.format", formatJSON)
	cmd := newTestCmd()
	cmd.AddCommand(logCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"log", "--all"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("log --format json failed: %v", err)
	}

	got := strings.TrimSpace(buf.String())
	if got != "[]" {
		t.Fatalf("empty log list must render [], got: %q", got)
	}
}
