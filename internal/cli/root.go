package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"charm.land/log/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/bus"
	kitcli "hop.top/kit/cli"
	"hop.top/kit/domain"
	kitlog "hop.top/kit/log"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/events"
	"hop.top/tlc/internal/extensions"
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

// kitRootInstance holds the kit Root so Execute() can call root.Execute().
// Initialized at var-declaration time so RootCmd is non-nil before any
// other file's init() calls RootCmd.AddCommand.
var kitRootInstance = kitRoot()

// RootCmd is the package-level root command reference. Subcommand files
// use RootCmd.AddCommand in their own init() functions.
var RootCmd = kitRootInstance.Cmd

// extMgr is the process-wide extension manager, initialised during the
// first PersistentPreRunE and torn down by a deferred CloseAll in Execute.
var extMgr *extensions.Manager

// eventBus is the process-wide kit/bus instance, created at startup and
// closed on shutdown. Subscribers and publishers use this to exchange
// lifecycle events.
var eventBus bus.Bus

// auditSub is the bus subscriber that persists events to task_logs.
var auditSub *events.AuditSubscriber

// busPublisher is the domain.EventPublisher adapter for the bus.
var busPublisher *events.BusPublisher

// GetBusPublisher returns the process-wide domain.EventPublisher, or nil
// if the bus has not been initialised yet. Callers should pass the result
// to events.DomainOptions when constructing domain services.
func GetBusPublisher() domain.EventPublisher {
	if busPublisher == nil {
		return nil
	}
	return busPublisher
}

// GetEventBus returns the process-wide bus.Bus, or nil if not initialised.
func GetEventBus() bus.Bus { return eventBus }

// kitRoot constructs the root command using kit/cli.New() and wires up
// TLC-specific flags, viper bindings, and lifecycle hooks.
func kitRoot() *kitcli.Root {
	root := kitcli.New(kitcli.Config{
		Name:    "tlc",
		Version: tlcVersion,
		Short:   "Task Line CLI - Multi-agent task orchestration",
	})

	cmd := root.Cmd
	cmd.Long = "TLC provides commands for task management, flow execution, and collaboration."

	// --- TLC-specific persistent flags ---
	// kit already provides --quiet, --no-color, and --format.
	cmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path")
	cmd.PersistentFlags().BoolP("verbose", "V", false, "verbose logging")

	// Add -f shorthand to kit's --format flag.
	if f := cmd.PersistentFlags().Lookup("format"); f != nil {
		f.Shorthand = "f"
	}

	// Bind TLC flags to the global viper using namespaced keys.
	if err := viper.BindPFlag("output.format", cmd.PersistentFlags().Lookup("format")); err != nil {
		log.Warn("Failed to bind format flag", "error", err)
	}
	if err := viper.BindPFlag("output.verbose", cmd.PersistentFlags().Lookup("verbose")); err != nil {
		log.Warn("Failed to bind verbose flag", "error", err)
	}

	// Bridge kit's flat viper keys to TLC's namespaced keys on the global viper.
	// kit binds --no-color to root.Viper["no-color"] and --quiet to root.Viper["quiet"].
	// TLC reads "output.color" and "output.quiet" from the global viper.
	if err := viper.BindPFlag("output.color", cmd.PersistentFlags().Lookup("no-color")); err != nil {
		log.Warn("Failed to bind color flag", "error", err)
	}
	if err := viper.BindPFlag("output.quiet", cmd.PersistentFlags().Lookup("quiet")); err != nil {
		log.Warn("Failed to bind quiet flag", "error", err)
	}

	// Register aliases so code reading flat keys gets the namespaced values.
	viper.RegisterAlias("quiet", "output.quiet")
	viper.RegisterAlias("no-color", "output.color")
	viper.RegisterAlias("format", "output.format")

	// --- Lifecycle hooks ---
	cmd.PersistentPreRunE = func(c *cobra.Command, args []string) error {
		if c.Name() != "upgrade" {
			upgrade.NotifyIfAvailable(c.Context(), newChecker(), os.Stderr)
		}

		// Initialise the event bus once per process.
		if eventBus == nil {
			eventBus = bus.New()
			busPublisher = events.NewBusPublisher(eventBus)
		}

		// Bootstrap extensions once per process.
		if extMgr == nil {
			extMgr = extensions.New(log.Default(), eventBus)
			extensions.RegisterBuiltins(extMgr)
			if err := extMgr.InitAll(c.Context()); err != nil {
				log.Warn("Failed to initialise extensions", "error", err)
			}
		}

		s, err := getStorage()
		if err != nil {
			return nil
		}

		// Wire audit subscriber once storage is available.
		if auditSub == nil && eventBus != nil {
			auditSub = events.NewAuditSubscriber(eventBus, s)
		}

		autoProcessInbox(c, s)
		return setupURICompletion(s)
	}

	// Register contextual post-command hints.
	registerHints(root.Hints)

	// Render hints after command output.
	cmd.PersistentPostRunE = func(c *cobra.Command, _ []string) error {
		renderPostRunHintsFor(c, root)
		return nil
	}

	cobra.OnInitialize(initConfig)

	// Discover tlc-* binary plugins on $PATH and register as subcommands.
	root.EnablePluginDispatch("tlc", "")

	return root
}

