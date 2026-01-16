package core

import (
	"time"
)

// FlowStatus represents the transient state of a flow run.
type FlowStatus string

const (
	FlowStatusQueued    FlowStatus = "queued"
	FlowStatusRunning   FlowStatus = "running"
	FlowStatusSucceeded FlowStatus = "succeeded"
	FlowStatusFailed    FlowStatus = "failed"
	FlowStatusCanceled  FlowStatus = "canceled"
	FlowStatusPaused    FlowStatus = "paused"
)

// StepStatus represents the transient state of a step within a flow run.
type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusQueued    StepStatus = "queued"
	StepStatusRunning   StepStatus = "running"
	StepStatusSucceeded StepStatus = "succeeded"
	StepStatusFailed    StepStatus = "failed"
	StepStatusCanceled  StepStatus = "canceled"
	StepStatusSkipped   StepStatus = "skipped"
)

// StepType defines the kind of step.
type StepType string

const (
	StepTypeTask     StepType = "task"
	StepTypeParallel StepType = "parallel"
	StepTypeBranch   StepType = "branch"
	StepTypeJoin     StepType = "join"
	StepTypeRetry    StepType = "retry"
	StepTypeSubflow  StepType = "subflow"
)

// Flow represents a declarative workflow definition.
type Flow struct {
	ID          string          `json:"flow_id" yaml:"flow_id"`
	Name        string          `json:"name" yaml:"name"`
	Version     string          `json:"version" yaml:"version"`
	Description string          `json:"description,omitempty" yaml:"description,omitempty"`
	EntryStep   string          `json:"entry_step" yaml:"entry_step"`
	Steps       map[string]Step `json:"steps" yaml:"steps"`
	Meta        map[string]any  `json:"meta,omitempty" yaml:"meta,omitempty"`
}

// Step represents a node inside a flow.
type Step struct {
	ID        string         `json:"step_id" yaml:"step_id"`
	Type      StepType       `json:"type" yaml:"type"`
	Title     string         `json:"title" yaml:"title"`
	DependsOn []string       `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Meta      map[string]any `json:"meta,omitempty" yaml:"meta,omitempty"`

	// Task specific
	TaskRef string `json:"task_ref,omitempty" yaml:"task_ref,omitempty"`

	// Parallel specific
	Children       []string `json:"children,omitempty" yaml:"children,omitempty"`
	MaxConcurrency int      `json:"max_concurrency,omitempty" yaml:"max_concurrency,omitempty"`

	// Branch specific
	Cases       []BranchCase `json:"cases,omitempty" yaml:"cases,omitempty"`
	DefaultNext string       `json:"default_next,omitempty" yaml:"default_next,omitempty"`

	// Join specific
	WaitFor []string `json:"wait_for,omitempty" yaml:"wait_for,omitempty"`

	// Retry specific
	Child  string       `json:"child,omitempty" yaml:"child,omitempty"`
	Policy *RetryPolicy `json:"policy,omitempty" yaml:"policy,omitempty"`

	// Subflow specific
	FlowRef string         `json:"flow_ref,omitempty" yaml:"flow_ref,omitempty"`
	Inputs  map[string]any `json:"inputs,omitempty" yaml:"inputs,omitempty"`
}

// BranchCase defines a conditional path for a branch step.
type BranchCase struct {
	When string `json:"when" yaml:"when"`
	Next string `json:"next" yaml:"next"`
}

// RetryPolicy defines configuration for a retry step.
type RetryPolicy struct {
	MaxAttempts int `json:"max_attempts" yaml:"max_attempts"`
	BackoffMS   int `json:"backoff_ms" yaml:"backoff_ms"`
}

// FlowRun represents an execution instance of a flow definition.
type FlowRun struct {
	ID        string         `json:"run_id" yaml:"run_id"`
	FlowID    string         `json:"flow_id" yaml:"flow_id"`
	Status    FlowStatus     `json:"status" yaml:"status"`
	Progress  float64        `json:"progress" yaml:"progress"` // 0.0 to 1.0
	StartedAt time.Time      `json:"started_at" yaml:"started_at"`
	EndedAt   *time.Time     `json:"ended_at,omitempty" yaml:"ended_at,omitempty"`
	Results   map[string]any `json:"results,omitempty" yaml:"results,omitempty"`
}
