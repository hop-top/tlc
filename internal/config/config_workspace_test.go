package config

import (
	"strings"
	"testing"
)

func TestWorkspaceConfig_ValidPasses(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workspaces = []WorkspaceConfig{
		{
			Name: "dev",
			Spaces: []SpaceConfig{
				{URI: "/tmp/dev", Adapter: "filesystem"},
			},
			Default: true,
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid workspace config, got error: %v", err)
	}
}

func TestWorkspaceConfig_DuplicateNamesFail(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workspaces = []WorkspaceConfig{
		{Name: "dev", Spaces: []SpaceConfig{{URI: "/a"}}},
		{Name: "dev", Spaces: []SpaceConfig{{URI: "/b"}}},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate workspace names, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate workspace name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorkspaceConfig_EmptyNameFails(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workspaces = []WorkspaceConfig{
		{Name: "", Spaces: []SpaceConfig{{URI: "/a"}}},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty workspace name, got nil")
	}
	if !strings.Contains(err.Error(), "name must not be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorkspaceConfig_EmptySpaceURIFails(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workspaces = []WorkspaceConfig{
		{Name: "dev", Spaces: []SpaceConfig{{URI: ""}}},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty space URI, got nil")
	}
	if !strings.Contains(err.Error(), "URI must not be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorkspaceConfig_MultipleDefaultsFail(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workspaces = []WorkspaceConfig{
		{Name: "a", Default: true, Spaces: []SpaceConfig{{URI: "/a"}}},
		{Name: "b", Default: true, Spaces: []SpaceConfig{{URI: "/b"}}},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for multiple default workspaces, got nil")
	}
	if !strings.Contains(err.Error(), "at most one workspace can be default") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultWorkspace_ReturnsMarkedDefault(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workspaces = []WorkspaceConfig{
		{Name: "first", Spaces: []SpaceConfig{{URI: "/a"}}},
		{Name: "second", Default: true, Spaces: []SpaceConfig{{URI: "/b"}}},
	}
	ws := cfg.DefaultWorkspace()
	if ws == nil {
		t.Fatal("expected non-nil workspace")
	}
	if ws.Name != "second" {
		t.Fatalf("expected default workspace 'second', got %q", ws.Name)
	}
}

func TestDefaultWorkspace_ReturnsFirstWhenNoDefault(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workspaces = []WorkspaceConfig{
		{Name: "alpha", Spaces: []SpaceConfig{{URI: "/a"}}},
		{Name: "beta", Spaces: []SpaceConfig{{URI: "/b"}}},
	}
	ws := cfg.DefaultWorkspace()
	if ws == nil {
		t.Fatal("expected non-nil workspace")
	}
	if ws.Name != "alpha" {
		t.Fatalf("expected first workspace 'alpha', got %q", ws.Name)
	}
}

func TestDefaultWorkspace_ReturnsNilWhenEmpty(t *testing.T) {
	cfg := DefaultConfig()
	ws := cfg.DefaultWorkspace()
	if ws != nil {
		t.Fatalf("expected nil, got %+v", ws)
	}
}

func TestFindWorkspace_Found(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workspaces = []WorkspaceConfig{
		{Name: "dev", Spaces: []SpaceConfig{{URI: "/dev"}}},
		{Name: "prod", Spaces: []SpaceConfig{{URI: "/prod"}}},
	}
	ws := cfg.FindWorkspace("prod")
	if ws == nil {
		t.Fatal("expected non-nil workspace")
	}
	if ws.Name != "prod" {
		t.Fatalf("expected workspace 'prod', got %q", ws.Name)
	}
}

func TestFindWorkspace_NotFound(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workspaces = []WorkspaceConfig{
		{Name: "dev", Spaces: []SpaceConfig{{URI: "/dev"}}},
	}
	ws := cfg.FindWorkspace("missing")
	if ws != nil {
		t.Fatalf("expected nil, got %+v", ws)
	}
}

func TestInferAdapterFromURI(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want string
	}{
		{"/tmp/project", "/tmp/project", "filesystem"},
		{"file:///tmp/project", "file:///tmp/project", "filesystem"},
		{"./relative/path", "./relative/path", "filesystem"},
		{"https://example.com/repo", "https://example.com/repo", ""},
		{"s3://bucket/key", "s3://bucket/key", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferAdapterFromURI(tt.uri)
			if got != tt.want {
				t.Errorf("InferAdapterFromURI(%q) = %q, want %q", tt.uri, got, tt.want)
			}
		})
	}
}
