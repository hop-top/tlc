package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

const (
	testInvalid = "invalid"
	testPrompt  = "prompt"
)

func TestLoadConfig_Merging(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	projectRoot := filepath.Join(tmpDir, "project")
	os.MkdirAll(filepath.Join(projectRoot, ".tlc"), 0o755)

	projectConfigPath := filepath.Join(projectRoot, ".tlc", "config.yaml")
	projectConfigData := `
output:
  format: json
task:
  default_status: IN_PROGRESS
`
	if err := os.WriteFile(projectConfigPath, []byte(projectConfigData), 0o644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg, err := LoadConfig(projectRoot)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Output.Format != "json" {
		t.Errorf("expected output format json, got %s", cfg.Output.Format)
	}
	if cfg.Task.DefaultStatus != "IN_PROGRESS" {
		t.Errorf("expected default status IN_PROGRESS, got %s", cfg.Task.DefaultStatus)
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	cfg := DefaultConfig()
	os.Setenv("TLC_OUTPUT_FORMAT", "yaml")
	defer os.Unsetenv("TLC_OUTPUT_FORMAT")

	applyEnvOverrides(cfg)

	if cfg.Output.Format != "yaml" {
		t.Errorf("expected output format yaml from env, got %s", cfg.Output.Format)
	}
}

func TestLoadConfig_UsesOSUserConfigDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-config-userdir-*")
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

	configPath, err := UserConfigPath()
	if err != nil {
		t.Fatalf("failed to resolve user config path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configData := `
output:
  format: yaml
`
	if err := os.WriteFile(configPath, []byte(configData), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Output.Format != "yaml" {
		t.Fatalf("expected output format yaml, got %q", cfg.Output.Format)
	}
}

func TestPrepareViperForWrite_FallsBackToUserConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-config-write-*")
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

	v := viper.New()
	v.SetConfigFile(SystemConfigPath())

	target, err := PrepareViperForWrite(v)
	if err != nil {
		t.Fatalf("PrepareViperForWrite() error = %v", err)
	}

	want, err := UserConfigPath()
	if err != nil {
		t.Fatalf("failed to resolve user config path: %v", err)
	}
	if target != want {
		t.Fatalf("PrepareViperForWrite() = %q, want %q", target, want)
	}

	if _, err := os.Stat(filepath.Dir(target)); err != nil {
		t.Fatalf("expected config dir to exist: %v", err)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Config)
		wantErr bool
	}{
		{
			name:    "valid default config",
			setup:   func(_ *Config) {},
			wantErr: false,
		},
		{
			name: testInvalid + " output format",
			setup: func(c *Config) {
				c.Output.Format = testInvalid
			},
			wantErr: true,
		},
		{
			name: "missing github repo when enabled",
			setup: func(c *Config) {
				c.Sync.GitHub.Enabled = true
				c.Sync.GitHub.Repo = ""
			},
			wantErr: true,
		},
		{
			name: testInvalid + " storage backend",
			setup: func(c *Config) {
				c.Storage.Backend = testInvalid
			},
			wantErr: true,
		},
		{
			name: testInvalid + " fallback_mode",
			setup: func(c *Config) {
				c.Project.FallbackMode = testInvalid
			},
			wantErr: true,
		},
		{
			name: testInvalid + " duplicate_id_strategy",
			setup: func(c *Config) {
				c.Project.DuplicateIDStrategy = testInvalid
			},
			wantErr: true,
		},
		{
			name: "valid fallback_mode auto",
			setup: func(c *Config) {
				c.Project.FallbackMode = "auto"
			},
			wantErr: false,
		},
		{
			name: "valid fallback_mode detected",
			setup: func(c *Config) {
				c.Project.FallbackMode = "detected"
			},
			wantErr: false,
		},
		{
			name: "valid fallback_mode " + testPrompt,
			setup: func(c *Config) {
				c.Project.FallbackMode = testPrompt
			},
			wantErr: false,
		},
		{
			name: "valid duplicate_id_strategy share",
			setup: func(c *Config) {
				c.Project.DuplicateIDStrategy = "share"
			},
			wantErr: false,
		},
		{
			name: "valid duplicate_id_strategy unique",
			setup: func(c *Config) {
				c.Project.DuplicateIDStrategy = "unique"
			},
			wantErr: false,
		},
		{
			name: "valid duplicate_id_strategy " + testPrompt,
			setup: func(c *Config) {
				c.Project.DuplicateIDStrategy = testPrompt
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.setup(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTaskConfig_TerminalDefaultStatusRejected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Task.DefaultStatus = "DONE"
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for terminal default_status, got nil")
	}
}

func TestTaskConfig_MissingInitialRoleRejected(t *testing.T) {
	cfg := DefaultConfig()
	// Remove the "initial" role from all statuses
	for i := range cfg.Task.Statuses {
		if cfg.Task.Statuses[i].Role == "initial" {
			cfg.Task.Statuses[i].Role = "completed"
		}
	}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for missing initial role, got nil")
	}
}

func TestTaskConfig_MissingActiveRoleRejected(t *testing.T) {
	cfg := DefaultConfig()
	// Remove the "active" role from all statuses
	for i := range cfg.Task.Statuses {
		if cfg.Task.Statuses[i].Role == "active" {
			cfg.Task.Statuses[i].Role = "completed"
		}
	}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for missing active role, got nil")
	}
}

func TestTaskConfig_DuplicateTLSMarkerRejected(t *testing.T) {
	cfg := DefaultConfig()
	// Set two statuses to the same TLS marker
	cfg.Task.Statuses[0].TLSMarker = "x"
	cfg.Task.Statuses[2].TLSMarker = "x"
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for duplicate TLS marker, got nil")
	}
}

func TestTaskConfig_DuplicateStatusNameRejected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Task.Statuses = append(cfg.Task.Statuses, StatusDefinition{
		Name:       "TODO",
		Label:      "Duplicate Todo",
		IsTerminal: false,
		Role:       "initial",
		TLSMarker:  "?",
	})
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for duplicate status name, got nil")
	}
}

func TestTaskConfig_StateMachineUnknownStatusRejected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Task.StateMachine.Rules["NONEXISTENT"] = []string{"TODO"}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for state machine rule referencing unknown status, got nil")
	}
}

func TestTaskConfig_StateMachineUnknownTargetStatusRejected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Task.StateMachine.Rules["TODO"] = []string{"NONEXISTENT"}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for state machine rule referencing unknown target status, got nil")
	}
}

func TestTaskConfig_EmptyStatusesPopulatedWithDefaults(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Task.Statuses = nil
	cfg.Task.StateMachine = nil
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Task.Statuses) != 4 {
		t.Errorf("expected 4 default statuses, got %d", len(cfg.Task.Statuses))
	}
	if cfg.Task.StateMachine == nil {
		t.Error("expected state machine to be populated with defaults")
	}
}

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
