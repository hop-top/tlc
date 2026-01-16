package core

import (
	"context"
)

type MockRepository struct {
	Tasks map[string]*Task
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		Tasks: make(map[string]*Task),
	}
}

func (m *MockRepository) CreateTask(ctx context.Context, task *Task) error {
	m.Tasks[task.ID] = task
	return nil
}

func (m *MockRepository) GetTask(ctx context.Context, id string) (*Task, error) {
	task, ok := m.Tasks[id]
	if !ok {
		return nil, nil
	}
	return task, nil
}

func (m *MockRepository) UpdateTask(ctx context.Context, task *Task) error {
	m.Tasks[task.ID] = task
	return nil
}

func (m *MockRepository) ListTasks(ctx context.Context, query Query) ([]*Task, error) {
	var tasks []*Task
	for _, t := range m.Tasks {
		tasks = append(tasks, t)
	}
	return tasks, nil
}

type MockLogRepository struct {
	Logs map[string][]*LogEntry
}

func NewMockLogRepository() *MockLogRepository {
	return &MockLogRepository{
		Logs: make(map[string][]*LogEntry),
	}
}

func (m *MockLogRepository) AddLog(ctx context.Context, entry *LogEntry) error {
	m.Logs[entry.TaskID] = append(m.Logs[entry.TaskID], entry)
	return nil
}

func (m *MockLogRepository) GetLogs(ctx context.Context, taskID string) ([]*LogEntry, error) {
	logs, ok := m.Logs[taskID]
	if !ok {
		return nil, nil
	}
	return logs, nil
}

func (m *MockLogRepository) ListLogs(ctx context.Context, query LogQuery) ([]*LogEntry, error) {
	var allLogs []*LogEntry
	for _, logs := range m.Logs {
		allLogs = append(allLogs, logs...)
	}
	// Simple slice for now, could implement filtering if needed for tests
	return allLogs, nil
}
