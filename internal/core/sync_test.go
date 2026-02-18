package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTask_NeedsPush(t *testing.T) {
	origin := "github"
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
