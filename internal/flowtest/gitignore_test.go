package flowtest

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGitignoreShimNamesAreScoped verifies that shim binary names in
// .gitignore only ignore files under internal/flowtest/shims/bin/, not
// files with the same name elsewhere in the repo tree.
//
// Regression: bare names like "gh", "git", "docker" must not match
// outside shims/bin/; the .gitignore uses a directory-scoped pattern.
func TestGitignoreShimNamesAreScoped(t *testing.T) {
	// Find the repo root (where .gitignore lives).
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("git rev-parse --show-toplevel: %v", err)
	}
	repoRoot := strings.TrimSpace(string(out))

	// Shim names taken from .gitignore that should ONLY apply to
	// internal/flowtest/shims/bin/<name>.
	shimNames := []string{
		"gh", "git", "docker", "npm", "claude", "codex",
	}

	// Create a temp directory inside the repo (not under shims/bin/).
	tmpDir := filepath.Join(repoRoot, "internal", "flowtest", "testdata_gitignore_tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", tmpDir, err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	for _, name := range shimNames {
		name := name
		t.Run(name+"_outside_shims_not_ignored", func(t *testing.T) {
			// Create a file with the shim name outside shims/bin/.
			p := filepath.Join(tmpDir, name)
			if err := os.WriteFile(p, []byte("test"), 0o644); err != nil {
				t.Fatalf("write %s: %v", p, err)
			}
			t.Cleanup(func() { os.Remove(p) })

			// git check-ignore exits 0 if the path IS ignored, 1 if NOT ignored,
			// 128 on error. We must distinguish 1 from real errors.
			cmd := exec.Command("git", "check-ignore", "-q", p)
			cmd.Dir = repoRoot
			err := cmd.Run()
			if err == nil {
				t.Errorf("%s is ignored by git outside shims/bin/ — "+
					".gitignore rule is too broad", p)
				return
			}
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				t.Fatalf("git check-ignore %s: unexpected error: %v", p, err)
			}
			// exit code 1 (not ignored) is the expected/correct behaviour.
		})

		t.Run(name+"_inside_shims_bin_ignored", func(t *testing.T) {
			// The intended path SHOULD be ignored.
			p := filepath.Join(repoRoot, "internal", "flowtest", "shims", "bin", name)
			cmd := exec.Command("git", "check-ignore", "-q", p)
			cmd.Dir = repoRoot
			err := cmd.Run()
			if err != nil {
				t.Errorf("%s is NOT ignored by git — "+
					"shims/bin/ binaries should be ignored", p)
			}
		})
	}
}
