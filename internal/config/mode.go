package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// EntryMode represents how tlc was invoked.
type EntryMode string

const (
	// ModeStandalone means tlc was invoked directly.
	ModeStandalone EntryMode = "standalone"
	// ModeHop means tlc was invoked through git hop tooling.
	ModeHop EntryMode = "hop"
)

// DetectMode checks how tlc was invoked.
// Returns ModeHop if:
//   - $TLC_MODE == "hop", OR
//   - the binary is running from within a .hop/ directory structure, OR
//   - the current or an ancestor directory holds a hop config
//     (.hop/tlc/ or .hop/tlc.yaml)
//
// A .hop/ directory without a tlc config inside it is NOT a hop
// indicator: repair tooling leaves a bare .hop/ in standalone hubs, and
// treating it as hop mode redirected every read to the global store.
//
// Returns ModeStandalone if $TLC_MODE == "standalone" or none of the
// hop indicators are found.
func DetectMode() EntryMode {
	return detectModeFrom("")
}

// detectModeFrom is the testable core; when startDir is empty it uses os.Getwd.
func detectModeFrom(startDir string) EntryMode {
	// 1. Explicit env override.
	if env := os.Getenv("TLC_MODE"); env != "" {
		switch strings.ToLower(env) {
		case "hop":
			return ModeHop
		case "standalone":
			return ModeStandalone
		}
	}

	dir := startDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return ModeStandalone
		}
	}

	// 2. Check if cwd itself sits inside a .hop/ path component.
	if containsHopSegment(dir) {
		return ModeHop
	}

	// 3. Walk up from cwd looking for a hop config.
	if findHopConfigDir(dir) {
		return ModeHop
	}

	return ModeStandalone
}

// containsHopSegment reports whether any path component is ".hop".
func containsHopSegment(path string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(path), "/") {
		if seg == hopDirName {
			return true
		}
	}
	return false
}

