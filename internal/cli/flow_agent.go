package cli

import (
	"hop.top/tlc/internal/core"
)

var (
	flowRunAgent        string
	flowRunAgentLocal   bool
	flowRunTrustProject bool
)

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
	if flowRunTrustProject {
		registry.TrustProject(registry.ProjectConfigPath())
	}

	return &ContainerAgentRunner{
		AgentName: flowRunAgent,
		Registry:  registry,
		Local:     flowRunAgentLocal,
	}
}

func init() {
	f := FlowRunCmd.Flags()
	f.StringVar(&flowRunAgent, "agent", "",
		"Agent to execute flow steps (enables container/local dispatch)")
	f.BoolVar(&flowRunAgentLocal, "agent-local", false,
		"Execute agent locally (no container)")
	f.BoolVar(&flowRunTrustProject, "trust-project", false,
		"Trust project-local agent config without prompting")
}
