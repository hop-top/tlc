package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T-0588: Container mode (local=false) returns /workspace;
// local mode (local=true) returns os.Getwd().
func TestRepoRootForMode_Container(t *testing.T) {
	got := repoRootForMode(false)
	assert.Equal(t, "/workspace", got,
		"container mode should always return /workspace")
}

func TestRepoRootForMode_Local(t *testing.T) {
	expected, err := os.Getwd()
	require.NoError(t, err)

	got := repoRootForMode(true)
	assert.Equal(t, expected, got,
		"local mode should return current working directory")
}
