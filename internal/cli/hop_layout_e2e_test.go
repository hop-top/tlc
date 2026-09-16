package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hopLayoutEnv is e2eEnv without the pinned TLC_STORAGE_DB_PATH: these
// runs must read storage.db_path from the PROJECT config, because the
// defect under test is precisely that the project config stops loading
// and reads fall through to the global store. The global store still
// lives under the private XDG_DATA_HOME, so nothing reaches the real
// registry. TLC_MODE is stripped so the on-disk layout alone decides.
func hopLayoutEnv(t *testing.T, home string) []string {
	t.Helper()
	var env []string
	for _, e := range e2eEnv(t, home, filepath.Join(home, "unused.sqlite")) {
		if strings.HasPrefix(e, "TLC_STORAGE_DB_PATH=") || strings.HasPrefix(e, "TLC_MODE=") {
			continue
		}
		env = append(env, e)
	}
	return env
}

func hopLayoutConfig(projectID, dbPath string) string {
	return fmt.Sprintf("version: 0.1\nproject:\n  id: %s\nstorage:\n  backend: sqlite\n  db_path: %s\n", projectID, dbPath)
}

func writeHopLayoutFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// bareHopDir drops what repair tooling leaves in a hub: .hop/ holding
// only repair.lock and backups/, no tlc config.
func bareHopDir(t *testing.T, hub string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(hub, ".hop", "backups"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeHopLayoutFile(t, filepath.Join(hub, ".hop", "repair.lock"), "")
}

// TestHopLayout_E2E drives the built binary through the hub layouts that
// produced silent data loss: a standalone project that later grows a
// bare .hop/ must keep reading its own store, a mirrored (agreeing) pair
// must stay quiet, and a disagreeing pair must fail loudly instead of
// listing zero rows.
func TestHopLayout_E2E(t *testing.T) {
	bin := buildTLCBinary(t)

	t.Run("bare .hop keeps standalone reads and init refuses", func(t *testing.T) {
		home := t.TempDir()
		hub := t.TempDir()
		env := hopLayoutEnv(t, home)
		writeHopLayoutFile(t, filepath.Join(hub, ".tlc", "config.yaml"),
			hopLayoutConfig("x/hub-b", filepath.Join(hub, ".tlc", "db.sqlite")))

		runTLCOK(t, bin, hub, env, "task", "create", "seed task")
		if out := runTLCOK(t, bin, hub, env, "task", "list"); !strings.Contains(out, "seed task") {
			t.Fatalf("precondition: seeded task not listed before .hop/ exists:\n%s", out)
		}

		bareHopDir(t, hub)

		out := runTLCOK(t, bin, hub, env, "task", "list")
		if !strings.Contains(out, "T-0001") || !strings.Contains(out, "seed task") {
			t.Errorf("task list lost the seeded task after a bare .hop/ appeared:\n%s", out)
		}
		if strings.Contains(out, "WARN") {
			t.Errorf("task list should not warn on a bare .hop/:\n%s", out)
		}

		initOut, code := runTLC(t, bin, hub, env, "init", "--no-track")
		if code == 0 {
			t.Errorf("init should refuse when .tlc/ already exists; got exit 0:\n%s", initOut)
		}
		if !strings.Contains(initOut, "already initialized") {
			t.Errorf("init refusal should say already initialized:\n%s", initOut)
		}
		if _, err := os.Stat(filepath.Join(hub, ".hop", "tlc")); err == nil {
			t.Errorf("init must not scaffold .hop/tlc next to an existing .tlc/")
		}
	})

	t.Run("TLC_MODE=hop with only .tlc still reads the standalone store", func(t *testing.T) {
		home := t.TempDir()
		hub := t.TempDir()
		env := hopLayoutEnv(t, home)
		writeHopLayoutFile(t, filepath.Join(hub, ".tlc", "config.yaml"),
			hopLayoutConfig("x/hub-f", filepath.Join(hub, ".tlc", "db.sqlite")))
		runTLCOK(t, bin, hub, env, "task", "create", "forced task")

		out := runTLCOK(t, bin, hub, append(env, "TLC_MODE=hop"), "task", "list")
		if !strings.Contains(out, "forced task") {
			t.Errorf("forced hop mode must fall back to the standalone config:\n%s", out)
		}
	})

	t.Run("agreeing mirror stays quiet", func(t *testing.T) {
		home := t.TempDir()
		hub := t.TempDir()
		env := hopLayoutEnv(t, home)
		cfg := hopLayoutConfig("x/hub-m", filepath.Join(hub, ".tlc", "db.sqlite"))
		writeHopLayoutFile(t, filepath.Join(hub, ".tlc", "config.yaml"), cfg)
		writeHopLayoutFile(t, filepath.Join(hub, ".hop", "tlc", "config.yaml"), cfg)

		runTLCOK(t, bin, hub, env, "task", "create", "mirrored task")
		out := runTLCOK(t, bin, hub, env, "task", "list")
		if !strings.Contains(out, "mirrored task") {
			t.Errorf("agreeing pair should list the seeded task:\n%s", out)
		}
		if strings.Contains(out, "WARN") || strings.Contains(out, "ambiguous") {
			t.Errorf("agreeing pair must not warn:\n%s", out)
		}
	})

	t.Run("disagreeing pair fails loudly", func(t *testing.T) {
		home := t.TempDir()
		// Resolved so the paths the error names compare equal on macOS,
		// where t.TempDir() is a /var symlink into /private/var.
		hub, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		env := hopLayoutEnv(t, home)
		standalone := filepath.Join(hub, ".tlc", "config.yaml")
		hop := filepath.Join(hub, ".hop", "tlc", "config.yaml")
		writeHopLayoutFile(t, standalone, hopLayoutConfig("x/one", filepath.Join(hub, ".tlc", "db.sqlite")))
		writeHopLayoutFile(t, hop, hopLayoutConfig("x/two", filepath.Join(hub, ".hop", "tlc", "db.sqlite")))

		out, code := runTLC(t, bin, hub, env, "task", "list")
		if code != ExitConflict {
			t.Errorf("disagreeing pair: exit = %d, want %d\n%s", code, ExitConflict, out)
		}
		// The error box hard-wraps long paths mid-word; collapse all
		// whitespace so a path split across lines still compares whole.
		flat := strings.Join(strings.Fields(out), "")
		for _, want := range []string{standalone, hop, "x/one", "x/two", "disagree"} {
			if !strings.Contains(flat, want) {
				t.Errorf("error should name %q:\n%s", want, out)
			}
		}
	})

	t.Run("bare .hop only: init scaffolds .tlc", func(t *testing.T) {
		home := t.TempDir()
		hub := t.TempDir()
		env := hopLayoutEnv(t, home)
		bareHopDir(t, hub)

		runTLCOK(t, bin, hub, env, "init", "--no-track")
		if _, err := os.Stat(filepath.Join(hub, ".tlc", "config.yaml")); err != nil {
			t.Errorf("init on a bare .hop/ should scaffold .tlc/config.yaml: %v", err)
		}
		if _, err := os.Stat(filepath.Join(hub, ".hop", "tlc")); err == nil {
			t.Errorf("init on a bare .hop/ must not scaffold .hop/tlc")
		}
	})
}
