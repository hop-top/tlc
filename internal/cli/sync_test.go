package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// TestSyncCommands tests sync command configuration
// Tests sync config, GitHub auto-configuration, and direction upgrades.
func TestSyncCommands(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "tlc-sync-test-*")
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.sqlite")
	viper.Reset()
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", dbPath)

	t.Run("SyncConfig", func(t *testing.T) {
		viper.Set("sync.github.repo", "google/oss-tlc-cli")
		viper.Set("sync.github.sync_direction", "bidirectional")

		cmd := newTestCmd()
		cmd.AddCommand(SyncCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"sync", "config", "github", "--force"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("sync config failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "google/oss-tlc-cli") {
			t.Errorf("expected repo in output, got: %s", output)
		}
	})

	t.Run("GitHubAutoConfiguration", func(t *testing.T) {
		// Reset viper to test auto-configuration
		viper.Reset()
		viper.Set("storage.backend", "sqlite")
		viper.Set("storage.db_path", dbPath)

		// Remove any existing config file to avoid state leakage
		os.Remove(viper.ConfigFileUsed())

		// This test will only work if we're in a valid git repo
		// If not in a git repo, auto-config should silently skip
		err := autoConfigureGitHub("pull")
		// Should not error
		if err != nil {
			t.Errorf("autoConfigureGitHub should not error: %v", err)
		}

		// If we're in a GitHub repo, config should be set
		// If not, config should be empty (silent skip)
		repo := viper.GetString("sync.github.repo")
		direction := viper.GetString("sync.github.sync_direction")

		// If repo is set, direction should be pull (or bidirectional if already configured)
		if repo != "" && direction != "pull" && direction != "bidirectional" {
			t.Errorf("expected pull or bidirectional direction, got: %s", direction)
		}

		// Cleanup: remove config file if created
		if viper.ConfigFileUsed() != "" {
			os.Remove(viper.ConfigFileUsed())
		}
	})

	// Note: Testing that sync config fails without --force is difficult in unit tests
	// because autoConfigureGitHub may detect the actual git repo and override the values.
	// This behavior is tested manually in the integration test suite.

	t.Run("AutoConfigure direction upgrades", func(t *testing.T) {
		viper.Reset()
		viper.Set("storage.backend", "sqlite")
		viper.Set("storage.db_path", dbPath)

		// First configure as pull
		err := autoConfigureGitHub("pull")
		if err != nil {
			t.Errorf("initial autoConfigureGitHub failed: %v", err)
		}

		direction := viper.GetString("sync.github.sync_direction")
		if direction != "pull" {
			t.Errorf("expected pull direction, got: %s", direction)
		}

		// Then call with push, should upgrade to bidirectional
		err = autoConfigureGitHub("push")
		if err != nil {
			t.Errorf("upgrade autoConfigureGitHub failed: %v", err)
		}

		direction = viper.GetString("sync.github.sync_direction")
		if direction != "bidirectional" {
			t.Errorf("expected bidirectional direction after upgrade, got: %s", direction)
		}
	})

	t.Run("AutoConfigure skips when repo already set", func(t *testing.T) {
		viper.Reset()
		viper.Set("storage.backend", "sqlite")
		viper.Set("storage.db_path", dbPath)

		// Set a different repo to simulate already configured
		viper.Set("sync.github.repo", "different/repo")
		viper.Set("sync.github.sync_direction", "bidirectional")

		// Try to configure again, should skip due to different repo
		err := autoConfigureGitHub("pull")
		if err != nil {
			t.Errorf("autoConfigureGitHub should not error when different repo: %v", err)
		}

		// Repo should remain unchanged
		repo := viper.GetString("sync.github.repo")
		if repo != "different/repo" {
			t.Errorf("repo should remain unchanged, got: %s", repo)
		}
	})

	t.Run("AutoConfigure with bidirectional hint", func(t *testing.T) {
		viper.Reset()
		viper.Set("storage.backend", "sqlite")
		viper.Set("storage.db_path", dbPath)

		err := autoConfigureGitHub("bidirectional")
		if err != nil {
			t.Errorf("autoConfigureGitHub with bidirectional failed: %v", err)
		}

		repo := viper.GetString("sync.github.repo")
		direction := viper.GetString("sync.github.sync_direction")

		if repo == "" {
			t.Error("expected repo to be set")
		}

		if direction != "bidirectional" && direction != "pull" {
			t.Errorf("expected bidirectional or pull direction, got: %s", direction)
		}
	})

	// Note: sync pull/push require external plugins/binaries which might not be available during unit tests.
	// We would need to mock the RPC client or ensure plugins are built.
}

// TestAutoConfigureGitHubUsesDocumentedKey pins the direction key to the
// spelling the config spec documents.
//
// The key was written and read as `sync.github.direction` while every
// doc, the config struct and the interactive hints said
// `sync.github.sync_direction` — so a user following the spec set a key
// nothing read, and the key tlc did read appeared nowhere. Reading back
// through the undocumented spelling must find nothing.
func TestAutoConfigureGitHubUsesDocumentedKey(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	if err := autoConfigureGitHub("bidirectional"); err != nil {
		t.Fatalf("autoConfigureGitHub: %v", err)
	}
	if viper.GetString("sync.github.repo") == "" {
		t.Skip("not in a GitHub checkout; nothing was configured")
	}

	if got := viper.GetString("sync.github.sync_direction"); got == "" {
		t.Error("sync.github.sync_direction unset; the documented key must be the one written")
	}
	if got := viper.GetString("sync.github.direction"); got != "" {
		t.Errorf("sync.github.direction = %q; the undocumented key must not be written", got)
	}
}
