package flowtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGitignoreShimNamesAreScoped verifies that shim binary names in
// .gitignore only ignore files under internal/flowtest/shims/bin/, not
// files with the same name elsewhere in the repo tree.
//
// This test FAILS with the current .gitignore because bare names like
// "gh", "git", "docker" match anywhere in the repo.
func TestGitignoreShimNamesAreScoped(t *testing.T) {
	// Find the repo root (where .gitignore lives).
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("git rev-parse --show-toplevel: %v", err)
	}
	repoRoot := string(out[:len(out)-1]) // trim newline

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

			// git check-ignore exits 0 if the path IS ignored, 1 if NOT ignored.
			cmd := exec.Command("git", "check-ignore", "-q", p)
			cmd.Dir = repoRoot
			err := cmd.Run()
			if err == nil {
				t.Errorf("%s is ignored by git outside shims/bin/ — "+
					".gitignore rule is too broad", p)
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
