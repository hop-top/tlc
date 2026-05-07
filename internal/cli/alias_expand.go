// Package cli provides the TLC CLI commands.
package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"charm.land/log/v2"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/alias"
	kitconfig "hop.top/kit/go/core/config"
	"hop.top/kit/go/core/xdg"
	"hop.top/tlc/internal/config"
)

// seededAliases are built-in aliases shipped with TLC. User-defined
// aliases (global or local) override seeded ones.
var seededAliases = map[string]string{
	"setup": "config interactive",
}

// globalAliasPath returns $XDG_CONFIG_HOME/tlc/aliases.yaml.
func globalAliasPath() (string, error) {
	dir, err := xdg.ConfigDir("tlc")
	if err != nil {
		return "", fmt.Errorf("resolve global config dir: %w", err)
	}
	return filepath.Join(dir, "aliases.yaml"), nil
}

// localAliasPath returns the path to the project-local aliases.yaml when
// invoked from inside a tlc project (the local config dir already exists
// somewhere up the tree). Returns "" otherwise so callers fall back to the
// global store.
func localAliasPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	mode := config.DetectMode()
	cfgDir := config.LocalConfigDir(mode)

	// Walk upward looking for an existing local config dir. Mirrors the
	// detection used by initConfig's findAllConfigsForMode.
	curr := cwd
	for {
		candidate := filepath.Join(curr, cfgDir)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return filepath.Join(candidate, "aliases.yaml")
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			return ""
		}
		curr = parent
	}
}

// loadMergedAliases returns seeded + global + local aliases with project
// aliases overriding global, and global overriding seeded.
func loadMergedAliases() (map[string]string, error) {
	merged := make(map[string]string, len(seededAliases))
	for k, v := range seededAliases {
		merged[k] = v
	}

	if gp, err := globalAliasPath(); err == nil {
		gs := alias.NewStore(gp)
		if err := gs.Load(); err != nil {
			return nil, fmt.Errorf("load global aliases: %w", err)
		}
		for k, v := range gs.All() {
			merged[k] = v
		}
	}

	if lp := localAliasPath(); lp != "" {
		ls := alias.NewStore(lp)
		if err := ls.Load(); err != nil {
			return nil, fmt.Errorf("load local aliases: %w", err)
		}
		for k, v := range ls.All() {
			merged[k] = v
		}
	}
	return merged, nil
}

// ExpandAliases rewrites os.Args in-place if args[1] (or the first
// non-flag arg) matches a known alias. The expansion is split on
// whitespace and spliced before any remaining arguments.
//
// Returns true if an expansion was applied.
func ExpandAliases(args []string) ([]string, bool) {
	if len(args) < 2 {
		return args, false
	}

	aliases, err := loadMergedAliases()
	if err != nil {
		// Fall back to seeded aliases if store loading fails.
		aliases = seededAliases
	}
	if len(aliases) == 0 {
		return args, false
	}

	// Find first non-flag argument position (skip leading flags like -c, --config).
	firstNonFlag, candidate := findFirstNonFlag(args[1:])
	if candidate == "" {
		return args, false
	}

	expansion, ok := aliases[candidate]
	if !ok {
		return args, false
	}

	// Build expanded args: program + flags before alias + expansion tokens +
	// args after alias.
	parts := strings.Fields(expansion)
	prefix := args[1 : 1+firstNonFlag] // flags before alias name
	suffix := args[1+firstNonFlag+1:]  // args after alias name
	result := make([]string, 0, 1+len(prefix)+len(parts)+len(suffix))
	result = append(result, args[0]) // program name
	result = append(result, prefix...)
	result = append(result, parts...)
	result = append(result, suffix...)
	return result, true
}

