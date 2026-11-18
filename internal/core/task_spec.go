package core

// EffectiveKind resolves an unset kind to agent. Nil-safe so executor
// code can read it off any task reference without a guard.
func (t *Task) EffectiveKind() TaskKind {
	if t == nil || t.Kind == "" {
		return TaskKindAgent
	}
	return t.Kind
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
