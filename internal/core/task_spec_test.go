package core

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// fullTaskSpec exercises every TaskSpec field so a round trip that drops
// any of them fails loudly.
func fullTaskSpec() *TaskSpec {
	return &TaskSpec{
		Agent: "claude",
		When:  "results.lint.exit_code == 0",
		Exec: &ExecSpec{
			Argv:      []string{"go", "test", "./..."},
			Cwd:       ".",
			Env:       map[string]string{"GOFLAGS": "-count=1"},
			Timeout:   "5m",
			StdoutMax: 1 << 20,
		},
		Human: &HumanSpec{
			Assignee:  "@lead",
			Timeout:   "48h",
			OnTimeout: "reject",
		},
		Retry: &RetrySpec{MaxAttempts: 3, Backoff: "30s"},
		Gate:  &StepGate{Contract: "review-quality", EvaURL: "http://eva.local"},
	}
}

// TestTask_EffectiveKind_DefaultsToAgent pins the reading of an unset kind:
// every task that predates recipes, and every task created without a
// recipe, is an agent task. The accessor must also be nil-safe because
// executor code reads kinds off tasks it did not necessarily load.
func TestTask_EffectiveKind_DefaultsToAgent(t *testing.T) {
	var nilTask *Task
	if got := nilTask.EffectiveKind(); got != TaskKindAgent {
		t.Errorf("nil task kind = %q; want %q", got, TaskKindAgent)
	}

	cases := []struct {
		kind TaskKind
		want TaskKind
	}{
		{"", TaskKindAgent},
		{TaskKindAgent, TaskKindAgent},
		{TaskKindExec, TaskKindExec},
		{TaskKindHuman, TaskKindHuman},
	}
	for _, tc := range cases {
		task := &Task{Kind: tc.kind}
		if got := task.EffectiveKind(); got != tc.want {
			t.Errorf("Kind %q: EffectiveKind() = %q; want %q", tc.kind, got, tc.want)
		}
	}
}

// TestTaskKind_Valid pins the closed kind vocabulary. Empty is valid
// because it means "default", mirroring ValidPriority's treatment of an
// optional field.
func TestTaskKind_Valid(t *testing.T) {
	for _, k := range []TaskKind{"", TaskKindAgent, TaskKindExec, TaskKindHuman} {
		if !k.Valid() {
			t.Errorf("kind %q rejected; want valid", k)
		}
	}
	for _, k := range []TaskKind{"robot", "AGENT", "exec "} {
		if k.Valid() {
			t.Errorf("kind %q accepted; want invalid", k)
		}
	}
}

// TestTaskSpec_JSONRoundTrip is the storage contract: spec is persisted
// as a JSON blob, so every field must survive Marshal/Unmarshal intact.
func TestTaskSpec_JSONRoundTrip(t *testing.T) {
	want := fullTaskSpec()

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got TaskSpec
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(&got, want) {
		t.Errorf("JSON round trip changed the spec:\n got %+v\nwant %+v", got, want)
	}

	// Wire keys follow the recipe format, not Go field names.
	for _, key := range []string{`"max_attempts"`, `"on_timeout"`, `"stdout_max"`, `"argv"`, `"when"`} {
		if !strings.Contains(string(data), key) {
			t.Errorf("JSON is missing wire key %s: %s", key, data)
		}
	}
}

// TestTaskSpec_YAMLRoundTrip covers the recipe file side: the same struct
// is decoded from recipe YAML, so the yaml tags must match the json ones.
func TestTaskSpec_YAMLRoundTrip(t *testing.T) {
	want := fullTaskSpec()

	data, err := yaml.Marshal(want)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var got TaskSpec
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(&got, want) {
		t.Errorf("YAML round trip changed the spec:\n got %+v\nwant %+v", got, want)
	}
	for _, key := range []string{"max_attempts:", "on_timeout:", "stdout_max:"} {
		if !strings.Contains(string(data), key) {
			t.Errorf("YAML is missing key %s:\n%s", key, data)
		}
	}
}

// TestTaskSpec_EmptyMarshalsCompact keeps an all-default spec from
// bloating every task row: nothing set means "{}" on the wire.
func TestTaskSpec_EmptyMarshalsCompact(t *testing.T) {
	data, err := json.Marshal(&TaskSpec{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != "{}" {
		t.Errorf("empty spec = %s; want {}", data)
	}
}

// TestTask_SpecFieldsOmittedWhenUnset guards the projection and JSON
// output of legacy tasks: a task that never touched recipes must not grow
// kind/spec/result/attempts noise in its serialized form.
func TestTask_SpecFieldsOmittedWhenUnset(t *testing.T) {
	data, err := json.Marshal(&Task{ID: "task_x", Title: "plain"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, key := range []string{`"kind"`, `"spec"`, `"result"`, `"attempts"`, `"claimed_at"`, `"run_id"`, `"step_id"`, `"step_ordinal"`} {
		if strings.Contains(string(data), key) {
			t.Errorf("unset field %s serialized: %s", key, data)
		}
	}
}
