package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUserConfigPath_UsesOSConfigDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-config-ospath-*")
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

	got, err := UserConfigPath()
	if err != nil {
		t.Fatal(err)
	}

	switch runtime.GOOS {
	case "darwin":
		want := filepath.Join(
			homeDir, "Library", "Application Support", "tlc", "config.yaml",
		)
		if got != want {
			t.Fatalf("macOS: UserConfigPath() = %q, want %q", got, want)
		}
	case "linux":
		want := filepath.Join(homeDir, ".config", "tlc", "config.yaml")
		if got != want {
			t.Fatalf("linux: UserConfigPath() = %q, want %q", got, want)
		}
	default:
		if !strings.Contains(got, "tlc") {
			t.Fatalf("UserConfigPath() = %q, expected it to contain 'tlc'", got)
		}
	}
}

func TestUserConfigPath_RespectsXDGConfigHome(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-config-xdg-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	customConfig := filepath.Join(tmpDir, "custom-config")
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() { _ = os.Setenv("XDG_CONFIG_HOME", oldXDG) }()

	_ = os.Setenv("XDG_CONFIG_HOME", customConfig)

	got, err := UserConfigPath()
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(customConfig, "tlc", "config.yaml")
	if got != want {
		t.Fatalf("UserConfigPath() = %q, want %q", got, want)
	}
}

func TestUserConfigDir_XDGOverrideAndFallback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-config-xdgdir-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() { _ = os.Setenv("XDG_CONFIG_HOME", oldXDG) }()

	// With XDG_CONFIG_HOME set, should use it.
	xdgDir := filepath.Join(tmpDir, "xdg-conf")
	_ = os.Setenv("XDG_CONFIG_HOME", xdgDir)

	got, err := UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir() with XDG set: %v", err)
	}
	want := filepath.Join(xdgDir, "tlc")
	if got != want {
		t.Fatalf("UserConfigDir() = %q, want %q", got, want)
	}

	// With XDG_CONFIG_HOME unset, should fall back to OS default.
	_ = os.Unsetenv("XDG_CONFIG_HOME")

	got, err = UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir() without XDG: %v", err)
	}
	if got == want {
		t.Fatalf("UserConfigDir() should not return XDG path when unset")
	}
	if !strings.HasSuffix(got, "tlc") {
		t.Fatalf("UserConfigDir() = %q, expected suffix 'tlc'", got)
	}
}

func TestUserCacheDir_WithXDGCacheHome(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-cache-xdg-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldXDG := os.Getenv("XDG_CACHE_HOME")
	defer func() {
		if oldXDG == "" {
			_ = os.Unsetenv("XDG_CACHE_HOME")
		} else {
			_ = os.Setenv("XDG_CACHE_HOME", oldXDG)
		}
	}()

	customCache := filepath.Join(tmpDir, "custom-cache")
	_ = os.Setenv("XDG_CACHE_HOME", customCache)

	got, err := UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(customCache, "tlc")
	if got != want {
		t.Fatalf("UserCacheDir() = %q, want %q", got, want)
	}
}

func TestUserCacheDir_WithoutEnvVar(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-cache-native-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	homeDir := filepath.Join(tmpDir, "home")
	oldHome := os.Getenv("HOME")
	oldXDG := os.Getenv("XDG_CACHE_HOME")
	defer func() {
		_ = os.Setenv("HOME", oldHome)
		if oldXDG == "" {
			_ = os.Unsetenv("XDG_CACHE_HOME")
		} else {
			_ = os.Setenv("XDG_CACHE_HOME", oldXDG)
		}
	}()
	_ = os.Setenv("HOME", homeDir)
	_ = os.Unsetenv("XDG_CACHE_HOME")

	got, err := UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}

	switch runtime.GOOS {
	case "darwin":
		want := filepath.Join(homeDir, "Library", "Caches", "tlc")
		if got != want {
			t.Fatalf("macOS: UserCacheDir() = %q, want %q", got, want)
		}
	case "linux":
		want := filepath.Join(homeDir, ".cache", "tlc")
		if got != want {
			t.Fatalf("linux: UserCacheDir() = %q, want %q", got, want)
		}
	default:
		if !strings.Contains(got, "tlc") {
			t.Fatalf("UserCacheDir() = %q, expected it to contain 'tlc'", got)
		}
	}
}

func TestEnsureCacheDir_CreatesDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-cache-ensure-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldXDG := os.Getenv("XDG_CACHE_HOME")
	defer func() {
		if oldXDG == "" {
			_ = os.Unsetenv("XDG_CACHE_HOME")
		} else {
			_ = os.Setenv("XDG_CACHE_HOME", oldXDG)
		}
	}()

	customCache := filepath.Join(tmpDir, "new-cache")
	_ = os.Setenv("XDG_CACHE_HOME", customCache)

	got, err := EnsureCacheDir()
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(customCache, "tlc")
	if got != want {
		t.Fatalf("EnsureCacheDir() = %q, want %q", got, want)
	}

	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("expected cache dir to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %q to be a directory", got)
	}
}

