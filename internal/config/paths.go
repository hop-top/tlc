package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/viper"
)

const systemConfigDir = "/etc/tlc"

// UserDataDir returns the TLC data directory under $XDG_DATA_HOME/tlc.
// Falls back to OS-native data directory when XDG_DATA_HOME is unset.
func UserDataDir() string {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, _ := os.UserHomeDir()
		switch runtime.GOOS {
		case "darwin":
			dataHome = filepath.Join(home, "Library", "Application Support")
		case "windows":
			dataHome = filepath.Join(home, "AppData", "Local")
		default:
			dataHome = filepath.Join(home, ".local", "share")
		}
	}
	return filepath.Join(dataHome, "tlc")
}

// UserCacheDir returns the user-level cache directory for tlc.
// It checks $XDG_CACHE_HOME first; if unset, falls back to OS-native paths:
//   - macOS:   ~/Library/Caches/tlc
//   - Windows: %LocalAppData%/tlc/cache
//   - Linux:   ~/.cache/tlc
func UserCacheDir() (string, error) {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "tlc"), nil
	}

	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return filepath.Join(home, "Library", "Caches", "tlc"), nil
	case "windows":
		local := os.Getenv("LocalAppData")
		if local == "" {
			return "", fmt.Errorf("%%LocalAppData%% is not set")
		}
		return filepath.Join(local, "tlc", "cache"), nil
	default: // linux and other unix
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return filepath.Join(home, ".cache", "tlc"), nil
	}
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

// UserStateDir returns the user-level state directory for tlc.
// It checks $XDG_STATE_HOME first; if unset, falls back to OS-native paths:
//   - macOS:   ~/Library/Application Support/tlc/state
//   - Windows: %LocalAppData%/tlc/state
//   - Linux:   ~/.local/state/tlc
func UserStateDir() (string, error) {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "tlc"), nil
	}

	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return filepath.Join(home, "Library", "Application Support", "tlc", "state"), nil
	case "windows":
		local := os.Getenv("LocalAppData")
		if local == "" {
			return "", fmt.Errorf("%%LocalAppData%% is not set")
		}
		return filepath.Join(local, "tlc", "state"), nil
	default: // linux and other unix
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return filepath.Join(home, ".local", "state", "tlc"), nil
	}
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

func UserConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "tlc"), nil
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}

	return filepath.Join(dir, "tlc"), nil
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
