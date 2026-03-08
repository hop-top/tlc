package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// containsAdapter extends stubAdapter with a configurable Contains response.
type containsAdapter struct {
	stubAdapter
	containsMap map[string]string // localPath -> returned value
}

func (a *containsAdapter) Contains(_ config.SpaceConfig, localPath string) string {
	if a.containsMap == nil {
		return ""
	}
	return a.containsMap[localPath]
}

func TestFindCandidates_MatchByDBPath(t *testing.T) {
	tmp := t.TempDir()
	projDir := filepath.Join(tmp, "org", "myproject")
	dbPath := filepath.Join(projDir, ".tlc", "tasks.db")
	if err := os.MkdirAll(filepath.Join(projDir, ".tlc"), 0o755); err != nil {
		t.Fatal(err)
	}

	ws := config.WorkspaceConfig{Name: "test"}
	reg := NewRegistry()

	projects := []core.RegisteredProject{
		{ProjectID: "org/myproject", DBPath: dbPath, SpaceURI: tmp},
	}

	// cwd inside the project directory
	cwd := filepath.Join(projDir, "src")
	candidates := findCandidates(ws, reg, projects, cwd)

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].ProjectID != "org/myproject" {
		t.Errorf("expected org/myproject, got %s", candidates[0].ProjectID)
	}
}

func TestFindCandidates_MatchByDBPath_ExactDir(t *testing.T) {
	tmp := t.TempDir()
	projDir := filepath.Join(tmp, "proj")
	dbPath := filepath.Join(projDir, ".tlc", "tasks.db")
	if err := os.MkdirAll(filepath.Join(projDir, ".tlc"), 0o755); err != nil {
		t.Fatal(err)
	}

	ws := config.WorkspaceConfig{Name: "test"}
	reg := NewRegistry()

	projects := []core.RegisteredProject{
		{ProjectID: "proj", DBPath: dbPath},
	}

	// cwd exactly at the project directory
	candidates := findCandidates(ws, reg, projects, projDir)

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].ProjectID != "proj" {
		t.Errorf("expected proj, got %s", candidates[0].ProjectID)
	}
}

func TestFindCandidates_MatchByAdapter(t *testing.T) {
	tmp := t.TempDir()
	cwd := filepath.Join(tmp, "spacedir", "sub")

	adapter := &containsAdapter{
		stubAdapter: stubAdapter{name: "filesystem", schemes: []string{"file", ""}},
		containsMap: map[string]string{
			cwd: "spacedir/sub",
		},
	}

	reg := NewRegistry()
	reg.Register(adapter)

	ws := config.WorkspaceConfig{
		Name: "test",
		Spaces: []config.SpaceConfig{
			{URI: tmp, Label: "lab"},
		},
	}

	projects := []core.RegisteredProject{
		{ProjectID: "proj-a", SpaceURI: tmp},
		{ProjectID: "proj-b", SpaceURI: "/other"},
	}

	candidates := findCandidates(ws, reg, projects, cwd)

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].ProjectID != "proj-a" {
		t.Errorf("expected proj-a, got %s", candidates[0].ProjectID)
	}
}

func TestFindCandidates_NoMatch(t *testing.T) {
	ws := config.WorkspaceConfig{Name: "test"}
	reg := NewRegistry()

	projects := []core.RegisteredProject{
		{ProjectID: "proj-a", DBPath: "/some/other/path/.tlc/tasks.db"},
	}

	candidates := findCandidates(ws, reg, projects, "/totally/different")

	if len(candidates) != 0 {
		t.Fatalf("expected 0 candidates, got %d", len(candidates))
	}
}

func TestResolveTargetProject_NoProjects(t *testing.T) {
	ws := config.WorkspaceConfig{Name: "test"}
	reg := NewRegistry()

	_, err := ResolveTargetProject(ws, reg, nil, "/tmp")
	if err == nil {
		t.Fatal("expected error for empty projects")
	}
	if err.Error() != "no projects found in workspace" {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestResolveTargetProject_SingleCandidate(t *testing.T) {
	tmp := t.TempDir()
	projDir := filepath.Join(tmp, "myproj")
	dbPath := filepath.Join(projDir, ".tlc", "tasks.db")
	if err := os.MkdirAll(filepath.Join(projDir, ".tlc"), 0o755); err != nil {
		t.Fatal(err)
	}

	ws := config.WorkspaceConfig{Name: "test"}
	reg := NewRegistry()

	projects := []core.RegisteredProject{
		{ProjectID: "myproj", DBPath: dbPath},
	}

	result, err := ResolveTargetProject(ws, reg, projects, projDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ProjectID != "myproj" {
		t.Errorf("expected myproj, got %s", result.ProjectID)
	}
}

func TestFindCandidates_MultipleDBPathMatches_NoSingleWinner(t *testing.T) {
	tmp := t.TempDir()

	// Two projects share the same parent — ambiguous via db_path alone.
	proj1Dir := filepath.Join(tmp, "shared")
	proj2Dir := filepath.Join(tmp, "shared")
	db1 := filepath.Join(proj1Dir, ".tlc", "a.db")
	db2 := filepath.Join(proj2Dir, ".tlc", "b.db")

	ws := config.WorkspaceConfig{Name: "test"}
	reg := NewRegistry()

	projects := []core.RegisteredProject{
		{ProjectID: "proj-1", DBPath: db1},
		{ProjectID: "proj-2", DBPath: db2},
	}

	// Both match, so matchByDBPath returns nil (ambiguous).
	candidates := findCandidates(ws, reg, projects, filepath.Join(tmp, "shared", "sub"))

	// Falls through to adapter match which also finds nothing.
	if len(candidates) != 0 {
		t.Fatalf("expected 0 candidates (ambiguous), got %d", len(candidates))
	}
}

func TestIsSubpath(t *testing.T) {
	tests := []struct {
		child  string
		parent string
		want   bool
	}{
		{"/a/b/c", "/a/b", true},
		{"/a/b", "/a/b", true},
		{"/a/b", "/a/b/c", false},
		{"/a/bc", "/a/b", false},
		{"/x/y", "/a/b", false},
	}

	for _, tt := range tests {
		got := isSubpath(tt.child, tt.parent)
		if got != tt.want {
			t.Errorf("isSubpath(%q, %q) = %v, want %v",
				tt.child, tt.parent, got, tt.want)
		}
	}
}
