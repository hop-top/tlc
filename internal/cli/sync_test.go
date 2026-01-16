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
	
	// Note: sync pull/push require external plugins/binaries which might not be available during unit tests.
	// We would need to mock the RPC client or ensure plugins are built.
}
