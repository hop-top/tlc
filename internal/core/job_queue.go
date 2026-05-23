package core

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// JobStatus represents the lifecycle state of an async job.
type JobStatus string

const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCanceled  JobStatus = "canceled"
)

// JobType classifies the kind of async agent dispatch.
type JobType string

const (
	JobTypeAgentTask  JobType = "tlc.agent.task"
	JobTypeAgentFlow  JobType = "tlc.agent.flow"
	JobTypeAgentTrack JobType = "tlc.agent.track"
)

// Job is an async agent execution job.
type Job struct {
	ID        string     `json:"id"`
	Queue     string     `json:"queue"`
	Type      JobType    `json:"type"`
	Status    JobStatus  `json:"status"`
	Payload   string     `json:"payload"`
	Result    string     `json:"result,omitempty"`
	Error     string     `json:"error,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	CreatedBy string     `json:"created_by,omitempty"`
}

// JobPayload is the serialized payload for an async agent job.
type JobPayload struct {
	AgentName        string   `json:"agent_name"`
	Tasks            []string `json:"tasks,omitempty"`
	FlowRef          string   `json:"flow_ref,omitempty"`
	TrackID          string   `json:"track_id,omitempty"`
	Image            string   `json:"image,omitempty"`
	Local            bool     `json:"local,omitempty"`
	Prompt           string   `json:"prompt,omitempty"`
	NoState          bool     `json:"no_state_update,omitempty"`
	KeepPod          bool     `json:"keep_pod,omitempty"`
	Network          string   `json:"network,omitempty"`
	Retries          int      `json:"retries,omitempty"`
	TimeoutSecs      int      `json:"timeout_secs,omitempty"`
	Env              []string `json:"env,omitempty"`
	Mounts           []string `json:"mounts,omitempty"`
	Context          []string `json:"context,omitempty"`
	TotalTimeoutSecs int      `json:"total_timeout_secs,omitempty"`
	TrustProject     bool     `json:"trust_project,omitempty"`
}

// MarshalPayload encodes a JobPayload to JSON string.
func MarshalPayload(p *JobPayload) (string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("marshal job payload: %w", err)
	}
	return string(data), nil
}

// UnmarshalPayload decodes a JSON string to JobPayload.
func UnmarshalPayload(s string) (*JobPayload, error) {
	var p JobPayload
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return nil, fmt.Errorf("unmarshal job payload: %w", err)
	}
	return &p, nil
}

// JobStore persists async job records.
type JobStore interface {
	CreateJob(ctx context.Context, job *Job) error
	UpdateJob(ctx context.Context, job *Job) error
	GetJob(ctx context.Context, id string) (*Job, error)
	ListJobs(ctx context.Context, queue string, status JobStatus, limit int) ([]*Job, error)
	ClaimNextJob(ctx context.Context, queue string) (*Job, error)
	CancelJob(ctx context.Context, id string) error
}
