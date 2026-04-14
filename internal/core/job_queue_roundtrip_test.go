package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T-0591: Async dispatch must round-trip all flags through JobPayload
// serialization. Verifies --env, --mount, --context, and
// --total-timeout survive marshal/unmarshal.
func TestJobPayload_RoundTrip_AllFlags(t *testing.T) {
	original := &JobPayload{
		AgentName:        "copilot",
		Tasks:            []string{"T-0042", "T-0043"},
		FlowRef:          "flow:build:1.0",
		TrackID:          "my-track",
		Image:            "ghcr.io/hop-top/pod-copilot:v2",
		Local:            false,
		Prompt:           "run all the things",
		NoState:          true,
		KeepPod:          true,
		Network:          "host",
		Retries:          3,
		TimeoutSecs:      1800,
		Env:              []string{"API_KEY=secret", "DEBUG=1"},
		Mounts:           []string{"/src:/workspace:ro", "/data:/data:rw"},
		Context:          []string{"extra.md", "notes.txt"},
		TotalTimeoutSecs: 7200,
		TrustProject:     true,
	}

	// Marshal.
	encoded, err := MarshalPayload(original)
	require.NoError(t, err)
	assert.NotEmpty(t, encoded)

	// Unmarshal.
	decoded, err := UnmarshalPayload(encoded)
	require.NoError(t, err)

	// Verify all fields round-trip.
	assert.Equal(t, original.AgentName, decoded.AgentName, "AgentName")
	assert.Equal(t, original.Tasks, decoded.Tasks, "Tasks")
	assert.Equal(t, original.FlowRef, decoded.FlowRef, "FlowRef")
	assert.Equal(t, original.TrackID, decoded.TrackID, "TrackID")
	assert.Equal(t, original.Image, decoded.Image, "Image")
	assert.Equal(t, original.Local, decoded.Local, "Local")
	assert.Equal(t, original.Prompt, decoded.Prompt, "Prompt")
	assert.Equal(t, original.NoState, decoded.NoState, "NoState")
	assert.Equal(t, original.KeepPod, decoded.KeepPod, "KeepPod")
	assert.Equal(t, original.Network, decoded.Network, "Network")
	assert.Equal(t, original.Retries, decoded.Retries, "Retries")
	assert.Equal(t, original.TimeoutSecs, decoded.TimeoutSecs, "TimeoutSecs")
	assert.Equal(t, original.TrustProject, decoded.TrustProject, "TrustProject")

	// The flags specifically requested by T-0591:
	assert.Equal(t, original.Env, decoded.Env, "--env round-trip")
	assert.Equal(t, original.Mounts, decoded.Mounts, "--mount round-trip")
	assert.Equal(t, original.Context, decoded.Context, "--context round-trip")
	assert.Equal(t, original.TotalTimeoutSecs, decoded.TotalTimeoutSecs,
		"--total-timeout round-trip")
}

// T-0591: Empty optional fields should not cause issues.
func TestJobPayload_RoundTrip_MinimalFields(t *testing.T) {
	original := &JobPayload{
		AgentName:   "claude",
		Tasks:       []string{"T-0001"},
		TimeoutSecs: 600,
	}

	encoded, err := MarshalPayload(original)
	require.NoError(t, err)

	decoded, err := UnmarshalPayload(encoded)
	require.NoError(t, err)

	assert.Equal(t, original.AgentName, decoded.AgentName)
	assert.Equal(t, original.Tasks, decoded.Tasks)
	assert.Empty(t, decoded.Env, "Env should be empty")
	assert.Empty(t, decoded.Mounts, "Mounts should be empty")
	assert.Empty(t, decoded.Context, "Context should be empty")
	assert.Zero(t, decoded.TotalTimeoutSecs, "TotalTimeoutSecs should be zero")
	assert.False(t, decoded.TrustProject, "TrustProject should be false")
}
