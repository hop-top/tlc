package config

import (
	"path/filepath"
)

// hopDirName is the directory git-hop tooling owns at a hub root. Its
// presence alone says nothing about tlc: repair tooling drops a bare
// .hop/ (repair.lock, backups/) into hubs that were initialized
// standalone. Only the tlc layout INSIDE it — .hop/tlc/ or .hop/tlc.yaml
// — is a hop config. Every ".hop" decision in the codebase goes through
// the helpers in this file so that rule is stated once.
const hopDirName = ".hop"

// hopConfigDirName is the tlc config directory inside .hop/.
const hopConfigDirName = "tlc"

// hopFlatConfigName is the flat tlc config file inside .hop/.
const hopFlatConfigName = "tlc.yaml"

// HasHopConfig reports whether dir holds a hop-layout tlc config: a
// .hop/tlc/ directory or a .hop/tlc.yaml file. A .hop/ directory holding
// neither is not a hop indicator.
func HasHopConfig(dir string) bool {
	return isDir(filepath.Join(dir, LocalConfigDir(ModeHop))) ||
		isFile(filepath.Join(dir, LocalConfigFile(ModeHop)))
}

// ExistingLocalConfig returns the project-local config file present at
// dir across both layouts, hop first because mode detection prefers hop
// whenever a hop config exists. Within a layout the directory form wins
// over the flat file. An empty .tlc/ or .hop/tlc/ directory with no
// config.yaml is not a config.
func ExistingLocalConfig(dir string) (path string, mode EntryMode, ok bool) {
	for _, m := range []EntryMode{ModeHop, ModeStandalone} {
		for _, candidate := range []string{
			filepath.Join(dir, LocalConfigDir(m), "config.yaml"),
			filepath.Join(dir, LocalConfigFile(m)),
		} {
			if isFile(candidate) {
				return candidate, m, true
			}
		}
	}
	return "", ModeStandalone, false
}

// ConfigDirForFile maps a loaded config file path to the config
// directory that pairs with it, by path shape rather than by the
// detected mode — the two can disagree when the mode was forced or
// inherited and the standalone config was loaded as a fallback:
//
//	<root>/.tlc/config.yaml      → <root>/.tlc
//	<root>/.tlc.yaml             → <root>/.tlc
//	<root>/.hop/tlc/config.yaml  → <root>/.hop/tlc
//	<root>/.hop/tlc.yaml         → <root>/.hop/tlc
//
// Any other file is treated as living inside its config directory.
// Empty input yields empty output.
func ConfigDirForFile(cfgPath string) string {
	if cfgPath == "" {
		return ""
	}
	dir := filepath.Dir(cfgPath)
	switch filepath.Base(cfgPath) {
	case LocalConfigFile(ModeStandalone):
		return filepath.Join(dir, LocalConfigDir(ModeStandalone))
	case hopFlatConfigName:
		return filepath.Join(dir, hopConfigDirName)
	default:
		return dir
	}
}

// ProjectRootFromConfigDir returns the project root for a config
// directory: one level up for .tlc, two for .hop/tlc. Empty input
// yields empty output.
func ProjectRootFromConfigDir(configDir string) string {
	if configDir == "" {
		return ""
	}
	root := filepath.Dir(configDir)
	if filepath.Base(root) == hopDirName {
		root = filepath.Dir(root)
	}
	return root
}
