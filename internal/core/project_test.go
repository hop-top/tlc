package core

import (
	"os"
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
	if err := createConfigWithInferredID("org/repo"); err != nil {
		t.Fatalf("first createConfigWithInferredID failed: %v", err)
	}

	info1, err := os.Stat(".tlc/config.yaml")
	if err != nil {
		t.Fatal("config file not created")
	}
	mtime1 := info1.ModTime()

	// Second call with same ID should skip
	if err := createConfigWithInferredID("org/repo"); err != nil {
		t.Fatalf("second createConfigWithInferredID failed: %v", err)
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

	if err := createConfigWithInferredID("org/repo-a"); err != nil {
		t.Fatal(err)
	}

	// Different ID should overwrite
	if err := createConfigWithInferredID("org/repo-b"); err != nil {
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
