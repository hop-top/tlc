package core

import "fmt"

// AgentContextVersion is the current schema version for AgentContext.
const AgentContextVersion = 1

// AgentContext carries all metadata an agent needs to execute a task,
// flow step, or track. Serialised as JSON and uploaded to the container
// at /workspace/.tlc/context.json.
type AgentContext struct {
	Version         int      `json:"version"`
	TaskID          string   `json:"task_id,omitempty"`
	TaskTitle       string   `json:"task_title,omitempty"`
	TaskDescription string   `json:"task_description,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	TrackID         string   `json:"track_id,omitempty"`
	FlowID          string   `json:"flow_id,omitempty"`
	FlowStepID      string   `json:"flow_step_id,omitempty"`
	StepType        string   `json:"step_type,omitempty"`
	StepTitle       string   `json:"step_title,omitempty"`
	RepoRoot        string   `json:"repo_root"`
	Files           []string `json:"files,omitempty"`
	Prompt          string   `json:"prompt"`
}

// Validate checks that required fields are present and the version is
// supported. Returns a descriptive error on failure.
func (c *AgentContext) Validate() error {
	if c.Version != AgentContextVersion {
		return fmt.Errorf(
			"unsupported agent context version %d; expected %d",
			c.Version, AgentContextVersion,
		)
	}
	if c.RepoRoot == "" {
		return fmt.Errorf("agent context: repo_root is required")
	}
	if c.Prompt == "" {
		return fmt.Errorf("agent context: prompt is required")
	}
	return nil
}