// findFirstNonFlag returns the index within slice and the value of the
// first element that is not a flag or a flag's value argument.
//
// Flags of the form "--key=value" are treated as single tokens.
// Flags of the form "--key value" or "-k value" (where value does not start
// with "-") consume two tokens; the second is skipped.
func findFirstNonFlag(slice []string) (int, string) {
	for i := 0; i < len(slice); i++ {
		a := slice[i]
		if !strings.HasPrefix(a, "-") {
			return i, a
		}
		// "--key=value" or "-k=value": no separate value token.
		if strings.Contains(a, "=") {
			continue
		}
		// Short boolean flags (-v, -q) vs. flags that take a value (-c /path).
		// Heuristic: if the next token doesn't start with "-", treat it as the
		// value of this flag and skip both.
		if i+1 < len(slice) && !strings.HasPrefix(slice[i+1], "-") {
			i++ // skip the value token
		}
	}
	return -1, ""
}

// migrateLegacyAliases reads the deprecated viper "aliases:" map and
// migrates entries into the YAML store(s). Idempotent: skips when the
// target store is already non-empty. Logs the deprecation warning once
// per process when the legacy key is present.
func migrateLegacyAliases() {
	migrateLegacyAliasesOnce.Do(func() {
		raw := viper.GetStringMapString("aliases")
		if len(raw) == 0 {
			return
		}
		log.Warn(
			"config key \"aliases:\" is deprecated; tlc has migrated entries " +
				"to <config-dir>/aliases.yaml — remove the key from config.yaml",
		)

		// Decide where the legacy entries live: when the closest config
		// file is a project-local file, treat them as project aliases;
		// otherwise treat them as global aliases.
		target := ""
		if used := viper.ConfigFileUsed(); used != "" {
			if isLocalConfigFile(used) {
				target = localAliasPath()
			}
		}
		if target == "" {
			gp, err := globalAliasPath()
			if err != nil {
				log.Warn("alias migration: cannot resolve global path", "error", err)
				return
			}
			target = gp
		}

		store := alias.NewStore(target)
		if err := store.Load(); err != nil {
			log.Warn("alias migration: load failed", "path", target, "error", err)
			return
		}

		// Migrate when the YAML store is empty. When non-empty, treat the
		// migration as already-done and skip straight to the strip step
		// (the legacy key may still be sitting in config.yaml from a
		// previous run that pre-dates the auto-strip behaviour).
		if len(store.All()) == 0 {
			var migrated int
			for k, v := range raw {
				if v == "" {
					continue
				}
				if err := store.Set(k, v); err != nil {
					log.Warn("alias migration: skip entry", "name", k, "error", err)
					continue
				}
				migrated++
			}
			if migrated == 0 {
				return
			}
			if err := store.Save(); err != nil {
				log.Warn("alias migration: save failed", "path", target, "error", err)
				return
			}
			log.Info("Migrated legacy aliases to YAML store",
				"count", migrated, "path", target)
			// Re-load so verifyMigratedKeys reads the freshly-written
			// values rather than the pre-Save in-memory state.
			if err := store.Load(); err != nil {
				log.Warn("alias migration: reload after save failed", "path", target, "error", err)
				return
			}
		}

		// Defensive verification: only strip the legacy aliases: key from
		// config.yaml after confirming the merged YAML alias view
		// actually contains every entry the legacy map had. The cost of
		// a recurring deprecation warning << the cost of silently
		// losing aliases.
		if !verifyMigratedKeys(raw) {
			log.Warn("alias migration: skipping config.yaml strip — merged YAML store is missing legacy entries")
			return
		}
		stripLegacyAliasesFromConfigs()
	})
}

// verifyMigratedKeys confirms every entry in `legacy` is reachable in
// the merged YAML alias view (seeded + global + project, with project
// overriding global per loadMergedAliases). Returns false on any load
// error or missing/divergent entry — callers must skip the strip step
// in that case.
//
// This must consult the merged view rather than just the migration
// target: the legacy key may have come from a different scope than
// where the migrator wrote (e.g. legacy in user-global config.yaml,
// migrator wrote to project-local store because cwd is inside a
// project). Strip safety requires the entries be reachable somewhere
// after migration, not necessarily co-located with the legacy key.
func verifyMigratedKeys(legacy map[string]string) bool {
	merged, err := loadMergedAliases()
	if err != nil {
		return false
	}
	for k, v := range legacy {
		if v == "" {
			continue // empty values are skipped by the migrator; ignore here
		}
		got, ok := merged[k]
		if !ok || got != v {
			return false
		}
	}
	return true
}

