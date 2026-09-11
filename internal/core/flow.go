package core

import (
	"encoding/json"
	"time"

	"gopkg.in/yaml.v3"
)

// AgentRef identifies an agent adapter by name with optional config overrides.
// It unmarshals from either a plain string ("claude") or a struct
// ({name: gemini, config: {dir: /custom}}).
type AgentRef struct {
	Name   string         `json:"name,omitempty" yaml:"name,omitempty"`
	Config map[string]any `json:"config,omitempty" yaml:"config,omitempty"`
}

// IsZero reports whether the ref carries no agent selection.
func (r AgentRef) IsZero() bool { return r.Name == "" }

// UnmarshalYAML supports both scalar string and mapping forms.
func (r *AgentRef) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		r.Name = value.Value
		return nil
	case yaml.MappingNode:
		type plain AgentRef
		return value.Decode((*plain)(r)) //nolint:wrapcheck // Unmarshaler contract; yaml decorates with node position
	default:
		return nil
	}
}

// MarshalYAML emits a plain string when Config is empty, struct otherwise.
func (r AgentRef) MarshalYAML() (any, error) {
	if len(r.Config) == 0 {
		return r.Name, nil
	}
	type plain AgentRef
	return plain(r), nil
}

// UnmarshalJSON supports both string and object forms.
func (r *AgentRef) UnmarshalJSON(b []byte) error {
	// try string first
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		r.Name = s
		return nil
	}
	type plain AgentRef
	return json.Unmarshal(b, (*plain)(r)) //nolint:wrapcheck // Unmarshaler contract; json decorates with type + offset
}

// MarshalJSON emits a plain string when Config is empty, object otherwise.
func (r AgentRef) MarshalJSON() ([]byte, error) {
	if len(r.Config) == 0 {
		return json.Marshal(r.Name) //nolint:wrapcheck // Marshaler contract; json decorates with type context
	}
	type plain AgentRef
	return json.Marshal(plain(r)) //nolint:wrapcheck // Marshaler contract; json decorates with type context
}

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
	StepTypeExec     StepType = "exec"
)

// ExecStep configures a `type: exec` step. Defined by task-flow-spec-0.1-dev
// (Step Type: exec). Runs argv literally and captures structured output.
type ExecStep struct {
	// Argv is the command and its arguments. argv[0] is resolved against PATH.
	// Required; MUST contain at least one element.
	Argv []string `json:"argv" yaml:"argv"`

	// Env is extra env merged onto the parent environment for the child.
	// Empty value unsets the variable. Optional.
	Env map[string]string `json:"env,omitempty" yaml:"env,omitempty"`

	// Cwd is the working directory for the child. Default = flow workdir.
	// Relative paths resolve against the flow workdir. Optional.
	Cwd string `json:"cwd,omitempty" yaml:"cwd,omitempty"`

	// TimeoutSec caps wall-clock runtime in seconds. Default 60. 0 = default.
	TimeoutSec int `json:"timeout_sec,omitempty" yaml:"timeout_sec,omitempty"`

	// AllowNonzeroExit, when true, treats non-zero exit as success.
	// Default false. Timeout always fails regardless.
	AllowNonzeroExit bool `json:"allow_nonzero_exit,omitempty" yaml:"allow_nonzero_exit,omitempty"`

	// StdoutMaxBytes truncates captured stdout/stderr beyond this limit.
	// Default 1 MiB. 0 = default.
	StdoutMaxBytes int `json:"stdout_max_bytes,omitempty" yaml:"stdout_max_bytes,omitempty"`
}

// Flow represents a declarative workflow definition.
type Flow struct {
	ID          string          `json:"flow_id" yaml:"flow_id"`
	Name        string          `json:"name" yaml:"name"`
	Version     string          `json:"version" yaml:"version"`
	Description string          `json:"description,omitempty" yaml:"description,omitempty"`
	EntryStep   string          `json:"entry_step" yaml:"entry_step"`
	Steps       map[string]Step `json:"steps" yaml:"steps"`
	Meta        map[string]any  `json:"meta,omitempty" yaml:"meta,omitempty"`
	Config      *FlowConfig     `json:"config,omitempty" yaml:"config,omitempty"`

	// Agent is the flow-level default adapter. Accepts string shorthand or struct.
	// Steps may override with their own Agent field.
	Agent AgentRef `json:"agent,omitempty" yaml:"agent,omitempty"`

	// Adapters configures capability-based adapter selection and the default
	// fallback for this flow. Takes precedence over the global adapter config.
	Adapters *FlowAdapters `json:"adapters,omitempty" yaml:"adapters,omitempty"`

	// Triggers groups all trigger sources on the flow (cron, keywords).
	// Story 024 — flow cron trigger.
	Triggers *FlowTriggers `json:"triggers,omitempty" yaml:"triggers,omitempty"`

	// Timezone is the IANA tz name applied to cron triggers. Default: system tz.
	Timezone string `json:"timezone,omitempty" yaml:"timezone,omitempty"`
}

