package core

import (
	"encoding/json"
	"testing"
)

func TestAgentContext_Validate(t *testing.T) {
	tests := []struct {
		name    string
		ctx     AgentContext
		wantErr string
	}{
		{
			name: "valid task context",
			ctx: AgentContext{
				Version:  1,
				TaskID:   "T-0042",
				RepoRoot: "/workspace",
				Prompt:   "Fix the bug",
			},
		},
		{
			name: "valid flow context",
			ctx: AgentContext{
				Version:    1,
				FlowID:     "code-review",
				FlowStepID: "understand-requirements",
				StepType:   "task",
				StepTitle:  "Understand requirements",
				RepoRoot:   "/workspace",
				Prompt:     "Review the code",
			},
		},
		{
			name: "wrong version",
			ctx: AgentContext{
				Version:  99,
				RepoRoot: "/workspace",
				Prompt:   "Fix the bug",
			},
			wantErr: "unsupported agent context version 99; expected 1",
		},
		{
			name: "zero version",
			ctx: AgentContext{
				Version:  0,
				RepoRoot: "/workspace",
				Prompt:   "Fix the bug",
			},
			wantErr: "unsupported agent context version 0; expected 1",
		},
		{
			name: "missing repo_root",
			ctx: AgentContext{
				Version: 1,
				Prompt:  "Fix the bug",
			},
			wantErr: "agent context: repo_root is required",
		},
		{
			name: "missing prompt",
			ctx: AgentContext{
				Version:  1,
				RepoRoot: "/workspace",
			},
			wantErr: "agent context: prompt is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ctx.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestAgentContext_JSONRoundTrip(t *testing.T) {
	ctx := AgentContext{
		Version:         1,
		TaskID:          "T-0042",
		TaskTitle:       "Fix parser crash",
		TaskDescription: "Parser panics on empty input",
		Tags:            []string{"type:bug", "priority:P1"},
		TrackID:         "parser-rewrite",
		RepoRoot:        "/workspace",
		Files:           []string{"internal/parser/parse.go"},
		Prompt:          "Fix the bug described in this task.",
	}

	data, err := json.Marshal(ctx)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got AgentContext
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Version != ctx.Version {
		t.Errorf("version = %d, want %d", got.Version, ctx.Version)
	}
	if got.TaskID != ctx.TaskID {
		t.Errorf("task_id = %q, want %q", got.TaskID, ctx.TaskID)
	}
	if got.RepoRoot != ctx.RepoRoot {
		t.Errorf("repo_root = %q, want %q", got.RepoRoot, ctx.RepoRoot)
	}
	if got.Prompt != ctx.Prompt {
		t.Errorf("prompt = %q, want %q", got.Prompt, ctx.Prompt)
	}
	if len(got.Tags) != len(ctx.Tags) {
		t.Errorf("tags len = %d, want %d", len(got.Tags), len(ctx.Tags))
	}
	if len(got.Files) != len(ctx.Files) {
		t.Errorf("files len = %d, want %d", len(got.Files), len(ctx.Files))
	}
}

func TestAgentContext_OmitsEmptyFields(t *testing.T) {
	ctx := AgentContext{
		Version:  1,
		RepoRoot: "/workspace",
		Prompt:   "do something",
	}

	data, err := json.Marshal(ctx)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}

	for _, key := range []string{
		"task_id", "task_title", "task_description",
		"tags", "track_id", "flow_id", "flow_step_id",
		"step_type", "step_title", "files",
	} {
		if _, ok := raw[key]; ok {
			t.Errorf("expected %q to be omitted from JSON", key)
		}
	}
}
