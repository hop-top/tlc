package flowtest

import (
	"testing"

	"hop.top/tlc/internal/core"
)

func TestTaskStepRefReadsTheTask(t *testing.T) {
	t.Parallel()
	task := &core.Task{
		ID: "task_1", Title: "Review it", StepID: "review", Kind: core.TaskKindAgent,
		Spec: &core.TaskSpec{Agent: "gemini"},
	}
	got := TaskStepRef(task)
	want := StepRef{ID: "review", Title: "Review it", Kind: "agent", Agent: "gemini"}
	if got != want {
		t.Errorf("TaskStepRef = %+v, want %+v", got, want)
	}
}

func TestTaskStepRefDefaultsKindAndTolerateNilSpec(t *testing.T) {
	t.Parallel()
	got := TaskStepRef(&core.Task{ID: "task_2", Title: "Lint", StepID: "lint"})
	if got.Kind != "agent" {
		t.Errorf("Kind = %q, want agent (unset kind resolves to agent)", got.Kind)
	}
	if got.Agent != "" {
		t.Errorf("Agent = %q, want empty for a nil spec", got.Agent)
	}
}

func TestIsCassetteMiss(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		result map[string]any
		want   bool
	}{
		{"nil result", nil, false},
		{"clean exit", map[string]any{"exit_code": 0, "stderr": ""}, false},
		{"plain failure", map[string]any{"exit_code": 1, "stderr": "boom"}, false},
		{"shim exit code as int", map[string]any{"exit_code": 2}, true},
		{"shim exit code after a json round trip", map[string]any{"exit_code": float64(2)}, true},
		{"shim exit code as int64", map[string]any{"exit_code": int64(2)}, true},
		{"marker on stderr", map[string]any{"exit_code": 1, "stderr": "tlc-flow-test: cassette miss for \"gh\": re-run with --record"}, true},
		{"marker in wrapped agent output", map[string]any{"output": "tlc-flow-test: cassette miss for \"claude\""}, true},
		{"marker in an unrelated key is ignored", map[string]any{"stdout": "cassette miss"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsCassetteMiss(tc.result); got != tc.want {
				t.Errorf("IsCassetteMiss(%v) = %v, want %v", tc.result, got, tc.want)
			}
		})
	}
}
