package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestSyncCommands(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "tlc-sync-test-*")
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.sqlite")
	viper.Reset()
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", dbPath)

	t.Run("SyncConfig", func(t *testing.T) {
		viper.Set("sync.github.repo", "google/oss-tlc-cli")
		viper.Set("sync.github.direction", "both")

		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"sync", "config", "github"})

		if err := rootCmd.Execute(); err != nil {
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

		// This test will only work if we're in a valid git repo
		// If not in a git repo, auto-config should silently skip
		err := autoConfigureGitHub()

		// Should not error
		if err != nil {
			t.Errorf("autoConfigureGitHub should not error: %v", err)
		}

		// If we're in a GitHub repo, config should be set
		// If not, config should be empty (silent skip)
		repo := viper.GetString("sync.github.repo")
		direction := viper.GetString("sync.github.direction")

		// If repo is set, direction should be bidirectional
		if repo != "" && direction != "bidirectional" {
			t.Errorf("expected bidirectional direction, got: %s", direction)
		}
	})

	// Note: sync pull/push require external plugins/binaries which might not be available during unit tests.
	// We would need to mock the RPC client or ensure plugins are built.
}
