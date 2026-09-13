// Tests for `tlc task exec`. Exercise the RunE handler directly so we
// don't pay the cost of cobra's full root lifecycle. Each test plants
// an agents.yaml under a t.TempDir-scoped HOME so the global registry
// lookup is sandboxed.
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"hop.top/tlc/internal/core"
)

// plantAgentsYAML writes a minimal global agents.yaml under HOME so
// core.NewAgentRegistry().LoadDefaults() finds the named agents. Caller
// must have already redirected HOME to a t.TempDir (via withHome).
func plantAgentsYAML(t *testing.T, body string) {
	t.Helper()
	cfgPath := filepath.Join(os.Getenv("HOME"), ".config", "tlc", "agents.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir cfg: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write agents.yaml: %v", err)
	}
}

// execTaskExecCmd invokes the command's RunE and captures stdout/stderr.
// Returns (output, err).
func execTaskExecCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := TaskExecCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	err := cmd.RunE(cmd, args)
	return buf.String(), err
}

// TestTaskExec_NoAgentNoAssigneeFallsThroughToCurrentUser asserts the
// chained default-agent resolution: --agent unset + task unassigned →
// resolver queries core.GetCurrentUser(). With TLC_USER set to a known
// agent name, the dry-run should report that name.
func TestTaskExec_NoAgentNoAssigneeFallsThroughToCurrentUser(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		t.Setenv("TLC_USER", "claude")
		plantAgentsYAML(t, `agents:
  claude:
    binary: /usr/local/bin/claude
    image: ghcr.io/example/claude:latest
`)
		defer resetTaskExecFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		task := &core.Task{
			ID:     "T-0001",
			Seq:    1,
			Title:  "smoke",
			Status: core.StatusTodo,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		resetTaskExecFlags()
		taskExecDryRun = true

		out, err := execTaskExecCmd(t, "T-0001")
		if err != nil {
			t.Fatalf("task exec: %v\n%s", err, out)
		}
		if !strings.Contains(out, "Agent:    claude") {
			t.Errorf("expected resolved agent claude (from TLC_USER):\n%s", out)
		}
	})
}

// TestTaskExec_NoAgentUsesAssignee asserts step 2 of the default-agent
// chain: --agent unset, task has AssignedTo → that's the resolved agent.
func TestTaskExec_NoAgentUsesAssignee(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		t.Setenv("TLC_USER", "wrong-fallback") // would mask the assignee path
		plantAgentsYAML(t, `agents:
  claude:
    image: ghcr.io/example/claude:latest
  codex:
    image: ghcr.io/example/codex:latest
`)
		defer resetTaskExecFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		assignee := "codex"
		task := &core.Task{
			ID:         "T-0002",
			Seq:        2,
			Title:      "assigned task",
			Status:     core.StatusTodo,
			AssignedTo: &assignee,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		resetTaskExecFlags()
		taskExecDryRun = true

		out, err := execTaskExecCmd(t, "T-0002")
		if err != nil {
			t.Fatalf("task exec: %v\n%s", err, out)
		}
		if !strings.Contains(out, "Agent:    codex") {
			t.Errorf("expected agent codex from assignee (not TLC_USER fallback):\n%s", out)
		}
	})
}

// TestTaskExec_ReassignGuardRequiresForce asserts the guard: --agent X
// when AssignedTo=Y and Y≠X → error mentioning --force.
func TestTaskExec_ReassignGuardRequiresForce(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		t.Setenv("TLC_USER", "current-user")
		plantAgentsYAML(t, `agents:
  claude:
    image: ghcr.io/example/claude:latest
  codex:
    image: ghcr.io/example/codex:latest
`)
		defer resetTaskExecFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		assignee := "claude"
		task := &core.Task{
			ID:         "T-0003",
			Seq:        3,
			Title:      "claimed by claude",
			Status:     core.StatusTodo,
			AssignedTo: &assignee,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		resetTaskExecFlags()
		taskExecDryRun = true
		taskExecAgentExpr = "codex"

		_, err = execTaskExecCmd(t, "T-0003")
		if err == nil {
			t.Fatal("expected reassign guard error, got nil")
		}
		if !strings.Contains(err.Error(), "--force") {
			t.Errorf("expected --force hint in error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "T-0003") {
			t.Errorf("error should reference task by alias, got: %v", err)
		}

		// With --force the same call succeeds.
		taskExecForce = true
		out, err := execTaskExecCmd(t, "T-0003")
		if err != nil {
			t.Fatalf("task exec --force: %v\n%s", err, out)
		}
		if !strings.Contains(out, "Agent:    codex") {
			t.Errorf("expected agent codex after --force, got:\n%s", out)
		}
	})
}

