package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectMode_EnvOverride(t *testing.T) {
	tests := []struct {
		env  string
		want EntryMode
	}{
		{"hop", ModeHop},
		{"HOP", ModeHop},
		{"standalone", ModeStandalone},
		{"STANDALONE", ModeStandalone},
	}
	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			t.Setenv("TLC_MODE", tt.env)
			if got := detectModeFrom("/tmp"); got != tt.want {
				t.Errorf("DetectMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectMode_CwdInsideHopPath(t *testing.T) {
	t.Setenv("TLC_MODE", "")
	tmp := t.TempDir()

	// Simulate a path with .hop segment: tmp/.hop/worktrees/feat-x
	hopWorktree := filepath.Join(tmp, ".hop", "worktrees", "feat-x")
	if err := os.MkdirAll(hopWorktree, 0o755); err != nil {
		t.Fatal(err)
	}

	got := detectModeFrom(hopWorktree)
	if got != ModeHop {
		t.Errorf("detectModeFrom(%q) = %q, want %q", hopWorktree, got, ModeHop)
	}
}

// TestDetectMode_AncestorLayouts pins the rule that a .hop/ directory
// is a hop indicator only when it carries a tlc config (.hop/tlc/ or
// .hop/tlc.yaml). Other tooling drops a bare .hop/ (repair.lock,
// backups/) into hubs that were initialized standalone; that must not
// flip the mode, or every read silently moves to the global store.
func TestDetectMode_AncestorLayouts(t *testing.T) {
	tests := []struct {
		name  string
		dirs  []string
		files []string
		want  EntryMode
	}{
		{name: "bare .hop", dirs: []string{".hop"}, want: ModeStandalone},
		{
			name:  "bare .hop with repair artifacts",
			dirs:  []string{filepath.Join(".hop", "backups")},
			files: []string{filepath.Join(".hop", "repair.lock")},
			want:  ModeStandalone,
		},
		{name: ".hop/tlc dir", dirs: []string{filepath.Join(".hop", "tlc")}, want: ModeHop},
		{name: ".hop/tlc.yaml", dirs: []string{".hop"}, files: []string{filepath.Join(".hop", "tlc.yaml")}, want: ModeHop},
		{name: ".hop/tlc dir beside .tlc", dirs: []string{filepath.Join(".hop", "tlc"), ".tlc"}, want: ModeHop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TLC_MODE", "")
			tmp := layoutFixture(t, tt.dirs, tt.files)
			subDir := filepath.Join(tmp, "src", "pkg")
			if err := os.MkdirAll(subDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if got := detectModeFrom(subDir); got != tt.want {
				t.Errorf("detectModeFrom(%q) = %q, want %q", subDir, got, tt.want)
			}
		})
	}
}

func TestDetectMode_DefaultStandalone(t *testing.T) {
	t.Setenv("TLC_MODE", "")
	tmp := t.TempDir()

	got := detectModeFrom(tmp)
	if got != ModeStandalone {
		t.Errorf("detectModeFrom(%q) = %q, want %q", tmp, got, ModeStandalone)
	}
}

func TestLocalConfigDir(t *testing.T) {
	if got := LocalConfigDir(ModeStandalone); got != ".tlc" {
		t.Errorf("LocalConfigDir(standalone) = %q, want %q", got, ".tlc")
	}
	if got := LocalConfigDir(ModeHop); got != filepath.Join(".hop", "tlc") {
		t.Errorf("LocalConfigDir(hop) = %q, want %q", got, filepath.Join(".hop", "tlc"))
	}
}

func TestLocalConfigFile(t *testing.T) {
	if got := LocalConfigFile(ModeStandalone); got != ".tlc.yaml" {
		t.Errorf("LocalConfigFile(standalone) = %q, want %q", got, ".tlc.yaml")
	}
	if got := LocalConfigFile(ModeHop); got != filepath.Join(".hop", "tlc.yaml") {
		t.Errorf("LocalConfigFile(hop) = %q, want %q", got, filepath.Join(".hop", "tlc.yaml"))
	}
}

func TestValidateLocalConfig_StandaloneNoError(t *testing.T) {
	tmp := t.TempDir()
	// Standalone mode: no config is not an error.
	if err := ValidateLocalConfig(ModeStandalone, tmp); err != nil {
		t.Errorf("ValidateLocalConfig(standalone) = %v, want nil", err)
	}
}

func TestValidateLocalConfig_HopMissingReturnsError(t *testing.T) {
	tmp := t.TempDir()
	err := ValidateLocalConfig(ModeHop, tmp)
	if err == nil {
		t.Fatal("ValidateLocalConfig(hop) = nil, want error for missing config")
	}
}

func TestValidateLocalConfig_HopDirPresent(t *testing.T) {
	tmp := t.TempDir()
	hopTLC := filepath.Join(tmp, ".hop", "tlc")
	if err := os.MkdirAll(hopTLC, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ValidateLocalConfig(ModeHop, tmp); err != nil {
		t.Errorf("ValidateLocalConfig(hop) = %v, want nil", err)
	}
}

func TestValidateLocalConfig_HopFlatFilePresent(t *testing.T) {
	tmp := t.TempDir()
	hopDir := filepath.Join(tmp, ".hop")
	if err := os.MkdirAll(hopDir, 0o755); err != nil {
		t.Fatal(err)
	}
	flatFile := filepath.Join(hopDir, "tlc.yaml")
	if err := os.WriteFile(flatFile, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ValidateLocalConfig(ModeHop, tmp); err != nil {
		t.Errorf("ValidateLocalConfig(hop) = %v, want nil", err)
	}
}

func TestValidateLocalConfig_HopWithStandaloneFallback(t *testing.T) {
	tmp := t.TempDir()
	// Create .hop/ (triggers hop mode detection) but no .hop/tlc/.
	if err := os.MkdirAll(filepath.Join(tmp, ".hop"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Create .tlc/ (standalone config) — should suppress the warning.
	if err := os.MkdirAll(filepath.Join(tmp, ".tlc"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ValidateLocalConfig(ModeHop, tmp); err != nil {
		t.Errorf("ValidateLocalConfig(hop) = %v, want nil when .tlc/ exists", err)
	}
}

const (
	conflictCfgA = "version: 0.1\nproject:\n  id: org/one\nstorage:\n  db_path: /srv/one/db.sqlite\n"
	conflictCfgB = "version: 0.1\nproject:\n  id: org/two\nstorage:\n  db_path: /srv/two/db.sqlite\n"
)

func writeConflictFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCheckConfigConflict_AgreeingPair covers hubs that mirror
// .tlc/config.yaml into .hop/tlc/config.yaml: same project.id and same
// absolute db_path is not a conflict and must stay silent.
func TestCheckConfigConflict_AgreeingPair(t *testing.T) {
	tmp := t.TempDir()
	writeConflictFile(t, filepath.Join(tmp, ".tlc", "config.yaml"), conflictCfgA)
	writeConflictFile(t, filepath.Join(tmp, ".hop", "tlc", "config.yaml"), conflictCfgA)

	if err := CheckConfigConflict(tmp); err != nil {
		t.Errorf("CheckConfigConflict() = %v, want nil for an agreeing pair", err)
	}
}

// TestCheckConfigConflict_DisagreeingPair requires the error to name
// both files and both (project.id, db_path) pairs so the fix is obvious.
func TestCheckConfigConflict_DisagreeingPair(t *testing.T) {
	tmp := t.TempDir()
	standalone := filepath.Join(tmp, ".tlc", "config.yaml")
	hop := filepath.Join(tmp, ".hop", "tlc", "config.yaml")
	writeConflictFile(t, standalone, conflictCfgA)
	writeConflictFile(t, hop, conflictCfgB)

	err := CheckConfigConflict(tmp)
	if err == nil {
		t.Fatal("CheckConfigConflict() = nil, want error for a disagreeing pair")
	}
	for _, want := range []string{standalone, hop, "org/one", "org/two", "/srv/one/db.sqlite", "/srv/two/db.sqlite", "ambiguous"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}

// TestCheckConfigConflict_DisagreeOnDBPathOnly: same id, different store
// is still ambiguous — the rows live in two places.
func TestCheckConfigConflict_DisagreeOnDBPathOnly(t *testing.T) {
	tmp := t.TempDir()
	writeConflictFile(t, filepath.Join(tmp, ".tlc", "config.yaml"), conflictCfgA)
	writeConflictFile(t, filepath.Join(tmp, ".hop", "tlc", "config.yaml"),
		"version: 0.1\nproject:\n  id: org/one\n")

	if err := CheckConfigConflict(tmp); err == nil {
		t.Fatal("CheckConfigConflict() = nil, want error when db_path differs")
	}
}

func TestCheckConfigConflict_OnlyStandalone(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".tlc"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := CheckConfigConflict(tmp); err != nil {
		t.Errorf("CheckConfigConflict() = %v, want nil with only .tlc/", err)
	}
}

func TestCheckConfigConflict_OnlyHop(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".hop", "tlc"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := CheckConfigConflict(tmp); err != nil {
		t.Errorf("CheckConfigConflict() = %v, want nil with only .hop/tlc/", err)
	}
}

func TestCheckConfigConflict_Neither(t *testing.T) {
	tmp := t.TempDir()
	if err := CheckConfigConflict(tmp); err != nil {
		t.Errorf("CheckConfigConflict() = %v, want nil with neither", err)
	}
}

func TestCheckConfigConflict_HopDirWithoutTLC(t *testing.T) {
	tmp := t.TempDir()
	// .hop/ exists but no .hop/tlc/ — no conflict with .tlc/
	if err := os.MkdirAll(filepath.Join(tmp, ".hop"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, ".tlc"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := CheckConfigConflict(tmp); err != nil {
		t.Errorf("CheckConfigConflict() = %v, want nil when .hop/ exists without .hop/tlc/", err)
	}
}

func TestCheckConfigConflict_FlatFiles(t *testing.T) {
	tmp := t.TempDir()
	// Both flat config files exist and disagree.
	writeConflictFile(t, filepath.Join(tmp, ".tlc.yaml"), conflictCfgA)
	writeConflictFile(t, filepath.Join(tmp, ".hop", "tlc.yaml"), conflictCfgB)

	err := CheckConfigConflict(tmp)
	if err == nil {
		t.Fatal("CheckConfigConflict() = nil, want error when .tlc.yaml and .hop/tlc.yaml disagree")
	}
}

func TestContainsHopSegment(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/home/user/.hop/worktrees/feat-x", true},
		{"/home/user/project/src", false},
		{"/home/user/.hopping/src", false},
		{"/.hop", true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := containsHopSegment(tt.path); got != tt.want {
				t.Errorf("containsHopSegment(%q) = %v, want %v",
					tt.path, got, tt.want)
			}
		})
	}
}
