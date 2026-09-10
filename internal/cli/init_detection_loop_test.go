package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// TestInitCmd_DetectionLoopWithGitRemote reproduces GH-1: when running
// `tlc init` in a directory with a git remote but no .tlc/, the
// PersistentPreRunE auto-detects the project and creates .tlc/config.yaml
// before runInit executes. Then runInit sees .tlc/ already exists and fails
// with "directory already exists", making `tlc init` unusable in any git
// repo with a remote.
//
// The root cause is that the PersistentPreRunE does not skip project
// auto-detection when the `init` subcommand is being invoked, creating a
// race where the config directory is created before init can create it.
//
// This test wires PersistentPreRunE with DetectProject() (mirroring the
// production root command) and asserts that init succeeds. It FAILS on the
// buggy code because DetectProject auto-creates .tlc/ first.
func TestInitCmd_DetectionLoopWithGitRemote(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-init-loop-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}

	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	// The command under test runs on its own goroutine so the detection
	// loop this test guards against can be caught by a timeout rather
	// than hanging the suite. That goroutine writes its config through
	// paths relative to the process working directory (runInit's
	// config.LocalConfigDir, and CreateConfigWithInferredID on the
	// detection path). Restoring the working directory or deleting the
	// sandbox while it is still running therefore redirects those writes
	// into whatever directory the process has moved to — the package
	// directory — where they are invisible to git because .tlc/ is
	// gitignored.
	//
	// Both cleanups are gated on no goroutine being able to write any
	// more — either because it finished or because it never started.
	// On the timeout path it is still live, so the working directory
	// stays on the sandbox and the sandbox stays on disk: the orphan
	// keeps writing where it was told to, the test still fails, and
	// nothing escapes into the source tree. Leaking a temp dir on an
	// already-failing run is the cheaper of the two outcomes.
	finished := make(chan struct{})
	started := false
	t.Cleanup(func() {
		if started {
			select {
			case <-finished:
			default:
				// Orphan still running: it owns the working directory
				// and the sandbox until the process exits.
				return
			}
		}
		_ = os.Chdir(origDir)
		_ = os.RemoveAll(tmpDir)
	})

	// Set up a real git repo with a GitHub remote.
	for _, args := range [][]string{
		{"init"},
		{"remote", "add", "origin", "https://github.com/testorg/testrepo.git"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s failed: %v\n%s", args[0], err, out)
		}
	}

	// Confirm no .tlc directory exists yet.
	if _, err := os.Stat(filepath.Join(tmpDir, ".tlc")); err == nil {
		t.Fatal(".tlc already exists before init")
	}

	// Reset all global state.
	viper.Reset()
	core.ResetDetectionCache()
	dbSyncOnce = sync.Once{}
	touchOnce = sync.Once{}
	cfgFile = ""

	// Isolate the storage DB so we don't touch the real global db.
	tmpDB, err := os.CreateTemp("", "tlc-init-loop-db-*.sqlite")
	if err != nil {
		t.Fatalf("CreateTemp db: %v", err)
	}
	tmpDB.Close()
	defer os.Remove(tmpDB.Name())

	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", tmpDB.Name())
	// Auto mode causes handleFallbackMode to create .tlc/config.yaml.
	viper.Set("project.fallback_mode", "auto")

	// Force standalone mode to avoid hop detection from parent dirs.
	t.Setenv("TLC_MODE", "standalone")

	// Build a root command with PersistentPreRunE that triggers project
	// detection, mirroring the production root command (root.go:121).
	// execErr is written before finished is closed and read only after
	// that close is observed, so the close alone orders the handoff.
	var execErr error
	started = true
	go func() {
		// Closing last makes "finished is closed" mean "this goroutine
		// will not touch the filesystem again", which is exactly the
		// condition the cleanup above waits for.
		defer close(finished)

		root := newTestCmd()
		root.PersistentPreRunE = func(c *cobra.Command, args []string) error {
			// Production PersistentPreRunE calls getStorage() which
			// indirectly calls DetectProject(). DetectProject() with
			// fallback_mode=auto creates .tlc/config.yaml when no
			// config exists and a git remote is found.
			_ = core.DetectProject()
			return nil
		}

		initCmd := newTestInitCmd()
		root.AddCommand(initCmd)

		buf := new(strings.Builder)
		root.SetOut(buf)
		root.SetErr(buf)
		root.SetArgs([]string{"init", "--no-track"})

		execErr = root.Execute()
	}()

	select {
	case <-finished:
		if execErr != nil {
			t.Fatalf("init should succeed in a git repo with remote (GH-1): %v", execErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("init did not complete within 5s — stuck in detection loop (GH-1)")
	}

	// Verify .tlc/config.yaml was created.
	configPath := filepath.Join(tmpDir, ".tlc", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal(".tlc/config.yaml was not created")
	}

	// Verify project ID was detected from git remote.
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile config: %v", err)
	}
	if !strings.Contains(string(data), "testorg/testrepo") {
		t.Errorf("config does not contain expected project ID; got:\n%s", data)
	}
}

// TestInitCmd_DetectionNoDoubleCreate verifies that .tlc/config.yaml is
// written exactly once when `tlc init` runs in a directory with a git
// remote. If PersistentPreRunE auto-creates the config before runInit,
// init should detect this and not error out.
func TestInitCmd_DetectionNoDoubleCreate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-init-double-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(origDir)

	// Set up a real git repo with a GitHub remote.
	for _, args := range [][]string{
		{"init"},
		{"remote", "add", "origin", "https://github.com/testorg/testrepo.git"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s failed: %v\n%s", args[0], err, out)
		}
	}

	viper.Reset()
	core.ResetDetectionCache()
	dbSyncOnce = sync.Once{}
	touchOnce = sync.Once{}
	cfgFile = ""

	tmpDB, err := os.CreateTemp("", "tlc-init-double-db-*.sqlite")
	if err != nil {
		t.Fatalf("CreateTemp db: %v", err)
	}
	tmpDB.Close()
	defer os.Remove(tmpDB.Name())

	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", tmpDB.Name())
	viper.Set("project.fallback_mode", "auto")
	t.Setenv("TLC_MODE", "standalone")

	// Simulate what happens in production: PersistentPreRunE triggers
	// DetectProject which auto-creates .tlc/config.yaml via
	// handleFallbackMode before runInit has a chance to run.
	det := core.DetectProject()
	if det == nil || !det.InProject {
		t.Fatal("DetectProject should detect project from git remote")
	}

	// At this point .tlc/config.yaml was auto-created by DetectProject.
	configPath := filepath.Join(tmpDir, ".tlc", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal(".tlc/config.yaml should exist after DetectProject with auto mode")
	}

	// Now runInit should NOT fail with "already exists" — it should
	// recognize that auto-detection already created the config.
	core.ResetDetectionCache()
	cmd := newTestCmd()
	cmd.AddCommand(newTestInitCmd())
	buf := new(strings.Builder)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"init", "--no-track"})

	err = cmd.Execute()
	if err != nil {
		t.Fatalf("init should succeed when .tlc/ was auto-created by "+
			"PersistentPreRunE detection (GH-1): %v", err)
	}
}