// TestTaskExec_CompositeAgentExpressionAppliesOverrides asserts the
// "name:k=v:k=v" shorthand layered onto AgentConfig is observable in
// the dry-run output (env entries appear in the Env: line).
func TestTaskExec_CompositeAgentExpressionAppliesOverrides(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		plantAgentsYAML(t, `agents:
  claude:
    image: ghcr.io/example/claude:latest
`)
		defer resetTaskExecFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		task := &core.Task{
			ID:     "T-0004",
			Seq:    4,
			Title:  "composite",
			Status: core.StatusTodo,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		resetTaskExecFlags()
		taskExecDryRun = true
		taskExecAgentExpr = "claude:env.MODEL=opus"

		out, err := execTaskExecCmd(t, "T-0004")
		if err != nil {
			t.Fatalf("task exec: %v\n%s", err, out)
		}
		// The composite expression must (a) resolve to the canonical
		// agent name, (b) layer the env override into the agent config
		// surfaced by the dry-run.
		if !strings.Contains(out, "Agent:    claude") {
			t.Errorf("expected agent claude:\n%s", out)
		}
		if !strings.Contains(out, "MODEL:opus") && !strings.Contains(out, "MODEL=opus") &&
			!strings.Contains(out, "MODEL: opus") && !strings.Contains(out, "map[MODEL:opus]") {
			t.Errorf("expected env override MODEL=opus in dry-run output:\n%s", out)
		}
	})
}

// TestTaskExec_NoFallback_FailsWithActionableError asserts the
// edge case: --agent unset, no assignee, no current user → command
// fails with a message naming all three exhausted sources.
func TestTaskExec_NoFallback_FailsWithActionableError(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		// Force core.GetCurrentUser() to return "" by clearing both
		// TLC_USER and USER, and making git config fail. withHome
		// already redirected HOME to a temp dir so ~/.gitconfig is
		// absent; also point GIT_CONFIG_{GLOBAL,SYSTEM} at /dev/null
		// to neutralize systems that ship an /etc/gitconfig with
		// user.name baked in (some CI runners do).
		t.Setenv("TLC_USER", "")
		t.Setenv("USER", "")
		t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
		t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
		defer resetTaskExecFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		task := &core.Task{
			ID:     "T-0005",
			Seq:    5,
			Title:  "no fallback",
			Status: core.StatusTodo,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		resetTaskExecFlags()
		taskExecDryRun = true

		_, err = execTaskExecCmd(t, "T-0005")
		if err == nil {
			t.Fatal("expected error when agent cannot be resolved, got nil")
		}
		msg := err.Error()
		// The error should be specific enough that the user knows what
		// to do next: set --agent, assign the task, or set $USER.
		for _, want := range []string{"--agent", "assignee", "current user"} {
			if !strings.Contains(msg, want) {
				t.Errorf("error %q missing actionable hint %q", msg, want)
			}
		}
	})
}

