package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/storage"
)

// TestResolveConfigFlag_ExistingFile verifies that an existing file
// path is returned as-is.
func TestResolveConfigFlag_ExistingFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(f, []byte("output:\n  format: json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveConfigFlag(f)
	if err != nil {
		t.Fatalf("resolveConfigFlag(%q) error: %v", f, err)
	}
	if got != f {
		t.Fatalf("resolveConfigFlag(%q) = %q, want %q", f, got, f)
	}
}

// TestResolveConfigFlag_Directory verifies that a directory resolves
// to <dir>/<localConfigDir>/config.yaml.
func TestResolveConfigFlag_Directory(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveConfigFlag(dir)
	if err != nil {
		t.Fatalf("resolveConfigFlag(%q) error: %v", dir, err)
	}
	if !strings.HasSuffix(got, "config.yaml") {
		t.Fatalf("expected config.yaml suffix, got %q", got)
	}
}

// TestResolveConfigFlag_MissingPathFails verifies that a path-like
// argument that doesn't exist returns an error.
func TestResolveConfigFlag_MissingPathFails(t *testing.T) {
	_, err := resolveConfigFlag("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' in error, got: %v", err)
	}
}

// TestResolveConfigFlag_ShortnameNotFound verifies that a shortname
// not in the registry returns an actionable error.
func TestResolveConfigFlag_ShortnameNotFound(t *testing.T) {
	// Override the resolver to use a temp DB with no projects.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "global.sqlite")
	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	orig := resolveConfigShortname
	t.Cleanup(func() { resolveConfigShortname = orig })
	resolveConfigShortname = func(shortname string) (string, error) {
		p, err := s.ResolveProjectByShortname(
			context.Background(), shortname,
		)
		if err != nil {
			return "", err
		}
		if p == nil {
			return "", errConfigNotFound(shortname)
		}
		return filepath.Join(filepath.Dir(p.DBPath), "config.yaml"), nil
	}

	_, err = resolveConfigFlag("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown shortname, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "tlc project list") {
		t.Fatalf("expected actionable hint, got: %v", err)
	}
}

// TestResolveConfigFlag_ShortnameResolvesFromRegistry verifies that
// a shortname matching a registered project resolves to its config.
func TestResolveConfigFlag_ShortnameResolvesFromRegistry(t *testing.T) {
	dir := t.TempDir()

	// Set up a fake project with config.
	projDir := filepath.Join(dir, "myproject", ".tlc")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(projDir, "config.yaml")
	if err := os.WriteFile(configPath,
		[]byte("output:\n  format: tls\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	projDBPath := filepath.Join(projDir, "db.sqlite")

	// Register in a global DB.
	globalDB := filepath.Join(dir, "global.sqlite")
	s, err := storage.NewSQLiteStorage(globalDB)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.RegisterProject(ctx, "hop-top/myproject",
		projDBPath, "", "My Project"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Override resolver to use our test DB.
	orig := resolveConfigShortname
	t.Cleanup(func() { resolveConfigShortname = orig })
	resolveConfigShortname = func(shortname string) (string, error) {
		ts, err := storage.NewSQLiteStorage(globalDB)
		if err != nil {
			return "", errConfigNotFound(shortname)
		}
		defer ts.Close()
		p, err := ts.ResolveProjectByShortname(ctx, shortname)
		if err != nil {
			return "", err
		}
		if p == nil {
			return "", errConfigNotFound(shortname)
		}
		cp := filepath.Join(filepath.Dir(p.DBPath), "config.yaml")
		if _, err := os.Stat(cp); err != nil {
			return "", err
		}
		return cp, nil
	}

	got, err := resolveConfigFlag("myproject")
	if err != nil {
		t.Fatalf("resolveConfigFlag(myproject) error: %v", err)
	}
	if got != configPath {
		t.Fatalf("resolveConfigFlag(myproject) = %q, want %q",
			got, configPath)
	}
}

// TestResolveConfigFlag_ExactMatchPriority verifies that an exact
// project_id match takes priority over suffix match.
func TestResolveConfigFlag_ExactMatchPriority(t *testing.T) {
	dir := t.TempDir()

	// Set up two projects: "tlc" (exact) and "hop-top/tlc" (suffix).
	for _, name := range []string{"tlc", "hop-top/tlc"} {
		projDir := filepath.Join(dir, strings.ReplaceAll(name, "/", "_"), ".tlc")
		if err := os.MkdirAll(projDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(projDir, "config.yaml"),
			[]byte("project:\n  id: "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	globalDB := filepath.Join(dir, "global.sqlite")
	s, err := storage.NewSQLiteStorage(globalDB)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	s.RegisterProject(ctx, "tlc",
		filepath.Join(dir, "tlc", ".tlc", "db.sqlite"), "", "TLC exact")
	s.RegisterProject(ctx, "hop-top/tlc",
		filepath.Join(dir, "hop-top_tlc", ".tlc", "db.sqlite"), "", "TLC namespaced")
	s.Close()

	orig := resolveConfigShortname
	t.Cleanup(func() { resolveConfigShortname = orig })
	resolveConfigShortname = func(shortname string) (string, error) {
		ts, err := storage.NewSQLiteStorage(globalDB)
		if err != nil {
			return "", errConfigNotFound(shortname)
		}
		defer ts.Close()
		p, err := ts.ResolveProjectByShortname(ctx, shortname)
		if err != nil {
			return "", err
		}
		if p == nil {
			return "", errConfigNotFound(shortname)
		}
		cp := filepath.Join(filepath.Dir(p.DBPath), "config.yaml")
		if _, err := os.Stat(cp); err != nil {
			return "", err
		}
		return cp, nil
	}

	got, err := resolveConfigFlag("tlc")
	if err != nil {
		t.Fatalf("resolveConfigFlag(tlc) error: %v", err)
	}
	// Should resolve to the exact match "tlc", not "hop-top/tlc".
	expected := filepath.Join(dir, "tlc", ".tlc", "config.yaml")
	if got != expected {
		t.Fatalf("resolveConfigFlag(tlc) = %q, want %q", got, expected)
	}
}

// TestInitConfig_ShortnameNeverFallsBackToCwd verifies that passing
// an unknown shortname to -c does not silently use cwd config.
func TestInitConfig_ShortnameNeverFallsBackToCwd(t *testing.T) {
	withTestLock(func() {
		dir := t.TempDir()

		// Create a cwd config that would be picked up on fallback.
		cwdCfg := filepath.Join(dir, ".tlc.yaml")
		if err := os.WriteFile(cwdCfg,
			[]byte("output:\n  format: cwd-sentinel\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		oldWd, _ := os.Getwd()
		os.Chdir(dir)
		defer os.Chdir(oldWd)

		// Override resolver to simulate "not found".
		orig := resolveConfigShortname
		defer func() { resolveConfigShortname = orig }()
		resolveConfigShortname = func(shortname string) (string, error) {
			return "", errConfigNotFound(shortname)
		}

		// resolveConfigFlag should return an error, not silently succeed.
		_, err := resolveConfigFlag("bogus-project")
		if err == nil {
			t.Fatal("expected error for unknown shortname, got nil — " +
				"silent fallback to cwd is the bug")
		}

		// Also verify the cwd config was NOT loaded.
		viper.Reset()
		if got := viper.GetString("output.format"); got == "cwd-sentinel" {
			t.Fatal("cwd config was loaded despite unknown -c shortname; " +
				"silent fallback is the bug")
		}
	})
}
