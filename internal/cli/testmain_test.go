package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain sets the COLORTERM env to truecolor before running the suite,
// so that lipgloss v2 detects TrueColor and tests exercise the same ANSI
// rendering path as a real terminal. This catches regressions where ANSI
// escape sequences embedded in table cell data corrupt runewidth
// calculations and truncate visible content.
//
// After the suite, it fails the run if a test dropped a config file into
// the package directory. Several commands persist configuration with
// viper.WriteConfig, which falls back to ".tlc.yaml" in the process
// working directory when no config file is set. A test that reaches such
// a command without chdir'ing away writes a real config file into the
// source tree: it shows up untracked in git status, and any later run in
// that directory discovers it as a project config and inherits its state.
// Tests that exercise those paths must chdir into a temp directory and
// point viper at a config path inside it.
func TestMain(m *testing.M) {
	os.Setenv("COLORTERM", "truecolor")

	// Resolve before running: a test may leave the process in another
	// directory, and the check must target the package directory itself.
	pkgDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: getwd: %v\n", err)
		os.Exit(1)
	}

	before := snapshotConfigPaths(pkgDir)

	code := m.Run()

	if code == 0 {
		if err := checkNoStrayConfig(pkgDir, before); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}

	os.Exit(code)
}

// strayConfigNames are the config paths viper writes when no explicit
// config file is set, relative to the working directory.
//
// ".tlc/config.yaml" is the dangerous one: ".tlc/" is gitignored
// repo-wide, so a file written there is invisible to git status. Which
// path a leak takes depends on whether the directory already exists, so
// two checkouts of the same commit can behave differently — one absorbs
// the write silently, the other leaves a visible ".tlc.yaml". Both are
// checked so neither shape can hide a regression.
var strayConfigNames = []string{
	".tlc.yaml",
	".tlc.yml",
	".tlc.json",
	filepath.Join(".tlc", "config.yaml"),
	filepath.Join(".tlc", "config.yml"),
	filepath.Join(".tlc", "config.json"),
}

// configFingerprint identifies one candidate path at a point in time, so
// the guard can tell "this file was already here" from "this run created
// or rewrote it". Size and modtime move on any rewrite; inode changes
// when a writer replaces the file rather than truncating it.
type configFingerprint struct {
	present bool
	size    int64
	modTime int64
	inode   uint64
}

// snapshotConfigPaths fingerprints every candidate path in dir before the
// suite runs.
func snapshotConfigPaths(dir string) map[string]configFingerprint {
	snap := make(map[string]configFingerprint, len(strayConfigNames))
	for _, name := range strayConfigNames {
		snap[name] = fingerprintConfigPath(filepath.Join(dir, name))
	}
	return snap
}

// fingerprintConfigPath stats one path. A missing path — or one that
// cannot be stat'ed — fingerprints as absent, so a file that appears
// later is still attributed to this run.
func fingerprintConfigPath(path string) configFingerprint {
	info, err := os.Stat(path)
	if err != nil {
		return configFingerprint{}
	}
	fp := configFingerprint{
		present: true,
		size:    info.Size(),
		modTime: info.ModTime().UnixNano(),
	}
	fp.inode = statInode(info)
	return fp
}

// checkNoStrayConfig reports a config file this run left behind in dir.
//
// Attribution, not just detection: it compares each candidate path against
// the pre-run snapshot and fails only on a path that APPEARED or CHANGED
// while the suite ran. A file that was already present and untouched
// belongs to something else — typically a sibling `go test` process
// against the same worktree, which is ordinary locally (a suite and a
// -race pass side by side, or several agents each running suites). The
// old guard blamed whichever process observed the file rather than the
// one that wrote it, so the innocent run went red while the leaking run
// finished clean.
//
// Concurrency assumption: the snapshot narrows blame, it does not
// eliminate it. A sibling process that creates the file entirely within
// this process's m.Run() window is still indistinguishable from a local
// leak and will be reported here. That residual overlap is accepted —
// closing it would need cross-process coordination, and erring toward
// reporting keeps a genuine leak from slipping through. The reverse
// error, blaming a run for a file that predated it, is the one this
// guard rules out.
func checkNoStrayConfig(dir string, before map[string]configFingerprint) error {
	for _, name := range strayConfigNames {
		path := filepath.Join(dir, name)
		now := fingerprintConfigPath(path)
		if !now.present {
			continue
		}
		if prev, ok := before[name]; ok && prev == now {
			// Pre-existing and untouched: not this run's doing.
			continue
		}

		// Removal is deliberately scoped to files this run is confident
		// it caused. Deleting a path a sibling process is actively using
		// is its own hazard — it would yank the config out from under a
		// concurrent run — and the pre-existing case is exactly where
		// that sibling is most likely to exist. Here the file appeared
		// or changed during m.Run(), so this run is the best candidate
		// for having written it, and leaving it in place would poison
		// every later run in the same checkout.
		removed := "The stray file has been removed."
		if err := os.Remove(path); err != nil {
			removed = fmt.Sprintf("Removing it failed: %v — delete it by hand.", err)
		}

		return fmt.Errorf("test wrote a config file into the package directory: %s\n"+
			"A test reached viper.WriteConfig without a config file set, so viper fell back to the\n"+
			"working directory. Chdir into t.TempDir() and call viper.SetConfigFile with a path\n"+
			"inside it — see setupSyncConfigTest in sync_test.go. Production code must route the\n"+
			"write through config.PrepareViperForWrite, which never falls back to the working\n"+
			"directory. Note that a leak under .tlc/ is gitignored and so invisible to git status.\n"+
			"%s", path, removed)
	}
	return nil
}
