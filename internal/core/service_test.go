package core

import (
	"context"
	"testing"
	"time"
)

type mockRepo struct {
	tasks map[string]*Task
	logs  []*LogEntry
	seq   int
}

func (m *mockRepo) GetNextSequenceID(_ context.Context, _ string) (int, error) {
	m.seq++
	return m.seq, nil
}

func (m *mockRepo) CreateTask(_ context.Context, t *Task) error {
	m.tasks[t.ID] = t
	return nil
}

func (m *mockRepo) GetTask(_ context.Context, id string) (*Task, error) {
	return m.tasks[id], nil
}

func (m *mockRepo) GetTaskBySeq(_ context.Context, projectID string, seq int64) (*Task, error) {
	for _, t := range m.tasks {
		var pid string
		if t.ProjectID != nil {
			pid = *t.ProjectID
		}
		if pid == projectID && t.Seq == seq {
			return t, nil
		}
	}
	return nil, nil
}

func (m *mockRepo) UpdateTask(_ context.Context, t *Task) error {
	m.tasks[t.ID] = t
	return nil
}

func (m *mockRepo) UpdateTaskWithLog(_ context.Context, t *Task, e *LogEntry) error {
	m.tasks[t.ID] = t
	m.logs = append(m.logs, e)
	return nil
}

func (m *mockRepo) ListTasks(_ context.Context, q Query) ([]*Task, error) {
	var tasks []*Task
	for _, t := range m.tasks {
		matches := true
		for _, filter := range q.Filters {
			if !m.applyFilter(t, filter) {
				matches = false
				break
			}
		}
		if matches {
			tasks = append(tasks, t)
		}
	}
	return tasks, nil
}

func (m *mockRepo) applyFilter(task *Task, filter FieldFilter) bool {
	switch filter.Field {
	case "tags":
		tags, ok := filter.Value.(string)
		if !ok {
			return false
		}
		switch filter.Operator {
		case OpContains:
			for _, tag := range task.Tags {
				if tag == tags {
					return true
				}
			}
			return false
		}
		return false
	case "assigned_to":
		switch filter.Operator {
		case OpEq:
			assignee, ok := filter.Value.(string)
			if !ok {
				return task.AssignedTo == nil
			}
			if task.AssignedTo == nil {
				return assignee == ""
			}
			return *task.AssignedTo == assignee
		case OpNotEq:
			assignee, ok := filter.Value.(string)
			if !ok {
				return task.AssignedTo != nil
			}
			if task.AssignedTo == nil {
				return assignee != ""
			}
			return *task.AssignedTo != assignee
		}
		return false
	case "status":
		status, ok := filter.Value.(TaskStatus)
		if !ok {
			return false
		}
		switch filter.Operator {
		case OpEq:
			return task.Status == status
		}
		return false
	}
	return false
}

func (m *mockRepo) DeleteTask(_ context.Context, id string) error {
	delete(m.tasks, id)
	return nil
}

func (m *mockRepo) GetTasksNeedingPush(_ context.Context) ([]*Task, error) {
	var result []*Task
	for _, t := range m.tasks {
		if t.NeedsPush() {
			result = append(result, t)
		}
	}
	return result, nil
}

func (m *mockRepo) FindTaskByOrigin(_ context.Context, system, originID string) (*Task, error) {
	for _, t := range m.tasks {
		if t.OriginSystem != nil && *t.OriginSystem == system {
			if id, ok := t.Meta["origin_id"].(string); ok && id == originID {
				return t, nil
			}
		}
	}
	return nil, nil
}

func (m *mockRepo) ArchiveTasks(_ context.Context, threshold time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-threshold)
	wm := DefaultWorkflow()
	var count int64
	for _, t := range m.tasks {
		if !t.Archived && wm.IsTerminal(t.Status) && t.UpdatedAt.Before(cutoff) {
			t.Archived = true
			count++
		}
	}
	return count, nil
}

