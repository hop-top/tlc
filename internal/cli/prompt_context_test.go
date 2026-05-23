package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

func TestTaskPrompt_MarkdownAllFields(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now().UTC()
	track := &core.Track{
		ID:        "auth-system",
		Title:     "Auth System",
		Type:      "feature",
		Status:    core.TrackStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	trackID := seedTrack(t, ctx, s, s, track)

	assignee := "jadb"
	s.CreateTask(ctx, &core.Task{
		ID:        "T-0038",
		Title:     "Add token storage",
		Status:    core.StatusDone,
		CreatedAt: now,
		UpdatedAt: now,
	})
	s.CreateTask(ctx, &core.Task{
		ID:          "T-0042",
		Title:       "Implement JWT refresh endpoint",
		Status:      core.StatusInProgress,
		Description: "Implement the JWT refresh flow\n\n---\n2026-04-03 14:22 · @jadb · CLAIMED",
		AssignedTo:  &assignee,
		Tags:        []string{"type:feat", "domain:auth"},
		Priority:    core.PriorityP1,
		TrackID:     &trackID,
		CreatedAt:   now,
		UpdatedAt:   now,
		Meta: map[string]interface{}{
			"blocked_by": []string{"T-0038"},
		},
	})
	s.CreateTask(ctx, &core.Task{
		ID:        "T-0045",
		Title:     "Session invalidation",
		Status:    core.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
		Meta: map[string]interface{}{
			"blocked_by": []string{"T-0042"},
		},
	})

	s.AddLog(ctx, &core.LogEntry{
		TaskID:    "T-0042",
		Timestamp: now.Add(-24 * time.Hour),
		By:        "exo",
		Action:    "CREATED",
	})
	s.AddLog(ctx, &core.LogEntry{
		TaskID:    "T-0042",
		Timestamp: now,
		By:        "jadb",
		Action:    "CLAIMED",
	})

	cmd := newTestCmd()
	cmd.AddCommand(PromptCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"prompt", "task", "T-0042"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task prompt failed: %v", err)
	}

	output := buf.String()
	t.Logf("Output:\n%s", output)

	// Header.
	if !contains(output, "# T-0042: Implement JWT refresh endpoint") {
		t.Errorf("expected markdown header, got: %s", output)
	}

	// Status line.
	if !contains(output, "Status: IN_PROGRESS") {
		t.Errorf("expected status, got: %s", output)
	}
	if !contains(output, "Assigned: @jadb") {
		t.Errorf("expected assignee, got: %s", output)
	}
	if !contains(output, "Priority: P1") {
		t.Errorf("expected priority, got: %s", output)
	}

	// Tags.
	if !contains(output, "Tags: type:feat, domain:auth") {
		t.Errorf("expected tags, got: %s", output)
	}

	// Track.
	if !contains(output, "Track: auth-system") {
		t.Errorf("expected track info, got: %s", output)
	}

	// Description (should NOT contain audit block).
	if !contains(output, "## Description") {
		t.Errorf("expected description section, got: %s", output)
	}
	if !contains(output, "Implement the JWT refresh flow") {
		t.Errorf("expected description text, got: %s", output)
	}
	if contains(output, "2026-04-03 14:22 · @jadb · CLAIMED\n\n## Description") {
		t.Errorf("audit block should not appear inside description")
	}

	// Dependencies.
	if !contains(output, "## Dependencies") {
		t.Errorf("expected dependencies section, got: %s", output)
	}
	if !contains(output, "blocked-by T-0038") {
		t.Errorf("expected blocked-by ref, got: %s", output)
	}
	if !contains(output, "blocks T-0045") {
		t.Errorf("expected blocking ref, got: %s", output)
	}

	// Recent activity.
	if !contains(output, "## Recent Activity") {
		t.Errorf("expected recent activity section, got: %s", output)
	}
	if !contains(output, "CLAIMED") {
		t.Errorf("expected CLAIMED log entry, got: %s", output)
	}
}

func TestTaskPrompt_MarkdownMinimalFields(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now().UTC()
	s.CreateTask(ctx, &core.Task{
		ID:        "T-0001",
		Title:     "Bare task",
		Status:    core.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
	})

	cmd := newTestCmd()
	cmd.AddCommand(PromptCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"prompt", "task", "T-0001"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task prompt failed: %v", err)
	}

	output := buf.String()
	t.Logf("Output:\n%s", output)

	if !contains(output, "# T-0001: Bare task") {
		t.Errorf("expected header, got: %s", output)
	}
	if !contains(output, "Status: TODO") {
		t.Errorf("expected status, got: %s", output)
	}

	// Should NOT have optional sections.
	if contains(output, "## Dependencies") {
		t.Errorf("should not have dependencies section for task with no deps")
	}
	if contains(output, "Track:") {
		t.Errorf("should not have track line for task with no track")
	}
	if contains(output, "## Recent Activity") {
		t.Errorf("should not have activity section for task with no logs")
	}
}

func TestTaskPrompt_JSONOutput(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	assignee := "alice"
	now := time.Now().UTC()
	s.CreateTask(ctx, &core.Task{
		ID:          "T-0010",
		Title:       "JSON test task",
		Status:      core.StatusInProgress,
		AssignedTo:  &assignee,
		Priority:    core.PriorityP2,
		Tags:        []string{"type:bug"},
		Description: "Fix the bug\n\n---\n2026-04-01 10:00 · @alice · CLAIMED",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	s.AddLog(ctx, &core.LogEntry{
		TaskID:    "T-0010",
		Timestamp: now,
		By:        "alice",
		Action:    "CLAIMED",
	})

	cmd := newTestCmd()
	cmd.AddCommand(PromptCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"prompt", "task", "T-0010", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task prompt --json failed: %v", err)
	}

	output := buf.String()
	t.Logf("JSON output:\n%s", output)

	var parsed promptJSONOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	if parsed.ID != "T-0010" {
		t.Errorf("expected id T-0010, got %s", parsed.ID)
	}
	if parsed.Title != "JSON test task" {
		t.Errorf("expected title, got %s", parsed.Title)
	}
	if parsed.Status != "IN_PROGRESS" {
		t.Errorf("expected IN_PROGRESS, got %s", parsed.Status)
	}
	if parsed.AssignedTo != "alice" {
		t.Errorf("expected assignee alice, got %s", parsed.AssignedTo)
	}
	if parsed.Priority != "P2" {
		t.Errorf("expected P2, got %s", parsed.Priority)
	}
	if parsed.Description != "Fix the bug" {
		t.Errorf("expected clean description, got %q", parsed.Description)
	}
	if len(parsed.RecentLogs) == 0 {
		t.Error("expected at least one log entry")
	} else if parsed.RecentLogs[0].Action != "CLAIMED" {
		t.Errorf("expected CLAIMED action, got %s", parsed.RecentLogs[0].Action)
	}
}

func TestTaskPrompt_NotFound(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	cmd := newTestCmd()
	cmd.AddCommand(PromptCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"prompt", "task", "T-9999"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-existent task, got nil")
	}

	msg := err.Error()
	if !contains(msg, "T-9999") {
		t.Errorf("expected task ID in error message, got: %q", msg)
	}
	if !contains(msg, "tlc task list") {
		t.Errorf("expected actionable hint in error message, got: %q", msg)
	}
}

func TestSplitDescription(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no audit block",
			input: "Just a description",
			want:  "Just a description",
		},
		{
			name:  "with audit block",
			input: "Real description\n\n---\n2026-04-01 · @bob · CREATED",
			want:  "Real description",
		},
		{
			name:  "audit only",
			input: "---\n2026-04-01 · @bob · CREATED",
			want:  "",
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
		{
			name:  "multiple separators",
			input: "Desc text\n\n---\nentry one\n\n---\nentry two",
			want:  "Desc text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitDescription(tt.input)
			if got != tt.want {
				t.Errorf("splitDescription(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
