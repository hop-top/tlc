package config

import (
	"path/filepath"
	"testing"
)

// T-0584: Regression — all dir config fields reject absolute paths via Validate().
// Validates that tracks.dir, flow.dir, flow.assignees_dir, storage.inbox.dir,
// and task.projection_dir each reject absolute paths when Validate() is called
// on the top-level Config.
func TestValidateRelativePath_AllDirFieldsRejectAbsolute(t *testing.T) {
	fields := []struct {
		name  string
		setup func(*Config)
	}{
		{"tracks.dir", func(c *Config) { c.Tracks.Dir = "/abs/tracks" }},
		{"flow.dir", func(c *Config) { c.Flow.Dir = "/abs/flows" }},
		{"flow.assignees_dir", func(c *Config) { c.Flow.AssigneesDir = "/abs/people" }},
		{"storage.inbox.dir", func(c *Config) { c.Storage.Inbox.Dir = "/abs/inbox" }},
		{"task.projection_dir", func(c *Config) { c.Task.ProjectionDir = "/abs/tasks" }},
	}
	for _, tt := range fields {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.setup(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Errorf("Validate() accepted absolute path for %s; want error", tt.name)
			}
		})
	}
}

// T-0585: Regression — TodoFilePath preserves subdirectory.
// Verifying that setting task.todo_file to "notes/todo.md" resolves
// to configDir/notes/todo.md (preserving the subdirectory), not
// configDir/todo.md.
func TestTodoFilePath_SubdirectoryPreserved(t *testing.T) {
	tc := &TaskConfig{TodoFile: "notes/todo.md"}
	got := tc.TodoFilePath()
	if got != "notes/todo.md" {
		t.Fatalf("TodoFilePath() = %q, want %q", got, "notes/todo.md")
	}

	// When joined with a config dir, the subdirectory must be preserved.
	configDir := filepath.Join("some", "project", ".tlc")
	full := filepath.Join(configDir, got)
	want := filepath.Join("some", "project", ".tlc", "notes", "todo.md")
	if full != want {
		t.Fatalf("filepath.Join(configDir, TodoFilePath()) = %q, want %q", full, want)
	}

	// Verify the parent dir is notes/, not .tlc/ directly.
	parentDir := filepath.Dir(full)
	wantParent := filepath.Join("some", "project", ".tlc", "notes")
	if parentDir != wantParent {
		t.Fatalf("parent dir = %q, want %q", parentDir, wantParent)
	}
}

// T-0586: Regression — FlowConfig accessor defaults match zero-value struct.
// When viper has no overrides, flowsDirFromConfig() and
// assigneesDirFromConfig() (in the CLI layer) delegate to
// FlowConfig{}.FlowsDir() and FlowConfig{}.AssigneesDirectory().
// This test verifies the zero-value accessors return the expected defaults.
func TestFlowConfig_AccessorDefaults(t *testing.T) {
	fc := &FlowConfig{}

	gotFlows := fc.FlowsDir()
	wantFlows := filepath.Join("examples", "flows")
	if gotFlows != wantFlows {
		t.Errorf("FlowConfig{}.FlowsDir() = %q, want %q", gotFlows, wantFlows)
	}

	gotAssignees := fc.AssigneesDirectory()
	wantAssignees := filepath.Join("examples", "assignees")
	if gotAssignees != wantAssignees {
		t.Errorf("FlowConfig{}.AssigneesDirectory() = %q, want %q",
			gotAssignees, wantAssignees)
	}
}
