package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
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

var (
	storageBackend string
	dbPath         string
	force          bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize TLC in current directory",
	Long:  "Create .tlc directory and default configuration file.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Create .tlc directory
		if _, err := os.Stat(".tlc"); err == nil && !force {
			return fmt.Errorf(".tlc directory already exists. Use --force to overwrite")
		}

		if err := os.MkdirAll(".tlc", 0755); err != nil {
			return fmt.Errorf("failed to create .tlc directory: %w", err)
		}

		// 2. Generate config.yaml with defaults or flags
		config := make(map[string]interface{})
		config["version"] = 0.1

		output := make(map[string]interface{})
		output["format"] = "table"
		output["color"] = true
		config["output"] = output

		storageCfg := make(map[string]interface{})
		storageCfg["backend"] = storageBackend
		if dbPath != "" {
			storageCfg["db_path"] = dbPath
		}
		config["storage"] = storageCfg

		gitCfg := make(map[string]interface{})
		config["git"] = gitCfg

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
	},
}

func init() {
	initCmd.Flags().StringVar(&storageBackend, "storage", "sqlite", "Storage backend: local, sqlite")
	initCmd.Flags().StringVar(&dbPath, "db-path", "", "Database file path (default: global)")
	initCmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config")

	rootCmd.AddCommand(initCmd)
}
