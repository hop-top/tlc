package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// TestRunInit_WritesToInvocationDirAfterChdir pins the invocation
// directory as the destination for everything `tlc init` writes.
//
// config.LocalConfigDir returns a bare relative name (".tlc"). A caller
// that joins onto it and writes later resolves that name against
// whatever directory the process is standing in AT WRITE TIME, not the
// one the command was invoked from. Anything that moves the process
// mid-command — a goroutine outliving its chdir, a deferred cleanup
// restoring a saved directory — therefore redirects the write. The
// damage is silent: .tlc/ is gitignored repo-wide, so a config that
// lands in a source directory never shows up in git status.
//
// The move here is deliberate and synchronous rather than racy: a
// PersistentPreRunE chdirs the process to a second sandbox after the
// command has been entered but before runInit does its work. That is
// the same hazard a stray goroutine creates, minus the timing
// flakiness. Correct behavior is that the config lands in the
// invocation directory and the directory the process moved to stays
// clean.
func TestRunInit_WritesToInvocationDirAfterChdir(t *testing.T) {
	invocationDir := t.TempDir()
	elsewhere := t.TempDir()

	// EvalSymlinks: on macOS t.TempDir() hands back a /var path that is
	// a symlink to /private/var. os.Getwd returns the resolved form, so
	// comparing against the unresolved name would report a spurious
	// mismatch.
	resolvedInvocation, err := filepath.EvalSymlinks(invocationDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(invocationDir): %v", err)
	}
	resolvedElsewhere, err := filepath.EvalSymlinks(elsewhere)
	if err != nil {
		t.Fatalf("EvalSymlinks(elsewhere): %v", err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(invocationDir); err != nil {
		t.Fatalf("Chdir(invocationDir): %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	viper.Reset()
	isolateInitTest(t)
	// Standalone keeps the layout at .tlc/ regardless of any .hop/
	// directory above the sandbox.
	t.Setenv("TLC_MODE", "standalone")

	// Execute() records the invocation directory before dispatching any
	// command body; this test drives the command directly, so it stands
	// in for that step. Restored afterwards so the recorded directory
	// does not leak into sibling tests.
	SetInvocationDir()
	t.Cleanup(resetInvocationDirForTest)

	root := newTestCmd()
	root.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		// Stand in for anything that moves the process after the
		// command is under way: an orphaned goroutine, a deferred
		// chdir restore, a sibling command with its own sandbox.
		return os.Chdir(elsewhere)
	}
	root.AddCommand(newTestInitCmd())

	buf := new(strings.Builder)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"init", "--no-track"})

	if err := root.Execute(); err != nil {
		t.Fatalf("init failed: %v\n%s", err, buf.String())
	}

	// Guard the premise: if the pre-run hook did not actually move the
	// process, the assertions below would pass without exercising
	// anything.
	cwdNow, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd after execute: %v", err)
	}
	if cwdNow != resolvedElsewhere {
		t.Fatalf("precondition: process should have moved to %q, but is at %q",
			resolvedElsewhere, cwdNow)
	}

	wantConfig := filepath.Join(resolvedInvocation, ".tlc", "config.yaml")
	if _, err := os.Stat(wantConfig); err != nil {
		t.Errorf("config not written to the invocation directory %q: %v", wantConfig, err)
	}

	strayConfig := filepath.Join(resolvedElsewhere, ".tlc", "config.yaml")
	if _, err := os.Stat(strayConfig); err == nil {
		t.Errorf("config leaked into the directory the process moved to: %s", strayConfig)
	}
	if _, err := os.Stat(filepath.Join(resolvedElsewhere, ".tlc")); err == nil {
		t.Errorf("config dir leaked into the directory the process moved to: %s",
			filepath.Join(resolvedElsewhere, ".tlc"))
	}
}

// TestCreateConfigWithInferredID_WritesToInvocationDirAfterChdir covers
// the second writer on the same defect.
//
// This is the path CI hits and a developer checkout hides.
// CreateConfigWithInferredID short-circuits when
// canonicalConfigClaimsProject finds an ancestor .tlc/config.yaml
// claiming the same project, and a checkout under a hop root always has
// one. A CI checkout does not, so the writer runs there and is
// suppressed locally.
//
// Running from an isolated temp directory — with a project ID no
// ancestor could plausibly claim — reproduces the unsuppressed path
// that CI exercises.
func TestCreateConfigWithInferredID_WritesToInvocationDirAfterChdir(t *testing.T) {
	invocationDir := t.TempDir()
	elsewhere := t.TempDir()

	resolvedInvocation, err := filepath.EvalSymlinks(invocationDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(invocationDir): %v", err)
	}
	resolvedElsewhere, err := filepath.EvalSymlinks(elsewhere)
	if err != nil {
		t.Fatalf("EvalSymlinks(elsewhere): %v", err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(invocationDir); err != nil {
		t.Fatalf("Chdir(invocationDir): %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	viper.Reset()
	isolateInitTest(t)
	t.Setenv("TLC_MODE", "standalone")

	// Confirm no ancestor claims this ID, so the walk-up short circuit
	// cannot mask the write. Without this the test could pass simply by
	// never writing anything.
	const projectID = "tlc-cwd-move-probe/unclaimed"
	if core.CanonicalConfigClaimsProjectForTest(projectID) {
		t.Fatalf("precondition: an ancestor of %q already claims %q; "+
			"the writer would be suppressed and the test would not "+
			"exercise the CI path", invocationDir, projectID)
	}

	// Resolve the destination the way a command entry point does, then
	// move the process before the write, exactly as the real hazard
	// does.
	if err := os.Chdir(elsewhere); err != nil { //nolint:usetesting // the move under test
		t.Fatalf("Chdir(elsewhere): %v", err)
	}

	if err := core.CreateConfigWithInferredIDAt(resolvedInvocation, projectID); err != nil {
		t.Fatalf("CreateConfigWithInferredIDAt: %v", err)
	}

	wantConfig := filepath.Join(resolvedInvocation, ".tlc", "config.yaml")
	data, err := os.ReadFile(wantConfig)
	if err != nil {
		t.Fatalf("config not written to the invocation directory %q: %v", wantConfig, err)
	}
	if !strings.Contains(string(data), projectID) {
		t.Errorf("config at %s does not name project %q; got:\n%s", wantConfig, projectID, data)
	}

	strayConfig := filepath.Join(resolvedElsewhere, ".tlc", "config.yaml")
	if _, err := os.Stat(strayConfig); err == nil {
		t.Errorf("config leaked into the directory the process moved to: %s", strayConfig)
	}
}

// TestLocalConfigDir_StaysRelative pins LocalConfigDir's contract.
//
// The fix resolves the config directory to an absolute path at command
// entry rather than making LocalConfigDir itself absolute. Callers use
// its return value as a relative NAME — joined onto an arbitrary
// directory during a walk-up, suffix-matched against a path, handed to
// kit as a project marker. An absolute return would silently break
// every one of those. This test states that so the durable-looking
// change is not made later without revisiting them.
func TestLocalConfigDir_StaysRelative(t *testing.T) {
	for _, mode := range []config.EntryMode{config.ModeStandalone, config.ModeHop} {
		got := config.LocalConfigDir(mode)
		if filepath.IsAbs(got) {
			t.Errorf("LocalConfigDir(%v) = %q, want a relative name: "+
				"callers join it onto other directories and suffix-match it", mode, got)
		}
	}
}
