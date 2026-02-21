//nolint:revive // Mock implementations don't use all parameters
package core

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

type MockRepository struct {
	mu       sync.Mutex
	Tasks    map[string]*Task
	FlowRuns map[string]*FlowRun
	logs     []*LogEntry
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		Tasks:    make(map[string]*Task),
		FlowRuns: make(map[string]*FlowRun),
		logs:     make([]*LogEntry, 0),
	}
}

func (m *MockRepository) CreateTask(ctx context.Context, task *Task) error {
	m.Tasks[task.ID] = task
	return nil
}

func (m *MockRepository) GetTask(ctx context.Context, id string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.Tasks[id]
	if !ok {
		return nil, nil
	}
	return task, nil
}

func (m *MockRepository) UpdateTask(ctx context.Context, task *Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Tasks[task.ID] = task
	return nil
}

func (m *MockRepository) UpdateTaskWithLog(ctx context.Context, task *Task, entry *LogEntry) error {
	m.Tasks[task.ID] = task
	if entry != nil {
		m.logs = append(m.logs, entry)
	}
	return nil
}

func (m *MockRepository) ListTasks(ctx context.Context, query Query) ([]*Task, error) {
	var tasks []*Task
	for _, t := range m.Tasks {
		matches := true
		for _, filter := range query.Filters {
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

func (m *MockRepository) applyFilter(task *Task, filter FieldFilter) bool {
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
		assignee, ok := filter.Value.(string)
		if !ok {
			return false
		}
		switch filter.Operator {
		case OpEq:
			if task.AssignedTo == nil {
				return assignee == ""
			}
			return *task.AssignedTo == assignee
		case OpNotEq:
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

func (m *MockRepository) DeleteTask(ctx context.Context, id string) error {
	delete(m.Tasks, id)
	return nil
}

func (m *MockRepository) GetTasksNeedingPush(ctx context.Context) ([]*Task, error) {
	var result []*Task
	for _, t := range m.Tasks {
		if t.NeedsPush() {
			result = append(result, t)
		}
	}
	return result, nil
}

func (m *MockRepository) FindTaskByOrigin(ctx context.Context, system, originID string) (*Task, error) {
	for _, t := range m.Tasks {
		if t.OriginSystem != nil && *t.OriginSystem == system {
			if id, ok := t.Meta["origin_id"].(string); ok && id == originID {
				return t, nil
			}
		}
	}
	return nil, nil
}

func (m *MockRepository) ArchiveTasks(ctx context.Context, threshold time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-threshold)
	wm := DefaultWorkflow()
	var count int64
	for _, t := range m.Tasks {
		if !t.Archived && wm.IsTerminal(t.Status) && t.UpdatedAt.Before(cutoff) {
			t.Archived = true
			count++
		}
	}
	return count, nil
}

func (m *MockRepository) CreateFlowRun(ctx context.Context, run *FlowRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FlowRuns[run.ID] = run
	return nil
}

func (m *MockRepository) GetFlowRun(ctx context.Context, id string) (*FlowRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.FlowRuns[id], nil
}

func (m *MockRepository) UpdateFlowRun(ctx context.Context, run *FlowRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FlowRuns[run.ID] = run
	return nil
}

func (m *MockRepository) ListFlowRuns(ctx context.Context, query Query) ([]*FlowRun, error) {
	runs := make([]*FlowRun, 0, len(m.FlowRuns))
	for _, r := range m.FlowRuns {
		runs = append(runs, r)
	}
	return runs, nil
}

type MockLogRepository struct {
	mu   sync.Mutex
	Logs []*LogEntry
}

func NewMockLogRepository() *MockLogRepository {
	return &MockLogRepository{
		Logs: make([]*LogEntry, 0),
	}
}

func (m *MockLogRepository) AddLog(ctx context.Context, entry *LogEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Logs = append(m.Logs, entry)
	return nil
}

func (m *MockLogRepository) GetLogs(ctx context.Context, taskID string, sortDirection string) ([]*LogEntry, error) {
	var filtered []*LogEntry
	for _, l := range m.Logs {
		if l.TaskID == taskID {
			filtered = append(filtered, l)
		}
	}
	// Simplified sorting for mock
	if strings.ToUpper(sortDirection) == "ASC" {
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Timestamp.Before(filtered[j].Timestamp)
		})
	} else {
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Timestamp.After(filtered[j].Timestamp)
		})
	}
	return filtered, nil
}

func (m *MockLogRepository) ListLogs(ctx context.Context, query LogQuery) ([]*LogEntry, error) {
	return m.Logs, nil
}
