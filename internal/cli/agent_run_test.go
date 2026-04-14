package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
	agentRunEnv = []string{"FOO=bar", "BAZ=qux"}
	cfg := map[string]string{"EXISTING": "val"}

	merged := mergeEnvVars(cfg)
	assert.Equal(t, "val", merged["EXISTING"])
	assert.Equal(t, "bar", merged["FOO"])
	assert.Equal(t, "qux", merged["BAZ"])
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