func TestUserStateDir_WithXDGStateHome(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-state-xdg-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldXDG := os.Getenv("XDG_STATE_HOME")
	defer func() {
		if oldXDG == "" {
			_ = os.Unsetenv("XDG_STATE_HOME")
		} else {
			_ = os.Setenv("XDG_STATE_HOME", oldXDG)
		}
	}()

	customState := filepath.Join(tmpDir, "custom-state")
	_ = os.Setenv("XDG_STATE_HOME", customState)

	got, err := UserStateDir()
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(customState, "tlc")
	if got != want {
		t.Fatalf("UserStateDir() = %q, want %q", got, want)
	}
}

func TestUserStateDir_WithoutEnvVar(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-state-native-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	homeDir := filepath.Join(tmpDir, "home")
	oldHome := os.Getenv("HOME")
	oldXDG := os.Getenv("XDG_STATE_HOME")
	defer func() {
		_ = os.Setenv("HOME", oldHome)
		if oldXDG == "" {
			_ = os.Unsetenv("XDG_STATE_HOME")
		} else {
			_ = os.Setenv("XDG_STATE_HOME", oldXDG)
		}
	}()
	_ = os.Setenv("HOME", homeDir)
	_ = os.Unsetenv("XDG_STATE_HOME")

	got, err := UserStateDir()
	if err != nil {
		t.Fatal(err)
	}

	switch runtime.GOOS {
	case "darwin":
		want := filepath.Join(homeDir, "Library", "Application Support", "tlc", "state")
		if got != want {
			t.Fatalf("macOS: UserStateDir() = %q, want %q", got, want)
		}
	case "linux":
		want := filepath.Join(homeDir, ".local", "state", "tlc")
		if got != want {
			t.Fatalf("linux: UserStateDir() = %q, want %q", got, want)
		}
	default:
		if !strings.Contains(got, "tlc") {
			t.Fatalf("UserStateDir() = %q, expected it to contain 'tlc'", got)
		}
	}
}

func TestEnsureStateDir_CreatesDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-state-ensure-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldXDG := os.Getenv("XDG_STATE_HOME")
	defer func() {
		if oldXDG == "" {
			_ = os.Unsetenv("XDG_STATE_HOME")
		} else {
			_ = os.Setenv("XDG_STATE_HOME", oldXDG)
		}
	}()

	customState := filepath.Join(tmpDir, "new-state")
	_ = os.Setenv("XDG_STATE_HOME", customState)

	got, err := EnsureStateDir()
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(customState, "tlc")
	if got != want {
		t.Fatalf("EnsureStateDir() = %q, want %q", got, want)
	}

	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("expected state dir to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %q to be a directory", got)
	}
}

func TestWritableConfigPath_ReturnsExplicitNonSystemPath(t *testing.T) {
	explicit := "/tmp/my-project/.tlc/config.yaml"
	got, err := WritableConfigPath(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if got != explicit {
		t.Fatalf("WritableConfigPath(%q) = %q, want same", explicit, got)
	}
}

func TestWritableConfigPath_FallsBackFromSystemToUser(t *testing.T) {
	got, err := WritableConfigPath(SystemConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if got == SystemConfigPath() {
		t.Fatalf("WritableConfigPath(system) should not return system path, got %q", got)
	}
	want, _ := UserConfigPath()
	if got != want {
		t.Fatalf("WritableConfigPath(system) = %q, want %q", got, want)
	}
}

func TestWritableConfigPath_EmptyCurrentFallsBackToUser(t *testing.T) {
	got, err := WritableConfigPath("")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := UserConfigPath()
	if got != want {
		t.Fatalf("WritableConfigPath('') = %q, want %q", got, want)
	}
}

func TestUserDataDir_RespectsXDGDataHome(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-data-xdg-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldXDG := os.Getenv("XDG_DATA_HOME")
	defer func() { _ = os.Setenv("XDG_DATA_HOME", oldXDG) }()

	_ = os.Setenv("XDG_DATA_HOME", tmpDir)
	got := UserDataDir()
	want := filepath.Join(tmpDir, "tlc")
	if got != want {
		t.Fatalf("UserDataDir() = %q, want %q", got, want)
	}
}

func TestUserDataDir_FallsBackToOSNative(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-data-native-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	homeDir := filepath.Join(tmpDir, "home")

	oldHome := os.Getenv("HOME")
	oldXDG := os.Getenv("XDG_DATA_HOME")
	defer func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("XDG_DATA_HOME", oldXDG)
	}()
	_ = os.Setenv("HOME", homeDir)
	_ = os.Unsetenv("XDG_DATA_HOME")

	got := UserDataDir()

	switch runtime.GOOS {
	case "darwin":
		want := filepath.Join(homeDir, "Library", "Application Support", "tlc")
		if got != want {
			t.Fatalf("macOS: UserDataDir() = %q, want %q", got, want)
		}
	case "linux":
		want := filepath.Join(homeDir, ".local", "share", "tlc")
		if got != want {
			t.Fatalf("linux: UserDataDir() = %q, want %q", got, want)
		}
	default:
		if !strings.Contains(got, "tlc") {
			t.Fatalf("UserDataDir() = %q, expected it to contain 'tlc'", got)
		}
	}
}
