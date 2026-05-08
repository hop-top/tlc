package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// defaultPoliciesYAML is the bundled tlc policy.  It enforces the
// minimum guard that motivates kit/runtime/policy adoption (T-1192):
// destructive deletes must carry a non-empty `--note`.  Non-destructive
// transitions stay open by default; adopters add custom rules in their
// own $XDG_CONFIG_HOME/tlc/policies.yaml.
//
//go:embed policies_default.yaml
var defaultPoliciesYAML []byte

// stagePoliciesYAML is the optional stage-aware bundle adopters append
// to their policies.yaml when they want CEL-driven enforcement on top
// of the in-process gate (core.GateTrackCreate / core.GateTaskCreate).
// See policies_stage.yaml's comment block for activation steps.
//
//go:embed policies_stage.yaml
var stagePoliciesYAML []byte

// DefaultPoliciesYAML returns the bundled policy bytes.  Callers use
// these when no user-supplied policies.yaml exists, and to seed the
// user file on first boot so adopters can edit it in place.
func DefaultPoliciesYAML() []byte {
	out := make([]byte, len(defaultPoliciesYAML))
	copy(out, defaultPoliciesYAML)
	return out
}

// StagePoliciesYAML returns the optional stage-aware policy bundle.
// Returned bytes are a defensive copy so callers can mutate them
// without poisoning the embedded blob.
//
// The bundle is NOT auto-applied: tlc's in-process gate already covers
// the create paths. Adopters that wire kit/runtime/policy with a
// stage resolver and want fanout coverage append these rules to their
// $XDG_CONFIG_HOME/tlc/policies.yaml manually.
func StagePoliciesYAML() []byte {
	out := make([]byte, len(stagePoliciesYAML))
	copy(out, stagePoliciesYAML)
	return out
}

// PoliciesPath returns the user-level policies.yaml path, alongside
// the existing config.yaml under $XDG_CONFIG_HOME/tlc.
func PoliciesPath() (string, error) {
	dir, err := UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "policies.yaml"), nil
}

// EnsureDefaultPoliciesFile writes DefaultPoliciesYAML() to the user
// policies path when the file does not exist or is empty (size 0).
// Returns the resolved path.  Existing non-empty user files are left
// untouched — never clobber custom rules an adopter may have authored.
func EnsureDefaultPoliciesFile() (string, error) {
	path, err := PoliciesPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", fmt.Errorf("create config directory for policies: %w", err)
	}
	st, err := os.Stat(path)
	switch {
	case err == nil && st.Size() > 0:
		// User file present and non-empty — keep as-is.
		return path, nil
	case err != nil && !os.IsNotExist(err):
		return "", fmt.Errorf("stat %s: %w", path, err)
	}
	// File missing or empty: write the bundled default.
	if err := os.WriteFile(path, DefaultPoliciesYAML(), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}
