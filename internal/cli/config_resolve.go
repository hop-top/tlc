package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/storage"
)

// resolveConfigFlag resolves the -c / --config flag value to a config
// file path. It handles three cases:
//  1. Directory: resolves to <dir>/<localConfigDir>/config.yaml
//  2. Existing file: uses the file directly
//  3. Shortname: looks up in the project registry, derives config path
//
// Returns an error if the value is not a file, directory, or known
// project shortname.
func resolveConfigFlag(value string) (string, error) {
	// Case 1: directory
	if info, err := os.Stat(value); err == nil && info.IsDir() {
		return filepath.Join(
			value,
			config.LocalConfigDir(config.DetectMode()),
			"config.yaml",
		), nil
	}

	// Case 2: existing file
	if _, err := os.Stat(value); err == nil {
		return value, nil
	}

	// Case 3: not a file/dir — try project registry lookup.
	// Heuristic: if value contains a path separator or looks like a
	// path (starts with . / ~), treat as a missing file path.
	if strings.ContainsAny(value, "/\\") ||
		strings.HasPrefix(value, ".") ||
		strings.HasPrefix(value, "~") {
		return "", fmt.Errorf(
			"config %q not found; check the path exists",
			value,
		)
	}

	resolved, err := resolveConfigShortname(value)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

// errConfigNotFound returns a standardized error for an unresolvable
// -c value with an actionable hint.
func errConfigNotFound(name string) error {
	return fmt.Errorf(
		"config %q not found as file or registry project; "+
			"run 'tlc project list' to see available projects",
		name,
	)
}

// resolveConfigShortname looks up a project shortname in the global
// registry and returns the config file path derived from db_path.
// The variable is a function so tests can override it.
var resolveConfigShortname = resolveConfigShortnameDefault

func resolveConfigShortnameDefault(shortname string) (string, error) {
	dbPath := filepath.Join(config.UserDataDir(), "db.sqlite")
	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		return "", errConfigNotFound(shortname)
	}
	defer func() { _ = s.Close() }()

	p, err := s.ResolveProjectByShortname(
		context.Background(), shortname,
	)
	if err != nil {
		return "", err
	}
	if p == nil {
		return "", errConfigNotFound(shortname)
	}

	// Derive config path: db_path is typically
	// <project>/.tlc/db.sqlite → replace db.sqlite with config.yaml
	configPath := filepath.Join(filepath.Dir(p.DBPath), "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		return "", fmt.Errorf(
			"project %q found in registry (db: %s) but config "+
				"file %s does not exist",
			shortname, p.DBPath, configPath,
		)
	}
	return configPath, nil
}
