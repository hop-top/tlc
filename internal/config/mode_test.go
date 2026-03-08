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

func TestDetectMode_HopDirInAncestor(t *testing.T) {
	t.Setenv("TLC_MODE", "")
	tmp := t.TempDir()

	// Create .hop/ in the project root, cwd is a subdirectory.
	hopDir := filepath.Join(tmp, ".hop")
	if err := os.Mkdir(hopDir, 0o755); err != nil {
		t.Fatal(err)
	}

	subDir := filepath.Join(tmp, "src", "pkg")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got := detectModeFrom(subDir)
	if got != ModeHop {
		t.Errorf("detectModeFrom(%q) = %q, want %q", subDir, got, ModeHop)
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

func TestCheckConfigConflict_BothExist(t *testing.T) {
	tmp := t.TempDir()
	// Create both .tlc/ and .hop/tlc/ at the same level.
	if err := os.MkdirAll(filepath.Join(tmp, ".tlc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, ".hop", "tlc"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := CheckConfigConflict(tmp)
	if err == nil {
		t.Fatal("CheckConfigConflict() = nil, want error when both .tlc/ and .hop/tlc/ exist")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error should mention ambiguous, got: %v", err)
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
	// Both flat config files exist.
	if err := os.WriteFile(filepath.Join(tmp, ".tlc.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	hopDir := filepath.Join(tmp, ".hop")
	if err := os.MkdirAll(hopDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hopDir, "tlc.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	err := CheckConfigConflict(tmp)
	if err == nil {
		t.Fatal("CheckConfigConflict() = nil, want error when both .tlc.yaml and .hop/tlc.yaml exist")
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