// stripLegacyAliasesFromConfigs removes the deprecated `aliases:` key
// from every tlc config file in the loaded cascade. Uses
// kit/core/config.Unset which preserves comments + ordering, writes
// atomically, and cleans empty parent mappings. Errors are logged but
// never fail the CLI run — leaving the key in place just means the
// deprecation warning fires again on the next invocation.
//
// Walks the full project cascade (all ancestor `.tlc/config.yaml`,
// `.tlc.yaml`, `.hop/tlc/config.yaml`, `.hop/tlc.yaml` files between
// cwd and the project boundary) plus the user-level config. System
// configs (/etc/tlc/) are skipped (root-only).
func stripLegacyAliasesFromConfigs() {
	for _, path := range allLegacyConfigPaths() {
		if err := unsetAliasesAtPath(path); err != nil {
			if errors.Is(err, kitconfig.ErrKeyNotFound) {
				continue // legacy key wasn't in this file; OK
			}
			log.Warn("alias migration: failed to strip legacy key from config",
				"path", path, "error", err)
			continue
		}
		log.Info("Removed deprecated aliases: key from config.yaml",
			"path", path)
	}
}

// unsetAliasesAtPath strips the top-level `aliases:` key from a single
// config file. Treats the file as the project-scope target so callers
// can iterate over an arbitrary cascade without conflating each entry
// with kit's user/project distinction.
func unsetAliasesAtPath(path string) error {
	opts := kitconfig.Options{ProjectConfigPath: path}
	return kitconfig.Unset("aliases", kitconfig.ScopeProject, opts)
}

// allLegacyConfigPaths enumerates every config file that may carry a
// legacy `aliases:` key in the current process's view: the full
// project cascade for the active mode (so flat `.tlc.yaml` + dir
// `.tlc/config.yaml` + ancestor files are all covered) plus the
// user-level config. Returns absolute paths in cascade order
// (closest-to-cwd first), deduplicated, and skips system configs.
func allLegacyConfigPaths() []string {
	seen := make(map[string]struct{})
	var out []string

	// Project cascade: walk findAllConfigs to honor the same flat/dir
	// + ancestor walking that initConfig uses for viper merging.
	if cwd, err := os.Getwd(); err == nil {
		for _, p := range findAllConfigs(cwd, "") {
			if abs, err := filepath.Abs(p); err == nil {
				if _, ok := seen[abs]; !ok {
					seen[abs] = struct{}{}
					out = append(out, abs)
				}
			}
		}
	}

	// User-level config (XDG / OS-native).
	if userPath, err := config.UserConfigPath(); err == nil && userPath != "" {
		if abs, err := filepath.Abs(userPath); err == nil {
			if _, ok := seen[abs]; !ok {
				seen[abs] = struct{}{}
				out = append(out, abs)
			}
		}
	}

	return out
}

var migrateLegacyAliasesOnce sync.Once

// isLocalConfigFile reports whether the given path points at a tlc
// project-local config file (.tlc/config.yaml, .hop/tlc/config.yaml, or
// the flat variants). Used by the legacy migration to decide whether
// entries should land in the project store or the global one.
func isLocalConfigFile(path string) bool {
	if path == "" {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, mode := range []config.EntryMode{config.ModeStandalone, config.ModeHop} {
		dirCfg := filepath.Join(config.LocalConfigDir(mode), "config.yaml")
		if strings.HasSuffix(abs, string(filepath.Separator)+dirCfg) {
			return true
		}
		flat := config.LocalConfigFile(mode)
		if strings.HasSuffix(abs, string(filepath.Separator)+flat) {
			return true
		}
	}
	return false
}
