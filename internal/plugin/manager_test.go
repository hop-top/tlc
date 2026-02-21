package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManager_Discover(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-plugins-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a mock plugin
	p1Dir := filepath.Join(tmpDir, "test-plugin")
	os.MkdirAll(p1Dir, 0o755)
	manifestData := `
name: test-plugin
version: 1.0.0
type: external-sync
description: A test plugin
entry_point: ./test-plugin
`
	os.WriteFile(filepath.Join(p1Dir, "manifest.yaml"), []byte(manifestData), 0o644)

	manager := NewManager(tmpDir)
	if err := manager.Discover(); err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	plugins := manager.List()
	if len(plugins) != 1 {
		t.Errorf("expected 1 plugin, got %d", len(plugins))
	}

	if plugins[0].Name != "test-plugin" {
		t.Errorf("expected plugin name test-plugin, got %s", plugins[0].Name)
	}
}
