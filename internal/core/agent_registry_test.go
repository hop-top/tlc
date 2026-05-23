package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeYAML(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAgentRegistry_GlobalOnly(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "agents.yaml", `
agents:
  claude:
    image: ghcr.io/hop-top/pod-claude:latest
    binary: claude
    default_timeout: 30m
`)

	reg := NewAgentRegistry()
	if err := reg.LoadFromFile(path, true); err != nil {
		t.Fatal(err)
	}

	cfg, err := reg.Get("claude")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Image != "ghcr.io/hop-top/pod-claude:latest" {
		t.Errorf("image = %q, want ghcr.io/hop-top/pod-claude:latest", cfg.Image)
	}
	if cfg.Binary != "claude" {
		t.Errorf("binary = %q, want claude", cfg.Binary)
	}
	if cfg.DefaultTimeout != 30*time.Minute {
		t.Errorf("timeout = %v, want 30m", cfg.DefaultTimeout)
	}
}

func TestAgentRegistry_ProjectOnly(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, ".tlc/agents.yaml", `
agents:
  gemini:
    image: ghcr.io/hop-top/pod-gemini:latest
    binary: gemini
`)

	reg := NewAgentRegistry()
	if err := reg.LoadFromFile(path, false); err != nil {
		t.Fatal(err)
	}

	// Without trust, Get should return ErrTrustRequired.
	_, err := reg.Get("gemini")
	if err == nil {
		t.Fatal("expected ErrTrustRequired, got nil")
	}
	trustErr, ok := err.(*ErrTrustRequired)
	if !ok {
		t.Fatalf("expected *ErrTrustRequired, got %T: %v", err, err)
	}
	if trustErr.Path != path {
		t.Errorf("trust path = %q, want %q", trustErr.Path, path)
	}

	// After trust, Get should succeed.
	reg.TrustProject(reg.ProjectConfigPath())
	cfg, err := reg.Get("gemini")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Image != "ghcr.io/hop-top/pod-gemini:latest" {
		t.Errorf("image = %q", cfg.Image)
	}
}

func TestAgentRegistry_MergeOverride(t *testing.T) {
	dir := t.TempDir()
	globalPath := writeYAML(t, dir, "global.yaml", `
agents:
  claude:
    image: ghcr.io/hop-top/pod-claude:v1
    binary: claude
    env:
      FOO: bar
    default_timeout: 30m
`)
	projectPath := writeYAML(t, dir, ".tlc/agents.yaml", `
agents:
  claude:
    image: ghcr.io/custom/claude:v2
    env:
      BAZ: qux
    default_timeout: 1h
`)

	reg := NewAgentRegistry()
	if err := reg.LoadFromFile(globalPath, true); err != nil {
		t.Fatal(err)
	}
	if err := reg.LoadFromFile(projectPath, false); err != nil {
		t.Fatal(err)
	}
	reg.TrustProject(reg.ProjectConfigPath())

	cfg, err := reg.Get("claude")
	if err != nil {
		t.Fatal(err)
	}

	// Project overrides image + timeout.
	if cfg.Image != "ghcr.io/custom/claude:v2" {
		t.Errorf("image = %q, want ghcr.io/custom/claude:v2", cfg.Image)
	}
	// Binary comes from global (project didn't set it).
	if cfg.Binary != "claude" {
		t.Errorf("binary = %q, want claude", cfg.Binary)
	}
	if cfg.DefaultTimeout != time.Hour {
		t.Errorf("timeout = %v, want 1h", cfg.DefaultTimeout)
	}
	// Env merged: FOO from global, BAZ from project.
	if cfg.Env["FOO"] != "bar" {
		t.Errorf("env FOO = %q, want bar", cfg.Env["FOO"])
	}
	if cfg.Env["BAZ"] != "qux" {
		t.Errorf("env BAZ = %q, want qux", cfg.Env["BAZ"])
	}
}

func TestAgentRegistry_EnvExpansionGlobalOnly(t *testing.T) {
	t.Setenv("TEST_API_KEY", "secret123")

	dir := t.TempDir()
	globalPath := writeYAML(t, dir, "global.yaml", `
agents:
  claude:
    image: test
    env:
      API_KEY: "${TEST_API_KEY}"
`)
	projectPath := writeYAML(t, dir, ".tlc/agents.yaml", `
agents:
  claude:
    env:
      PROJECT_KEY: "${TEST_API_KEY}"
`)

	reg := NewAgentRegistry()
	if err := reg.LoadFromFile(globalPath, true); err != nil {
		t.Fatal(err)
	}
	if err := reg.LoadFromFile(projectPath, false); err != nil {
		t.Fatal(err)
	}
	reg.TrustProject(reg.ProjectConfigPath())

	cfg, err := reg.Get("claude")
	if err != nil {
		t.Fatal(err)
	}

	// Global env is expanded.
	if cfg.Env["API_KEY"] != "secret123" {
		t.Errorf("global env API_KEY = %q, want secret123", cfg.Env["API_KEY"])
	}
	// Project env is literal (NOT expanded).
	if cfg.Env["PROJECT_KEY"] != "${TEST_API_KEY}" {
		t.Errorf(
			"project env PROJECT_KEY = %q, want literal ${TEST_API_KEY}",
			cfg.Env["PROJECT_KEY"],
		)
	}
}

func TestAgentRegistry_MissingAgent(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "agents.yaml", `
agents:
  claude:
    image: test
`)

	reg := NewAgentRegistry()
	if err := reg.LoadFromFile(path, true); err != nil {
		t.Fatal(err)
	}

	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
	// Error should list available agents.
	if got := err.Error(); !strings.Contains(got, "claude") {
		t.Errorf("error = %q, should mention available agent 'claude'", got)
	}
}

func TestAgentRegistry_List(t *testing.T) {
	dir := t.TempDir()
	globalPath := writeYAML(t, dir, "global.yaml", `
agents:
  claude:
    image: test
  codex:
    image: test
`)
	projectPath := writeYAML(t, dir, ".tlc/agents.yaml", `
agents:
  gemini:
    image: test
  claude:
    image: override
`)

	reg := NewAgentRegistry()
	if err := reg.LoadFromFile(globalPath, true); err != nil {
		t.Fatal(err)
	}
	if err := reg.LoadFromFile(projectPath, false); err != nil {
		t.Fatal(err)
	}

	names := reg.List()
	want := []string{"claude", "codex", "gemini"}
	if len(names) != len(want) {
		t.Fatalf("List() = %v, want %v", names, want)
	}
	for i, n := range want {
		if names[i] != n {
			t.Errorf("List()[%d] = %q, want %q", i, names[i], n)
		}
	}
}

func TestAgentRegistry_TrustRequired(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, ".tlc/agents.yaml", `
agents:
  claude:
    image: test
`)

	reg := NewAgentRegistry()
	if err := reg.LoadFromFile(path, false); err != nil {
		t.Fatal(err)
	}

	_, err := reg.Get("claude")
	if err == nil {
		t.Fatal("expected trust error")
	}

	var trustErr *ErrTrustRequired
	if te, ok := err.(*ErrTrustRequired); !ok {
		t.Fatalf("expected *ErrTrustRequired, got %T", err)
	} else {
		trustErr = te
	}
	_ = trustErr
}
