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
	"gopkg.in/yaml.v3"
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

// migrateLegacyAliases reads the deprecated `aliases:` block from each
// loaded config file directly (not via viper's merged view) and routes
// each block to the YAML alias store that matches the source's scope:
//
//   - user-level config → global aliases.yaml (xdg ConfigDir/tlc/)
//   - project-level config (any layout) → project-local aliases.yaml
//     (next to the config file's tlc dir, or alongside the flat config)
//
// Per-source routing prevents scope drift: legacy entries from
// user-global config never get co-mingled into a project-local store
// just because the user happens to be running tlc inside a project at
// migration time. Each source's entries land in the store of matching
// visibility, and the legacy key is stripped from each source file
// independently after a successful per-source migration.
//
// Idempotent: when a target store already contains every entry from a
// given source, the migration step is a no-op and we proceed straight
// to the strip step (the legacy key may still be sitting in config.yaml
// from a previous run that pre-dates per-source routing).
//
// Errors during migration of one source do not block other sources;
// each is logged independently and the strip step for that one source
// is skipped.
func migrateLegacyAliases() {
	migrateLegacyAliasesOnce.Do(func() {
		// Cheap upfront probe via viper's merged view: when no source
		// has any legacy entries, viper.GetStringMapString returns an
		// empty map and we can early-return without any I/O.
		if len(viper.GetStringMapString("aliases")) == 0 {
			return
		}
		log.Warn(
			"config key \"aliases:\" is deprecated; tlc has migrated entries " +
				"to <config-dir>/aliases.yaml — remove the key from config.yaml",
		)

		for _, cfgPath := range allLegacyConfigPaths() {
			migrateLegacyAliasesFromSource(cfgPath)
		}
	})
}

// migrateLegacyAliasesFromSource handles one config file's `aliases:`
// block: parse it directly, route to the matching scope's YAML store,
// migrate (if the target store is missing entries), verify, and strip
// the legacy key from the source file. Errors are logged but never
// fail the function — other sources still get processed.
func migrateLegacyAliasesFromSource(cfgPath string) {
	entries, err := readAliasesBlock(cfgPath)
	if err != nil {
		log.Warn("alias migration: cannot read source config", "path", cfgPath, "error", err)
		return
	}
	if len(entries) == 0 {
		return // no legacy block in this file; nothing to do
	}

	target, err := storePathForConfig(cfgPath)
	if err != nil {
		log.Warn("alias migration: cannot resolve target store", "source", cfgPath, "error", err)
		return
	}
	store := alias.NewStore(target)
	if err := store.Load(); err != nil {
		log.Warn("alias migration: load target store failed", "path", target, "error", err)
		return
	}

	// Migrate any entries the target store is missing. Pre-existing
	// entries in the target are preserved (don't clobber a user-edited
	// alias with a stale legacy value). When a key collides with a
	// divergent value in the target, mark the source as having a
	// conflict and skip the strip step at the end — but keep migrating
	// the source's other non-conflicting keys so partial progress isn't
	// lost on the next run.
	stored := store.All()
	var migrated int
	hasConflict := false
	for k, v := range entries {
		if v == "" {
			continue
		}
		if existing, ok := stored[k]; ok && existing == v {
			continue // already migrated
		}
		if _, ok := stored[k]; ok {
			// Same key, different value: target wins. Don't overwrite;
			// flag the source for skip-strip; continue with other keys.
			log.Warn("alias migration: target store has divergent value; will skip strip for this source",
				"alias", k, "source", cfgPath, "target", target)
			hasConflict = true
			continue
		}
		if err := store.Set(k, v); err != nil {
			log.Warn("alias migration: set failed", "alias", k, "error", err)
			hasConflict = true
			continue
		}
		migrated++
	}
	if migrated > 0 {
		if err := store.Save(); err != nil {
			log.Warn("alias migration: save failed", "path", target, "error", err)
			return
		}
		log.Info("Migrated legacy aliases to YAML store",
			"count", migrated, "source", cfgPath, "target", target)
		if err := store.Load(); err != nil {
			log.Warn("alias migration: reload after save failed", "path", target, "error", err)
			return
		}
	}

	// Skip strip when any key conflicted with the target store: leaving
	// the legacy block in place is how the user notices the divergence
	// and decides which value to keep. Non-conflicting keys above were
	// still migrated (partial progress preserved for the next run).
	if hasConflict {
		log.Warn("alias migration: skipping strip — at least one key diverges between source and target",
			"source", cfgPath, "target", target)
		return
	}

	// Defensive verification: every entry from this source must now be
	// present in the target store with matching value.
	if !verifyEntriesInStore(entries, store) {
		log.Warn("alias migration: skipping strip — target store is missing entries from source",
			"source", cfgPath, "target", target)
		return
	}

	// Strip the legacy `aliases:` key from this source.
	if err := unsetAliasesAtPath(cfgPath); err != nil {
		if errors.Is(err, kitconfig.ErrKeyNotFound) {
			return // already stripped
		}
		log.Warn("alias migration: strip failed", "path", cfgPath, "error", err)
		return
	}
	log.Info("Removed deprecated aliases: key from config.yaml", "path", cfgPath)
}

