package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindXrayCache_ReturnsContent(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	_ = os.Chdir(tmp)

	content := "# Project Map\n- src/\n- go.mod\n"
	if err := os.WriteFile(".xray_root.md", []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got := findXrayCache()
	if got != content {
		t.Errorf("findXrayCache() = %q, want %q", got, content)
	}
}

func TestFindXrayCache_PicksFirstAlphabetically(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	_ = os.Chdir(tmp)

	_ = os.WriteFile(".xray_a_root.md", []byte("first"), 0644)
	_ = os.WriteFile(".xray_b_chunk.md", []byte("second"), 0644)

	got := findXrayCache()
	if got != "first" {
		t.Errorf("findXrayCache() = %q, want %q", got, "first")
	}
}

func TestFindXrayCache_NoFiles(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	_ = os.Chdir(tmp)

	got := findXrayCache()
	if got != "" {
		t.Errorf("findXrayCache() = %q, want empty", got)
	}
}

func TestGenerateXrayJIT_NoXray(t *testing.T) {
	// Override PATH so xray is not found.
	orig := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", orig) })
	_ = os.Setenv("PATH", t.TempDir())

	got := generateXrayJIT()
	if got != "" {
		t.Errorf("generateXrayJIT() = %q, want empty", got)
	}
}

func TestLoadXrayContext_CachedFile(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	_ = os.Chdir(tmp)

	content := "# Root chunk\nfiles listed here\n"
	_ = os.WriteFile(".xray_root.md", []byte(content), 0644)

	got := loadXrayContext()
	if got != content {
		t.Errorf("loadXrayContext() = %q, want %q", got, content)
	}
}

func TestLoadXrayContext_NoCacheNoXray(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	_ = os.Chdir(tmp)

	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", t.TempDir())

	got := loadXrayContext()
	if got != "" {
		t.Errorf("loadXrayContext() = %q, want empty", got)
	}
}

func TestLoadXrayContext_TruncatesToMaxBytes(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	_ = os.Chdir(tmp)

	// Create content larger than xrayMaxBytes (4096).
	big := strings.Repeat("x", xrayMaxBytes+500)
	_ = os.WriteFile(".xray_root.md", []byte(big), 0644)

	got := loadXrayContext()
	if len(got) != xrayMaxBytes {
		t.Errorf("len(loadXrayContext()) = %d, want %d", len(got), xrayMaxBytes)
	}
}

func TestTruncateBytes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"under limit", "hello", 10, "hello"},
		{"at limit", "hello", 5, "hello"},
		{"over limit", "hello world", 5, "hello"},
		{"empty", "", 5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateBytes(tt.in, tt.n)
			if got != tt.want {
				t.Errorf("truncateBytes(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
			}
		})
	}
}

func TestFindXrayCache_UnreadableFile(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	_ = os.Chdir(tmp)

	path := filepath.Join(tmp, ".xray_root.md")
	_ = os.WriteFile(path, []byte("content"), 0000)

	got := findXrayCache()
	if got != "" {
		t.Errorf("findXrayCache() = %q, want empty for unreadable file", got)
	}
}
