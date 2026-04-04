package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

func shouldTrackTLC(cmd *cobra.Command) (bool, error) {
	// Check CLI flags first (highest priority)
	if cmd.Flags().Changed("track") {
		return true, nil
	}
	if cmd.Flags().Changed("no-track") {
		return false, nil
	}

	// Check config value
	if viper.IsSet("git.track") {
		return viper.GetBool("git.track"), nil
	}

	// Skip prompt if not in a git repo
	if _, err := os.Stat(".git"); os.IsNotExist(err) {
		return false, nil
	}

	// Skip prompt if stdin is not a terminal
	if fi, err := os.Stdin.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
		return false, nil
	}

	// Prompt user
	var shouldTrack bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Add .tlc/ to .gitignore?").
				Description("This will exclude your TLC configuration from version control").
				Value(&shouldTrack),
		),
	)

	// Use accessible mode for testing (bypasses TUI)
	if err := form.WithAccessible(true).Run(); err != nil {
		return false, fmt.Errorf("failed to run form: %w", err)
	}

	return shouldTrack, nil
}

var (
	storageBackend      string
	dbPath              string
	force               bool
	fallbackMode        string
	duplicateIDStrategy string
)

const (
	strategyShare = "share"
)

// localProjectDBPath returns the absolute path to the project's local
// SQLite database, respecting the current entry mode.
func localProjectDBPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	mode := config.DetectMode()
	return filepath.Join(cwd, config.LocalConfigDir(mode), "db.sqlite")
}

// inferSpaceURI detects the workspace space URI from the directory
// structure. If cwd is under $HOME/.w/<org>/, returns that path.
func inferSpaceURI() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	home := os.Getenv("HOME")
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return ""
		}
	}
	wDir := filepath.Join(home, ".w")
	compareWDir := wDir
	compareCwd := cwd
	if resolvedWDir, err := filepath.EvalSymlinks(compareWDir); err == nil {
		compareWDir = resolvedWDir
	}
	if resolvedCwd, err := filepath.EvalSymlinks(compareCwd); err == nil {
		compareCwd = resolvedCwd
	}
	rel, err := filepath.Rel(compareWDir, compareCwd)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	// rel is like "org/repo/..." — extract the first segment
	parts := strings.SplitN(rel, string(filepath.Separator), 2)
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}
	return filepath.Join(wDir, parts[0])
}

// inferLabel returns the last path segment of a project ID as label.
func inferLabel(projectID string) string {
	parts := strings.Split(projectID, "/")
	if len(parts) == 0 {
		return projectID
	}
	return parts[len(parts)-1]
}

