package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
	"hop.top/upgrade"
)

const backendSQLite = "sqlite"

var (
	cfgFile    string
	tlcVersion = "dev" // overridden at build time via -ldflags
)

// SetVersion sets the version string injected at build time.
func SetVersion(v string) { tlcVersion = v }

var RootCmd = &cobra.Command{
	Use:   "tlc",
	Short: "Task Line CLI - Multi-agent task orchestration",
	Long:  "TLC provides commands for task management, flow execution, and collaboration.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() != "upgrade" {
			upgrade.NotifyIfAvailable(cmd.Context(), newChecker(), os.Stderr)
		}
		// Initialize storage early to enable completion and other features
		s, err := getStorage()
		if err != nil {
			// Don't fail if we can't open storage (e.g. for 'init' or 'help' commands)
			// but we can't setup completion without it.
			return nil
		}
		return setupURICompletion(s)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if ok, _ := cmd.Flags().GetBool("version"); ok {
			fmt.Fprintf(cmd.OutOrStdout(), "tlc version %s\n", tlcVersion)
			return nil
		}
		// If no command is specified, run the TUI
		return tuiCmd.RunE(cmd, args)
	},
}

// Execute runs the root command and handles any errors.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path")
	RootCmd.PersistentFlags().StringP("format", "f", "", "output format (table, json, yaml, tls, summary)")
	RootCmd.PersistentFlags().Bool("no-color", false, "disable colored output")
	RootCmd.PersistentFlags().BoolP("verbose", "V", false, "verbose logging")
	RootCmd.PersistentFlags().BoolP("quiet", "q", false, "suppress non-essential output")
	RootCmd.Flags().BoolP("version", "v", false, "print version and exit")

	if err := viper.BindPFlag("output.format", RootCmd.PersistentFlags().Lookup("format")); err != nil {
		log.Warn("Failed to bind format flag", "error", err)
	}
	if err := viper.BindPFlag("output.color", RootCmd.PersistentFlags().Lookup("no-color")); err != nil {
		log.Warn("Failed to bind color flag", "error", err)
	}
	if err := viper.BindPFlag("output.verbose", RootCmd.PersistentFlags().Lookup("verbose")); err != nil {
		log.Warn("Failed to bind verbose flag", "error", err)
	}
	if err := viper.BindPFlag("output.quiet", RootCmd.PersistentFlags().Lookup("quiet")); err != nil {
		log.Warn("Failed to bind quiet flag", "error", err)
	}
}

func initConfig() {
	setDefaults()

	// normalizedCfgFile holds the resolved explicit config path (if any).
	// We always run the normal cascade first so that system/user/project
	// defaults are present; then we merge the explicit file on top.
	normalizedCfgFile := ""
	if cfgFile != "" {
		// If cfgFile is a directory, resolve to <dir>/<localConfigDir>/config.yaml.
		if info, err := os.Stat(cfgFile); err == nil && info.IsDir() {
			cfgFile = filepath.Join(cfgFile, config.LocalConfigDir(config.DetectMode()), "config.yaml")
		}
		normalizedCfgFile = cfgFile
	}

	{
		// Prefer user config over system config when picking a base file.
		if userConfigDir, err := config.UserConfigDir(); err == nil {
			viper.AddConfigPath(userConfigDir)
		}
		viper.AddConfigPath(config.SystemConfigDir())
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")

		// Read base config if it exists.
		if err := viper.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				log.Warn("Error reading base config", "error", err)
			}
		}

		// 2. Detect entry mode and cascade project-specific configs from
		// current dir up to the common ancestor of cwd and the
		// user-global config directory.
		curr, err := os.Getwd()
		if err == nil {
			mode := config.DetectMode()

			// Check for ambiguous config (both .tlc/ and .hop/tlc/).
			if conflictErr := config.CheckConfigConflict(curr); conflictErr != nil {
				log.Warn("Config conflict detected", "error", conflictErr)
			}

			// Validate that expected local config exists for this mode.
			if valErr := config.ValidateLocalConfig(mode, curr); valErr != nil {
				log.Warn("Local config validation failed", "error", valErr)
			}

			configs := findAllConfigsForMode(
				curr, resolveProjectConfigBoundary(curr), mode,
			)
			// Merge them in order from root-most to closest
			// so that closer files overwrite further ones.
			for i := len(configs) - 1; i >= 0; i-- {
				viper.SetConfigFile(configs[i])
				if err := viper.MergeInConfig(); err != nil {
					log.Warn("Failed to merge config", "path", configs[i], "error", err)
				}
			}
			// Set the closest config as the active file so
			// ConfigFileUsed() returns it downstream.
			if len(configs) > 0 {
				viper.SetConfigFile(configs[0])
			}
		}
	}

	// Merge the explicit --config file on top of the cascade so its keys
	// override anything loaded from system/user/project configs.
	if normalizedCfgFile != "" {
		viper.SetConfigFile(normalizedCfgFile)
		if err := viper.MergeInConfig(); err != nil {
			log.Warn("Failed to read config file", "path", normalizedCfgFile, "error", err)
		}
	}

	viper.SetEnvPrefix("TLC")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if viper.ConfigFileUsed() != "" && viper.GetBool("output.verbose") {
		log.Debug("Using config file", "path", viper.ConfigFileUsed())
	}

	// Validate merged configuration
	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		log.Warn("Failed to unmarshal config for validation", "error", err)
	} else {
		if err := cfg.Validate(); err != nil {
			log.Warn("Invalid configuration", "error", err)
		}
	}

	setupLogging()
}

