package core

import (
	"context"
)

type MockRepository struct {
	Tasks    map[string]*Task
	FlowRuns map[string]*FlowRun
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		Tasks:    make(map[string]*Task),
		FlowRuns: make(map[string]*FlowRun),
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

func (m *MockRepository) DeleteTask(ctx context.Context, id string) error {
	delete(m.Tasks, id)
	return nil
}

func (m *MockRepository) CreateFlowRun(ctx context.Context, run *FlowRun) error {
	m.FlowRuns[run.ID] = run
	return nil
}

func (m *MockRepository) GetFlowRun(ctx context.Context, id string) (*FlowRun, error) {
	return m.FlowRuns[id], nil
}

func (m *MockRepository) UpdateFlowRun(ctx context.Context, run *FlowRun) error {
	m.FlowRuns[run.ID] = run
	return nil
}

type MockLogRepository struct {
	Logs []*LogEntry
}

func NewMockLogRepository() *MockLogRepository {
	return &MockLogRepository{
		Logs: make([]*LogEntry, 0),
	}
}

func (m *MockLogRepository) AddLog(ctx context.Context, entry *LogEntry) error {
	m.Logs = append(m.Logs, entry)
	return nil
}

func (m *MockLogRepository) GetLogs(ctx context.Context, taskID string) ([]*LogEntry, error) {
	var filtered []*LogEntry
	for _, l := range m.Logs {
		if l.TaskID == taskID {
			filtered = append(filtered, l)
		}
	}
	return filtered, nil
}

func (m *MockLogRepository) ListLogs(ctx context.Context, query LogQuery) ([]*LogEntry, error) {
	return m.Logs, nil
}