func runInit(cmd *cobra.Command, storageBackend *string, dbPath *string, force *bool, fallbackMode *string, duplicateIDStrategy *string) error {
	mode := config.DetectMode()
	configDir := config.LocalConfigDir(mode)

	// Refuse to init if the other mode's config already exists.
	cwd, _ := os.Getwd()
	if cwd == "" {
		cwd = "."
	}
	if conflictErr := config.CheckConfigConflict(cwd); conflictErr != nil && !*force {
		return conflictErr
	}

	if _, err := os.Stat(configDir); err == nil && !*force {
		return fmt.Errorf("%s directory already exists. Use --force to overwrite", configDir)
	}

	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return fmt.Errorf("failed to create %s directory: %w", configDir, err)
	}

	config := make(map[string]interface{})
	config["version"] = 0.1

	output := make(map[string]interface{})
	output["format"] = "table"
	output["color"] = true
	config["output"] = output

	projectCfg := make(map[string]interface{})
	detectedID := core.DetectProjectID()
	finalProjectID := detectedID

	s, err := getStorageRaw()
	if err == nil {
		defer func() { _ = s.Close() }()
		ctx := context.Background()

		existingTasks, err := s.ListTasks(ctx, core.Query{
			Filters: []core.FieldFilter{{Field: "project_id", Value: detectedID}},
			Limit:   1,
		})
		if err == nil && len(existingTasks) > 0 {
			strategy := *duplicateIDStrategy
			if strategy == "" {
				strategy = strategyShare
			}

			switch strategy {
			case strategyShare:
				log.Warn("Sharing existing project", "project_id", detectedID, "task_count", len(existingTasks))
				finalProjectID = detectedID

			case "unique":
				finalProjectID = generateUniqueProjectID(ctx, s, detectedID)
				log.Info("Created unique project", "project_id", finalProjectID, "parent", detectedID)

			case "prompt":
				if cmd.Flags().Changed("duplicate-id-strategy") {
					// Strategy explicitly set via flag — just save it, don't prompt now
					log.Warn("Sharing existing project (prompt strategy set for future use)", "project_id", detectedID, "task_count", len(existingTasks))
				} else {
					choice, err := promptDuplicateIDStrategy(detectedID, len(existingTasks))
					if err != nil {
						return fmt.Errorf("failed to prompt for duplicate strategy: %w", err)
					}
					if choice == "unique" {
						finalProjectID = generateUniqueProjectID(ctx, s, detectedID)
						log.Info("Created unique project", "project_id", finalProjectID, "parent", detectedID)
					} else {
						log.Warn("Sharing existing project", "project_id", detectedID, "task_count", len(existingTasks))
					}
				}
			}
		}

		// Register or reconnect the project in the global projects table.
		projDBPath := localProjectDBPath()
		spaceURI := inferSpaceURI()
		label := inferLabel(finalProjectID)

		existing, lookupErr := s.LookupProject(ctx, finalProjectID)
		if lookupErr == nil && existing != nil {
			// Reconnect: update the stored db_path
			if updateErr := s.UpdateProjectPath(ctx, finalProjectID, projDBPath); updateErr != nil {
				log.Warn("Failed to update project path", "error", updateErr)
			} else {
				log.Info("Reconnected to existing project",
					"project_id", finalProjectID,
					"registered", existing.RegisteredAt.Format("2006-01-02"))
			}
		} else {
			// New project: register it
			if regErr := s.RegisterProject(ctx, finalProjectID, projDBPath, spaceURI, label); regErr != nil {
				log.Warn("Failed to register project", "error", regErr)
			} else {
				log.Info("Registered new project", "project_id", finalProjectID)
			}
		}
	}

	projectCfg["id"] = finalProjectID

	fallbackModeVal := *fallbackMode
	if fallbackModeVal == "" {
		fallbackModeVal = "auto"
	}
	projectCfg["fallback_mode"] = fallbackModeVal

	duplicateIDStrategyVal := *duplicateIDStrategy
	if duplicateIDStrategyVal == "" {
		duplicateIDStrategyVal = strategyShare
	}
	projectCfg["duplicate_id_strategy"] = duplicateIDStrategyVal
	config["project"] = projectCfg

	storageCfg := make(map[string]interface{})
	storageCfg["backend"] = *storageBackend
	if *dbPath != "" {
		storageCfg["db_path"] = *dbPath
	}
	config["storage"] = storageCfg

	gitCfg := make(map[string]interface{})
	config["git"] = gitCfg

	track, err := shouldTrackTLC(cmd)
	if err != nil {
		return fmt.Errorf("failed to determine track setting: %w", err)
	}

	if !cmd.Flags().Changed("track") && !cmd.Flags().Changed("no-track") && !viper.IsSet("git.track") && track {
		gitCfg["track"] = true
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configFilePath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configFilePath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write config.yaml: %w", err)
	}

	gitignoreEntry := configDir + "/"
	if _, err := os.Stat(".git"); err == nil {
		f, err := os.OpenFile(".gitignore", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			defer func() { _ = f.Close() }()
			content, _ := os.ReadFile(".gitignore")
			contentStr := string(content)

			if track && !strings.Contains(contentStr, gitignoreEntry) {
				if _, err := f.WriteString(gitignoreEntry + "\n"); err != nil {
					log.Warn("Failed to update .gitignore", "error", err)
				}
			}
		}
	}

	// Ensure tasks/ is in the config dir's .gitignore so projected task
	// files are never tracked, even when the user commits .tlc/ itself.
	innerGitignore := filepath.Join(configDir, ".gitignore")
	innerContent, _ := os.ReadFile(innerGitignore)
	if !strings.Contains(string(innerContent), "tasks/") {
		igf, err := os.OpenFile(innerGitignore, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			log.Warn("Failed to open inner .gitignore", "error", err)
		} else {
			entry := "tasks/\n"
			if len(innerContent) > 0 && !strings.HasSuffix(string(innerContent), "\n") {
				entry = "\n" + entry
			}
			if _, err := igf.WriteString(entry); err != nil {
				log.Warn("Failed to update inner .gitignore", "error", err)
			}
			_ = igf.Close()
		}
	}

	log.Info("Initialized TLC", "directory", filepath.Base(os.Getenv("PWD")), "project_id", finalProjectID, "mode", mode)
	return nil
}

