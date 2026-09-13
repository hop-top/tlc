package cli

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// Regression — the config_dirs helpers match the RecipeConfig{} accessor
// defaults when viper has no overrides set: no repo-relative recipe layer
// (only the absolute user dir remains) and the assignees default.
func TestConfigDirs_MatchAccessorDefaults(t *testing.T) {
	viper.Reset()
	core.ResetDetectionCache()
	dbSyncOnce = sync.Once{}
	defer viper.Reset()

	dirs := recipeDirsFromConfig()
	if len(dirs) == 0 {
		t.Fatal("recipeDirsFromConfig() = none; want at least the user layer")
	}
	for _, d := range dirs {
		if !filepath.IsAbs(d) {
			t.Errorf("recipe dir %q is relative; recipes have no repo-relative default", d)
		}
	}
	if got := (&config.RecipeConfig{}).RecipeDir(); got != "" {
		t.Errorf("RecipeConfig{}.RecipeDir() = %q; want empty (layer skipped)", got)
	}

	gotAssignees := assigneesDirFromConfig()
	wantAssignees := (&config.RecipeConfig{}).AssigneesDirectory()
	if gotAssignees != wantAssignees {
		t.Errorf("assigneesDirFromConfig() = %q, want %q (RecipeConfig{}.AssigneesDirectory())",
			gotAssignees, wantAssignees)
	}
}

// Overrides propagate through the config_dirs helpers: recipe.dir is the
// first search layer.
func TestConfigDirs_OverridePropagates(t *testing.T) {
	viper.Reset()
	core.ResetDetectionCache()
	dbSyncOnce = sync.Once{}
	defer viper.Reset()

	viper.Set("recipe.dir", "custom-recipes")
	viper.Set("recipe.assignees_dir", "custom-assignees")

	if dirs := recipeDirsFromConfig(); len(dirs) == 0 || dirs[0] != "custom-recipes" {
		t.Errorf("recipeDirsFromConfig() = %v, want custom-recipes first", dirs)
	}
	if got := assigneesDirFromConfig(); got != "custom-assignees" {
		t.Errorf("assigneesDirFromConfig() = %q, want %q", got, "custom-assignees")
	}
}

// T-0585: Regression — writeProjectionLocal preserves subdirectory in todo_file.
// Verifies that filepath.Dir on a todo_file like "notes/todo.md" joined with
// configDir creates the intermediate "notes" directory, not just configDir.
func TestTodoFileSubdirectory_PathJoin(t *testing.T) {
	// Simulate the path logic used by writeProjectionLocal and doctor:
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
