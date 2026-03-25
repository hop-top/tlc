package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInferLabel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hop-top/tlc", "tlc"},
		{"org/sub/repo", "repo"},
		{"simple", "simple"},
		{"a/b", "b"},
	}
	for _, tt := range tests {
		if got := inferLabel(tt.input); got != tt.want {
			t.Errorf("inferLabel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestInferSpaceURI(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	wDir := filepath.Join(fakeHome, ".w", "testorg-init-reconnect")
	projDir := filepath.Join(wDir, "myrepo")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("failed to create test dir: %v", err)
	}

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	os.Chdir(projDir)
	got := inferSpaceURI()
	if got != wDir {
		t.Errorf("inferSpaceURI() = %q, want %q", got, wDir)
	}

	// Outside of .w/ should return empty
	os.Chdir(origDir)
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	if got := inferSpaceURI(); got != "" {
		t.Errorf("inferSpaceURI() outside .w = %q, want empty", got)
	}
}
