package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitHubPluginIntegration(t *testing.T) {
	wd, _ := os.Getwd()
	// Navigate from internal/plugin to root
	root := filepath.Join(wd, "..", "..")
	binPath := filepath.Join(root, "plugins", "github-sync", "bin", "github-sync")

	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		t.Skipf("GitHub plugin binary not found at %s, skipping integration test", binPath)
	}

	client, err := NewRPCClient(binPath)
	if err != nil {
		t.Fatalf("failed to start plugin: %v", err)
	}
	defer client.Close()

	// Test auth.status
	var authStatus struct {
		Authenticated bool   `json:"authenticated"`
		User          string `json:"user"`
	}
	err = client.Call("auth.status", map[string]interface{}{}, &authStatus)
	if err != nil {
		t.Errorf("auth.status failed: %v", err)
	}

	// Test sync.pull with invalid params (should return error)
	err = client.Call("sync.pull", map[string]interface{}{}, nil)
	if err == nil {
		t.Errorf("expected error for empty params in sync.pull, got nil")
	}
}
