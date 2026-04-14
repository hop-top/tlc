package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalUnmarshalPayload(t *testing.T) {
	p := &JobPayload{
		AgentName:   "claude",
		Tasks:       []string{"T-0042", "T-0043"},
		Local:       true,
		TimeoutSecs: 1800,
	}

	s, err := MarshalPayload(p)
	require.NoError(t, err)
	assert.Contains(t, s, "claude")

	got, err := UnmarshalPayload(s)
	require.NoError(t, err)
	assert.Equal(t, p.AgentName, got.AgentName)
	assert.Equal(t, p.Tasks, got.Tasks)
	assert.True(t, got.Local)
	assert.Equal(t, 1800, got.TimeoutSecs)
}

func TestUnmarshalPayload_invalid(t *testing.T) {
	_, err := UnmarshalPayload("not json")
	assert.Error(t, err)
}
