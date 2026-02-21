package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	testOriginGitHub = "github"
	testAgent1       = "agent-1"
)

func TestTask_NeedsPush(t *testing.T) {
	origin := testOriginGitHub
	now := time.Now().UTC()

	tests := []struct {
		name         string
		originSystem *string
		updatedAt    time.Time
		lastSyncAt   *time.Time
		expected     bool
	}{
		{
			name:         "No origin system",
			originSystem: nil,
			updatedAt:    now,
			lastSyncAt:   nil,
			expected:     false,
		},
		{
			name:         "Origin system but never synced",
			originSystem: &origin,
			updatedAt:    now,
			lastSyncAt:   nil,
			expected:     true,
		},
		{
			name:         "Synced and not modified",
			originSystem: &origin,
			updatedAt:    now,
			lastSyncAt:   &now,
			expected:     false,
		},
		{
			name:         "Synced and then modified",
			originSystem: &origin,
			updatedAt:    now.Add(time.Minute),
			lastSyncAt:   &now,
			expected:     true,
		},
		{
			name:         "Synced after modification",
			originSystem: &origin,
			updatedAt:    now,
			lastSyncAt:   func() *time.Time { t := now.Add(time.Minute); return &t }(),
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{
				OriginSystem: tt.originSystem,
				UpdatedAt:    tt.updatedAt,
				LastSyncAt:   tt.lastSyncAt,
			}
			assert.Equal(t, tt.expected, task.NeedsPush())
		})
	}
}

func TestTask_NeedsPush_EdgeCases(t *testing.T) {
	github := "github"
	jira := "jira"
	now := time.Now().UTC()

	t.Run("Empty origin system string", func(t *testing.T) {
		empty := ""
		task := &Task{
			OriginSystem: &empty,
			UpdatedAt:    now,
			LastSyncAt:   nil,
		}
		assert.False(t, task.NeedsPush(), "should not need push with empty origin system")
	})

	t.Run("Different origin systems", func(t *testing.T) {
		task := &Task{
			OriginSystem: &jira,
			UpdatedAt:    now.Add(time.Minute),
			LastSyncAt:   &now,
		}
		assert.True(t, task.NeedsPush(), "should need push for any origin system with modifications")
	})

	t.Run("Updated exactly at sync time", func(t *testing.T) {
		syncTime := now
		task := &Task{
			OriginSystem: &github,
			UpdatedAt:    syncTime,
			LastSyncAt:   &syncTime,
		}
		assert.False(t, task.NeedsPush(), "should not need push when updated at sync time")
	})

	t.Run("Updated one millisecond after sync", func(t *testing.T) {
		syncTime := now
		task := &Task{
			OriginSystem: &github,
			UpdatedAt:    syncTime.Add(2 * time.Millisecond),
			LastSyncAt:   &syncTime,
		}
		assert.True(t, task.NeedsPush(), "should need push when updated after sync")
	})

	t.Run("Last sync is in the future", func(t *testing.T) {
		futureSync := now.Add(1 * time.Hour)
		task := &Task{
			OriginSystem: &github,
			UpdatedAt:    now,
			LastSyncAt:   &futureSync,
		}
		assert.False(t, task.NeedsPush(), "should not need push when last sync is in future")
	})

	t.Run("Multiple modifications with sync", func(t *testing.T) {
		secondSync := now.Add(-30 * time.Minute)
		update2 := now.Add(-10 * time.Minute)

		task := &Task{
			OriginSystem: &github,
			UpdatedAt:    update2,
			LastSyncAt:   &secondSync,
		}
		assert.True(t, task.NeedsPush(), "should need push when last update is after last sync")
	})

	t.Run("Updated long before sync", func(t *testing.T) {
		oldUpdate := now.Add(-24 * time.Hour)
		recentSync := now.Add(-1 * time.Hour)

		task := &Task{
			OriginSystem: &github,
			UpdatedAt:    oldUpdate,
			LastSyncAt:   &recentSync,
		}
		assert.False(t, task.NeedsPush(), "should not need push when updated before last sync")
	})
}

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

func TestGitHubSyncTaskUpdate(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{tasks: make(map[string]*Task)}
	service := NewTaskService(repo, repo)

	task := &Task{
		ID:     "T-456",
		Title:  "Task to sync",
		Status: StatusInProgress,
		Meta: map[string]interface{}{
			"origin_system": "github",
			"origin_id":     "456",
			"needs_push":    true,
		},
	}

	repo.CreateTask(ctx, task)

	task.Title = "Updated title"
	err := service.UpdateTask(ctx, task, "test-user", "syncing update")
	if err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	updatedTask, _ := repo.GetTask(ctx, "T-456")
	if updatedTask == nil {
		t.Fatal("task not found after update")
	}

	if updatedTask.Title != "Updated title" {
		t.Errorf("expected title 'Updated title', got %s", updatedTask.Title)
	}

	needsPush, ok := updatedTask.Meta["needs_push"]
	if !ok || needsPush != true {
		t.Error("expected needs_push flag to be true after update")
	}
}

func TestGitHubSyncTaskDelete(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{tasks: make(map[string]*Task)}

	task := &Task{
		ID:     "T-789",
		Title:  "Task to delete and sync",
		Status: StatusDone,
		Meta: map[string]interface{}{
			"origin_system": "github",
			"origin_id":     "789",
		},
	}

	repo.CreateTask(ctx, task)

	err := repo.DeleteTask(ctx, "T-789")
	if err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}

	deletedTask, _ := repo.GetTask(ctx, "T-789")
	if deletedTask != nil {
		t.Error("expected task to be deleted from local storage")
	}
}

