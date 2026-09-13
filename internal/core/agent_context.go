package core

import "fmt"

// AgentContextVersion is the current schema version for AgentContext.
const AgentContextVersion = 1

// AgentContext carries all metadata an agent needs to execute a task
// or track. Serialized as JSON and handed to the agent as a
// file — /workspace/.tlc/context.json in a container, a per-run
// .tlc/runs/<run-id>/context.json on the host — whose path the agent
// reads from EnvContextPath.
type AgentContext struct {
	Version         int      `json:"version"`
	TaskID          string   `json:"task_id,omitempty"`
	TaskTitle       string   `json:"task_title,omitempty"`
	TaskDescription string   `json:"task_description,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	TrackID         string   `json:"track_id,omitempty"`
	RepoRoot        string   `json:"repo_root"`
	Files           []string `json:"files,omitempty"`
	CtxtRefs        []string `json:"ctxt_refs,omitempty"`
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
