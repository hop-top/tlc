// Package cli provides the TLC CLI commands.
package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"hop.top/tlc/internal/config"
)

// aliasConfigKey is the YAML key used to store aliases.
const aliasConfigKey = "aliases"

// aliasMap is a map of alias name → expansion string.
type aliasMap map[string]string

// seededAliases are built-in aliases shipped with TLC. User-defined
// aliases (global or local) override seeded ones.
var seededAliases = aliasMap{
	"setup": "config interactive",
}

// loadAliases loads aliases from both global and local config files.
// Local aliases take precedence over global ones, and both override
// seeded (built-in) aliases.
func loadAliases() (aliasMap, error) {
	global, err := loadAliasesFrom(globalAliasPath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load global aliases: %w", err)
	}

	local, err := loadAliasesFrom(localAliasPath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load local aliases: %w", err)
	}

	merged := make(aliasMap, len(seededAliases)+len(global)+len(local))
	// Lowest priority: seeded
	for k, v := range seededAliases {
		merged[k] = v
	}
	// Mid priority: global
	for k, v := range global {
		merged[k] = v
	}
	// Highest priority: local
	for k, v := range local {
		merged[k] = v
	}
	return merged, nil
}

// loadAliasesFrom reads aliases from a single YAML config file.
func loadAliasesFrom(path string) (aliasMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Aliases aliasMap `yaml:"aliases"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if raw.Aliases == nil {
		return aliasMap{}, nil
	}
	return raw.Aliases, nil
}

// saveAliasesTo writes the aliases map into the aliases section of a YAML
// config file, preserving any existing keys outside the aliases block.
func saveAliasesTo(path string, aliases aliasMap) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// Read existing content to preserve non-alias keys.
	existing := make(map[string]interface{})
	if data, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(data, &existing)
	}

	if len(aliases) == 0 {
		delete(existing, aliasConfigKey)
	} else {
		existing[aliasConfigKey] = map[string]string(aliases)
	}

	out, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, out, 0o600)
}

// globalAliasPath returns the path to the global config file.
func globalAliasPath() string {
	p, _ := config.UserConfigPath()
	return p
}

// localAliasPath returns the path to the closest local config file.
// Falls back to .tlc/config.yaml in the current directory when no
// local config is found by the cascade.
func localAliasPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.Join(".tlc", "config.yaml")
	}
	mode := config.DetectMode()
	cfgDir := config.LocalConfigDir(mode)
	return filepath.Join(cwd, cfgDir, "config.yaml")
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

	aliases, err := loadAliases()
	if err != nil {
		// Fall back to seeded aliases if config loading fails.
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
	prefix := args[1 : 1+firstNonFlag]          // flags before alias name
	suffix := args[1+firstNonFlag+1:]            // args after alias name
	result := make([]string, 0, 1+len(prefix)+len(parts)+len(suffix))
	result = append(result, args[0])             // program name
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