// findHopConfigDir walks from dir up to the filesystem root looking for
// a directory that holds a hop config (see HasHopConfig).
func findHopConfigDir(dir string) bool {
	dir = filepath.Clean(dir)
	for {
		if HasHopConfig(dir) {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return false
}

// LocalConfigDir returns the project-local config directory NAME for
// the given mode.
//   - ModeStandalone -> ".tlc"
//   - ModeHop        -> ".hop/tlc"
//
// The result is deliberately relative: callers join it onto an
// arbitrary directory while walking up the tree, suffix-match paths
// against it, and hand it to kit as a project marker. Returning an
// absolute path would silently break all of those.
//
// Because it is relative, a caller that joins onto it and writes LATER
// resolves it against wherever the process happens to be standing at
// write time. Any command that writes through it must resolve it
// against a directory captured at entry — see LocalConfigDirAt.
func LocalConfigDir(mode EntryMode) string {
	switch mode {
	case ModeHop:
		return filepath.Join(hopDirName, hopConfigDirName)
	default:
		return ".tlc"
	}
}

// LocalConfigDirAt returns the project-local config directory for the
// given mode, resolved against baseDir.
//
// Writers must use this rather than LocalConfigDir. Passing the
// directory the command was invoked from pins the destination at entry,
// so a later chdir — a goroutine outliving its sandbox, a deferred
// cleanup restoring a saved directory — cannot redirect the write. A
// misdirected write is silent: .tlc/ is gitignored, so a config that
// lands in the wrong directory never appears in git status.
//
// An empty or relative baseDir is resolved against the current working
// directory, which preserves the old behavior for callers that have
// nothing better to offer.
func LocalConfigDirAt(baseDir string, mode EntryMode) string {
	dir := LocalConfigDir(mode)
	if baseDir == "" {
		return dir
	}
	if !filepath.IsAbs(baseDir) {
		if abs, err := filepath.Abs(baseDir); err == nil {
			baseDir = abs
		}
	}
	return filepath.Join(baseDir, dir)
}

// LocalConfigFile returns the "flat" project config filename for the
// given mode.
//   - ModeStandalone -> ".tlc.yaml"
//   - ModeHop        -> ".hop/tlc.yaml"
func LocalConfigFile(mode EntryMode) string {
	switch mode {
	case ModeHop:
		return filepath.Join(hopDirName, hopFlatConfigName)
	default:
		return ".tlc.yaml"
	}
}

// ConfigConflictError reports a standalone and a hop config at the same
// directory that resolve to different projects or stores.
type ConfigConflictError struct {
	Dir            string
	StandalonePath string
	HopPath        string
	Standalone     ConfigIdentity
	Hop            ConfigIdentity
}

// ConfigIdentity is the (project.id, storage.db_path) pair a config
// resolves to. DBPath is absolute when set; empty means the global store.
type ConfigIdentity struct {
	ProjectID string
	DBPath    string
}

func (e *ConfigConflictError) Error() string {
	return fmt.Sprintf(
		"ambiguous config at %s: standalone %s (project.id=%s, db_path=%s) and hop %s (project.id=%s, db_path=%s) disagree; "+
			"make both files declare the same project.id and storage.db_path, or remove one of them",
		e.Dir,
		e.StandalonePath, orGlobal(e.Standalone.ProjectID), orGlobal(e.Standalone.DBPath),
		e.HopPath, orGlobal(e.Hop.ProjectID), orGlobal(e.Hop.DBPath),
	)
}

func orGlobal(v string) string {
	if v == "" {
		return "<unset>"
	}
	return v
}

// BothLayoutsPresent reports whether dir has a config in each layout.
func BothLayoutsPresent(dir string) bool {
	_, hasStandalone := localConfigPath(dir, ModeStandalone)
	_, hasHop := localConfigPath(dir, ModeHop)
	return hasStandalone && hasHop
}

// CheckConfigConflict returns a *ConfigConflictError when both a
// standalone (.tlc/ or .tlc.yaml) and a hop (.hop/tlc/ or .hop/tlc.yaml)
// config exist at dir AND they resolve to a different project.id or a
// different absolute storage.db_path. Hubs that mirror one config into
// the other layout agree on both and are not a conflict.
func CheckConfigConflict(dir string) error {
	standalonePath, hasStandalone := localConfigPath(dir, ModeStandalone)
	hopPath, hasHop := localConfigPath(dir, ModeHop)
	if !hasStandalone || !hasHop {
		return nil
	}
	standalone := readConfigIdentity(dir, standalonePath)
	hop := readConfigIdentity(dir, hopPath)
	if standalone == hop {
		return nil
	}
	return &ConfigConflictError{
		Dir:            dir,
		StandalonePath: standalonePath,
		HopPath:        hopPath,
		Standalone:     standalone,
		Hop:            hop,
	}
}

// localConfigPath returns the config file for mode at dir: the directory
// form when it holds a config.yaml, else the flat file. A bare config
// directory without config.yaml counts as present (it still anchors
// tracks, tasks and the DB) but contributes no identity.
func localConfigPath(dir string, mode EntryMode) (string, bool) {
	dirForm := filepath.Join(dir, LocalConfigDir(mode))
	if isDir(dirForm) {
		return filepath.Join(dirForm, "config.yaml"), true
	}
	flat := filepath.Join(dir, LocalConfigFile(mode))
	if isFile(flat) {
		return flat, true
	}
	return "", false
}

// readConfigIdentity parses just project.id and storage.db_path from
// path. A relative db_path is anchored at dir so two configs at the same
// directory compare by the store they actually open. Unreadable or
// malformed files yield an empty identity.
func readConfigIdentity(dir, path string) ConfigIdentity {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ConfigIdentity{}
	}
	var cfg struct {
		Project struct {
			ID string `yaml:"id"`
		} `yaml:"project"`
		Storage struct {
			DBPath string `yaml:"db_path"`
		} `yaml:"storage"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return ConfigIdentity{}
	}
	id := ConfigIdentity{ProjectID: cfg.Project.ID, DBPath: cfg.Storage.DBPath}
	if id.DBPath != "" && !filepath.IsAbs(id.DBPath) {
		id.DBPath = filepath.Join(dir, id.DBPath)
	}
	if id.DBPath != "" {
		id.DBPath = filepath.Clean(id.DBPath)
	}
	return id
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// ValidateLocalConfig checks that the expected local config directory
// exists at or above startDir. Returns an error when the mode is
// detected (especially hop) but the expected directory is missing.
func ValidateLocalConfig(mode EntryMode, startDir string) error {
	if startDir == "" {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("determine working directory: %w", err)
		}
	}

	configDir := LocalConfigDir(mode)
	dir := filepath.Clean(startDir)

	for {
		candidate := filepath.Join(dir, configDir)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return nil
		}
		// Also accept the flat file.
		flat := filepath.Join(dir, LocalConfigFile(mode))
		if _, err := os.Stat(flat); err == nil {
			return nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	if mode == ModeHop {
		// Before warning, check if standalone config exists -- the project
		// may have been initialized with `tlc init` (standalone) in a repo
		// that happens to contain a .hop/ directory.
		standaloneDir := filepath.Clean(startDir)
		for {
			if info, err := os.Stat(filepath.Join(standaloneDir, LocalConfigDir(ModeStandalone))); err == nil && info.IsDir() {
				return nil
			}
			if _, err := os.Stat(filepath.Join(standaloneDir, LocalConfigFile(ModeStandalone))); err == nil {
				return nil
			}
			parent := filepath.Dir(standaloneDir)
			if parent == standaloneDir {
				break
			}
			standaloneDir = parent
		}
		return fmt.Errorf(
			"hop mode detected but no %s or %s found at or above %s",
			LocalConfigDir(mode), LocalConfigFile(mode), startDir,
		)
	}

	// Standalone mode: missing local config is not an error (user/system
	// config may still apply).
	return nil
}
