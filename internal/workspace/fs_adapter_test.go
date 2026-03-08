package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// mockLister implements ProjectLister for testing.
type mockLister struct {
	projects map[string][]core.RegisteredProject
}

func (m *mockLister) ListProjectsBySpace(
	_ context.Context, spaceURI string,
) ([]core.RegisteredProject, error) {
	return m.projects[spaceURI], nil
}

func TestFilesystemAdapter_Name(t *testing.T) {
	a := NewFilesystemAdapter(nil)
	if a.Name() != "filesystem" {
		t.Errorf("got %q, want filesystem", a.Name())
	}
}

func TestFilesystemAdapter_Schemes(t *testing.T) {
	a := NewFilesystemAdapter(nil)
	schemes := a.Schemes()
	if len(schemes) != 2 || schemes[0] != "file" || schemes[1] != "" {
		t.Errorf("got schemes %v, want [file, \"\"]", schemes)
	}
}

func TestFilesystemAdapter_Contains(t *testing.T) {
	a := NewFilesystemAdapter(nil)

	// Use a temp dir so paths resolve cleanly.
	base := t.TempDir()
	sub := filepath.Join(base, "project", "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	space := config.SpaceConfig{URI: base}

	// Subdirectory should match.
	got := a.Contains(space, sub)
	want := filepath.Join("project", "src")
	if got != want {
		t.Errorf("Contains(%q) = %q, want %q", sub, got, want)
	}

	// Exact match should return ".".
	got = a.Contains(space, base)
	if got != "." {
		t.Errorf("Contains(%q) = %q, want \".\"", base, got)
	}

	// Outside path should return empty.
	got = a.Contains(space, "/completely/different")
	if got != "" {
		t.Errorf("Contains(outside) = %q, want empty", got)
	}
}

func TestFilesystemAdapter_ContainsFileURI(t *testing.T) {
	a := NewFilesystemAdapter(nil)
	base := t.TempDir()
	sub := filepath.Join(base, "repo")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	space := config.SpaceConfig{URI: "file://" + base}
	got := a.Contains(space, sub)
	if got != "repo" {
		t.Errorf("Contains with file:// URI = %q, want \"repo\"", got)
	}
}

func TestFilesystemAdapter_Discover(t *testing.T) {
	lister := &mockLister{
		projects: map[string][]core.RegisteredProject{
			"/spaces/lab": {
				{ProjectID: "p1", DBPath: "/spaces/lab/p1/.tlc/db.sqlite"},
				{ProjectID: "p2", DBPath: "/spaces/lab/p2/.tlc/db.sqlite"},
			},
		},
	}
	a := NewFilesystemAdapter(lister)

	projects, err := a.Discover(config.SpaceConfig{URI: "/spaces/lab"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("got %d projects, want 2", len(projects))
	}
}
