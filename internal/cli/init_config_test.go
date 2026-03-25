package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// TestInitCmd_WithGlobalConfig tests init command with global configuration
// Verifies project-specific config doesn't interfere with global settings.
func TestInitCmd_WithGlobalConfig(t *testing.T) {
	tests := []struct {
		name               string
		setupGit           bool
		globalConfig       string
		checkGitignore     bool
		wantInGitignore    []string
		wantNotInGitignore []string
	}{
		{
			name:     "global config track: true",
			setupGit: true,
			globalConfig: `
git:
  track: true
`,
			checkGitignore:     true,
			wantInGitignore:    []string{".tlc/"},
			wantNotInGitignore: []string{},
		},
		{
			name:     "global config track: false",
			setupGit: true,
			globalConfig: `
git:
  track: false
`,
			checkGitignore:     true,
			wantInGitignore:    []string{},
			wantNotInGitignore: []string{".tlc/"},
		},
		{
			name:     "global config track: true - no git repo",
			setupGit: false,
			globalConfig: `
git:
  track: true
`,
			checkGitignore:     true,
			wantInGitignore:    []string{},
			wantNotInGitignore: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "tlc-init-global-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			oldWd, _ := os.Getwd()
			os.Chdir(tmpDir)
			defer os.Chdir(oldWd)

			viper.Reset()

			if tt.setupGit {
				if err := os.Mkdir(".git", 0o755); err != nil {
					t.Fatalf("failed to create .git: %v", err)
				}
			}

			homeDir := filepath.Join(tmpDir, "home")
			os.MkdirAll(filepath.Join(homeDir, ".config", "tlc"), 0o755)
			configPath := filepath.Join(homeDir, ".config", "tlc", "config.yaml")
			if err := os.WriteFile(configPath, []byte(tt.globalConfig), 0o644); err != nil {
				t.Fatalf("failed to write global config: %v", err)
			}

			t.Setenv("HOME", homeDir)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))

			viper.Reset()
			initConfig()

			cmd := newTestCmd()
			cmd.AddCommand(newTestInitCmd())

			buf := new(strings.Builder)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"init"})

			err = cmd.Execute()
			if err != nil {
				t.Fatalf("Execute() failed: %v", err)
			}

			if tt.checkGitignore && tt.setupGit {
				content, err := os.ReadFile(".gitignore")
				if err != nil {
					t.Fatalf("failed to read .gitignore: %v", err)
				}
				contentStr := string(content)

				for _, want := range tt.wantInGitignore {
					if !strings.Contains(contentStr, want) {
						t.Errorf(".gitignore does not contain expected entry %q. Got: %q", want, contentStr)
					}
				}
				for _, notWant := range tt.wantNotInGitignore {
					if strings.Contains(contentStr, notWant) {
						t.Errorf(".gitignore contains unexpected entry %q. Got: %q", notWant, contentStr)
					}
				}
			}
		})
	}
}

