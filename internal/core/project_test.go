package core

import (
	"os"
	"context"
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
