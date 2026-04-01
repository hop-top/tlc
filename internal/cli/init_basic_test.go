package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
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
			isolateInitTest(t)

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
			cmd.AddCommand(newTestInitCmd())

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
				if err := os.MkdirAll(tlcDir, 0o755); err != nil {
					return fmt.Errorf("failed to create .tlc dir: %w", err)
				}
				if err := os.WriteFile(filepath.Join(tlcDir, "old.txt"), []byte("old content"), 0o644); err != nil {
					return fmt.Errorf("failed to write old.txt: %w", err)
				}
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
			isolateInitTest(t)

			if err := tt.setup(tmpDir); err != nil {
				t.Fatalf("setup failed: %v", err)
			}

			cmd := newTestCmd()
			cmd.AddCommand(newTestInitCmd())

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