func init() {
	// Wire NL prompt handler after all package vars are initialized to avoid
	// an init cycle: kitRoot() → runNLPrompt → runCommand → RootCmd → kitRoot().
	RootCmd.Args = cobra.ArbitraryArgs
	RootCmd.RunE = func(c *cobra.Command, args []string) error {
		if len(args) > 0 {
			return runNLPrompt(c, args)
		}
		return tuiCmd.RunE(c, args)
	}
}

// Execute runs the root command and handles any errors.
// Alias expansion is applied to os.Args before cobra parses them.
func Execute() {
	expanded, ok := ExpandAliases(os.Args)
	if ok {
		os.Args = expanded
	}
	defer func() {
		if auditSub != nil {
			auditSub.Close()
		}
		if extMgr != nil {
			for _, err := range extMgr.CloseAll() {
				log.Warn("extension close error", "error", err)
			}
		}
		if eventBus != nil {
			if err := eventBus.Close(context.Background()); err != nil {
				log.Warn("bus close error", "error", err)
			}
		}
	}()
	if err := kitRootInstance.Execute(context.Background()); err != nil {
		os.Exit(1)
	}
}

func initConfig() {
	setDefaults()

	// normalizedCfgFile holds the resolved explicit config path (if any).
	// We always run the normal cascade first so that system/user/project
	// defaults are present; then we merge the explicit file on top.
	normalizedCfgFile := ""
	if cfgFile != "" {
		resolved, err := resolveConfigFlag(cfgFile)
		if err != nil {
			// Hard fail: shortname not found as file or registry project.
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			os.Exit(1)
		}
		normalizedCfgFile = resolved
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
	level := log.InfoLevel
	if viper.GetBool("output.verbose") {
		level = log.DebugLevel
	}

	// kit/log handles quiet (→ WarnLevel) and no-color automatically
	// from the global viper, and applies hop.top theme styles.
	logger := kitlog.WithLevel(viper.GetViper(), level)
	logger.SetReportTimestamp(true)
	logger.SetTimeFormat("15:04:05")
	logger.SetPrefix("tlc 🚀")

	// Redirect to log file when configured.
	logFile := viper.GetString("output.log_file")
	if logFile != "" {
		if err := os.MkdirAll(filepath.Dir(logFile), 0o750); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create log directory: %v\n", err)
		} else {
			f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
			if err == nil {
				logger.SetOutput(f)
			} else {
				fmt.Fprintf(os.Stderr, "Failed to open log file %s: %v\n", logFile, err)
			}
		}
	}

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
	SetupProjector(s)
	return s, nil
}
