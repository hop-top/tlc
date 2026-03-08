package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// TestInitCmd tests tlc init command functionality
// Tests basic init, --track flag, .gitignore handling, and config structure.
func TestInitCmd(t *testing.T) {
	tests := []struct {
		name               string
		setupGit           bool
		existingGitignore  string
		args               []string
		wantErr            bool
		checkGitignore     bool
		wantInGitignore    []string
		wantNotInGitignore []string
		validateConfig     func(*testing.T, string)
	}{
		{
			name:     "basic init",
			setupGit: false,
			args:     []string{"init"},
			wantErr:  false,
			validateConfig: func(t *testing.T, tlcDir string) {
				configPath := filepath.Join(tlcDir, "config.yaml")
				if _, err := os.Stat(configPath); os.IsNotExist(err) {
					t.Error("config.yaml was not created")
				}
			},
		},
		{
			name:               "init with --track flag",
			setupGit:           true,
			args:               []string{"init", "--track"},
			wantErr:            false,
			checkGitignore:     true,
			wantInGitignore:    []string{".tlc/"},
			wantNotInGitignore: []string{},
			validateConfig: func(t *testing.T, tlcDir string) {
				configPath := filepath.Join(tlcDir, "config.yaml")
				data, err := os.ReadFile(configPath)
				if err != nil {
					t.Fatalf("failed to read config: %v", err)
				}

				var config map[string]interface{}
				if err := yaml.Unmarshal(data, &config); err != nil {
					t.Fatalf("failed to unmarshal config: %v", err)
				}

				if gitCfg, ok := config["git"].(map[string]interface{}); ok {
					if track, ok := gitCfg["track"].(bool); ok && track {
						t.Error("track should not be saved when using --track flag")
					}
				}
			},
		},
		{
			name:               "init with --no-track flag",
			setupGit:           true,
			args:               []string{"init", "--no-track"},
			wantErr:            false,
			checkGitignore:     true,
			wantInGitignore:    []string{},
			wantNotInGitignore: []string{".tlc/"},
			validateConfig: func(t *testing.T, tlcDir string) {
				configPath := filepath.Join(tlcDir, "config.yaml")
				if _, err := os.Stat(configPath); os.IsNotExist(err) {
					t.Error("config.yaml was not created")
				}
			},
		},
		{
			name:               "init in non-git repo",
			setupGit:           false,
			args:               []string{"init"},
			wantErr:            false,
			checkGitignore:     true,
			wantInGitignore:    []string{},
			wantNotInGitignore: []string{".tlc/"},
		},
		{
			name:     "init with custom storage backend",
			setupGit: false,
			args:     []string{"init", "--storage", "local"},
			wantErr:  false,
			validateConfig: func(t *testing.T, tlcDir string) {
				configPath := filepath.Join(tlcDir, "config.yaml")
				data, err := os.ReadFile(configPath)
				if err != nil {
					t.Fatalf("failed to read config: %v", err)
				}

				var config map[string]interface{}
				if err := yaml.Unmarshal(data, &config); err != nil {
					t.Fatalf("failed to unmarshal config: %v", err)
				}

				if storageCfg, ok := config["storage"].(map[string]interface{}); ok {
					if backend, ok := storageCfg["backend"].(string); !ok || backend != "local" {
						t.Errorf("expected storage backend local, got %v", storageCfg["backend"])
					}
				}
			},
		},
		{
			name:     "init with custom db-path",
			setupGit: false,
			args:     []string{"init", "--db-path", "/custom/path.db"},
			wantErr:  false,
			validateConfig: func(t *testing.T, tlcDir string) {
				configPath := filepath.Join(tlcDir, "config.yaml")
				data, err := os.ReadFile(configPath)
				if err != nil {
					t.Fatalf("failed to read config: %v", err)
				}

				var config map[string]interface{}
				if err := yaml.Unmarshal(data, &config); err != nil {
					t.Fatalf("failed to unmarshal config: %v", err)
				}

				if storageCfg, ok := config["storage"].(map[string]interface{}); ok {
					if dbPath, ok := storageCfg["db_path"].(string); !ok || dbPath != "/custom/path.db" {
						t.Errorf("expected db_path /custom/path.db, got %v", storageCfg["db_path"])
					}
				}
			},
		},
		{
			name:               "init with existing .gitignore",
			setupGit:           true,
			existingGitignore:  "*.log\n*.tmp\n",
			args:               []string{"init", "--track"},
			wantErr:            false,
			checkGitignore:     true,
			wantInGitignore:    []string{"*.log", "*.tmp", ".tlc/"},
			wantNotInGitignore: []string{},
		},
		{
			name:               "init with .tlc/ already in gitignore",
			setupGit:           true,
			existingGitignore:  ".tlc/\n*.log\n",
			args:               []string{"init", "--track"},
			wantErr:            false,
			checkGitignore:     true,
			wantInGitignore:    []string{".tlc/", "*.log"},
			wantNotInGitignore: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "tlc-init-test-*")
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

			if tt.existingGitignore != "" {
				if err := os.WriteFile(".gitignore", []byte(tt.existingGitignore), 0o644); err != nil {
					t.Fatalf("failed to create .gitignore: %v", err)
				}
			}

			cmd := newTestCmd()
			initCmd := newTestInitCmd()
			cmd.AddCommand(initCmd)

			buf := new(strings.Builder)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err = cmd.Execute()
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if _, err := os.Stat(".tlc"); os.IsNotExist(err) {
				t.Error(".tlc directory was not created")
			}

			if tt.validateConfig != nil {
				tt.validateConfig(t, ".tlc")
			}

			if tt.checkGitignore {
				content, err := os.ReadFile(".gitignore")
				if err != nil && tt.setupGit {
					t.Fatalf("failed to read .gitignore: %v", err)
				}

				if tt.setupGit {
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
			}
		})
	}
}