// TestInitCmd_ConfigStructure tests config file structure
// Verifies all required config sections are present.
func TestInitCmd_ConfigStructure(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-init-config-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	viper.Reset()

	cmd := newTestCmd()
	cmd.AddCommand(newTestInitCmd())

	buf := new(strings.Builder)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	configPath := filepath.Join(".tlc", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	requiredFields := []string{"version", "output", "storage", "git"}
	for _, field := range requiredFields {
		if _, ok := config[field]; !ok {
			t.Errorf("config missing required field %q", field)
		}
	}

	if output, ok := config["output"].(map[string]interface{}); ok {
		if format, ok := output["format"].(string); !ok || format != "table" {
			t.Errorf("expected output format table, got %v", output["format"])
		}
		if color, ok := output["color"].(bool); !ok || !color {
			t.Errorf("expected output color true, got %v", output["color"])
		}
	}

	if storage, ok := config["storage"].(map[string]interface{}); ok {
		if backend, ok := storage["backend"].(string); !ok || backend != "sqlite" {
			t.Errorf("expected storage backend sqlite, got %v", storage["backend"])
		}
	}

	if git, ok := config["git"].(map[string]interface{}); ok {
		if track, exists := git["track"]; exists {
			t.Errorf("track should not be in default config, got %v", track)
		}
	}
}

func runInitConfigTest(t *testing.T, args []string, wantFallback, wantDupStrategy string) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "tlc-init-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	viper.Reset()

	cmd := newTestCmd()
	cmd.AddCommand(newTestInitCmd())

	buf := new(strings.Builder)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	err = cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	configPath := filepath.Join(".tlc", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	if projectCfg, ok := config["project"].(map[string]interface{}); ok {
		if fallbackMode, ok := projectCfg["fallback_mode"].(string); !ok || fallbackMode != wantFallback {
			t.Errorf("expected fallback_mode %s, got %v", wantFallback, projectCfg["fallback_mode"])
		}
		if dupStrategy, ok := projectCfg["duplicate_id_strategy"].(string); !ok || dupStrategy != wantDupStrategy {
			t.Errorf("expected duplicate_id_strategy %s, got %v", wantDupStrategy, projectCfg["duplicate_id_strategy"])
		}
	} else {
		t.Error("project config section not found")
	}
}

func TestInitCmd_WithFallbackMode(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantFallback    string
		wantDupStrategy string
	}{
		{
			name:            "init with fallback-mode auto",
			args:            []string{"init", "--fallback-mode", "auto"},
			wantFallback:    "auto",
			wantDupStrategy: "share",
		},
		{
			name:            "init with fallback-mode detected",
			args:            []string{"init", "--fallback-mode", "detected"},
			wantFallback:    "detected",
			wantDupStrategy: "share",
		},
		{
			name:            "init with fallback-mode prompt",
			args:            []string{"init", "--fallback-mode", "prompt"},
			wantFallback:    "prompt",
			wantDupStrategy: "share",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runInitConfigTest(t, tt.args, tt.wantFallback, tt.wantDupStrategy)
		})
	}
}

func TestInitCmd_WithDuplicateIDStrategy(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantStrategy string
		wantFallback string
	}{
		{
			name:         "init with duplicate-id-strategy share",
			args:         []string{"init", "--duplicate-id-strategy", "share"},
			wantStrategy: "share",
			wantFallback: "auto",
		},
		{
			name:         "init with duplicate-id-strategy unique",
			args:         []string{"init", "--duplicate-id-strategy", "unique"},
			wantStrategy: "unique",
			wantFallback: "auto",
		},
		{
			name:         "init with duplicate-id-strategy prompt",
			args:         []string{"init", "--duplicate-id-strategy", "prompt"},
			wantStrategy: "prompt",
			wantFallback: "auto",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runInitConfigTest(t, tt.args, tt.wantFallback, tt.wantStrategy)
		})
	}
}

// TestInitCmd_ConfigStructureWithProject tests config includes project section
// Verifies project.id, project.fallback_mode, and project.duplicate_id_strategy exist.
func TestInitCmd_ConfigStructureWithProject(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-init-project-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	viper.Reset()

	cmd := newTestCmd()
	cmd.AddCommand(newTestInitCmd())

	buf := new(strings.Builder)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	configPath := filepath.Join(".tlc", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	requiredFields := []string{"version", "output", "project", "storage", "git"}
	for _, field := range requiredFields {
		if _, ok := config[field]; !ok {
			t.Errorf("config missing required field %q", field)
		}
	}

	if project, ok := config["project"].(map[string]interface{}); ok {
		if _, ok := project["id"]; !ok {
			t.Error("project.id is missing")
		}
		if _, ok := project["fallback_mode"]; !ok {
			t.Error("project.fallback_mode is missing")
		}
		if _, ok := project["duplicate_id_strategy"]; !ok {
			t.Error("project.duplicate_id_strategy is missing")
		}
	} else {
		t.Error("project config section not found")
	}
}
