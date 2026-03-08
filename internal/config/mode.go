package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
//   - a .hop/ directory exists in the current or ancestor directory
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

	// 3. Walk up from cwd looking for a .hop/ directory.
	if findHopDir(dir) {
		return ModeHop
	}

	return ModeStandalone
}

// containsHopSegment reports whether any path component is ".hop".
func containsHopSegment(path string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(path), "/") {
		if seg == ".hop" {
			return true
		}
	}
	return false
}

// findHopDir walks from dir up to the filesystem root looking for a
// child directory named ".hop".
func findHopDir(dir string) bool {
	dir = filepath.Clean(dir)
	for {
		candidate := filepath.Join(dir, ".hop")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
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

// LocalConfigDir returns the project-local config directory name for
// the given mode.
//   - ModeStandalone -> ".tlc"
//   - ModeHop        -> ".hop/tlc"
func LocalConfigDir(mode EntryMode) string {
	switch mode {
	case ModeHop:
		return filepath.Join(".hop", "tlc")
	default:
		return ".tlc"
	}
}

// LocalConfigFile returns the "flat" project config filename for the
// given mode.
//   - ModeStandalone -> ".tlc.yaml"
//   - ModeHop        -> ".hop/tlc.yaml"
func LocalConfigFile(mode EntryMode) string {
	switch mode {
	case ModeHop:
		return filepath.Join(".hop", "tlc.yaml")
	default:
		return ".tlc.yaml"
	}
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
		return fmt.Errorf(
			"hop mode detected but no %s or %s found at or above %s",
			LocalConfigDir(mode), LocalConfigFile(mode), startDir,
		)
	}

	// Standalone mode: missing local config is not an error (user/system
	// config may still apply).
	return nil
}
