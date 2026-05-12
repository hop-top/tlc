package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	kitcliconfig "hop.top/kit/go/console/cli/config"
	"hop.top/tlc/internal/config"
)

var ConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management",
}

var ConfigValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration",
	Long: `Validate the merged tlc configuration.

Reads the active configuration through viper, applies struct validation,
and reports any errors. Exits non-zero on failure.`,
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
	RunE: func(_ *cobra.Command, _ []string) error {
		var cfg config.Config
		if err := viper.Unmarshal(&cfg); err != nil {
			return fmt.Errorf("failed to unmarshal config: %w", err)
		}
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("configuration invalid: %w", err)
		}
		fmt.Println("✓ Configuration is valid")
		return nil
	},
}

var ConfigListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration",
	Long: `List every resolved configuration key with its current value.

Keys reflect the full precedence chain (cwd → project → user → system →
defaults).`,
	Annotations: map[string]string{
		"kit/side-effect": "read",
	},
	Run: func(_ *cobra.Command, _ []string) {
		keys := viper.AllKeys()
		sort.Strings(keys)

		for _, key := range keys {
			fmt.Printf("%s: %v\n", key, viper.Get(key))
		}
	},
}

var ConfigGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get config value",
	Long: `Print the resolved value for a single configuration key.

Unset keys exit non-zero with an error message.`,
	Args: cobra.ExactArgs(1),
	Annotations: map[string]string{
		"kit/side-effect": "read",
	},
	Run: func(_ *cobra.Command, args []string) {
		key := args[0]
		if !viper.IsSet(key) {
			fmt.Printf("Error: key %s not set\n", key)
			return
		}
		fmt.Println(viper.Get(key))
	},
}

var ConfigSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set config value",
	Long: `Set a configuration key to a value and persist it to disk.

The target file is the highest-precedence existing config file; falls
back to creating one when none exists.`,
	Args: cobra.ExactArgs(2),
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "yes",
	},
	RunE: func(_ *cobra.Command, args []string) error {
		key := args[0]
		value := args[1]

		viper.Set(key, value)
		target, err := config.PrepareViperForWrite(viper.GetViper())
		if err != nil {
			return fmt.Errorf("failed to prepare config file: %w", err)
		}

		if err := viper.WriteConfig(); err != nil {
			if err := viper.SafeWriteConfig(); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
		}

		fmt.Printf("Set %s = %s (written to: %s)\n", key, value, target)
		return nil
	},
}

// tlcConfigPathsResolver returns the tlc config precedence chain for cwd,
// highest-precedence first. Wired into kit's shared `config path` /
// `config paths` subcommands.
//
// Order, highest first: cwd marker(s) → walk-up to project root → user
// (`$XDG_CONFIG_HOME/tlc/config.yaml`) → system (`/etc/tlc/config.yaml`)
// → synthetic "default" entry. The walk-up stops at $HOME so user-scope
// discovery never escapes ~.
func tlcConfigPathsResolver(cwd string) []kitcliconfig.ResolvedPath {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		abs = cwd
	}

	// Project marker candidates per directory, in order. The mode-aware
	// names mirror config.LocalConfigDir / LocalConfigFile so both
	// standalone (.tlc) and hop (.hop/tlc) layouts are surfaced.
	mode := config.DetectMode()
	markers := []string{
		filepath.Join(config.LocalConfigDir(mode), "config.yaml"),
		config.LocalConfigFile(mode),
	}
	// If the detected mode is "hop" but a .tlc layout also exists, surface
	// both so users can see the standalone fallback row.
	if mode == config.ModeHop {
		markers = append(markers,
			filepath.Join(config.LocalConfigDir(config.ModeStandalone), "config.yaml"),
			config.LocalConfigFile(config.ModeStandalone),
		)
	}

	out := make([]kitcliconfig.ResolvedPath, 0, len(markers)*4+3)

	// 1) cwd marker(s).
	for _, m := range markers {
		p := filepath.Join(abs, m)
		out = append(out, kitcliconfig.ResolvedPath{
			Path:   p,
			Source: "cwd",
			Exists: regularFileExists(p),
		})
	}

	// 2) Walk up looking for project root. Stop at fs root or $HOME.
	home, _ := os.UserHomeDir()
	dir := abs
	const maxDepth = 32
	for depth := 0; depth < maxDepth; depth++ {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
		if home != "" && dir == home {
			break
		}
		for _, m := range markers {
			p := filepath.Join(dir, m)
			out = append(out, kitcliconfig.ResolvedPath{
				Path:   p,
				Source: "project",
				Exists: regularFileExists(p),
			})
		}
	}

	// 3) User config: $XDG_CONFIG_HOME/tlc/config.yaml.
	if userPath, err := config.UserConfigPath(); err == nil && userPath != "" {
		out = append(out, kitcliconfig.ResolvedPath{
			Path:   userPath,
			Source: "user",
			Exists: regularFileExists(userPath),
		})
	}

	// 4) System config: /etc/tlc/config.yaml.
	sysPath := config.SystemConfigPath()
	out = append(out, kitcliconfig.ResolvedPath{
		Path:   sysPath,
		Source: "system",
		Exists: regularFileExists(sysPath),
	})

	// 5) Synthetic defaults entry — represents in-binary fallbacks.
	out = append(out, kitcliconfig.ResolvedPath{
		Path:   "<defaults>",
		Source: "default",
		Exists: true,
	})

	return out
}

// regularFileExists is a tiny helper around os.Stat that returns true
// only for regular files (and followed symlinks), false otherwise.
func regularFileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

func init() {
	ConfigCmd.AddCommand(ConfigValidateCmd)
	ConfigCmd.AddCommand(ConfigListCmd)
	ConfigCmd.AddCommand(ConfigGetCmd)
	ConfigCmd.AddCommand(ConfigSetCmd)
	// Attach kit's shared `config path` and `config paths` subcommands so
	// `tlc config paths` shows the same precedence chain other kit-built
	// CLIs surface. See kit/docs/inspect-config-paths.md.
	kitcliconfig.RegisterPathSubcommands(
		ConfigCmd, "tlc",
		kitcliconfig.WithResolver(tlcConfigPathsResolver),
	)
	RootCmd.AddCommand(ConfigCmd)
}