func projectExists(ctx context.Context, s *storage.SQLiteStorage, projectID string) bool {
	tasks, err := s.ListTasks(ctx, core.Query{
		Filters: []core.FieldFilter{{Field: "project_id", Value: projectID}},
		Limit:   1,
	})
	return err == nil && len(tasks) > 0
}

func generateUniqueProjectID(ctx context.Context, s *storage.SQLiteStorage, baseID string) string {
	cwd, _ := os.Getwd()
	dirName := filepath.Base(cwd)

	if !strings.Contains(baseID, dirName) {
		candidate := fmt.Sprintf("%s-%s", baseID, dirName)
		if !projectExists(ctx, s, candidate) {
			return candidate
		}
	}

	for i := 2; i <= 10; i++ {
		candidate := fmt.Sprintf("%s-%d", baseID, i)
		if !projectExists(ctx, s, candidate) {
			return candidate
		}
	}

	ts := time.Now().Format("20060102")
	return fmt.Sprintf("%s-%s", baseID, ts)
}

func promptDuplicateIDStrategy(projectID string, taskCount int) (string, error) {
	var choice string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Project '%s' already exists with %d tasks", projectID, taskCount)).
				Description("How should TLC handle this clone?").
				Options(
					huh.NewOption("Share existing project (recommended)", strategyShare),
					huh.NewOption("Create unique project for this clone", "unique"),
				).
				Value(&choice),
		),
	)

	err := form.Run()
	return choice, fmt.Errorf("failed to run form: %w", err)
}

func addInitFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&storageBackend, "storage", "sqlite", "Storage backend: local, sqlite")
	cmd.Flags().StringVar(&dbPath, "db-path", "", "Database file path (default: global)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config")
	cmd.Flags().Bool("track", false, "Add .tlc/ to .gitignore")
	cmd.Flags().Bool("no-track", false, "Do not add .tlc/ to .gitignore")
	cmd.Flags().StringVar(&fallbackMode, "fallback-mode", "", "Project fallback mode: auto, detected, prompt (default: auto)")
	cmd.Flags().StringVar(&duplicateIDStrategy, "duplicate-id-strategy", "", "Duplicate ID strategy: share, unique, prompt (default: "+strategyShare+")")
}

var InitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize TLC in current directory",
	Long:  "Create .tlc directory and default configuration file.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runInit(cmd, &storageBackend, &dbPath, &force, &fallbackMode, &duplicateIDStrategy)
	},
}

// ProjectInitCmd is InitCmd registered under `tlc project init`.
var ProjectInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize TLC in current directory",
	Long:  "Create .tlc directory and default configuration file.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runInit(cmd, &storageBackend, &dbPath, &force, &fallbackMode, &duplicateIDStrategy)
	},
}

func init() {
	addInitFlags(InitCmd)
	addInitFlags(ProjectInitCmd)

	RootCmd.AddCommand(InitCmd)
}