// TestInitCmd_ExistingTLC tests init command when .tlc directory already exists
// Verifies behavior with existing TLC directory and --force flag.
func TestInitCmd_ExistingTLC(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(string) error
		args    []string
		wantErr bool
	}{
		{
			name: "fail without --force",
			setup: func(_ string) error {
				return os.MkdirAll(".tlc", 0o755)
			},
			args:    []string{"init"},
			wantErr: true,
		},
		{
			name: "succeed with --force",
			setup: func(tmpDir string) error {
				tlcDir := filepath.Join(tmpDir, ".tlc")
				os.MkdirAll(tlcDir, 0o755)
				os.WriteFile(filepath.Join(tlcDir, "old.txt"), []byte("old content"), 0o644)
				return nil
			},
			args:    []string{"init", "--force"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "tlc-init-existing-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			oldWd, _ := os.Getwd()
			os.Chdir(tmpDir)
			defer os.Chdir(oldWd)

			viper.Reset()

			if err := tt.setup(tmpDir); err != nil {
				t.Fatalf("setup failed: %v", err)
			}

			cmd := newTestCmd()
			initCmd := newTestInitCmd()
			cmd.AddCommand(initCmd)

			buf := new(strings.Builder)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err = cmd.Execute()
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if _, err := os.Stat(".tlc/config.yaml"); os.IsNotExist(err) {
					t.Error("config.yaml was not created")
				}
			}
		})
	}
}

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

			os.Setenv("HOME", homeDir)
			defer os.Unsetenv("HOME")

			oldHome := os.Getenv("XDG_CONFIG_HOME")
			os.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))
			defer os.Setenv("XDG_CONFIG_HOME", oldHome)

			viper.Reset()
			initConfig()

			cmd := newTestCmd()
			initCmd := newTestInitCmd()
			cmd.AddCommand(initCmd)

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

				if tt.name == "init with --no-track flag" {
					t.Logf(".gitignore content length: %d, bytes: %q", len(content), content)
					t.Logf(".gitignore bytes: %v", content)
					t.Logf(".gitignore hex: %x", content)
				}

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
	initCmd := newTestInitCmd()
	cmd.AddCommand(initCmd)

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
	initCmd := newTestInitCmd()
	cmd.AddCommand(initCmd)

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
	initCmd := newTestInitCmd()
	cmd.AddCommand(initCmd)

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