// FlowAdapters holds capability-based adapter mappings and a default adapter
// for a flow. Used by AdapterResolver to select the right agent adapter.
type FlowAdapters struct {
	// Default is the fallback adapter when no mapping matches.
	// If zero and no global default exists, resolution is a fatal error.
	Default AgentRef `json:"default,omitempty" yaml:"default,omitempty"`

	// Configs holds per-adapter config keyed by adapter name.
	// Key "dir" is the canonical config directory for the adapter.
	Configs map[string]map[string]any `json:"configs,omitempty" yaml:"configs,omitempty"`

	// Mappings is an ordered list of capability-to-adapter rules.
	// First match wins (any capability intersection triggers the mapping).
	Mappings []AdapterMapping `json:"mappings,omitempty" yaml:"mappings,omitempty"`
}

// AdapterMapping maps a set of capabilities or tools to an adapter (with optional config).
type AdapterMapping struct {
	// Capabilities are matched against a step's task_template.requirements.capabilities.
	// Any intersection (at least one common capability) triggers this mapping.
	Capabilities []string `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	// Tools are matched against a step's task_template.requirements.tools.
	// Any intersection with capabilities OR tools triggers this mapping.
	Tools []string `json:"tools,omitempty" yaml:"tools,omitempty"`
	Agent AgentRef `json:"agent" yaml:"agent"`
}

// FlowInputDef describes a named input variable accepted by a flow.
type FlowInputDef struct {
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Default     any    `json:"default,omitempty" yaml:"default,omitempty"`
}

// FlowConfig holds flow configuration including procedural instructions.
type FlowConfig struct {
	// Procedural instructions (markdown format, like superpowers skills)
	Procedure string `json:"procedure,omitempty" yaml:"procedure,omitempty"`
	// Category for organization (e.g., "creative", "debugging", "deployment")
	Category string `json:"category,omitempty" yaml:"category,omitempty"`
	// Triggers that suggest using this flow
	Triggers []string `json:"triggers,omitempty" yaml:"triggers,omitempty"`
	// Named input variables accepted by this flow
	Inputs map[string]FlowInputDef `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	// Additional config parameters
	Params map[string]any `json:"params,omitempty" yaml:"params,omitempty"`
}

// Step represents a node inside a flow.
type Step struct {
	ID        string         `json:"step_id" yaml:"step_id"`
	Type      StepType       `json:"type" yaml:"type"`
	Title     string         `json:"title" yaml:"title"`
	DependsOn []string       `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Meta      map[string]any `json:"meta,omitempty" yaml:"meta,omitempty"`

	// Condition gates step execution against flow inputs. Empty = always run.
	// Syntax: `<lhs> == '<value>'` or `<lhs> != '<value>'` where lhs is
	// `inputs.<key>` or bare `<key>`. See docs/task-flow-spec-0.1-dev.md
	// "Conditional execution" for full grammar.
	Condition string `json:"condition,omitempty" yaml:"condition,omitempty"`

	// Agent overrides the flow-level default adapter for this step.
	// Accepts string shorthand or struct with config. Zero value = inherit.
	Agent AgentRef `json:"agent,omitempty" yaml:"agent,omitempty"`

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

	// Exec specific. Set when Type == StepTypeExec.
	Exec *ExecStep `json:"exec,omitempty" yaml:"exec,omitempty"`

	// Task template for task generation (flows & assignees feature)
	TaskTemplate *TaskTemplate `json:"task_template,omitempty" yaml:"task_template,omitempty"`

	// Gate is optional. When set, the step output is validated by EVA before
	// the step is marked succeeded.
	Gate *StepGate `json:"gate,omitempty" yaml:"gate,omitempty"`

	// Human is config for type:human steps (story 025). Optional;
	// missing means "no timeout, no capability gate, no webhook".
	Human *HumanStepConfig `json:"human,omitempty" yaml:"human,omitempty"`
}

// TaskTemplate defines how to generate tasks from flow steps.
type TaskTemplate struct {
	Title        string            `json:"title" yaml:"title"`
	Description  string            `json:"description,omitempty" yaml:"description,omitempty"`
	Requirements *TaskRequirements `json:"requirements,omitempty" yaml:"requirements,omitempty"`
	Context      map[string]any    `json:"context,omitempty" yaml:"context,omitempty"`
}

// TaskRequirements specifies capabilities needed to execute a task.
type TaskRequirements struct {
	Capabilities []string `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	Domains      []string `json:"domains,omitempty" yaml:"domains,omitempty"`
	Tools        []string `json:"tools,omitempty" yaml:"tools,omitempty"`
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

// StepGate configures optional EVA contract validation for a step output.
// When present, the step is only marked succeeded if EVA returns a pass result.
type StepGate struct {
	Contract string `json:"contract" yaml:"contract"` // named EVA contract
	EvaURL   string `json:"eva_url"  yaml:"eva_url"`  // EVA gateway base URL
}

// FlowRun represents an execution instance of a flow definition.
type FlowRun struct {
	ID        string         `json:"run_id" yaml:"run_id" table:"Run ID"`
	FlowID    string         `json:"flow_id" yaml:"flow_id" table:"Flow"`
	Status    FlowStatus     `json:"status" yaml:"status" table:"Status"`
	Progress  float64        `json:"progress" yaml:"progress" table:"Progress"` // 0.0 to 1.0
	StartedAt time.Time      `json:"started_at" yaml:"started_at" table:"Started"`
	EndedAt   *time.Time     `json:"ended_at,omitempty" yaml:"ended_at,omitempty"`
	Results   map[string]any `json:"results,omitempty" yaml:"results,omitempty"`
}
