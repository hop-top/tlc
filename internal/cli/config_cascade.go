package cli

import (
	"errors"
	"path/filepath"

	"charm.land/log/v2"
	"hop.top/tlc/internal/config"
)

// pendingConfigErr is a project-config error found while the cascade
// was assembled in initConfig, which runs from cobra.OnInitialize and
// cannot return one. PersistentPreRunE surfaces it before any command
// body runs, so an ambiguous config aborts the command instead of
// running it against whatever the cascade happened to load.
var pendingConfigErr error

// resolveProjectConfigs returns the project config cascade for startDir,
// closest first, for the files the process will actually merge.
//
// Every directory on the walk is checked for a standalone/hop pair that
// disagrees; that is refused with both files named. A pair that agrees
// (a hub mirroring .tlc/config.yaml into .hop/tlc/config.yaml) loads the
// mode's copy and is logged at debug level only.
//
// In hop mode with no hop config anywhere on the walk, the standalone
// layout is loaded instead. Hop mode can be forced (TLC_MODE=hop) or
// inherited, and a project whose only config is .tlc/ must keep reading
// its own store rather than silently falling through to the global one.
func resolveProjectConfigs(startDir, stopDir string, mode config.EntryMode) ([]string, error) {
	if err := checkConfigConflicts(startDir, stopDir); err != nil {
		return nil, err
	}
	configs := findAllConfigsForMode(startDir, stopDir, mode)
	if mode == config.ModeHop && len(configs) == 0 {
		configs = findAllConfigsForMode(startDir, stopDir, config.ModeStandalone)
	}
	return configs, nil
}

// checkConfigConflicts runs config.CheckConfigConflict on each directory
// findAllConfigsForMode visits, closest first, and wraps the first
// disagreement as a conflict-class exit.
func checkConfigConflicts(startDir, stopDir string) error {
	curr := normalizeConfigPath(startDir)
	stopDir = normalizeConfigPath(stopDir)
	if curr == "" {
		return nil
	}
	for {
		if err := config.CheckConfigConflict(curr); err != nil {
			return configConflictExit(err)
		}
		if config.BothLayoutsPresent(curr) {
			log.Debug("standalone and hop configs agree", "dir", curr)
		}
		if stopDir != "" && curr == stopDir {
			return nil
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			return nil
		}
		curr = parent
	}
}

// configConflictExit maps a config conflict onto the conflict exit
// class (4) so scripts can tell "ambiguous config" from a generic
// failure. Non-conflict errors pass through unchanged.
func configConflictExit(err error) error {
	var cce *config.ConfigConflictError
	if !errors.As(err, &cce) {
		return err
	}
	return &ExitCodeError{Code: ExitConflict, Message: err.Error()}
}

// errAlreadyInitialized is the init refusal when a local config already
// exists at the target directory in either layout.
func errAlreadyInitialized(path string) error {
	return &ExitCodeError{
		Code:    ExitConflict,
		Message: "already initialized at " + path + "; re-run with: tlc init --force to overwrite",
	}
}
