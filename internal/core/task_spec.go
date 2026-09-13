package core

// TaskKind selects how the executor dispatches a task: to an agent (the
// default and the only kind that existed before recipes), as a literal
// argv run, or to a human who approves or rejects it.
type TaskKind string

const (
	TaskKindAgent TaskKind = "agent"
	TaskKindExec  TaskKind = "exec"
	TaskKindHuman TaskKind = "human"
)

// Valid reports whether k names a known kind. Empty is valid because it
// means "default" — kind is optional on the wire, like priority.
func (k TaskKind) Valid() bool {
	switch k {
	case "", TaskKindAgent, TaskKindExec, TaskKindHuman:
		return true
	}
	return false
}

// EffectiveKind resolves an unset kind to agent. Nil-safe so executor
// code can read it off any task reference without a guard.
func (t *Task) EffectiveKind() TaskKind {
	if t == nil || t.Kind == "" {
		return TaskKindAgent
	}
	return t.Kind
}

// ExecSpec describes an exec-kind task: a literal command run without an
// agent. Timeout is a duration string parsed on use so the spec stays a
// plain JSON/YAML document.
type ExecSpec struct {
	Argv      []string          `json:"argv" yaml:"argv"`
	Cwd       string            `json:"cwd,omitempty" yaml:"cwd,omitempty"`
	Env       map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Timeout   string            `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	StdoutMax int               `json:"stdout_max,omitempty" yaml:"stdout_max,omitempty"`
}

// HumanSpec describes a human-kind task. OnTimeout is "approve" or
// "reject" and is applied lazily once Timeout has elapsed since creation.
type HumanSpec struct {
	Assignee  string `json:"assignee,omitempty" yaml:"assignee,omitempty"`
	Timeout   string `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	OnTimeout string `json:"on_timeout,omitempty" yaml:"on_timeout,omitempty"`
}

// RetrySpec bounds re-dispatch after a failed attempt. MaxAttempts counts
// the first attempt; Backoff is a duration string.
type RetrySpec struct {
	MaxAttempts int    `json:"max_attempts,omitempty" yaml:"max_attempts,omitempty"`
	Backoff     string `json:"backoff,omitempty" yaml:"backoff,omitempty"`
}

// StepGate names an EVA contract that a task's result must satisfy before
// the task is marked done.
type StepGate struct {
	Contract string `json:"contract" yaml:"contract"` // named EVA contract
	EvaURL   string `json:"eva_url"  yaml:"eva_url"`  // EVA gateway base URL
}

// TaskSpec is everything a recipe step carries into a task that the
// executor consults at dispatch time. It is persisted as one JSON blob;
// the fields the readiness query needs (kind, attempts, run/step
// identity) live in their own columns on Task instead.
type TaskSpec struct {
	Agent string     `json:"agent,omitempty" yaml:"agent,omitempty"`
	When  string     `json:"when,omitempty" yaml:"when,omitempty"`
	Exec  *ExecSpec  `json:"exec,omitempty" yaml:"exec,omitempty"`
	Human *HumanSpec `json:"human,omitempty" yaml:"human,omitempty"`
	Retry *RetrySpec `json:"retry,omitempty" yaml:"retry,omitempty"`
	Gate  *StepGate  `json:"gate,omitempty" yaml:"gate,omitempty"`
}

// HumanTimeoutAction is the auto-action applied when a human task's
// timeout elapses.
type HumanTimeoutAction string

const (
	HumanTimeoutApprove     HumanTimeoutAction = "approve"
	HumanTimeoutReject      HumanTimeoutAction = "reject"
	HumanTimeoutKeepWaiting HumanTimeoutAction = "keep_waiting"
)