func userGlobalConfigPath() (string, error) {
	path, err := config.UserConfigPath()
	if err != nil {
		return "", fmt.Errorf("resolve user config path: %w", err)
	}

	return path, nil
}

func normalizeConfigPath(path string) string {
	if path == "" {
		return ""
	}

	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	path = filepath.Clean(path)

	// Best effort only. Non-existent paths still participate in boundary
	// calculation via their cleaned absolute form.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}

	return filepath.Clean(path)
}

func commonAncestorDir(a, b string) (string, error) {
	a = normalizeConfigPath(a)
	b = normalizeConfigPath(b)
	if a == "" || b == "" {
		return "", fmt.Errorf("paths must not be empty")
	}

	if volA, volB := filepath.VolumeName(a), filepath.VolumeName(b); volA != volB {
		return "", fmt.Errorf("paths are on different volumes")
	}

	ancestors := make(map[string]struct{})
	for curr := a; ; curr = filepath.Dir(curr) {
		ancestors[curr] = struct{}{}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
	}

	for curr := b; ; curr = filepath.Dir(curr) {
		if _, ok := ancestors[curr]; ok {
			return curr, nil
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
	}

	return "", fmt.Errorf("no common ancestor")
}

func resolveProjectConfigBoundary(startDir string) string {
	globalConfigPath, err := userGlobalConfigPath()
	if err != nil {
		return ""
	}

	boundary, err := commonAncestorDir(startDir, filepath.Dir(globalConfigPath))
	if err != nil {
		return ""
	}

	return boundary
}

func findAllConfigs(startDir, stopDir string) []string {
	return findAllConfigsForMode(startDir, stopDir, config.DetectMode())
}

func findAllConfigsForMode(startDir, stopDir string, mode config.EntryMode) []string {
	var configs []string
	curr := normalizeConfigPath(startDir)
	stopDir = normalizeConfigPath(stopDir)
	if curr == "" {
		return configs
	}

	flatFile := config.LocalConfigFile(mode)
	dirConfig := filepath.Join(config.LocalConfigDir(mode), "config.yaml")

	for {
		// Check for flat config (e.g. .tlc.yaml or .hop/tlc.yaml)
		flat := filepath.Join(curr, flatFile)
		if _, err := os.Stat(flat); err == nil {
			configs = append(configs, flat)
		}

		// Check for dir config (e.g. .tlc/config.yaml or .hop/tlc/config.yaml)
		dir := filepath.Join(curr, dirConfig)
		if _, err := os.Stat(dir); err == nil {
			configs = append(configs, dir)
		}

		if stopDir != "" && curr == stopDir {
			break
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

	writer := os.Stderr
	logFile := viper.GetString("output.log_file")
	if logFile != "" {
		if err := os.MkdirAll(filepath.Dir(logFile), 0o750); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create log directory: %v\n", err)
		} else {
			f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
			if err == nil {
				writer = f
			} else {
				fmt.Fprintf(os.Stderr, "Failed to open log file %s: %v\n", logFile, err)
			}
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

	dataDir := config.UserDataDir()
	viper.SetDefault("task.todo_file", filepath.Join(dataDir, "todo.txt"))
	viper.SetDefault("output.log_file", filepath.Join(dataDir, "tlc.log"))
	viper.SetDefault("storage.db_path", filepath.Join(dataDir, "db.sqlite"))

	viper.SetDefault("task.default_status", "TODO")
	viper.SetDefault("task.id_format", "T-{seq:04d}")
	viper.SetDefault("task.auto_assign", false)
	viper.SetDefault("task.require_reference", true)
	viper.SetDefault("task.archive_threshold", 7*24*time.Hour)

	viper.SetDefault("git.track", false)
	viper.SetDefault("git.branch.prefix_from_type", true)
	viper.SetDefault("git.branch.zero_pad_issue", 4)
	viper.SetDefault("git.branch.separator", "/")
	viper.SetDefault("git.commit.auto_generate", true)
	viper.SetDefault("git.commit.template", "{type}: {description} (closes #{issue})")

	viper.SetDefault("storage.backend", backendSQLite)
	viper.SetDefault("ui.pager", "auto")
	viper.SetDefault("ui.editor", os.Getenv("EDITOR"))
	viper.SetDefault("ui.date_format", "2006-01-02 15:04:05")
	viper.SetDefault("ui.timezone", "local")
	viper.SetDefault("ui.table_style", "unicode")
}

var dbSyncOnce sync.Once

// ensureDBSynced runs TODO ingestion and auto-archive once per process.
// Called lazily on first getStorage() so commands that don't touch the
// DB (doctor, init, help, config, version, etc.) pay zero startup cost.
func ensureDBSynced(s *storage.SQLiteStorage) {
	dbSyncOnce.Do(func() {
		// Sync local TODO file into SQLite
		if err := ingestTODOWith(s); err != nil {
			log.Warn("Failed to ingest TODO file", "error", err)
		}

		// Auto-archive tasks
		threshold := viper.GetDuration("task.archive_threshold")
		if threshold > 0 {
			count, err := s.ArchiveTasks(context.Background(), threshold)
			if err == nil && count > 0 {
				log.Info("Auto-archived tasks", "count", count)
			}
		}
	})
}

// getStorageRaw opens the SQLite database without running TODO ingestion
// or auto-archive. Use this for diagnostic commands (doctor) that just
// need to test the connection.
func getStorageRaw() (*storage.SQLiteStorage, error) {
	backend := viper.GetString("storage.backend")
	if backend != backendSQLite {
		return nil, fmt.Errorf("unsupported storage backend: %s", backend)
	}

	dbPath := viper.GetString("storage.db_path")
	if dbPath == "" {
		dbPath = filepath.Join(config.UserDataDir(), "db.sqlite")
	}
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o750); err != nil { //nolint:gosec // G703: dbPath from config or standard data dir
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create sqlite storage: %w", err)
	}
	return s, nil
}

// touchProjectIfNeeded bumps last_seen_at for the current project.
// Fire-and-forget: logs on error but never fails the caller.
var touchOnce sync.Once

func touchProjectIfNeeded(s *storage.SQLiteStorage) {
	touchOnce.Do(func() {
		det := core.DetectProject()
		if det == nil || !det.InProject || det.ProjectID == "" {
			return
		}
		if err := s.TouchProject(context.Background(), det.ProjectID); err != nil {
			log.Warn("Failed to touch project", "project", det.ProjectID, "error", err)
		}
	})
}

// getStorage opens the SQLite database and ensures TODO ingestion and
// auto-archive have run (once per process).
func getStorage() (*storage.SQLiteStorage, error) {
	s, err := getStorageRaw()
	if err != nil {
		return nil, err
	}

	ensureDBSynced(s)
	touchProjectIfNeeded(s)
	return s, nil
}
