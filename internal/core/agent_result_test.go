package core

import (
	"encoding/json"
	"testing"
)

func TestAgentResult_Validate(t *testing.T) {
	tests := []struct {
		name    string
		result  AgentResult
		wantErr string
	}{
		{
			name: "valid succeeded",
			result: AgentResult{
				Version:  1,
				Status:   AgentStatusSucceeded,
				ExitCode: 0,
				Summary:  "Fixed parser crash; added test",
			},
		},
		{
			name: "valid failed",
			result: AgentResult{
				Version:  1,
				Status:   AgentStatusFailed,
				ExitCode: 1,
				Summary:  "Could not reproduce the bug",
			},
		},
		{
			name: "valid partial",
			result: AgentResult{
				Version:  1,
				Status:   AgentStatusPartial,
				ExitCode: 0,
				Summary:  "Fixed 2 of 3 issues",
			},
		},
		{
			name: "valid timeout",
			result: AgentResult{
				Version:  1,
				Status:   AgentStatusTimeout,
				ExitCode: 124,
				Summary:  "Timed out after 30m",
			},
		},
		{
			name: "wrong version",
			result: AgentResult{
				Version:  2,
				Status:   AgentStatusSucceeded,
				ExitCode: 0,
				Summary:  "done",
			},
			wantErr: "unsupported agent result version 2; expected 1",
		},
		{
			name: "invalid status",
			result: AgentResult{
				Version:  1,
				Status:   "unknown",
				ExitCode: 0,
				Summary:  "done",
			},
			wantErr: `invalid agent result status "unknown"; valid: succeeded, failed, partial, timeout`,
		},
		{
			name: "empty status",
			result: AgentResult{
				Version:  1,
				Status:   "",
				ExitCode: 0,
				Summary:  "done",
			},
			wantErr: `invalid agent result status ""; valid: succeeded, failed, partial, timeout`,
		},
		{
			name: "missing summary",
			result: AgentResult{
				Version:  1,
				Status:   AgentStatusSucceeded,
				ExitCode: 0,
			},
			wantErr: "agent result: summary is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.result.Validate()
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

func TestAgentResult_JSONRoundTrip(t *testing.T) {
	result := AgentResult{
		Version:   1,
		Status:    AgentStatusSucceeded,
		ExitCode:  0,
		Summary:   "Fixed parser crash; added test",
		Agent:     "claude",
		StartedAt: "2026-04-14T10:00:00Z",
		EndedAt:   "2026-04-14T10:05:00Z",
		Outputs: map[string]any{
			"files_changed": []any{"internal/parser/parse.go"},
			"commit_sha":    "abc123",
		},
		Artifacts: []AgentArtifact{
			{
				Path:        "internal/parser/parse.go",
				Type:        "modified",
				Description: "Added nil check for empty input",
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got AgentResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Version != result.Version {
		t.Errorf("version = %d, want %d", got.Version, result.Version)
	}
	if got.Status != result.Status {
		t.Errorf("status = %q, want %q", got.Status, result.Status)
	}
	if got.ExitCode != result.ExitCode {
		t.Errorf("exit_code = %d, want %d", got.ExitCode, result.ExitCode)
	}
	if got.Summary != result.Summary {
		t.Errorf("summary = %q, want %q", got.Summary, result.Summary)
	}
	if got.Agent != result.Agent {
		t.Errorf("agent = %q, want %q", got.Agent, result.Agent)
	}
	if len(got.Artifacts) != 1 {
		t.Fatalf("artifacts len = %d, want 1", len(got.Artifacts))
	}
	if got.Artifacts[0].Path != "internal/parser/parse.go" {
		t.Errorf("artifact path = %q", got.Artifacts[0].Path)
	}
}

func TestAgentResult_OmitsEmptyFields(t *testing.T) {
	result := AgentResult{
		Version:  1,
		Status:   AgentStatusSucceeded,
		ExitCode: 0,
		Summary:  "done",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}

	for _, key := range []string{
		"agent", "started_at", "ended_at", "outputs", "artifacts",
	} {
		if _, ok := raw[key]; ok {
			t.Errorf("expected %q to be omitted from JSON", key)
		}
	}
}

func TestAgentArtifact_JSON(t *testing.T) {
	a := AgentArtifact{
		Path:        "src/main.go",
		Type:        "created",
		Description: "New entry point",
	}

	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got AgentArtifact
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Path != a.Path || got.Type != a.Type || got.Description != a.Description {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
}
