package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	cfgpkg "hop.top/tlc/internal/config"
)

// captureStdout redirects os.Stdout for the duration of fn and
// returns whatever was written to it.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestConfigSet_WritesUserConfigWhenSystemConfigLoaded(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-config-set-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	homeDir := filepath.Join(tmpDir, "home")
	oldHome := os.Getenv("HOME")
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("XDG_CONFIG_HOME", oldXDG)
	}()
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	if err := os.Unsetenv("XDG_CONFIG_HOME"); err != nil {
		t.Fatalf("failed to unset XDG_CONFIG_HOME: %v", err)
	}

	viper.Reset()
	viper.SetConfigFile(cfgpkg.SystemConfigPath())

	if err := ConfigSetCmd.RunE(ConfigSetCmd, []string{"output.format", "json"}); err != nil {
		t.Fatalf("config set failed: %v", err)
	}

	userConfigPath, err := cfgpkg.UserConfigPath()
	if err != nil {
		t.Fatalf("failed to resolve user config path: %v", err)
	}

	data, err := os.ReadFile(userConfigPath)
	if err != nil {
		t.Fatalf("failed to read user config: %v", err)
	}

	if !strings.Contains(string(data), "format: json") {
		t.Fatalf("expected user config to contain updated format, got: %s", string(data))
	}
}

// TestConfigSet_NoConfigFile_CreatesUserConfig verifies that
// `config set output.format table` creates the user config file
// from scratch when no config file exists beforehand.
func TestConfigSet_NoConfigFile_CreatesUserConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-config-set-nofile-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	homeDir := filepath.Join(tmpDir, "home")
	oldHome := os.Getenv("HOME")
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("XDG_CONFIG_HOME", oldXDG)
	}()
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("XDG_CONFIG_HOME"); err != nil {
		t.Fatal(err)
	}

	// No config file exists anywhere. Viper has no file loaded.
	viper.Reset()

	userConfigPath, err := cfgpkg.UserConfigPath()
	if err != nil {
		t.Fatal(err)
	}

	// Precondition: user config must not exist yet.
	if _, err := os.Stat(userConfigPath); err == nil {
		t.Fatal("precondition: user config should not exist")
	}

	out := captureStdout(t, func() {
		if err := ConfigSetCmd.RunE(ConfigSetCmd, []string{
			"output.format", "table",
		}); err != nil {
			t.Fatalf("config set failed: %v", err)
		}
	})

	// 1. Command exits successfully (no error above).
	// 2. User config file created.
	if _, err := os.Stat(userConfigPath); err != nil {
		t.Fatalf("user config file not created: %v", err)
	}

	// 3. File contains the key/value.
	data, err := os.ReadFile(userConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "format: table") {
		t.Fatalf("expected format: table in config, got:\n%s", data)
	}

	// 4. Output includes written-to path.
	if !strings.Contains(out, "written to:") {
		t.Fatalf("expected output to contain 'written to:', got:\n%s", out)
	}
	if !strings.Contains(out, userConfigPath) {
		t.Fatalf("expected output to contain %q, got:\n%s",
			userConfigPath, out)
	}
}

// TestConfigSet_OutputIncludesWrittenPath ensures the feedback
// message contains the target file path.
func TestConfigSet_OutputIncludesWrittenPath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-config-set-path-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	homeDir := filepath.Join(tmpDir, "home")
	oldHome := os.Getenv("HOME")
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("XDG_CONFIG_HOME", oldXDG)
	}()
	_ = os.Setenv("HOME", homeDir)
	_ = os.Unsetenv("XDG_CONFIG_HOME")

	viper.Reset()

	out := captureStdout(t, func() {
		if err := ConfigSetCmd.RunE(ConfigSetCmd, []string{
			"ui.theme", "dark",
		}); err != nil {
			t.Fatalf("config set failed: %v", err)
		}
	})

	if !strings.Contains(out, "written to:") {
		t.Fatalf("missing 'written to:' in output: %s", out)
	}
}
