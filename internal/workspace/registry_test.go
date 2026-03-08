package workspace

import (
	"testing"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// stubAdapter is a minimal SpaceAdapter for testing registry resolution.
type stubAdapter struct {
	name    string
	schemes []string
}

func (a *stubAdapter) Name() string           { return a.name }
func (a *stubAdapter) Schemes() []string      { return a.schemes }
func (a *stubAdapter) Discover(config.SpaceConfig) ([]core.RegisteredProject, error) {
	return nil, nil
}
func (a *stubAdapter) Contains(config.SpaceConfig, string) string { return "" }

func TestRegistry_ResolveByName(t *testing.T) {
	reg := NewRegistry()
	fs := &stubAdapter{name: "filesystem", schemes: []string{"file", ""}}
	reg.Register(fs)

	got, err := reg.Resolve(config.SpaceConfig{
		URI:     "/some/path",
		Adapter: "filesystem",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name() != "filesystem" {
		t.Errorf("got adapter %q, want filesystem", got.Name())
	}
}

func TestRegistry_ResolveByInfer(t *testing.T) {
	reg := NewRegistry()
	fs := &stubAdapter{name: "filesystem", schemes: []string{"file", ""}}
	reg.Register(fs)

	// No explicit Adapter, bare path -> InferAdapterFromURI returns "filesystem".
	got, err := reg.Resolve(config.SpaceConfig{URI: "/home/user/code"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name() != "filesystem" {
		t.Errorf("got adapter %q, want filesystem", got.Name())
	}
}

func TestRegistry_ResolveByScheme(t *testing.T) {
	reg := NewRegistry()
	gh := &stubAdapter{name: "github", schemes: []string{"github"}}
	reg.Register(gh)

	got, err := reg.Resolve(config.SpaceConfig{URI: "github://hop-top"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name() != "github" {
		t.Errorf("got adapter %q, want github", got.Name())
	}
}

func TestRegistry_ResolveUnknown(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.Resolve(config.SpaceConfig{URI: "unknown://foo"})
	if err == nil {
		t.Fatal("expected error for unknown adapter")
	}
}

func TestRegistry_ResolveUnknownExplicit(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.Resolve(config.SpaceConfig{
		URI:     "/path",
		Adapter: "nosuchadapter",
	})
	if err == nil {
		t.Fatal("expected error for unknown explicit adapter")
	}
}
