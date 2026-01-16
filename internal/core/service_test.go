package core

import (
	"context"
	"testing"
	"time"
)

type mockRepo struct {
	tasks map[string]*Task
	logs  []*LogEntry
}

func (m *mockRepo) CreateTask(ctx context.Context, t *Task) error {
	m.tasks[t.ID] = t
	return nil
}

func (m *mockRepo) GetTask(ctx context.Context, id string) (*Task, error) {
	return m.tasks[id], nil
}

func (m *mockRepo) UpdateTask(ctx context.Context, t *Task) error {
	m.tasks[t.ID] = t
	return nil
}

func (m *mockRepo) UpdateTaskWithLog(ctx context.Context, t *Task, e *LogEntry) error {
	m.tasks[t.ID] = t
	m.logs = append(m.logs, e)
	return nil
}

func (m *mockRepo) ListTasks(ctx context.Context, q Query) ([]*Task, error) {
	return nil, nil
}

func (m *mockRepo) DeleteTask(ctx context.Context, id string) error {
	delete(m.tasks, id)
	return nil
}

func (m *mockRepo) GetTasksNeedingPush(ctx context.Context) ([]*Task, error) {
	var result []*Task
	for _, t := range m.tasks {
		if t.NeedsPush() {
			result = append(result, t)
		}
	}
	return result, nil
}

func (m *mockRepo) FindTaskByOrigin(ctx context.Context, system, originID string) (*Task, error) {
	for _, t := range m.tasks {
		if t.OriginSystem != nil && *t.OriginSystem == system {
			if id, ok := t.Meta["origin_id"].(string); ok && id == originID {
				return t, nil
			}
		}
	}
	return nil, nil
}

func (m *mockRepo) ArchiveTasks(ctx context.Context, threshold time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-threshold)
	var count int64
	for _, t := range m.tasks {
		if !t.Archived && (t.Status == StatusDone || t.Status == StatusSkipped) && t.UpdatedAt.Before(cutoff) {
			t.Archived = true
			count++
		}
	}
	return count, nil
}

func (m *mockRepo) CreateFlowRun(ctx context.Context, run *FlowRun) error {
	return nil
}

func (m *mockRepo) GetFlowRun(ctx context.Context, id string) (*FlowRun, error) {
	return nil, nil
}

func (m *mockRepo) UpdateFlowRun(ctx context.Context, run *FlowRun) error {
	return nil
}

func (m *mockRepo) ListFlowRuns(ctx context.Context, q Query) ([]*FlowRun, error) {
	return nil, nil
}

func (m *mockRepo) AddLog(ctx context.Context, e *LogEntry) error {
	m.logs = append(m.logs, e)
	return nil
}

func (m *mockRepo) GetLogs(ctx context.Context, taskID string, sortDirection string) ([]*LogEntry, error) {
	return m.logs, nil
}

func (m *mockRepo) ListLogs(ctx context.Context, q LogQuery) ([]*LogEntry, error) {
	return m.logs, nil
}

func TestTaskService_Transition(t *testing.T) {
	repo := &mockRepo{tasks: make(map[string]*Task)}
	service := NewTaskService(repo, repo)

	ctx := context.Background()
	task := &Task{
		ID:        "T-1",
		Title:     "Test",
		Status:    StatusTodo,
		CreatedAt: time.Now(),
	}

	service.CreateTask(ctx, task, "user", "initial")

	if len(repo.logs) != 1 {
		t.Errorf("expected 1 log, got %d", len(repo.logs))
	}

	err := service.TransitionStatus(ctx, "T-1", StatusInProgress, "user", "starting")
	if err != nil {
		t.Fatalf("transition failed: %v", err)
	}

	if repo.tasks["T-1"].Status != StatusInProgress {
		t.Errorf("expected status %s, got %s", StatusInProgress, repo.tasks["T-1"].Status)
	}

	if len(repo.logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(repo.logs))
	}

	// Test invalid transition
	err = service.TransitionStatus(ctx, "T-1", StatusTodo, "user", "reverting")
	// StatusInProgress -> StatusTodo IS allowed per spec
	if err != nil {
		t.Errorf("expected transition StatusInProgress -> StatusTodo to be allowed, got error: %v", err)
	}

	// Test forbidden transition: StatusTodo -> StatusDone
	task2 := &Task{ID: "T-2", Status: StatusTodo}
	repo.CreateTask(ctx, task2)
	err = service.TransitionStatus(ctx, "T-2", StatusDone, "user", "shortcut")
	if err == nil {
		t.Error("expected error for TODO -> DONE transition, got nil")
	}
}
