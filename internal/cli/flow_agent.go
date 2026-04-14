package cli

import (
	"os"

	"hop.top/tlc/internal/core"
)

var flowRunAgent string

// buildFlowAgentRunner creates a ContainerAgentRunner when --agent is
// set on flow run. Returns nil if no agent is configured.
func buildFlowAgentRunner() core.AgentRunner {
	if flowRunAgent == "" {
		return nil
	}

	registry := core.NewAgentRegistry()
	if err := registry.LoadDefaults(); err != nil {
		return nil
	}
	// Auto-trust for interactive CLI use.
	registry.TrustProject(".tlc/agents.yaml")

	return &ContainerAgentRunner{
		AgentName: flowRunAgent,
		Registry:  registry,
		Local:     agentRunLocal,
		EnvExtra:  parseEnvSlice(os.Environ()),
	}
}

// parseEnvSlice is intentionally a no-op — we only pass agent-specific
// env vars, not the entire shell environment.
func parseEnvSlice(_ []string) map[string]string {
	return nil
}

func init() {
	FlowRunCmd.Flags().StringVar(&flowRunAgent, "agent", "",
		"Agent to execute flow steps (enables container/local dispatch)")
}
