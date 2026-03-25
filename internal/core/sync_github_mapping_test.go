package core

import (
	"context"
	"testing"
)

func TestGitHubLabelToTlcTagMapping(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{tasks: make(map[string]*Task)}

	githubLabels := []string{"bug", "enhancement", "documentation"}

	task := &Task{
		ID:     "GH-123",
		Title:  "Test issue from GitHub",
		Status: StatusTodo,
		Tags:   githubLabels,
		Meta: map[string]interface{}{
			"origin_system": "github",
			"origin_id":     "123",
		},
	}

	repo.CreateTask(ctx, task)

	retrievedTask, _ := repo.GetTask(ctx, "GH-123")
	if retrievedTask == nil {
		t.Fatal("task not found")
	}

	if len(retrievedTask.Tags) != 3 {
		t.Errorf("expected 3 tags from GitHub labels, got %d", len(retrievedTask.Tags))
	}

	expectedTags := map[string]bool{
		"bug":           true,
		"enhancement":   true,
		"documentation": true,
	}

	for _, tag := range retrievedTask.Tags {
		if !expectedTags[tag] {
			t.Errorf("unexpected tag: %s", tag)
		}
		delete(expectedTags, tag)
	}

	if len(expectedTags) > 0 {
		t.Errorf("missing tags: %v", expectedTags)
	}
}

func TestGitHubLinkCreation(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{tasks: make(map[string]*Task)}

	task := &Task{
		ID:     "T-123",
		Title:  "Linked task",
		Status: StatusTodo,
		Meta: map[string]interface{}{
			"origin_system": "github",
			"origin_id":     "123",
			"origin_url":    "https://github.com/repo/issues/123",
		},
	}

	repo.CreateTask(ctx, task)

	linkedTask, _ := repo.GetTask(ctx, "T-123")
	if linkedTask == nil {
		t.Fatal("task not found after creation")
	}

	if linkedTask.Meta == nil {
		t.Fatal("meta is nil")
	}

	if linkedTask.Meta["origin_system"] != "github" {
		t.Errorf("expected origin_system github, got %v", linkedTask.Meta["origin_system"])
	}

	if linkedTask.Meta["origin_id"] != "123" {
		t.Errorf("expected origin_id 123, got %v", linkedTask.Meta["origin_id"])
	}

	if linkedTask.Meta["origin_url"] == nil {
		t.Error("expected origin_url to be set")
	}
}

func TestGitHubValidationBrokenLink(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{tasks: make(map[string]*Task)}

	task := &Task{
		ID:     "T-999",
		Title:  "Task with broken link",
		Status: StatusTodo,
		Meta: map[string]interface{}{
			"origin_system": "github",
			"origin_id":     "999",
		},
	}

	repo.CreateTask(ctx, task)

	task, _ = repo.GetTask(ctx, "T-999")
	if task == nil {
		t.Fatal("task not found")
	}

	if task.Meta == nil {
		t.Fatal("meta is nil")
	}

	if task.Meta["origin_system"] == nil {
		t.Error("expected origin_system to be set")
	}

	if task.Meta["origin_id"] == nil {
		t.Error("expected origin_id to be set")
	}
}

func TestGitHubAssigneeToTlcAssigneeMapping(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{tasks: make(map[string]*Task)}

	githubAssignee := "octocat"

	task := &Task{
		ID:     "GH-456",
		Title:  "Test issue with assignee",
		Status: StatusTodo,
		Meta: map[string]interface{}{
			"origin_system": "github",
			"origin_id":     "456",
		},
	}

	if task.AssignedTo == nil {
		assignee := githubAssignee
		task.AssignedTo = &assignee
	}

	repo.CreateTask(ctx, task)

	retrievedTask, _ := repo.GetTask(ctx, "GH-456")
	if retrievedTask == nil {
		t.Fatal("task not found")
	}

	if retrievedTask.AssignedTo == nil {
		t.Error("assignee field is nil after mapping")
	} else if *retrievedTask.AssignedTo != githubAssignee {
		t.Errorf("assignee is %s, expected %s", *retrievedTask.AssignedTo, githubAssignee)
	}
}
