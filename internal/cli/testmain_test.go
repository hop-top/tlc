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
// It also moves the suite's working directory out of the source tree
// before running, and fails the run afterwards if a config file landed
// in the package directory anyway.
//
// The chdir is the fix, not a workaround. Project detection creates a
// config file on first use when none is found — `project.fallback_mode:
// auto` is the documented default — so a command run with no ambient
// project legitimately writes one into the working directory. Go sets
// that directory to the package source directory, so the write lands in
// internal/cli/. Sandboxing the individual test that trips it first does
// not help: detection and DB sync both latch through sync.Once, so the
// write simply migrates to whichever test wins the race next. Moving the
// whole process out of the tree once, before any test runs, sends every
// such write to a temp directory instead.
//
// The post-run guard stays as a regression net for the other shape of
// the same leak: a command that reaches viper.WriteConfig with no config
// file set falls back to ".tlc.yaml" in the working directory. A test
// that chdirs into the package directory and then triggers one would
// reintroduce the stray file the chdir prevents.
func TestMain(m *testing.M) {
	os.Setenv("COLORTERM", "truecolor")

	// Capture before the chdir below: the helpers that locate repo
	// fixtures walk up from here, and the post-run check must target the
	// package directory itself rather than wherever the suite ended up.
	pkgDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: getwd: %v\n", err)
		os.Exit(1)
	}
	testPkgDir = pkgDir

	// Run outside the source tree. A config file the suite legitimately
	// creates then lands in a temp directory that goes away with the
	// process, instead of in the checkout.
	runDir, err := os.MkdirTemp("", "tlc-cli-suite")
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: tempdir: %v\n", err)
		os.Exit(1)
	}
	if err := os.Chdir(runDir); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: chdir %s: %v\n", runDir, err)
		os.Exit(1)
	}

	before := snapshotConfigPaths(pkgDir)

	// Name the writer if one appears. checkNoStrayConfig below runs too
	// late to do that: it sees the finished file, not the goroutine that
	// wrote it. The watcher dumps stacks while the write is happening.
	watchForStrayConfig(pkgDir)

	code := m.Run()

	if code == 0 {
		if err := checkNoStrayConfig(pkgDir, before); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}

	// Leave the temp run directory before removing it: a process cannot
	// reliably operate from a deleted directory, and the removal is
	// best-effort anyway — the OS reclaims it if this fails.
	if err := os.Chdir(pkgDir); err == nil {
		os.RemoveAll(runDir)
	}

	os.Exit(code)
}

// testPkgDir is the package source directory, captured in TestMain
// before the suite chdirs out of the tree.
//
// Helpers that resolve repo-relative paths must start from here rather
// than from os.Getwd: the suite runs from a temp directory, and
// individual tests chdir freely on top of that, so the working
// directory says nothing about where the checkout is.
var testPkgDir string

// testRepoRoot returns the repo root — the nearest ancestor of the
// package source directory holding go.mod.
//
// Three helpers used to each walk up from os.Getwd for this. They are
// one function now, anchored to testPkgDir, so none of them depends on
// where the process happens to be standing.
//
// Distinct from cli.repoRoot (agent_helpers.go), which returns the
// working directory or "/workspace" depending on agent run mode.
func testRepoRoot(t *testing.T) string {
	t.Helper()

	if testPkgDir == "" {
		t.Fatal("testRepoRoot: package directory not captured; TestMain must run first")
	}
	dir := testPkgDir
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod found above %s", testPkgDir)
		}
		dir = parent
	}
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
			"Scroll up for the \"stray config appeared\" report: it dumps the goroutine stacks taken\n"+
			"while the write was in flight, which names the test and the exact write path.\n"+
			"A test reached viper.WriteConfig without a config file set, so viper fell back to the\n"+
			"working directory. Chdir into t.TempDir() and call viper.SetConfigFile with a path\n"+
			"inside it — see setupSyncConfigTest in sync_test.go. Production code must route the\n"+
			"write through config.PrepareViperForWrite, which never falls back to the working\n"+
			"directory. Note that a leak under .tlc/ is gitignored and so invisible to git status.\n"+
			"%s", path, removed)
	}
	return nil
}
