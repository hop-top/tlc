package core

import (
	"context"
	"strings"
	"testing"
)

func TestContextBuilder_BuildForTask(t *testing.T) {
	repo := NewMockRepository()
	ctx := context.Background()

	trackID := "parser-rewrite"
	_ = repo.CreateTask(ctx, &Task{
		ID:          "T-0042",
		Title:       "Fix parser crash",
		Description: "Parser panics in internal/parser/parse.go when input is empty",
		Tags:        []string{"type:bug", "priority:P1"},
		Status:      StatusInProgress,
		TrackID:     &trackID,
	})

	cb := NewContextBuilder(repo)
	ac, err := cb.BuildForTask(ctx, "T-0042", BuildOpts{})
	if err != nil {
		t.Fatal(err)
	}

	if ac.Version != AgentContextVersion {
		t.Errorf("version = %d, want %d", ac.Version, AgentContextVersion)
	}
	if ac.TaskID != "T-0042" {
		t.Errorf("task_id = %q", ac.TaskID)
	}
	if ac.TaskTitle != "Fix parser crash" {
		t.Errorf("title = %q", ac.TaskTitle)
	}
	if ac.TrackID != "parser-rewrite" {
		t.Errorf("track_id = %q", ac.TrackID)
	}
	if ac.RepoRoot != "/workspace" {
		t.Errorf("repo_root = %q", ac.RepoRoot)
	}
	// Should extract file ref from description.
	found := false
	for _, f := range ac.Files {
		if f == "internal/parser/parse.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("files = %v, want internal/parser/parse.go", ac.Files)
	}
	if ac.Prompt == "" {
		t.Error("prompt is empty")
	}
	if err := ac.Validate(); err != nil {
		t.Errorf("validate: %v", err)
	}
}

func TestContextBuilder_BuildForTask_CustomPrompt(t *testing.T) {
	repo := NewMockRepository()
	ctx := context.Background()
	_ = repo.CreateTask(ctx, &Task{
		ID:     "T-0001",
		Title:  "Test",
		Status: StatusTodo,
	})

	cb := NewContextBuilder(repo)
	ac, err := cb.BuildForTask(ctx, "T-0001", BuildOpts{
		PromptFile: "custom prompt content",
		RepoRoot:   "/custom",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ac.Prompt != "custom prompt content" {
		t.Errorf("prompt = %q, want custom", ac.Prompt)
	}
	if ac.RepoRoot != "/custom" {
		t.Errorf("repo_root = %q", ac.RepoRoot)
	}
}

func TestContextBuilder_BuildForTask_NotFound(t *testing.T) {
	repo := NewMockRepository()
	cb := NewContextBuilder(repo)
	_, err := cb.BuildForTask(context.Background(), "T-9999", BuildOpts{})
	if err == nil {
		t.Fatal("expected error for missing task")
	}
	if !strings.Contains(err.Error(), "T-9999") {
		t.Errorf("error = %q, should mention task ID", err.Error())
	}
}

func TestContextBuilder_BuildForFlowStep(t *testing.T) {
	flow := &Flow{
		ID:   "code-review",
		Name: "Code Review",
		Steps: map[string]Step{
			"understand": {
				ID:    "understand",
				Type:  StepTypeTask,
				Title: "Understand requirements",
				TaskTemplate: &TaskTemplate{
					Description: "Review the code changes",
				},
			},
		},
	}

	cb := NewContextBuilder(NewMockRepository())
	ac, err := cb.BuildForFlowStep(
		context.Background(), flow, "understand", BuildOpts{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if ac.FlowID != "code-review" {
		t.Errorf("flow_id = %q", ac.FlowID)
	}
	if ac.FlowStepID != "understand" {
		t.Errorf("flow_step_id = %q", ac.FlowStepID)
	}
	if ac.StepType != "task" {
		t.Errorf("step_type = %q", ac.StepType)
	}
	if ac.StepTitle != "Understand requirements" {
		t.Errorf("step_title = %q", ac.StepTitle)
	}
	if ac.Prompt == "" {
		t.Error("prompt is empty")
	}
}

func TestContextBuilder_BuildForFlowStep_MissingStep(t *testing.T) {
	flow := &Flow{
		ID:    "test",
		Steps: map[string]Step{},
	}

	cb := NewContextBuilder(NewMockRepository())
	_, err := cb.BuildForFlowStep(
		context.Background(), flow, "nonexistent", BuildOpts{},
	)
	if err == nil {
		t.Fatal("expected error for missing step")
	}
}

func TestContextBuilder_BuildForTrack(t *testing.T) {
	repo := NewMockRepository()
	ctx := context.Background()

	trackID := "my-track"
	_ = repo.CreateTask(ctx, &Task{
		ID:      "T-0001",
		Title:   "First task",
		Status:  StatusTodo,
		TrackID: &trackID,
	})
	_ = repo.CreateTask(ctx, &Task{
		ID:      "T-0002",
		Title:   "Second task (depends on first)",
		Status:  StatusTodo,
		TrackID: &trackID,
		Meta:    map[string]interface{}{"blocked_by": []interface{}{"T-0001"}},
	})
	// Done task should be excluded.
	_ = repo.CreateTask(ctx, &Task{
		ID:      "T-0003",
		Title:   "Done task",
		Status:  StatusDone,
		TrackID: &trackID,
	})

	cb := NewContextBuilder(repo)
	contexts, err := cb.BuildForTrack(ctx, trackID, BuildOpts{})
	if err != nil {
		t.Fatal(err)
	}

	// Only TODO tasks should be included.
	if len(contexts) != 2 {
		t.Fatalf("got %d contexts, want 2", len(contexts))
	}

	// T-0001 should come before T-0002 (blocked-by ordering).
	if contexts[0].TaskID != "T-0001" {
		t.Errorf("first context task = %q, want T-0001", contexts[0].TaskID)
	}
	if contexts[1].TaskID != "T-0002" {
		t.Errorf("second context task = %q, want T-0002", contexts[1].TaskID)
	}
}

func TestExtractFileRefs(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want []string
	}{
		{
			name: "single file",
			desc: "Fix internal/parser/parse.go",
			want: []string{"internal/parser/parse.go"},
		},
		{
			name: "multiple files",
			desc: "Check cmd/main.go and internal/core/service.go",
			want: []string{"cmd/main.go", "internal/core/service.go"},
		},
		{
			name: "no files",
			desc: "Just a plain description",
			want: nil,
		},
		{
			name: "deduplicate",
			desc: "Fix internal/foo/bar.go also internal/foo/bar.go",
			want: []string{"internal/foo/bar.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFileRefs(tt.desc)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("file[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSortByBlockedBy(t *testing.T) {
	a := &Task{ID: "T-0001", Meta: nil}
	b := &Task{
		ID:   "T-0002",
		Meta: map[string]interface{}{"blocked_by": []interface{}{"T-0001"}},
	}
	c := &Task{
		ID:   "T-0003",
		Meta: map[string]interface{}{"blocked_by": []interface{}{"T-0002"}},
	}

	// Input in reverse order.
	result := sortByBlockedBy([]*Task{c, b, a})
	if len(result) != 3 {
		t.Fatalf("got %d, want 3", len(result))
	}
	if result[0].ID != "T-0001" {
		t.Errorf("result[0] = %q", result[0].ID)
	}
	if result[1].ID != "T-0002" {
		t.Errorf("result[1] = %q", result[1].ID)
	}
	if result[2].ID != "T-0003" {
		t.Errorf("result[2] = %q", result[2].ID)
	}
}
