package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

func getDataHome() string {
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
	return dataHome
}

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
		return false, err
	}

	return shouldTrack, nil
}

var (
	storageBackend string
	dbPath         string
	force          bool
)

func runInit(cmd *cobra.Command, storageBackend *string, dbPath *string, force *bool) error {
	if _, err := os.Stat(".tlc"); err == nil && !*force {
		return fmt.Errorf(".tlc directory already exists. Use --force to overwrite")
	}

	if err := os.MkdirAll(".tlc", 0755); err != nil {
		return fmt.Errorf("failed to create .tlc directory: %w", err)
	}

	config := make(map[string]interface{})
	config["version"] = 0.1

	output := make(map[string]interface{})
	output["format"] = "table"
	output["color"] = true
	config["output"] = output

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
		data, err := yaml.Marshal(config)
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}
		if err := os.WriteFile(".tlc/config.yaml", data, 0644); err != nil {
			return fmt.Errorf("failed to write config.yaml: %w", err)
		}
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(".tlc/config.yaml", data, 0644); err != nil {
		return fmt.Errorf("failed to write config.yaml: %w", err)
	}

	if _, err := os.Stat(".git"); err == nil {
		f, err := os.OpenFile(".gitignore", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			defer f.Close()
			content, _ := os.ReadFile(".gitignore")
			contentStr := string(content)

			if track && !strings.Contains(contentStr, ".tlc/") {
				if _, err := f.WriteString(".tlc/\n"); err != nil {
					log.Warn("Failed to update .gitignore", "error", err)
				}
			}
		}
	}

	log.Info("Initialized TLC", "directory", filepath.Base(os.Getenv("PWD")))
	return nil
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize TLC in current directory",
	Long:  "Create .tlc directory and default configuration file.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInit(cmd, &storageBackend, &dbPath, &force)
	},
}

func init() {
	initCmd.Flags().StringVar(&storageBackend, "storage", "sqlite", "Storage backend: local, sqlite")
	initCmd.Flags().StringVar(&dbPath, "db-path", "", "Database file path (default: global)")
	initCmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config")
	initCmd.Flags().Bool("track", false, "Add .tlc/ to .gitignore")
	initCmd.Flags().Bool("no-track", false, "Do not add .tlc/ to .gitignore")

	rootCmd.AddCommand(initCmd)
}
