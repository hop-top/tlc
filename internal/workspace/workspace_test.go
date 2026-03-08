package workspace

import (
	"context"
	"testing"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// fakeAdapter returns canned projects per URI for testing ListProjects.
type fakeAdapter struct {
	projects map[string][]core.RegisteredProject
}

func (a *fakeAdapter) Name() string      { return "fake" }
func (a *fakeAdapter) Schemes() []string { return []string{"fake"} }
func (a *fakeAdapter) Discover(space config.SpaceConfig) ([]core.RegisteredProject, error) {
	return a.projects[space.URI], nil
}
func (a *fakeAdapter) Contains(config.SpaceConfig, string) string { return "" }

// fakeProjectLister satisfies ProjectLister for the filesystem adapter.
type fakeProjectLister struct {
	projects map[string][]core.RegisteredProject
}

func (l *fakeProjectLister) ListProjectsBySpace(
	_ context.Context, spaceURI string,
) ([]core.RegisteredProject, error) {
	return l.projects[spaceURI], nil
}

func TestListProjects_AggregatesAcrossSpaces(t *testing.T) {
	lister := &fakeProjectLister{
		projects: map[string][]core.RegisteredProject{
			"/spaces/a": {
				{ProjectID: "a1", DBPath: "/a1/db"},
			},
			"/spaces/b": {
				{ProjectID: "b1", DBPath: "/b1/db"},
				{ProjectID: "b2", DBPath: "/b2/db"},
			},
		},
	}

	fs := NewFilesystemAdapter(lister)
	reg := NewRegistry()
	reg.Register(fs)

	ws := config.WorkspaceConfig{
		Name: "test",
		Spaces: []config.SpaceConfig{
			{URI: "/spaces/a"},
			{URI: "/spaces/b"},
		},
	}

	projects, err := ListProjects(ws, reg)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 3 {
		t.Fatalf("got %d projects, want 3", len(projects))
	}
}

func TestListProjects_SkipsUnresolvableSpaces(t *testing.T) {
	reg := NewRegistry()
	// No adapters registered.

	ws := config.WorkspaceConfig{
		Name: "test",
		Spaces: []config.SpaceConfig{
			{URI: "unknown://foo"},
		},
	}

	projects, err := ListProjects(ws, reg)
	if err == nil {
		t.Fatal("expected error for unresolvable space")
	}
	if len(projects) != 0 {
		t.Errorf("got %d projects, want 0", len(projects))
	}
}

func TestListProjects_MixedAdapters(t *testing.T) {
	lister := &fakeProjectLister{
		projects: map[string][]core.RegisteredProject{
			"/local": {{ProjectID: "local1"}},
		},
	}

	fake := &fakeAdapter{
		projects: map[string][]core.RegisteredProject{
			"fake://remote": {{ProjectID: "remote1"}},
		},
	}

	fs := NewFilesystemAdapter(lister)
	reg := NewRegistry()
	reg.Register(fs)
	reg.Register(fake)

	ws := config.WorkspaceConfig{
		Name: "mixed",
		Spaces: []config.SpaceConfig{
			{URI: "/local"},
			{URI: "fake://remote", Adapter: "fake"},
		},
	}

	projects, err := ListProjects(ws, reg)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("got %d projects, want 2", len(projects))
	}
}

func TestListProjects_EmptyWorkspace(t *testing.T) {
	reg := NewRegistry()
	ws := config.WorkspaceConfig{Name: "empty"}

	projects, err := ListProjects(ws, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("got %d projects, want 0", len(projects))
	}
}
