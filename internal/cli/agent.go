package cli

import (
	"github.com/spf13/cobra"
)

// AgentCmd is the parent command for agent operations.
var AgentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Agent execution and management",
	Long: `Execute and manage agent-driven task automation.

Agents are configured via ~/.config/tlc/agents.yaml (global) and
.tlc/agents.yaml (project-local). Each agent defines an image, binary,
env vars, and default timeout.`,
}

func init() {
	AgentCmd.AddCommand(AgentRunCmd)
	RootCmd.AddCommand(AgentCmd)
}
