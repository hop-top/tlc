package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	cfgpkg "hop.top/tlc/internal/config"
)

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

	if err := configSetCmd.RunE(configSetCmd, []string{"output.format", "json"}); err != nil {
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
