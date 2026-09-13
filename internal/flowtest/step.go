package flowtest

import (
	"strings"

	"hop.top/tlc/internal/core"
)

// StepRef is the recipe-era view of the step a task materializes: what an
// adapter needs to shape its invocation, and what the sandbox keys
// cassettes on. Agent is the task's own agent, empty when the step named
// none.
type StepRef struct {
	ID    string
	Title string
	Kind  string
	Agent string
}

// TaskStepRef reads the StepRef off a materialized task.
func TaskStepRef(task *core.Task) StepRef {
	ref := StepRef{ID: task.StepID, Title: task.Title, Kind: string(task.EffectiveKind())}
	if task.Spec != nil {
		ref.Agent = task.Spec.Agent
	}
	return ref
}

// cassetteMissMarker is what every shim prints on stderr when replay finds
// no cassette; cassetteMissExit is the exit code it dies with.
const (
	cassetteMissMarker = "cassette miss"
	cassetteMissExit   = 2
	// resultKeyOutput is where an adapter's unparsed stdout lands when it
	// is not JSON the adapter could shape.
	resultKeyOutput = "output"
	// resultKeyStderr is the exec result field a shim writes its miss
	// marker to.
	resultKeyStderr = "stderr"
)

// IsCassetteMiss reports whether a task result is a shim's cassette miss:
// the shim exit code, or its stderr marker in the exec result's stderr or
// an adapter's wrapped raw output.
func IsCassetteMiss(result map[string]any) bool {
	if result == nil {
		return false
	}
	if code, ok := resultExitCode(result); ok && code == cassetteMissExit {
		return true
	}
	for _, key := range []string{resultKeyStderr, resultKeyOutput} {
		if s, ok := result[key].(string); ok && strings.Contains(s, cassetteMissMarker) {
			return true
		}
	}
	return false
}

// resultExitCode reads exit_code whether it was set in memory (int) or
// came back through a JSON column (float64).
func resultExitCode(result map[string]any) (int, bool) {
	switch v := result[core.ResultKeyExitCode].(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}
