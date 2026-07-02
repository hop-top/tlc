package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDetectProject_CalledOnce(t *testing.T) {
	ResetDetectionCache()
	defer ResetDetectionCache()

	result1 := DetectProject()
	result2 := DetectProject()

	if result1 != result2 {
		t.Errorf("DetectProject() returned different pointers: %p vs %p; expected cached result", result1, result2)
	}
}

func TestResetDetectionCache(t *testing.T) {
	ResetDetectionCache()
	defer ResetDetectionCache()

	result1 := DetectProject()
	ResetDetectionCache()
	result2 := DetectProject()

	if result1 == result2 {
		t.Error("expected different pointers after ResetDetectionCache")
	}
}

func TestCreateConfigWithInferredID_SkipsExisting(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// First call creates the config
	if err := CreateConfigWithInferredID("org/repo"); err != nil {
		t.Fatalf("first CreateConfigWithInferredID failed: %v", err)
	}

	info1, err := os.Stat(".tlc/config.yaml")
	if err != nil {
		t.Fatal("config file not created")
	}
	mtime1 := info1.ModTime()

	// Second call with same ID should skip
	if err := CreateConfigWithInferredID("org/repo"); err != nil {
		t.Fatalf("second CreateConfigWithInferredID failed: %v", err)
	}

	info2, _ := os.Stat(".tlc/config.yaml")
	if !info2.ModTime().Equal(mtime1) {
		t.Error("config file was rewritten; expected skip for same project ID")
	}
}

func TestCreateConfigWithInferredID_UpdatesDifferentID(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	if err := CreateConfigWithInferredID("org/repo-a"); err != nil {
		t.Fatal(err)
	}

	// Different ID should overwrite
	if err := CreateConfigWithInferredID("org/repo-b"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(".tlc/config.yaml")
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}

	proj, ok := cfg["project"].(map[string]interface{})
	if !ok {
		t.Fatal("missing project key in config")
	}
	if proj["id"] != "org/repo-b" {
		t.Errorf("expected project ID 'org/repo-b', got %v", proj["id"])
	}
}

// TestCreateConfigWithInferredID_T1303_HopMode_HonorsExistingTlcConfig
// reproduces T-1303: when both .tlc/ and .hop/ exist at the project root
// (e.g. .hop/ is used by git-hop tooling alongside a standalone .tlc/),
// DetectMode picks ModeHop and CreateConfigWithInferredID would write
// .hop/tlc/config.yaml on every invocation — even though the canonical
// project config is .tlc/config.yaml with the same project ID.
//
// Pre-fix: .hop/tlc/config.yaml gets recreated every time.
// Post-fix: existing .tlc/config.yaml claims the project ID; no write.
func TestCreateConfigWithInferredID_T1303_HopMode_HonorsExistingTlcConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-test-t1303-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Seed the canonical .tlc/config.yaml with the project ID.
	if err := os.MkdirAll(".tlc", 0o750); err != nil {
		t.Fatal(err)
	}
	canonicalYAML := []byte("version: 0.1\nproject:\n  id: org/repo\n")
	if err := os.WriteFile(".tlc/config.yaml", canonicalYAML, 0o600); err != nil {
		t.Fatal(err)
	}

	// Make .hop/ exist so DetectMode returns ModeHop. Empty is enough —
	// any presence of .hop/ at this level flips the mode in the real
	// detection path used by git-hop tooling.
	if err := os.MkdirAll(".hop", 0o750); err != nil {
		t.Fatal(err)
	}

	if err := CreateConfigWithInferredID("org/repo"); err != nil {
		t.Fatalf("CreateConfigWithInferredID failed: %v", err)
	}

	if _, err := os.Stat(".hop/tlc/config.yaml"); !os.IsNotExist(err) {
		t.Errorf(".hop/tlc/config.yaml was created despite .tlc/config.yaml claiming the same project ID; T-1303 regression")
	}
}

// TestCreateConfigWithInferredID_T1303_StandaloneMode_HonorsExistingHopConfig
// is the symmetric case: when the canonical config lives in .hop/tlc/ and
// CreateConfigWithInferredID is invoked under ModeStandalone (no .hop/ at
// the level being inspected), the existing .hop/tlc/config.yaml with the
// same project ID should still suppress the .tlc/ rewrite.
//
// Less likely to trigger in practice (DetectMode usually flips to ModeHop
// when .hop/ exists), but the helper must be symmetric so we don't ship a
// half-fix.
func TestCreateConfigWithInferredID_T1303_StandaloneMode_HonorsExistingHopConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-test-t1303-sym-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Seed .hop/tlc/config.yaml with the project ID.
	if err := os.MkdirAll(".hop/tlc", 0o750); err != nil {
		t.Fatal(err)
	}
	hopYAML := []byte("version: 0.1\nproject:\n  id: org/repo\n")
	if err := os.WriteFile(".hop/tlc/config.yaml", hopYAML, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := CreateConfigWithInferredID("org/repo"); err != nil {
		t.Fatalf("CreateConfigWithInferredID failed: %v", err)
	}

	// Neither .tlc/config.yaml nor .tlc.yaml should have been created.
	if _, err := os.Stat(".tlc/config.yaml"); !os.IsNotExist(err) {
		t.Errorf(".tlc/config.yaml was created despite .hop/tlc/config.yaml claiming the same project ID")
	}
	if _, err := os.Stat(".tlc.yaml"); !os.IsNotExist(err) {
		t.Errorf(".tlc.yaml was created despite .hop/tlc/config.yaml claiming the same project ID")
	}
}

