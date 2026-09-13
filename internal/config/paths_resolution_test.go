package config

import (
	"path/filepath"
	"testing"
)

// --- TrackConfig.TracksDir ---

func TestTracksDir_DefaultWhenEmpty(t *testing.T) {
	tc := &TrackConfig{}
	if got := tc.TracksDir(); got != "tracks" {
		t.Errorf("TracksDir() = %q; want %q", got, "tracks")
	}
}

func TestTracksDir_CustomValue(t *testing.T) {
	tc := &TrackConfig{Dir: "docs/tracks"}
	if got := tc.TracksDir(); got != "docs/tracks" {
		t.Errorf("TracksDir() = %q; want %q", got, "docs/tracks")
	}
}

func TestTracksDir_AbsolutePathRejectedByValidation(t *testing.T) {
	abs := filepath.Join(string(filepath.Separator), "opt", "tracks")
	tc := &TrackConfig{Dir: abs}
	if err := tc.Validate(); err == nil {
		t.Error("Validate() should reject absolute tracks dir")
	}
}

// --- RecipeConfig.RecipeDir ---

func TestRecipeDir_DefaultWhenEmpty(t *testing.T) {
	fc := &RecipeConfig{}
	want := "" // no repo-relative default: an unset layer is skipped
	if got := fc.RecipeDir(); got != want {
		t.Errorf("RecipeDir() = %q; want %q", got, want)
	}
}

func TestRecipeDir_CustomValue(t *testing.T) {
	fc := &RecipeConfig{Dir: "workflows"}
	if got := fc.RecipeDir(); got != "workflows" {
		t.Errorf("RecipeDir() = %q; want %q", got, "workflows")
	}
}

func TestRecipeDir_AbsolutePathRejectedByValidation(t *testing.T) {
	abs := filepath.Join(string(filepath.Separator), "opt", "flows")
	fc := &RecipeConfig{Dir: abs}
	if err := fc.Validate(); err == nil {
		t.Error("Validate() should reject absolute flows dir")
	}
}

// --- RecipeConfig.AssigneesDirectory ---

func TestAssigneesDirectory_DefaultWhenEmpty(t *testing.T) {
	fc := &RecipeConfig{}
	want := filepath.Join("examples", "assignees")
	if got := fc.AssigneesDirectory(); got != want {
		t.Errorf("AssigneesDirectory() = %q; want %q", got, want)
	}
}

func TestAssigneesDirectory_CustomValue(t *testing.T) {
	fc := &RecipeConfig{AssigneesDir: "people"}
	if got := fc.AssigneesDirectory(); got != "people" {
		t.Errorf("AssigneesDirectory() = %q; want %q", got, "people")
	}
}

// --- InboxConfig.InboxDir ---

func TestInboxDir_DefaultWhenEmpty(t *testing.T) {
	ic := &InboxConfig{}
	if got := ic.InboxDir(); got != "inbox" {
		t.Errorf("InboxDir() = %q; want %q", got, "inbox")
	}
}

func TestInboxDir_CustomValue(t *testing.T) {
	ic := &InboxConfig{Dir: "incoming"}
	if got := ic.InboxDir(); got != "incoming" {
		t.Errorf("InboxDir() = %q; want %q", got, "incoming")
	}
}

// --- StorageConfig.DBFilePath ---

func TestDBFilePath_DefaultWhenEmpty(t *testing.T) {
	sc := &StorageConfig{}
	if got := sc.DBFilePath(); got != "db.sqlite" {
		t.Errorf("DBFilePath() = %q; want %q", got, "db.sqlite")
	}
}

func TestDBFilePath_CustomValue(t *testing.T) {
	sc := &StorageConfig{DBPath: "data/store.db"}
	if got := sc.DBFilePath(); got != "data/store.db" {
		t.Errorf("DBFilePath() = %q; want %q", got, "data/store.db")
	}
}

func TestDBFilePath_AbsolutePath(t *testing.T) {
	abs := filepath.Join(string(filepath.Separator), "var", "lib", "tlc.db")
	sc := &StorageConfig{DBPath: abs}
	if got := sc.DBFilePath(); got != abs {
		t.Errorf("DBFilePath() = %q; want %q", got, abs)
	}
}

// --- TaskConfig.ProjectionDirectory ---

