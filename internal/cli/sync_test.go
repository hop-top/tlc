package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// testSyncRepo is the origin the synthetic checkout in setupSyncConfigTest
// advertises, so auto-configuration assertions do not depend on the ambient
// repository's real remote.
const testSyncRepo = "hop-top/tlc-test-fixture"

// TestSyncCommands tests sync command configuration
// Tests sync config, GitHub auto-configuration, and direction upgrades.
func TestSyncCommands(t *testing.T) {
	t.Run("SyncConfig", func(t *testing.T) {
		setupSyncConfigTest(t)
		viper.Set("sync.github.repo", "google/oss-tlc-cli")
		viper.Set("sync.github.sync_direction", "bidirectional")

		cmd := newTestCmd()
		cmd.AddCommand(SyncCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"sync", "config", "github", "--force"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("sync config failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "google/oss-tlc-cli") {
			t.Errorf("expected repo in output, got: %s", output)
		}
	})

	t.Run("GitHubAutoConfiguration", func(t *testing.T) {
		setupSyncConfigTest(t)

		if err := autoConfigureGitHub("pull"); err != nil {
			t.Fatalf("autoConfigureGitHub should not error: %v", err)
		}

		if repo := viper.GetString("sync.github.repo"); repo != testSyncRepo {
			t.Errorf("repo = %q, want %q", repo, testSyncRepo)
		}
		if direction := viper.GetString("sync.github.sync_direction"); direction != "pull" {
			t.Errorf("direction = %q, want pull", direction)
		}
	})

	// Note: Testing that sync config fails without --force is difficult in unit tests
	// because autoConfigureGitHub may detect the actual git repo and override the values.
	// This behavior is tested manually in the integration test suite.

	t.Run("AutoConfigure direction upgrades", func(t *testing.T) {
		setupSyncConfigTest(t)

		// First configure as pull
		err := autoConfigureGitHub("pull")
		if err != nil {
			t.Errorf("initial autoConfigureGitHub failed: %v", err)
		}

		direction := viper.GetString("sync.github.sync_direction")
		if direction != "pull" {
			t.Errorf("expected pull direction, got: %s", direction)
		}

		// Then call with push, should upgrade to bidirectional
		err = autoConfigureGitHub("push")
		if err != nil {
			t.Errorf("upgrade autoConfigureGitHub failed: %v", err)
		}

		direction = viper.GetString("sync.github.sync_direction")
		if direction != "bidirectional" {
			t.Errorf("expected bidirectional direction after upgrade, got: %s", direction)
		}
	})

	t.Run("AutoConfigure skips when repo already set", func(t *testing.T) {
		setupSyncConfigTest(t)

		// Set a different repo to simulate already configured
		viper.Set("sync.github.repo", "different/repo")
		viper.Set("sync.github.sync_direction", "bidirectional")

		// Try to configure again, should skip due to different repo
		err := autoConfigureGitHub("pull")
		if err != nil {
			t.Errorf("autoConfigureGitHub should not error when different repo: %v", err)
		}

		// Repo should remain unchanged
		repo := viper.GetString("sync.github.repo")
		if repo != "different/repo" {
			t.Errorf("repo should remain unchanged, got: %s", repo)
		}
	})

	t.Run("AutoConfigure with bidirectional hint", func(t *testing.T) {
		setupSyncConfigTest(t)

		if err := autoConfigureGitHub("bidirectional"); err != nil {
			t.Fatalf("autoConfigureGitHub with bidirectional failed: %v", err)
		}

		if repo := viper.GetString("sync.github.repo"); repo != testSyncRepo {
			t.Errorf("repo = %q, want %q", repo, testSyncRepo)
		}
		if direction := viper.GetString("sync.github.sync_direction"); direction != "bidirectional" {
			t.Errorf("direction = %q, want bidirectional", direction)
		}
	})

	// Note: sync pull/push require external plugins/binaries which might not be available during unit tests.
	// We would need to mock the RPC client or ensure plugins are built.
}

// TestAutoConfigureGitHubUsesDocumentedKey pins the direction key to the
// spelling the config spec documents.
//
// The key was written and read as `sync.github.direction` while every
// doc, the config struct and the interactive hints said
// `sync.github.sync_direction` — so a user following the spec set a key
// nothing read, and the key tlc did read appeared nowhere. Reading back
// through the undocumented spelling must find nothing.
func TestAutoConfigureGitHubUsesDocumentedKey(t *testing.T) {
	setupSyncConfigTest(t)

	if err := autoConfigureGitHub("bidirectional"); err != nil {
		t.Fatalf("autoConfigureGitHub: %v", err)
	}
	if got := viper.GetString("sync.github.repo"); got != testSyncRepo {
		t.Fatalf("repo = %q, want %q", got, testSyncRepo)
	}

	if got := viper.GetString("sync.github.sync_direction"); got == "" {
		t.Error("sync.github.sync_direction unset; the documented key must be the one written")
	}
	if got := viper.GetString("sync.github.direction"); got != "" {
		t.Errorf("sync.github.direction = %q; the undocumented key must not be written", got)
	}
}

// setupSyncConfigTest isolates a sync auto-configuration test from the
// source tree and from viper's package-global state.
//
// autoConfigureGitHub persists its result with viper.WriteConfig. When no
// config file is set — which viper.Reset guarantees — viper falls back to
// ".tlc.yaml" relative to the process working directory, so a test run from
// the package directory drops a real config file into the source tree. That
// file is untracked, and any later run in that directory discovers it as a
// project config, seeding sync state into unrelated tests.
//
// The helper chdirs into a temp directory holding a synthetic git checkout
// with a GitHub origin, and points viper at a config path inside it. The
// synthetic remote also makes the assertions deterministic: detection no
// longer depends on the ambient checkout having a GitHub origin.
func setupSyncConfigTest(t *testing.T) {
	t.Helper()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	// Synthetic checkout so autoConfigureGitHub's git probes resolve to a
	// fixed repo rather than whatever the ambient tree points at.
	runGit(t, tmpDir, "init", "-q", ".")
	runGit(t, tmpDir, "remote", "add", "origin", "git@github.com:"+testSyncRepo+".git")

	t.Chdir(tmpDir)

	// ensureGitHubToken shells out to `gh auth token` and exports the result
	// into the process environment. Pin it so the test neither depends on the
	// developer's gh login nor leaks a token into sibling tests.
	t.Setenv("GITHUB_TOKEN", "test-token")

	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigFile(filepath.Join(tmpDir, ".tlc.yaml"))
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", filepath.Join(tmpDir, "test.sqlite"))
}

// runGit runs a git command in dir and fails the test if it errors.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	// Tests must not inherit ambient git configuration.
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
