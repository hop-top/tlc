package core

import (
	"testing"
)

// T-0590: Loading a project-local agents.yaml and calling Get()
// without TrustProject must return ErrTrustRequired.
func TestAgentRegistry_Get_NoTrust_ReturnsErrTrustRequired(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, ".tlc/agents.yaml", `
agents:
  copilot:
    image: ghcr.io/hop-top/pod-copilot:latest
    binary: copilot
`)

	reg := NewAgentRegistry()
	if err := reg.LoadFromFile(path, false); err != nil {
		t.Fatal(err)
	}

	// Get without TrustProject → ErrTrustRequired.
	_, err := reg.Get("copilot")
	if err == nil {
		t.Fatal("expected ErrTrustRequired, got nil")
	}

	trustErr, ok := err.(*ErrTrustRequired)
	if !ok {
		t.Fatalf("expected *ErrTrustRequired, got %T: %v", err, err)
	}
	if trustErr.Path != path {
		t.Errorf("trust error path = %q, want %q", trustErr.Path, path)
	}

	// After trust, Get must succeed.
	reg.TrustProject(reg.ProjectConfigPath())
	cfg, err := reg.Get("copilot")
	if err != nil {
		t.Fatalf("Get after trust: %v", err)
	}
	if cfg.Binary != "copilot" {
		t.Errorf("binary = %q, want copilot", cfg.Binary)
	}
}

// T-0590: Global-only agent does not require trust.
func TestAgentRegistry_Get_GlobalOnly_NoTrustNeeded(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "agents.yaml", `
agents:
  claude:
    image: test
    binary: claude
`)

	reg := NewAgentRegistry()
	if err := reg.LoadFromFile(path, true); err != nil {
		t.Fatal(err)
	}

	cfg, err := reg.Get("claude")
	if err != nil {
		t.Fatalf("global-only agent should not require trust: %v", err)
	}
	if cfg.Binary != "claude" {
		t.Errorf("binary = %q", cfg.Binary)
	}
}

// T-0590: Mixed global + project, project agent requires trust.
func TestAgentRegistry_Get_MixedConfig_ProjectRequiresTrust(t *testing.T) {
	dir := t.TempDir()
	globalPath := writeYAML(t, dir, "global.yaml", `
agents:
  claude:
    image: test-global
`)
	projectPath := writeYAML(t, dir, ".tlc/agents.yaml", `
agents:
  claude:
    image: test-project
`)

	reg := NewAgentRegistry()
	if err := reg.LoadFromFile(globalPath, true); err != nil {
		t.Fatal(err)
	}
	if err := reg.LoadFromFile(projectPath, false); err != nil {
		t.Fatal(err)
	}

	// Without trust, Get returns ErrTrustRequired even though
	// a global config exists — project config needs trust.
	_, err := reg.Get("claude")
	if err == nil {
		t.Fatal("expected ErrTrustRequired with untrusted project config")
	}
	if _, ok := err.(*ErrTrustRequired); !ok {
		t.Fatalf("expected *ErrTrustRequired, got %T", err)
	}
}