// TestTaskExec_RepeatableAgentConfigOverride pins the -A flag wiring
// end-to-end: the override applied via -A should be visible in the
// dry-run AgentConfig.
func TestTaskExec_RepeatableAgentConfigOverride(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		plantAgentsYAML(t, `agents:
  claude:
    image: ghcr.io/example/claude:latest
`)
		defer resetTaskExecFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		task := &core.Task{
			ID:     "T-0006",
			Seq:    6,
			Title:  "repeatable -A",
			Status: core.StatusTodo,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		resetTaskExecFlags()
		taskExecDryRun = true
		taskExecAgentExpr = "claude"
		taskExecAgentOverrides = []string{
			"env.MODEL=opus",
			"default_timeout=10m",
		}

		out, err := execTaskExecCmd(t, "T-0006")
		if err != nil {
			t.Fatalf("task exec: %v\n%s", err, out)
		}
		if !strings.Contains(out, "Timeout:  10m") {
			t.Errorf("expected default_timeout override in dry-run:\n%s", out)
		}
		if !strings.Contains(out, "MODEL:opus") {
			t.Errorf("expected env.MODEL override in dry-run:\n%s", out)
		}
	})
}

// TestTaskExec_WithPodImageOverride pins the --with-pod=<image-ref>
// branch end-to-end through dry-run output.
func TestTaskExec_WithPodImageOverride(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		plantAgentsYAML(t, `agents:
  claude:
    image: ghcr.io/example/claude:latest
`)
		defer resetTaskExecFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		task := &core.Task{
			ID:     "T-0007",
			Seq:    7,
			Title:  "with-pod image",
			Status: core.StatusTodo,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		resetTaskExecFlags()
		taskExecDryRun = true
		taskExecAgentExpr = "claude"
		taskExecWithPod = "ghcr.io/me/agent:dev"

		out, err := execTaskExecCmd(t, "T-0007")
		if err != nil {
			t.Fatalf("task exec: %v\n%s", err, out)
		}
		if !strings.Contains(out, "Mode:     container (ghcr.io/me/agent:dev)") {
			t.Errorf("expected --with-pod image override in dry-run, got:\n%s", out)
		}
	})
}

// TestTaskExec_WithPodFalseRunsLocal pins the --with-pod=false branch.
func TestTaskExec_WithPodFalseRunsLocal(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		plantAgentsYAML(t, `agents:
  claude:
    image: ghcr.io/example/claude:latest
`)
		defer resetTaskExecFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		task := &core.Task{
			ID:     "T-0008",
			Seq:    8,
			Title:  "with-pod false",
			Status: core.StatusTodo,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		resetTaskExecFlags()
		taskExecDryRun = true
		taskExecAgentExpr = "claude"
		taskExecWithPod = "false"

		out, err := execTaskExecCmd(t, "T-0008")
		if err != nil {
			t.Fatalf("task exec: %v\n%s", err, out)
		}
		if !strings.Contains(out, "Mode:     local") {
			t.Errorf("expected --with-pod false → local mode, got:\n%s", out)
		}
	})
}

// TestTaskExecParams_FromFlags pins that `task execute` hands the exec
// path its own env/mount/network/keep-pod flags instead of stashing them
// into the `agent run` bindings.
func TestTaskExecParams_FromFlags(t *testing.T) {
	withTestLock(func() {
		defer resetTaskExecFlags()
		taskExecEnv = []string{"MODEL=opus"}
		taskExecMounts = []string{"/a:/b"}
		taskExecNetwork = "bridge"
		taskExecKeepPod = true

		p := taskExecParams("codex", false)
		assert.Equal(t, "codex", p.agent)
		assert.False(t, p.local)
		assert.Equal(t, []string{"MODEL=opus"}, p.env)
		assert.Equal(t, []core.MountSpec{{Source: "/a", Target: "/b"}}, p.mounts)
		assert.Equal(t, "bridge", p.network)
		assert.True(t, p.keepPod)
		assert.Equal(t, "/workspace", p.repoRoot)
		assert.Zero(t, p.timeout, "the command wraps ctx with --timeout itself")

		p = taskExecParams("codex", true)
		cwd, _ := os.Getwd()
		assert.True(t, p.local)
		assert.Equal(t, cwd, p.repoRoot)
		assert.Equal(t, filepath.Join(cwd, ".tlc", "runs"), p.runsDir)
	})
}
