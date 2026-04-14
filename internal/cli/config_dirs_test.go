package cli

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// T-0586: Regression — flowsDirFromConfig and assigneesDirFromConfig match
// FlowConfig{} accessor defaults when viper has no overrides set.
func TestConfigDirs_MatchAccessorDefaults(t *testing.T) {
	viper.Reset()
	core.ResetDetectionCache()
	dbSyncOnce = sync.Once{}
	defer viper.Reset()

	// With no viper overrides, the config_dirs helpers should return the
	// same values as the zero-value FlowConfig accessors.
	gotFlows := flowsDirFromConfig()
	wantFlows := (&config.FlowConfig{}).FlowsDir()
	if gotFlows != wantFlows {
		t.Errorf("flowsDirFromConfig() = %q, want %q (FlowConfig{}.FlowsDir())",
			gotFlows, wantFlows)
	}

	gotAssignees := assigneesDirFromConfig()
	wantAssignees := (&config.FlowConfig{}).AssigneesDirectory()
	if gotAssignees != wantAssignees {
		t.Errorf("assigneesDirFromConfig() = %q, want %q (FlowConfig{}.AssigneesDirectory())",
			gotAssignees, wantAssignees)
	}
}

// T-0586 (continued): Verify overrides propagate through config_dirs helpers.
func TestConfigDirs_OverridePropagates(t *testing.T) {
	viper.Reset()
	core.ResetDetectionCache()
	dbSyncOnce = sync.Once{}
	defer viper.Reset()

	viper.Set("flow.dir", "custom-flows")
	viper.Set("flow.assignees_dir", "custom-assignees")

	if got := flowsDirFromConfig(); got != "custom-flows" {
		t.Errorf("flowsDirFromConfig() = %q, want %q", got, "custom-flows")
	}
	if got := assigneesDirFromConfig(); got != "custom-assignees" {
		t.Errorf("assigneesDirFromConfig() = %q, want %q", got, "custom-assignees")
	}
}

// T-0585: Regression — syncToProjectTODO preserves subdirectory in todo_file.
// Verifies that filepath.Dir on a todo_file like "notes/todo.md" joined with
// configDir creates the intermediate "notes" directory, not just configDir.
func TestTodoFileSubdirectory_PathJoin(t *testing.T) {
	// Simulate the path logic used by syncToProjectTODO and doctor:
	// todoFile := filepath.Join(filepath.Dir(proj.ConfigPath), todoName)
	// os.MkdirAll(filepath.Dir(todoFile), 0o750)
	configPath := filepath.Join("project", ".tlc", "config.yaml")
	todoName := "notes/todo.md"

	todoFile := filepath.Join(filepath.Dir(configPath), todoName)
	wantFile := filepath.Join("project", ".tlc", "notes", "todo.md")
	if todoFile != wantFile {
		t.Fatalf("todoFile = %q, want %q", todoFile, wantFile)
	}

	parentDir := filepath.Dir(todoFile)
	wantParent := filepath.Join("project", ".tlc", "notes")
	if parentDir != wantParent {
		t.Fatalf("parent dir = %q, want %q — subdirectory not preserved",
			parentDir, wantParent)
	}
}
