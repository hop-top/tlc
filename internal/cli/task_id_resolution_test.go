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
