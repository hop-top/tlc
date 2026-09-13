package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hop.top/tlc/internal/core"
)

func TestValidateAgentRunFlags_noTarget(t *testing.T) {
	agentRunTasks = nil
	agentRunFlow = ""
	agentRunTrack = ""
	err := validateAgentRunFlags()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one target required")
}

func TestValidateAgentRunFlags_taskOnly(t *testing.T) {
	agentRunTasks = []string{"T-0042"}
	agentRunFlow = ""
	agentRunTrack = ""
	err := validateAgentRunFlags()
	assert.NoError(t, err)
}

func TestValidateAgentRunFlags_multipleTasks(t *testing.T) {
	agentRunTasks = []string{"T-0042", "T-0043"}
	agentRunFlow = ""
	agentRunTrack = ""
	err := validateAgentRunFlags()
	assert.NoError(t, err)
}

func TestValidateAgentRunFlags_flowOnly(t *testing.T) {
	agentRunTasks = nil
	agentRunFlow = "flow:example:1.0"
	agentRunTrack = ""
	err := validateAgentRunFlags()
	assert.NoError(t, err)
}

func TestValidateAgentRunFlags_trackOnly(t *testing.T) {
	agentRunTasks = nil
	agentRunFlow = ""
	agentRunTrack = "my-track"
	err := validateAgentRunFlags()
	assert.NoError(t, err)
}

func TestValidateAgentRunFlags_flowAndTrack(t *testing.T) {
	agentRunTasks = nil
	agentRunFlow = "flow:example:1.0"
	agentRunTrack = "my-track"
	err := validateAgentRunFlags()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestValidateAgentRunFlags_taskAndFlow(t *testing.T) {
	agentRunTasks = []string{"T-0042"}
	agentRunFlow = "flow:example:1.0"
	agentRunTrack = ""
	err := validateAgentRunFlags()
	assert.Error(t, err)
}

func TestParseMounts(t *testing.T) {
	tests := []struct {
		input    []string
		expected []core.MountSpec
	}{
		{
			input:    []string{"/src:/workspace:ro"},
			expected: []core.MountSpec{{Source: "/src", Target: "/workspace", Mode: "ro"}},
		},
		{
			input:    []string{"/src:/workspace"},
			expected: []core.MountSpec{{Source: "/src", Target: "/workspace"}},
		},
		{
			input:    nil,
			expected: []core.MountSpec{},
		},
	}

	for _, tt := range tests {
		got := parseMounts(tt.input)
		assert.Equal(t, tt.expected, got)
	}
}

func TestMergeEnvVars(t *testing.T) {
	cfg := map[string]string{"EXISTING": "val", "FOO": "cfg"}

	merged := mergeEnvVars(cfg, []string{"FOO=bar", "BAZ=qux", "novalue"})
	assert.Equal(t, "val", merged["EXISTING"])
	assert.Equal(t, "bar", merged["FOO"], "overlay wins over agent config")
	assert.Equal(t, "qux", merged["BAZ"])
	assert.Len(t, merged, 3, "entries without '=' are dropped")
	assert.Equal(t, "cfg", cfg["FOO"], "agent config is not mutated")
}

// TestAgentRunParams_FromFlags pins that the exec path's parameters are
// built from the `agent run` flag bindings once, per mode.
func TestAgentRunParams_FromFlags(t *testing.T) {
	withTestLock(func() {
		defer resetAgentRunFlags()
		agentRunAgent = "claude"
		agentRunImage = "ghcr.io/me/agent:dev"
		agentRunMounts = []string{"/src:/workspace:ro"}
		agentRunEnv = []string{"K=v"}
		agentRunNetwork = "host"
		agentRunKeepPod = true
		agentRunTimeout = 7 * time.Minute

		p := agentRunParams()
		assert.Equal(t, "claude", p.agent)
		assert.False(t, p.local)
		assert.Equal(t, "ghcr.io/me/agent:dev", p.image)
		assert.Equal(t, []core.MountSpec{{Source: "/src", Target: "/workspace", Mode: "ro"}}, p.mounts)
		assert.Equal(t, []string{"K=v"}, p.env)
		assert.Equal(t, "host", p.network)
		assert.True(t, p.keepPod)
		assert.Equal(t, 7*time.Minute, p.timeout)
		assert.Equal(t, "/workspace", p.repoRoot)
		assert.Nil(t, p.runner, "production params use the os/exec runner")

		agentRunLocal = true
		p = agentRunParams()
		cwd, err := os.Getwd()
		require.NoError(t, err)
		assert.True(t, p.local)
		assert.Equal(t, cwd, p.repoRoot)
		assert.Equal(t, filepath.Join(cwd, ".tlc", "runs"), p.runsDir)
	})
}

func TestSplitMount(t *testing.T) {
	parts := splitMount("/src:/workspace:ro")
	assert.Equal(t, []string{"/src", "/workspace", "ro"}, parts)

	parts = splitMount("/src:/workspace")
	assert.Equal(t, []string{"/src", "/workspace"}, parts)
}

func TestContainerAgentRunner_CanHandle(t *testing.T) {
	r := &ContainerAgentRunner{AgentName: "claude"}
	step := core.Step{}
	assert.True(t, r.CanHandle(step))

	r2 := &ContainerAgentRunner{}
	assert.False(t, r2.CanHandle(step))

	step.Agent = core.AgentRef{Name: "custom"}
	assert.True(t, r2.CanHandle(step))
}
