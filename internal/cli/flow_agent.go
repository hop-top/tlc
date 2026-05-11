package cli

import (
	"hop.top/tlc/internal/core"
)

var (
	flowRunAgent        string
	flowRunAgentLocal   bool
	flowRunTrustProject bool
)

// buildFlowAgentRunner creates a ContainerAgentRunner when --agent or
// --agent-local is set on flow run. Returns nil when neither flag is
// present (steps fall back to the DB-only ephemeral path).
//
// --agent-local alone is sufficient because individual flow steps may
// declare their own `agent: <name>` in YAML; the runner consults
// step.Agent.Name when its own AgentName is empty (see
// ContainerAgentRunner.CanHandle / Run). Requiring --agent here caused
// T-0948: flow steps with a YAML-declared agent silently no-op'd when
// the user passed only --agent-local because no runner was wired up
// and executeTemplateStep fell through to the DB-only completion path.
func buildFlowAgentRunner() core.AgentRunner {
	if flowRunAgent == "" && !flowRunAgentLocal {
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