func TestProjectionDirectory_DefaultWhenEmpty(t *testing.T) {
	tc := &TaskConfig{}
	if got := tc.ProjectionDirectory(); got != "tasks" {
		t.Errorf("ProjectionDirectory() = %q; want %q", got, "tasks")
	}
}

func TestProjectionDirectory_CustomValue(t *testing.T) {
	tc := &TaskConfig{ProjectionDir: "work-items"}
	if got := tc.ProjectionDirectory(); got != "work-items" {
		t.Errorf("ProjectionDirectory() = %q; want %q", got, "work-items")
	}
}

// --- TaskConfig.TodoFilePath ---

func TestTodoFilePath_DefaultWhenEmpty(t *testing.T) {
	tc := &TaskConfig{}
	if got := tc.TodoFilePath(); got != "todo.txt" {
		t.Errorf("TodoFilePath() = %q; want %q", got, "todo.txt")
	}
}

func TestTodoFilePath_CustomValue(t *testing.T) {
	tc := &TaskConfig{TodoFile: "TODO.md"}
	if got := tc.TodoFilePath(); got != "TODO.md" {
		t.Errorf("TodoFilePath() = %q; want %q", got, "TODO.md")
	}
}

// --- DefaultConfig integration ---

func TestDefaultConfig_PathMethodsReturnDefaults(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"TracksDir", cfg.Tracks.TracksDir(), "tracks"},
		{"RecipeDir", cfg.Recipe.RecipeDir(), ""},
		{"AssigneesDirectory", cfg.Recipe.AssigneesDirectory(), filepath.Join("examples", "assignees")},
		{"InboxDir", cfg.Storage.Inbox.InboxDir(), "inbox"},
		{"ProjectionDirectory", cfg.Task.ProjectionDirectory(), "tasks"},
		{"TodoFilePath", cfg.Task.TodoFilePath(), "todo.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q; want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

// --- Config with custom paths round-trip through Validate ---

func TestConfigValidate_CustomPathsPreserved(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Tracks.Dir = "custom/tracks"
	cfg.Recipe.Dir = "custom/flows"
	cfg.Recipe.AssigneesDir = "custom/assignees"
	cfg.Storage.Inbox.Dir = "custom/inbox"
	cfg.Task.ProjectionDir = "custom/projection"
	cfg.Task.TodoFile = "custom/todo.md"
	cfg.Storage.DBPath = "custom/db.sqlite"

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() with custom paths: %v", err)
	}

	if got := cfg.Tracks.TracksDir(); got != "custom/tracks" {
		t.Errorf("TracksDir() = %q after validate", got)
	}
	if got := cfg.Recipe.RecipeDir(); got != "custom/flows" {
		t.Errorf("RecipeDir() = %q after validate", got)
	}
	if got := cfg.Recipe.AssigneesDirectory(); got != "custom/assignees" {
		t.Errorf("AssigneesDirectory() = %q after validate", got)
	}
	if got := cfg.Storage.Inbox.InboxDir(); got != "custom/inbox" {
		t.Errorf("InboxDir() = %q after validate", got)
	}
	if got := cfg.Task.ProjectionDirectory(); got != "custom/projection" {
		t.Errorf("ProjectionDirectory() = %q after validate", got)
	}
	if got := cfg.Task.TodoFilePath(); got != "custom/todo.md" {
		t.Errorf("TodoFilePath() = %q after validate", got)
	}
	if got := cfg.Storage.DBFilePath(); got != "custom/db.sqlite" {
		t.Errorf("DBFilePath() = %q after validate", got)
	}
}

// --- Edge cases ---

func TestPathMethods_WhitespaceOnlyTreatedAsSet(t *testing.T) {
	// Whitespace-only strings are technically non-empty,
	// so they are returned as-is (user's responsibility).
	tc := &TrackConfig{Dir: "  "}
	if got := tc.TracksDir(); got != "  " {
		t.Errorf("TracksDir() = %q; want whitespace", got)
	}
}

func TestPathMethods_DotRelative(t *testing.T) {
	tc := &TrackConfig{Dir: "./my-tracks"}
	if got := tc.TracksDir(); got != "./my-tracks" {
		t.Errorf("TracksDir() = %q; want %q", got, "./my-tracks")
	}
}

func TestPathMethods_ParentRelative(t *testing.T) {
	fc := &RecipeConfig{Dir: "../shared/flows"}
	if got := fc.RecipeDir(); got != "../shared/flows" {
		t.Errorf("RecipeDir() = %q; want %q", got, "../shared/flows")
	}
}
