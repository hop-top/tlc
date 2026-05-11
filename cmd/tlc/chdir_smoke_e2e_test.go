// Package main exercises the `-C / --chdir` flag end-to-end against a
// freshly built tlc binary. This is the pre-merge smoke for the chdir
// feature shipped in 2b9335e / 814c397: it must remain green on every
// PR to main so that "tlc -C <project> ..." stays a reliable way to
// run read-only commands against a project other than the current cwd.
package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestChdir_Smoke_E2E_SwitchesProject builds the tlc binary, sets up
// two projects (projA, projB) each with one task, and asserts that
// from cwd=projA, `tlc -C projB task list` returns projB's task —
// not projA's. Failure means the chdir pre-parse has regressed.
func TestChdir_Smoke_E2E_SwitchesProject(t *testing.T) {
	bin := buildTLC(t)

	tmpHome := t.TempDir()
	root := t.TempDir()
	projA := filepath.Join(root, "projA")
	projB := filepath.Join(root, "projB")

	for _, p := range []struct{ dir, id, title string }{
		{projA, "projA", "Task in A"},
		{projB, "projB", "Task in B"},
	} {
		tlcDir := filepath.Join(p.dir, ".tlc")
		if err := os.MkdirAll(tlcDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", tlcDir, err)
		}
		body := "project:\n  id: " + p.id + "\nstorage:\n  db_path: " +
			filepath.Join(tlcDir, "db.sqlite") + "\n"
		if err := os.WriteFile(filepath.Join(tlcDir, "config.yaml"), []byte(body), 0o644); err != nil {
			t.Fatalf("write config for %s: %v", p.id, err)
		}
		if out, err := runTLC(bin, p.dir, tmpHome, "task", "create", p.title); err != nil {
			t.Fatalf("seed task in %s: %v\n%s", p.id, err, out)
		}
	}

	// Sanity: from cwd=projA, plain `task list` shows A's task.
	outA, err := runTLC(bin, projA, tmpHome, "task", "list")
	if err != nil {
		t.Fatalf("task list in projA: %v\n%s", err, outA)
	}
	if !bytes.Contains(outA, []byte("Task in A")) {
		t.Fatalf("projA task list missing 'Task in A':\n%s", outA)
	}

	// The real assertion: from cwd=projA, `-C projB task list` must
	// resolve into projB and return B's task, not A's.
	outB, err := runTLC(bin, projA, tmpHome, "-C", projB, "task", "list")
	if err != nil {
		t.Fatalf("tlc -C projB task list (cwd=projA): %v\n%s", err, outB)
	}
	if !bytes.Contains(outB, []byte("Task in B")) {
		t.Fatalf("tlc -C projB task list missing 'Task in B':\n%s", outB)
	}
	if bytes.Contains(outB, []byte("Task in A")) {
		t.Fatalf("tlc -C projB task list leaked projA's task:\n%s", outB)
	}
}

// runTLC invokes bin with args from cwd, isolating HOME/XDG_CONFIG_HOME
// so the smoke never touches the developer's real config or DB.
func runTLC(bin, cwd, home string, args ...string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	cmd.Dir = cwd
	cmd.Env = []string{
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"PATH=" + os.Getenv("PATH"),
	}
	return cmd.CombinedOutput()
}