// TestCreateConfigWithInferredID_T1303_WalksUpToFindCanonical reproduces
// the subdirectory variant of T-1303: tlc invoked from a project subdir
// must honor the canonical config at the project root rather than
// recreate one at cwd. A cwd-only check misses the parent config; the
// walk-up scan finds it.
func TestCreateConfigWithInferredID_T1303_WalksUpToFindCanonical(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-test-t1303-walkup-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Project root with canonical .tlc/config.yaml.
	if err := os.MkdirAll(filepath.Join(tmpDir, ".tlc"), 0o750); err != nil {
		t.Fatal(err)
	}
	canonicalYAML := []byte("version: 0.1\nproject:\n  id: org/repo\n")
	if err := os.WriteFile(filepath.Join(tmpDir, ".tlc", "config.yaml"), canonicalYAML, 0o600); err != nil {
		t.Fatal(err)
	}

	// Subdirectory tlc gets invoked from.
	subDir := filepath.Join(tmpDir, "internal", "feature")
	if err := os.MkdirAll(subDir, 0o750); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	os.Chdir(subDir)
	defer os.Chdir(origDir)

	if err := CreateConfigWithInferredID("org/repo"); err != nil {
		t.Fatalf("CreateConfigWithInferredID failed: %v", err)
	}

	// No new config should appear in the subdirectory; the ancestor
	// canonical claims the project ID.
	if _, err := os.Stat(filepath.Join(subDir, ".tlc", "config.yaml")); !os.IsNotExist(err) {
		t.Errorf("subdirectory .tlc/config.yaml was created despite ancestor claiming the same project ID")
	}
	if _, err := os.Stat(filepath.Join(subDir, ".hop", "tlc", "config.yaml")); !os.IsNotExist(err) {
		t.Errorf("subdirectory .hop/tlc/config.yaml was created despite ancestor claiming the same project ID")
	}
}

func TestDetectFromGitToplevel_ReturnsDirectoryName(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Resolve symlinks so macOS /var -> /private/var doesn't break comparison
	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(context.Background(), "git", "init", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	got := detectFromGitToplevel()
	want := filepath.Base(tmpDir)
	if got != want {
		t.Errorf("detectFromGitToplevel() = %q, want %q", got, want)
	}
}

func TestDetectFromGitToplevel_NotGitRepo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	got := detectFromGitToplevel()
	if got != "" {
		t.Errorf("detectFromGitToplevel() = %q, want empty string", got)
	}
}

func TestDetectFromDirectory_FindsCurrentDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create .git in tmpDir so detectFromDirectory finds it
	if err := os.Mkdir(filepath.Join(tmpDir, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	got := detectFromDirectory()
	want := filepath.Base(tmpDir)
	if got != want {
		t.Errorf("detectFromDirectory() = %q, want %q (should find current dir)", got, want)
	}
}

func TestDetectProjectID_NoRemote_UsesGitToplevel(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(context.Background(), "git", "init", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	got := DetectProjectID()
	want := filepath.Base(tmpDir)
	if got != want {
		t.Errorf("DetectProjectID() = %q, want %q", got, want)
	}
}

func TestDetectProjectID_NoGit_UsesDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create .git marker so detectFromDirectory finds it (not a real repo)
	if err := os.Mkdir(filepath.Join(tmpDir, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	got := DetectProjectID()
	want := filepath.Base(tmpDir)
	if got != want {
		t.Errorf("DetectProjectID() = %q, want %q", got, want)
	}
}

func TestHandleFallbackMode_NoRemote_StillDetectsProject(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(context.Background(), "git", "init", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	ResetDetectionCache()
	defer ResetDetectionCache()

	result := handleFallbackMode()
	if !result.InProject {
		t.Error("handleFallbackMode() InProject = false, want true")
	}
	want := filepath.Base(tmpDir)
	if result.ProjectID != want {
		t.Errorf("handleFallbackMode() ProjectID = %q, want %q", result.ProjectID, want)
	}
}
