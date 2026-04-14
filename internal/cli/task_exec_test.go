package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskExecCmd_requiresAgent(t *testing.T) {
	cmd := TaskExecCmd
	cmd.SetArgs([]string{"T-0042"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := cmd.RunE(cmd, []string{"T-0042"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--agent is required")
}

func TestBuildFlowAgentRunner_empty(t *testing.T) {
	flowRunAgent = ""
	runner := buildFlowAgentRunner()
	assert.Nil(t, runner)
}