// readAliasesBlock parses path directly (not via viper) and extracts
// the top-level `aliases:` map. Returns an empty map when the file is
// missing, has no `aliases:` key, or the key holds a non-map value
// (e.g. scalar or list — treated as "no legacy aliases here" rather
// than as a fatal parse error so a user with an unexpected shape
// still gets the rest of the migration).
//
// Errors only on I/O failures or YAML that fails the top-level parse.
// Per-entry shape problems (non-string keys/values under aliases:)
// are silently skipped: the entry is left out of the returned map but
// the function does not error. Callers see "no legacy block here" and
// move on.
func readAliasesBlock(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	// Parse permissively into a yaml.Node so we can branch on the
	// shape of the `aliases:` value rather than relying on
	// strict map[string]string unmarshalling (which errors on any
	// scalar/list/non-string value).
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	mapping := unwrapDocumentMapping(&root)
	if mapping == nil {
		return nil, nil
	}
	aliasesNode := findMappingValue(mapping, "aliases")
	if aliasesNode == nil || aliasesNode.Kind != yaml.MappingNode {
		// No aliases: key, or it holds a non-map value. Treat both as
		// "no legacy block here" — caller sees an empty result and
		// moves on without warning the user about exotic shapes.
		return nil, nil
	}
	out := make(map[string]string, len(aliasesNode.Content)/2)
	for i := 0; i+1 < len(aliasesNode.Content); i += 2 {
		k := aliasesNode.Content[i]
		v := aliasesNode.Content[i+1]
		if k.Kind != yaml.ScalarNode || v.Kind != yaml.ScalarNode {
			continue // skip non-scalar entries silently
		}
		out[k.Value] = v.Value
	}
	return out, nil
}

// unwrapDocumentMapping returns the top-level mapping node of a parsed
// YAML doc, walking past the document wrapper. Returns nil for empty
// or non-mapping documents.
func unwrapDocumentMapping(root *yaml.Node) *yaml.Node {
	if root == nil {
		return nil
	}
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			return nil
		}
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return nil
	}
	return root
}

// findMappingValue returns the value node paired with key in mapping,
// or nil if key is absent. mapping must be a YAML MappingNode.
func findMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		k := mapping.Content[i]
		if k.Kind == yaml.ScalarNode && k.Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// storePathForConfig returns the YAML alias store path that should
// receive entries from cfgPath. Project-shaped config files route to
// the project-local store sitting next to the config (or alongside the
// flat config); other paths (user-level, system) route to the global
// store under the user's xdg ConfigDir.
func storePathForConfig(cfgPath string) (string, error) {
	if isLocalConfigFile(cfgPath) {
		return projectStorePathFor(cfgPath), nil
	}
	return globalAliasPath()
}

// projectStorePathFor returns the project-local aliases.yaml path that
// pairs with cfgPath. Layout:
//
//	<root>/.tlc/config.yaml      → <root>/.tlc/aliases.yaml
//	<root>/.tlc.yaml             → <root>/.tlc/aliases.yaml
//	<root>/.hop/tlc/config.yaml  → <root>/.hop/tlc/aliases.yaml
//	<root>/.hop/tlc.yaml         → <root>/.hop/tlc/aliases.yaml
//
// The flat-layout variants intentionally land in a sibling dir so a
// project that mixes flat and dir layouts (transient state during
// migration) shares one store.
func projectStorePathFor(cfgPath string) string {
	return filepath.Join(config.ConfigDirForFile(cfgPath), "aliases.yaml")
}

// verifyEntriesInStore confirms every entry in src is present in store
// with matching value. Returns false on any divergence.
func verifyEntriesInStore(src map[string]string, store *alias.Store) bool {
	stored := store.All()
	for k, v := range src {
		if v == "" {
			continue
		}
		got, ok := stored[k]
		if !ok || got != v {
			return false
		}
	}
	return true
}

// unsetAliasesAtPath strips the top-level `aliases:` key from a single
// config file. Treats the file as the project-scope target so callers
// can iterate over an arbitrary cascade without conflating each entry
// with kit's user/project distinction.
func unsetAliasesAtPath(path string) error {
	opts := kitconfig.Options{ProjectConfigPath: path}
	if err := kitconfig.Unset("aliases", kitconfig.ScopeProject, opts); err != nil {
		return fmt.Errorf("unset aliases in %s: %w", path, err)
	}
	return nil
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
