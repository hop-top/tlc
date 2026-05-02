package cli

import (
	"bytes"
	"testing"

	"hop.top/tlc/internal/core"
)

// TestFlexibleTaskIDResolution verifies that all supported ID shorthand forms
// resolve to the canonical task stored as T-0046.
func TestFlexibleTaskIDResolution(t *testing.T) {
	forms := []struct {
		name  string
		input string
	}{
		{"canonical", "T-0046"},
		{"at-prefix", "@T-0046"},
		{"bare-number", "46"},
		{"zero-padded", "0046"},
		{"T-no-pad", "T-46"},
	}

	for _, tc := range forms {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cleanup := setupTestDir(t)
			defer cleanup()
			s, _ := getStorageRaw()
			defer s.Close()

			s.CreateTask(ctx, &core.Task{
				ID:     "T-0046",
				Title:  "Flexible ID Test",
				Status: core.StatusTodo,
			})

			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"task", "show", tc.input})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("task show %q failed: %v", tc.input, err)
			}

			output := buf.String()
			if !contains(output, "Flexible ID Test") {
				t.Errorf("task show %q: expected title in output, got: %s", tc.input, output)
			}
		})
	}
}

// TestTypeIDTaskRefResolution verifies that CLI subcommands accept the
// durable task_<typeid> form and resolve it through core.ParseTaskRef.
// Covers task show + the T-NNNN alias path with a matching seq column.
func TestTypeIDTaskRefResolution(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	id := core.NewTaskID()
	if err := s.CreateTask(ctx, &core.Task{
		ID:     id,
		Title:  "TypeID Resolution Test",
		Status: core.StatusTodo,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// CreateTask auto-allocated a seq; reload to pick it up.
	stored, err := s.GetTask(ctx, id)
	if err != nil || stored == nil {
		t.Fatalf("reload task: %v", err)
	}
	alias := core.FormatTaskAlias(stored)

	cases := []struct {
		name  string
		input string
	}{
		{"typeid", id},
		{"alias-from-seq", alias},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"task", "show", tc.input})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("task show %q failed: %v", tc.input, err)
			}

			output := buf.String()
			if !contains(output, "TypeID Resolution Test") {
				t.Errorf("task show %q: expected title in output, got: %s", tc.input, output)
			}
		})
	}
}