func (m *mockRepo) CreateFlowRun(_ context.Context, _ *FlowRun) error {
	return nil
}

func (m *mockRepo) GetFlowRun(_ context.Context, _ string) (*FlowRun, error) {
	return nil, nil
}

func (m *mockRepo) UpdateFlowRun(_ context.Context, _ *FlowRun) error {
	return nil
}

func (m *mockRepo) ListFlowRuns(_ context.Context, _ Query) ([]*FlowRun, error) {
	return nil, nil
}

func (m *mockRepo) AddLog(_ context.Context, e *LogEntry) error {
	m.logs = append(m.logs, e)
	return nil
}

func (m *mockRepo) GetLogs(_ context.Context, _ string, _ string) ([]*LogEntry, error) {
	return m.logs, nil
}

func (m *mockRepo) ListLogs(_ context.Context, _ LogQuery) ([]*LogEntry, error) {
	return m.logs, nil
}

func (m *mockRepo) UpdateLogNote(_ context.Context, logID int64, note string, meta map[string]any) error {
	for _, l := range m.logs {
		if l.ID == logID {
			l.Note = note
			l.Meta = meta
			return nil
		}
	}
	return nil
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

func TestTaskService_UnblockTasks_RemovesCompletedBlocker(t *testing.T) {
	repo := &mockRepo{tasks: make(map[string]*Task)}
	service := NewTaskService(repo, repo)

	repo.tasks["T-1"] = &Task{
		ID:     "T-1",
		Title:  "Blocked task",
		Status: StatusTodo,
		Meta: map[string]interface{}{
			"blocked_by": []string{"T-9", "T-10"},
			"requirements": map[string]interface{}{
				"capabilities": []interface{}{"review"},
			},
		},
	}

	assignee := &Assignee{
		ID: "worker",
		Delegation: &DelegationRules{
			Unblocks: []string{"review"},
		},
	}

	if err := service.UnblockTasks(context.Background(), "T-9", assignee, "worker"); err != nil {
		t.Fatalf("UnblockTasks failed: %v", err)
	}

	blockedBy := repo.tasks["T-1"].BlockedBy()
	if len(blockedBy) != 1 || blockedBy[0] != "T-10" {
		t.Fatalf("blocked_by = %v, want [T-10]", blockedBy)
	}
	if len(repo.logs) != 0 {
		t.Fatalf("expected no unblock log while blockers remain, got %d logs", len(repo.logs))
	}
}

func TestTaskService_UnblockTasks_LegacyStringFullyUnblocks(t *testing.T) {
	repo := &mockRepo{tasks: make(map[string]*Task)}
	service := NewTaskService(repo, repo)

	repo.tasks["T-1"] = &Task{
		ID:     "T-1",
		Title:  "Blocked task",
		Status: StatusTodo,
		Meta: map[string]interface{}{
			"blocked_by": "T-9",
			"requirements": map[string]interface{}{
				"capabilities": []interface{}{"review"},
			},
		},
	}

	assignee := &Assignee{
		ID: "worker",
		Delegation: &DelegationRules{
			Unblocks: []string{"review"},
		},
	}

	if err := service.UnblockTasks(context.Background(), "T-9", assignee, "worker"); err != nil {
		t.Fatalf("UnblockTasks failed: %v", err)
	}

	if len(repo.tasks["T-1"].BlockedBy()) != 0 {
		t.Fatalf("expected blockers to be cleared, got %v", repo.tasks["T-1"].BlockedBy())
	}
	if _, ok := repo.tasks["T-1"].Meta["blocked_by"]; ok {
		t.Fatalf("blocked_by key should be removed, got %v", repo.tasks["T-1"].Meta["blocked_by"])
	}
	if len(repo.logs) != 1 {
		t.Fatalf("expected 1 unblock log, got %d", len(repo.logs))
	}
	if repo.logs[0].Action != ActionUnblocked {
		t.Fatalf("log action = %q, want %q", repo.logs[0].Action, ActionUnblocked)
	}
}
