package core

import "fmt"

// AgentResultVersion is the current schema version for AgentResult.
const AgentResultVersion = 1

// AgentResultStatus represents the outcome of an agent execution.
type AgentResultStatus string

const (
	AgentStatusSucceeded AgentResultStatus = "succeeded"
	AgentStatusFailed    AgentResultStatus = "failed"
	AgentStatusPartial   AgentResultStatus = "partial"
	AgentStatusTimeout   AgentResultStatus = "timeout"
)

// validAgentStatuses is the set of recognised AgentResultStatus values.
var validAgentStatuses = map[AgentResultStatus]bool{
	AgentStatusSucceeded: true,
	AgentStatusFailed:    true,
	AgentStatusPartial:   true,
	AgentStatusTimeout:   true,
}

// AgentResult captures the outcome of an agent run. The stdout contract
// carries the status signal; the volume contract carries the artifact
// payload.
type AgentResult struct {
	Version   int               `json:"version"`
	Status    AgentResultStatus `json:"status"`
	ExitCode  int               `json:"exit_code"`
	Summary   string            `json:"summary"`
	Agent     string            `json:"agent,omitempty"`
	StartedAt string            `json:"started_at,omitempty"`
	EndedAt   string            `json:"ended_at,omitempty"`
	Outputs   map[string]any    `json:"outputs,omitempty"`
	Artifacts []AgentArtifact   `json:"artifacts,omitempty"`
}

// AgentArtifact describes a single file produced or modified by the agent.
type AgentArtifact struct {
	Path        string `json:"path"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// Validate checks that required fields are present, the version is
// supported, and the status is a recognised value.
func (r *AgentResult) Validate() error {
	if r.Version != AgentResultVersion {
		return fmt.Errorf(
			"unsupported agent result version %d; expected %d",
			r.Version, AgentResultVersion,
		)
	}
	if !validAgentStatuses[r.Status] {
		return fmt.Errorf(
			"invalid agent result status %q; valid: succeeded, failed, partial, timeout",
			r.Status,
		)
	}
	if r.Summary == "" {
		return fmt.Errorf("agent result: summary is required")
	}
	return nil
}
