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

func runInit(cmd *cobra.Command, storageBackend *string, dbPath *string, force *bool, fallbackMode *string, duplicateIDStrategy *string) error {
	if _, err := os.Stat(".tlc"); err == nil && !*force {
		return fmt.Errorf(".tlc directory already exists. Use --force to overwrite")
	}

	if err := os.MkdirAll(".tlc", 0o750); err != nil {
		return fmt.Errorf("failed to create .tlc directory: %w", err)
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

	if err := os.WriteFile(".tlc/config.yaml", data, 0o600); err != nil {
		return fmt.Errorf("failed to write config.yaml: %w", err)
	}

	if _, err := os.Stat(".git"); err == nil {
		f, err := os.OpenFile(".gitignore", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			defer func() { _ = f.Close() }()
			content, _ := os.ReadFile(".gitignore")
			contentStr := string(content)

			if track && !strings.Contains(contentStr, ".tlc/") {
				if _, err := f.WriteString(".tlc/\n"); err != nil {
					log.Warn("Failed to update .gitignore", "error", err)
				}
			}
		}
	}

	log.Info("Initialized TLC", "directory", filepath.Base(os.Getenv("PWD")), "project_id", finalProjectID)
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

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize TLC in current directory",
	Long:  "Create .tlc directory and default configuration file.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runInit(cmd, &storageBackend, &dbPath, &force, &fallbackMode, &duplicateIDStrategy)
	},
}

func init() {
	initCmd.Flags().StringVar(&storageBackend, "storage", "sqlite", "Storage backend: local, sqlite")
	initCmd.Flags().StringVar(&dbPath, "db-path", "", "Database file path (default: global)")
	initCmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config")
	initCmd.Flags().Bool("track", false, "Add .tlc/ to .gitignore")
	initCmd.Flags().Bool("no-track", false, "Do not add .tlc/ to .gitignore")
	initCmd.Flags().StringVar(&fallbackMode, "fallback-mode", "", "Project fallback mode: auto, detected, prompt (default: auto)")
	initCmd.Flags().StringVar(&duplicateIDStrategy, "duplicate-id-strategy", "", "Duplicate ID strategy: share, unique, prompt (default: "+strategyShare+")")

	rootCmd.AddCommand(initCmd)
}