func TestGitHubBatchSync(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{tasks: make(map[string]*Task)}

	tasks := []*Task{
		{
			ID:     "T-1",
			Title:  "Task 1",
			Status: StatusTodo,
			Meta: map[string]interface{}{
				"origin_system": "github",
				"origin_id":     "1",
			},
		},
		{
			ID:     "T-2",
			Title:  "Task 2",
			Status: StatusTodo,
			Meta: map[string]interface{}{
				"origin_system": "github",
				"origin_id":     "2",
			},
		},
	}

	for _, task := range tasks {
		repo.CreateTask(ctx, task)
	}

	allTasks, _ := repo.ListTasks(ctx, Query{})
	if len(allTasks) < 2 {
		t.Fatal("expected at least 2 tasks for batch sync")
	}

	githubTasks := 0
	for _, task := range allTasks {
		if task.Meta != nil {
			if origin, ok := task.Meta["origin_system"].(string); ok {
				if origin == "github" {
					githubTasks++
				}
			}
		}
	}

	if githubTasks != 2 {
		t.Errorf("expected 2 GitHub tasks, got %d", githubTasks)
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

func TestClaimSyncToOrigin(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{tasks: make(map[string]*Task)}
	service := NewTaskService(repo, repo)

	assignee := testAgent1
	task := &Task{
		ID:         "T-101",
		Title:      "Task to claim and sync",
		Status:     StatusTodo,
		AssignedTo: &assignee,
		Meta: map[string]interface{}{
			"origin_system": testOriginGitHub,
			"origin_id":     "101",
		},
	}

	repo.CreateTask(ctx, task)

	err := service.ClaimTask(ctx, "T-101", assignee, "claiming for sync")
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}

	claimedTask, _ := repo.GetTask(ctx, "T-101")
	if claimedTask == nil {
		t.Fatal("task not found after claim")
	}

	if claimedTask.Status != StatusInProgress {
		t.Errorf("expected status IN_PROGRESS, got %s", claimedTask.Status)
	}

	needsPush, ok := claimedTask.Meta["needs_push"]
	if !ok || needsPush != true {
		t.Error("expected needs_push flag to be true after claim")
	}
}

func TestUnclaimSyncToOrigin(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{tasks: make(map[string]*Task)}
	service := NewTaskService(repo, repo)

	assignee := "agent-1"
	task := &Task{
		ID:         "T-102",
		Title:      "Task to unclaim and sync",
		Status:     StatusInProgress,
		AssignedTo: &assignee,
		Meta: map[string]interface{}{
			"origin_system": "github",
			"origin_id":     "102",
		},
	}

	repo.CreateTask(ctx, task)

	err := service.UnclaimTask(ctx, "T-102", assignee, "unclaiming for sync")
	if err != nil {
		t.Fatalf("UnclaimTask failed: %v", err)
	}

	unclaimedTask, _ := repo.GetTask(ctx, "T-102")
	if unclaimedTask == nil {
		t.Fatal("task not found after unclaim")
	}

	if unclaimedTask.Status != StatusTodo {
		t.Errorf("expected status TODO, got %s", unclaimedTask.Status)
	}

	if unclaimedTask.AssignedTo != nil {
		t.Error("expected assignee to be nil after unclaim")
	}

	needsPush, ok := unclaimedTask.Meta["needs_push"]
	if !ok || needsPush != true {
		t.Error("expected needs_push flag to be true after unclaim")
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

func TestGitHubSyncLabelUpdate(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{tasks: make(map[string]*Task)}
	service := NewTaskService(repo, repo)

	task := &Task{
		ID:     "GH-789",
		Title:  "Issue for label sync",
		Status: StatusTodo,
		Tags:   []string{"bug", "urgent"},
		Meta: map[string]interface{}{
			"origin_system": "github",
			"origin_id":     "789",
		},
	}

	repo.CreateTask(ctx, task)

	updatedTask, _ := repo.GetTask(ctx, "GH-789")
	if updatedTask == nil {
		t.Fatal("task not found")
	}

	updatedTask.Tags = []string{"bug", "fixed", "verified"}
	err := service.UpdateTask(ctx, updatedTask, "github-sync", "labels updated from GitHub")
	if err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	finalTask, _ := repo.GetTask(ctx, "GH-789")
	if len(finalTask.Tags) != 3 {
		t.Errorf("expected 3 tags after GitHub sync, got %d", len(finalTask.Tags))
	}

	expectedTags := map[string]bool{
		"bug":      true,
		"fixed":    true,
		"verified": true,
	}

	for _, tag := range finalTask.Tags {
		if !expectedTags[tag] {
			t.Errorf("unexpected tag: %s", tag)
		}
		delete(expectedTags, tag)
	}
}

func TestGitHubSyncAssigneeUpdate(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{tasks: make(map[string]*Task)}
	service := NewTaskService(repo, repo)

	assignee1 := "octocat"
	task := &Task{
		ID:         "GH-999",
		Title:      "Issue for assignee sync",
		Status:     StatusTodo,
		AssignedTo: &assignee1,
		Meta: map[string]interface{}{
			"origin_system": "github",
			"origin_id":     "999",
		},
	}

	repo.CreateTask(ctx, task)

	updatedTask, _ := repo.GetTask(ctx, "GH-999")
	if updatedTask == nil {
		t.Fatal("task not found")
	}

	assignee2 := "newcontributor"
	updatedTask.AssignedTo = &assignee2
	err := service.UpdateTask(ctx, updatedTask, "github-sync", "assignee updated from GitHub")
	if err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	finalTask, _ := repo.GetTask(ctx, "GH-999")
	if finalTask.AssignedTo == nil {
		t.Error("assignee field is nil after GitHub sync")
	} else if *finalTask.AssignedTo != assignee2 {
		t.Errorf("assignee is %s, expected %s", *finalTask.AssignedTo, assignee2)
	}
}
