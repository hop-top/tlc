package flowtest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hop.top/tlc/internal/flowtest"
	xrr "hop.top/xrr"
)

// envGet extracts a value from an os.Environ()-style slice.
func envGet(env []string, key string) string {
	prefix := key + "="
	for _, kv := range env {
		if len(kv) > len(prefix) && kv[:len(prefix)] == prefix {
			return kv[len(prefix):]
		}
	}
	return ""
}

// initGitRepo sets up a git repo with one commit so git clone --local works.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		out, err := c.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("init")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README"), []byte("test\n"), 0o644))
	run("add", ".")
	run("-c", "user.email=test@test.com", "-c", "user.name=test", "commit", "-m", "init")
}

func TestSandboxCreate(t *testing.T) {
	cwd := t.TempDir()
	initGitRepo(t, cwd)

	sb, err := flowtest.NewSandbox(cwd)
	require.NoError(t, err)
	defer sb.Teardown()

	_, err = os.Stat(filepath.Join(sb.RepoDir, ".git"))
	assert.NoError(t, err, "repo/.git must exist")

	_, err = os.Stat(sb.BinDir)
	assert.NoError(t, err, "bin/ must exist")

	_, err = os.Stat(sb.HomeDir)
	assert.NoError(t, err, "home/ must exist")
}

func TestSandboxEnv(t *testing.T) {
	cwd := t.TempDir()
	initGitRepo(t, cwd)

	sb, err := flowtest.NewSandbox(cwd)
	require.NoError(t, err)
	defer sb.Teardown()

	run := &flowtest.Run{
		RecordDir:   t.TempDir(),
		Passthrough: []string{"docker"},
	}
	env := sb.Env("step-foo", xrr.ModeReplay, run)

	assert.Equal(t, sb.HomeDir, envGet(env, "HOME"))
	assert.Equal(t, filepath.Join(sb.RepoDir, ".git"), envGet(env, "GIT_DIR"))
	assert.Equal(t, sb.RepoDir, envGet(env, "GIT_WORK_TREE"))
	assert.Equal(t, "replay", envGet(env, "TLC_FLOW_TEST_MODE"))
	assert.Equal(t, "step-foo", envGet(env, "TLC_FLOW_TEST_STEP"))
	assert.Equal(t, "docker", envGet(env, "TLC_FLOW_TEST_PASSTHROUGH"))

	path := envGet(env, "PATH")
	assert.Contains(t, path, sb.BinDir, "BinDir must be prepended to PATH")
}

func TestSandboxKeep(t *testing.T) {
	cwd := t.TempDir()
	initGitRepo(t, cwd)

	sb, err := flowtest.NewSandbox(cwd)
	require.NoError(t, err)

	sb.Keep = true
	dir := sb.RootDir
	sb.Teardown()

	_, err = os.Stat(dir)
	assert.NoError(t, err, "sandbox root must still exist when Keep=true")
	_ = os.RemoveAll(dir)
}
