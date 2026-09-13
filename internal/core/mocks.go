//nolint:revive // Mock implementations don't use all parameters
package core

import (
	"context"
	"fmt"
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
	seqs     map[string]int
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		Tasks:    make(map[string]*Task),
		FlowRuns: make(map[string]*FlowRun),
		logs:     make([]*LogEntry, 0),
		seqs:     make(map[string]int),
	}
}

func (m *MockRepository) GetNextSequenceID(_ context.Context, projectID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if projectID == "" {
		projectID = "default"
	}
	m.seqs[projectID]++
	return m.seqs[projectID], nil
}

func (m *MockRepository) CreateTask(ctx context.Context, task *Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.Tasks[task.ID]; exists {
		return fmt.Errorf("UNIQUE constraint failed: tasks.id")
	}
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

// GetTaskBySeq looks up a task by (project_id, seq) display alias.
func (m *MockRepository) GetTaskBySeq(ctx context.Context, projectID string, seq int64) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, task := range m.Tasks {
		var pid string
		if task.ProjectID != nil {
			pid = *task.ProjectID
		}
		if pid == projectID && task.Seq == seq {
			return task, nil
		}
	}
	return nil, nil
}

func (m *MockRepository) UpdateTask(ctx context.Context, task *Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Tasks[task.ID] = task
	return nil
}

func (m *MockRepository) UpdateTaskWithLog(ctx context.Context, task *Task, entry *LogEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Tasks[task.ID] = task
	if entry != nil {
		m.logs = append(m.logs, entry)
	}
	return nil
}

func (m *MockRepository) ListTasks(ctx context.Context, query Query) ([]*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var tasks []*Task
	for _, t := range m.Tasks {
		if m.matchesQuery(t, query) {
			tasks = append(tasks, t)
		}
	}
	return tasks, nil
}

// matchesQuery applies the field filters plus the column predicates the
// SQL store pushes down (archive exclusion, blocked, run id, claim age),
// so executor tests on the mock exercise the same predicates.
func (m *MockRepository) matchesQuery(t *Task, query Query) bool {
	for _, filter := range query.Filters {
		if !m.applyFilter(t, filter) {
			return false
		}
	}
	if !query.IncludeArchived && t.Archived {
		return false
	}
	if query.Blocked != nil {
		blocked := t.BlockedReason != nil && *t.BlockedReason != ""
		if blocked != *query.Blocked {
			return false
		}
	}
	if query.HasRunID != nil && (t.RunID != "") != *query.HasRunID {
		return false
	}
	if query.ClaimedBefore != nil && (t.ClaimedAt == nil || !t.ClaimedAt.Before(*query.ClaimedBefore)) {
		return false
	}
	return true
}

// ClaimTask mirrors the SQL compare-and-set: one claimant wins, the rest
// see false.
func (m *MockRepository) ClaimTask(ctx context.Context, id, projectID string, from, to TaskStatus, actor string, now time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.Tasks[id]
	if !ok {
		return false, nil
	}
	pid := ""
	if t.ProjectID != nil {
		pid = *t.ProjectID
	}
	if pid != projectID || t.Status != from {
		return false, nil
	}
	if t.AssignedTo != nil && *t.AssignedTo != "" && *t.AssignedTo != actor {
		return false, nil
	}
	owner, claimed := actor, now
	t.Status = to
	t.AssignedTo = &owner
	t.ClaimedAt = &claimed
	t.UpdatedAt = now
	return true, nil
}

func (m *MockRepository) applyFilter(task *Task, filter FieldFilter) bool {
	switch filter.Field {
	case filterKind, filterRunID, filterProjectID:
		val, ok := filter.Value.(string)
		if !ok || filter.Operator != OpEq {
			return false
		}
		switch filter.Field {
		case filterKind:
			return string(task.EffectiveKind()) == val
		case filterRunID:
			return task.RunID == val
		default:
			pid := ""
			if task.ProjectID != nil {
				pid = *task.ProjectID
			}
			return pid == val
		}
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
	case "track_id":
		val, ok := filter.Value.(string)
		if !ok {
			return false
		}
		switch filter.Operator {
		case OpEq:
			return task.TrackID != nil && *task.TrackID == val
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

// UpdateLogNote rewrites the note and meta of an existing log entry by
// ID. Used by `tlc task update --amend` to edit logs in place.
func (m *MockLogRepository) UpdateLogNote(_ context.Context, logID int64, note string, meta map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, l := range m.Logs {
		if l.ID == logID {
			l.Note = note
			l.Meta = meta
			return nil
		}
	}
	return fmt.Errorf("log entry %d not found", logID)
}

// MockAgentRunner is a configurable AgentRunner for unit tests.
// CanHandleFunc defaults to always-true when nil.
// RunFunc defaults to returning an empty output map when nil.
type MockAgentRunner struct {
	CanHandleFunc func(Step) bool
	RunFunc       func(context.Context, Step, string) (map[string]any, error)
	Calls         []Step
	mu            sync.Mutex
}

func (m *MockAgentRunner) CanHandle(step Step) bool {
	if m.CanHandleFunc != nil {
		return m.CanHandleFunc(step)
	}
	return true
}

func (m *MockAgentRunner) Run(ctx context.Context, step Step, prompt string) (map[string]any, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, step)
	m.mu.Unlock()
	if m.RunFunc != nil {
		return m.RunFunc(ctx, step, prompt)
	}
	return map[string]any{"mock": fmt.Sprintf("step:%s", step.ID)}, nil
}
