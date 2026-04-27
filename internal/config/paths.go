package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"hop.top/kit/go/core/xdg"
)

const (
	toolName        = "tlc"
	systemConfigDir = "/etc/tlc"
)

// UserDataDir returns the TLC data directory via kit/xdg.DataDir.
// Falls back to OS-native data directory when XDG_DATA_HOME is unset.
func UserDataDir() string {
	dir, err := xdg.DataDir(toolName)
	if err != nil {
		// Best-effort fallback — callers historically never checked error.
		return ""
	}
	return dir
}

// UserCacheDir returns the user-level cache directory for tlc
// via kit/xdg.CacheDir.
func UserCacheDir() (string, error) {
	return xdg.CacheDir(toolName)
}

// EnsureCacheDir returns the cache directory path, creating it if needed.
func EnsureCacheDir() (string, error) {
	dir, err := UserCacheDir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create cache directory: %w", err)
	}

	return dir, nil
}

// UserStateDir returns the user-level state directory for tlc
// via kit/xdg.StateDir.
func UserStateDir() (string, error) {
	return xdg.StateDir(toolName)
}

// EnsureStateDir returns the state directory path, creating it if needed.
func EnsureStateDir() (string, error) {
	dir, err := UserStateDir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create state directory: %w", err)
	}

	return dir, nil
}

// UserConfigDir returns the user-level config directory for tlc
// via kit/xdg.ConfigDir.
func UserConfigDir() (string, error) {
	return xdg.ConfigDir(toolName)
}

func UserConfigPath() (string, error) {
	dir, err := UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "config.yaml"), nil
}

func SystemConfigDir() string {
	return systemConfigDir
}

func SystemConfigPath() string {
	return filepath.Join(systemConfigDir, "config.yaml")
}

func WritableConfigPath(current string) (string, error) {
	if current != "" && !samePath(current, SystemConfigPath()) {
		return current, nil
	}

	return UserConfigPath()
}

func PrepareViperForWrite(v *viper.Viper) (string, error) {
	target, err := WritableConfigPath(v.ConfigFileUsed())
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}

	v.SetConfigFile(target)
	v.SetConfigType("yaml")

	return target, nil
}

func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)

	if abs, err := filepath.Abs(a); err == nil {
		a = abs
	}
	if abs, err := filepath.Abs(b); err == nil {
		b = abs
	}

	return a == b
}
