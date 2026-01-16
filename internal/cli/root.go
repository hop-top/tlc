package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/google/oss-tlc-cli/internal/config"
	"github.com/google/oss-tlc-cli/internal/storage"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "tlc",
	Short: "Task Line CLI - Multi-agent task orchestration",
	Long:  "TLC provides commands for task management, flow execution, and collaboration.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path")
	rootCmd.PersistentFlags().StringP("format", "f", "", "output format (table, json, yaml, tls)")
	rootCmd.PersistentFlags().Bool("no-color", false, "disable colored output")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose logging")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "suppress non-essential output")

	viper.BindPFlag("output.format", rootCmd.PersistentFlags().Lookup("format"))
	viper.BindPFlag("output.color", rootCmd.PersistentFlags().Lookup("no-color"))
	viper.BindPFlag("output.verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("output.quiet", rootCmd.PersistentFlags().Lookup("quiet"))
}

func initConfig() {
	setDefaults()

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
		if err := viper.ReadInConfig(); err != nil {
			log.Warn("Failed to read config file", "path", cfgFile, "error", err)
		}
	} else {
		// 1. Load global/system fallbacks first
		viper.AddConfigPath("/etc/tlc")
		home, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(filepath.Join(home, ".config", "tlc"))
		}
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")

		// Read base config if exists (e.g. ~/.config/tlc/config.yaml)
		if err := viper.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				log.Warn("Error reading base config", "error", err)
			}
		}

		// 2. Cascade project-specific configs from current dir up to root
		curr, err := os.Getwd()
		if err == nil {
			configs := findAllConfigs(curr)
			// Merge them in order from root-most to closest
			// so that closer files overwrite further ones.
			for i := len(configs) - 1; i >= 0; i-- {
				viper.SetConfigFile(configs[i])
				if err := viper.MergeInConfig(); err != nil {
					log.Warn("Failed to merge config", "path", configs[i], "error", err)
				}
			}
		}
	}

	viper.SetEnvPrefix("TLC")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if viper.ConfigFileUsed() != "" && viper.GetBool("output.verbose") {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}

	// Validate merged configuration
	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		log.Warn("Failed to unmarshal config for validation", "error", err)
	} else {
		if err := cfg.Validate(); err != nil {
			log.Fatal("Invalid configuration", "error", err)
		}
	}

	// Sync local TODO file into SQLite
	if err := ingestTODO(); err != nil {
		log.Warn("Failed to ingest TODO file", "error", err)
	}

	setupLogging()
}

func findAllConfigs(startDir string) []string {
	var configs []string
	curr := startDir
	for {
		// Check for .tlc.yaml
		tlcYaml := filepath.Join(curr, ".tlc.yaml")
		if _, err := os.Stat(tlcYaml); err == nil {
			configs = append(configs, tlcYaml)
		}

		// Check for .tlc/config.yaml
		tlcDirConfig := filepath.Join(curr, ".tlc", "config.yaml")
		if _, err := os.Stat(tlcDirConfig); err == nil {
			configs = append(configs, tlcDirConfig)
		}

		// Move up
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return configs
}

func setupLogging() {
	options := log.Options{
		ReportTimestamp: true,
		TimeFormat:      "15:04:05",
		Prefix:          "tlc 🚀",
	}

	if viper.GetBool("output.verbose") {
		options.Level = log.DebugLevel
	} else {
		options.Level = log.InfoLevel
	}

	var writer = os.Stderr
	logFile := viper.GetString("output.log_file")
	if logFile != "" {
		// Ensure directory exists
		os.MkdirAll(filepath.Dir(logFile), 0755)
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			writer = f
		} else {
			fmt.Fprintf(os.Stderr, "Failed to open log file %s: %v\n", logFile, err)
		}
	}

	logger := log.NewWithOptions(writer, options)
	log.SetDefault(logger)
}

func setDefaults() {
	viper.SetDefault("output.format", "table")
	viper.SetDefault("output.color", true)
	viper.SetDefault("output.verbose", false)
	viper.SetDefault("output.quiet", false)

	// Determine XDG_DATA_HOME
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

	viper.SetDefault("task.todo_file", filepath.Join(dataHome, "tlc", "todo.txt"))
	viper.SetDefault("output.log_file", filepath.Join(dataHome, "tlc", "tlc.log"))
	viper.SetDefault("storage.db_path", filepath.Join(dataHome, "tlc", "db.sqlite"))

	viper.SetDefault("task.default_status", "TODO")
	viper.SetDefault("task.id_format", "T-{seq:04d}")
	viper.SetDefault("task.auto_assign", false)
	viper.SetDefault("task.require_reference", true)

	viper.SetDefault("git.worktree.directory", ".worktrees")
	viper.SetDefault("git.worktree.auto_create", true)
	viper.SetDefault("git.branch.prefix_from_type", true)
	viper.SetDefault("git.branch.zero_pad_issue", 4)
	viper.SetDefault("git.branch.separator", "/")
	viper.SetDefault("git.commit.auto_generate", true)
	viper.SetDefault("git.commit.template", "{type}: {description} (closes #{issue})")

	viper.SetDefault("storage.backend", "sqlite")
	viper.SetDefault("ui.pager", "auto")
	viper.SetDefault("ui.editor", os.Getenv("EDITOR"))
	viper.SetDefault("ui.date_format", "2006-01-02 15:04:05")
	viper.SetDefault("ui.timezone", "local")
	viper.SetDefault("ui.table_style", "unicode")
}

func getStorage() (*storage.SQLiteStorage, error) {
	backend := viper.GetString("storage.backend")
	if backend != "sqlite" {
		return nil, fmt.Errorf("unsupported storage backend: %s", backend)
	}

	dbPath := viper.GetString("storage.db_path")
	if dbPath == "" {
		// This case is unlikely given setDefaults, but handled for safety
		dataHome := os.Getenv("XDG_DATA_HOME")
		if dataHome == "" {
			home, _ := os.UserHomeDir()
			dataHome = filepath.Join(home, ".local", "share")
		}
		dbPath = filepath.Join(dataHome, "tlc", "db.sqlite")
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	return storage.NewSQLiteStorage(dbPath)
}
