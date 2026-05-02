// Package main tests the `tlc config paths` end-to-end by building the
// tlc binary and invoking it against an isolated HOME so the user-scope
// row in the chain points to a known location. This is the primary
// integration test for kit's shared `config path` / `config paths`
// subcommands wired via internal/cli.tlcConfigPathsResolver.
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// buildTLC builds the tlc binary into a temp directory and returns the
// path to the executable.
func buildTLC(t *testing.T) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "tlc")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", bin, ".")
	cmd.Dir = mustModuleDir(t, "cmd/tlc")
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build tlc: %v\n%s", err, out)
	}
	return bin
}

// mustModuleDir returns an absolute path to subPath relative to the
// module root (the directory containing go.mod).
func mustModuleDir(t *testing.T, subPath string) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// We're already in cmd/tlc when tests run; walk up to find go.mod.
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, subPath)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find go.mod above %s", wd)
		}
		dir = parent
	}
}

// TestConfigPaths_E2E_Chain builds the tlc binary, invokes
// `tlc config paths --format json` against a temp HOME, and asserts the
// chain contains rows for user, system, and default scopes.
func TestConfigPaths_E2E_Chain(t *testing.T) {
	bin := buildTLC(t)

	tmpHome := t.TempDir()
	tmpCwd := t.TempDir()

	cmd := exec.Command(bin, "config", "paths", "--format", "json")
	cmd.Dir = tmpCwd
	cmd.Env = append(
		[]string{},
		"HOME="+tmpHome,
		"XDG_CONFIG_HOME="+filepath.Join(tmpHome, ".config"),
		"PATH="+os.Getenv("PATH"),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run %s config paths: %v\nstderr: %s", bin, err, stderr.String())
	}

	var chain []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &chain); err != nil {
		t.Fatalf("unmarshal json output: %v\nraw: %s", err, stdout.String())
	}
	if len(chain) == 0 {
		t.Fatalf("empty chain: %s", stdout.String())
	}

	// Collect sources for assertions.
	sources := make(map[string]int)
	for _, row := range chain {
		if s, ok := row["source"].(string); ok {
			sources[s]++
		}
	}

	for _, want := range []string{"user", "system", "default"} {
		if sources[want] == 0 {
			t.Errorf("expected at least one row with source=%q; got sources=%v\nchain=%s", want, sources, stdout.String())
		}
	}

	// User-scope path should resolve under the temp HOME we set.
	var userPath string
	for _, row := range chain {
		if s, _ := row["source"].(string); s == "user" {
			userPath, _ = row["path"].(string)
			break
		}
	}
	if userPath == "" {
		t.Fatalf("user scope row has empty path: %s", stdout.String())
	}
	if !strings.HasPrefix(userPath, tmpHome) {
		t.Errorf("user-scope path %q should be under HOME=%q\nchain=%s",
			userPath, tmpHome, stdout.String())
	}
}

// TestConfigPath_E2E_FallsBackToDefaults ensures `tlc config path`
// (singular) succeeds and prints the synthetic <defaults> sentinel when
// no real config file exists in the chain. The chain is never truly
// empty because the resolver always appends a defaults row.
func TestConfigPath_E2E_FallsBackToDefaults(t *testing.T) {
	bin := buildTLC(t)

	tmpHome := t.TempDir()
	tmpCwd := t.TempDir()

	cmd := exec.Command(bin, "config", "path")
	cmd.Dir = tmpCwd
	cmd.Env = append(
		[]string{},
		"HOME="+tmpHome,
		"XDG_CONFIG_HOME="+filepath.Join(tmpHome, ".config"),
		"PATH="+os.Getenv("PATH"),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run %s config path: %v\nstderr: %s", bin, err, stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	if got != "<defaults>" {
		t.Errorf("expected <defaults> when no real config file exists; got %q", got)
	}
}

// TestConfigPath_E2E_PrefersProjectFile ensures `tlc config path`
// returns the project file when one exists, taking precedence over
// user / system / defaults.
func TestConfigPath_E2E_PrefersProjectFile(t *testing.T) {
	bin := buildTLC(t)

	tmpHome := t.TempDir()
	tmpCwd := t.TempDir()

	// Create a project-scope config so it wins the chain.
	projDir := filepath.Join(tmpCwd, ".tlc")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir .tlc: %v", err)
	}
	projConfig := filepath.Join(projDir, "config.yaml")
	if err := os.WriteFile(projConfig, []byte("flow:\n  dir: examples/flows\n"), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	cmd := exec.Command(bin, "config", "path")
	cmd.Dir = tmpCwd
	cmd.Env = append(
		[]string{},
		"HOME="+tmpHome,
		"XDG_CONFIG_HOME="+filepath.Join(tmpHome, ".config"),
		"PATH="+os.Getenv("PATH"),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run %s config path: %v\nstderr: %s", bin, err, stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	// macOS resolves /var → /private/var. Compare via filepath.EvalSymlinks.
	wantResolved, _ := filepath.EvalSymlinks(projConfig)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != wantResolved && got != projConfig {
		t.Errorf("expected project config %q to win; got %q", projConfig, got)
	}
}