func TestInferLabel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hop-top/tlc", "tlc"},
		{"org/sub/repo", "repo"},
		{"simple", "simple"},
		{"a/b", "b"},
	}
	for _, tt := range tests {
		if got := inferLabel(tt.input); got != tt.want {
			t.Errorf("inferLabel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestInferSpaceURI(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	// Create a temp dir under $HOME/.w/testorg
	wDir := filepath.Join(home, ".w", "testorg-init-reconnect")
	projDir := filepath.Join(wDir, "myrepo")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("failed to create test dir: %v", err)
	}
	defer os.RemoveAll(wDir)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	os.Chdir(projDir)
	got := inferSpaceURI()
	if got != wDir {
		t.Errorf("inferSpaceURI() = %q, want %q", got, wDir)
	}

	// Outside of .w/ should return empty
	os.Chdir(origDir)
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	if got := inferSpaceURI(); got != "" {
		t.Errorf("inferSpaceURI() outside .w = %q, want empty", got)
	}
}

// TestInitCmd_RegistersNewProject verifies that init registers a new
// project in the global projects table when no prior registration exists.
func TestInitCmd_RegistersNewProject(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-init-register-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	viper.Reset()
	core.ResetDetectionCache()
	dbSyncOnce = sync.Once{}

	dbFile := filepath.Join(tmpDir, "test-global.sqlite")
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", dbFile)

	cmd := newTestCmd()
	initCmd := newTestInitCmd()
	cmd.AddCommand(initCmd)

	buf := new(strings.Builder)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	// Read back the project ID from generated config
	data, err := os.ReadFile(filepath.Join(".tlc", "config.yaml"))
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	var cfg map[string]interface{}
	yaml.Unmarshal(data, &cfg)
	projCfg := cfg["project"].(map[string]interface{})
	projectID := projCfg["id"].(string)

	// Verify the project was registered in the global DB
	s, err := storage.NewSQLiteStorage(dbFile)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer s.Close()

	proj, err := s.LookupProject(context.Background(), projectID)
	if err != nil {
		t.Fatalf("LookupProject() failed: %v", err)
	}
	if proj == nil {
		t.Fatal("expected project to be registered, got nil")
	}
	if proj.ProjectID != projectID {
		t.Errorf("project_id = %q, want %q", proj.ProjectID, projectID)
	}
	if proj.Label == "" {
		t.Error("expected non-empty label")
	}
}

// TestInitCmd_ReconnectsExistingProject verifies that running init
// a second time reconnects to an already-registered project.
func TestInitCmd_ReconnectsExistingProject(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-init-reconnect-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	dbFile := filepath.Join(tmpDir, "test-global.sqlite")

	// Pre-register a project with an old db_path
	s, err := storage.NewSQLiteStorage(dbFile)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	ctx := context.Background()
	projectID := "unknown" // DetectProjectID returns "unknown" in temp dirs
	oldPath := "/old/path/db.sqlite"
	if err := s.RegisterProject(ctx, projectID, oldPath, "", "unknown"); err != nil {
		t.Fatalf("RegisterProject() failed: %v", err)
	}
	s.Close()

	// Run init
	viper.Reset()
	core.ResetDetectionCache()
	dbSyncOnce = sync.Once{}

	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", dbFile)

	cmd := newTestCmd()
	initCmd := newTestInitCmd()
	cmd.AddCommand(initCmd)

	buf := new(strings.Builder)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	// Verify the path was updated (not the old path)
	s2, err := storage.NewSQLiteStorage(dbFile)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer s2.Close()

	proj, err := s2.LookupProject(ctx, projectID)
	if err != nil {
		t.Fatalf("LookupProject() failed: %v", err)
	}
	if proj.DBPath == oldPath {
		t.Errorf("db_path was not updated, still %q", oldPath)
	}
	if !strings.Contains(proj.DBPath, "db.sqlite") {
		t.Errorf("expected db_path to contain db.sqlite, got %q", proj.DBPath)
	}
}
