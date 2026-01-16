package core

import "context"

// Repository defines the interface for task persistence.
type Repository interface {
	// CreateTask persists a new task.
	CreateTask(ctx context.Context, task *Task) error
	// GetTask retrieves a task by its ID. Returns nil, nil if not found.
	GetTask(ctx context.Context, id string) (*Task, error)
	// UpdateTask updates an existing task.
	UpdateTask(ctx context.Context, task *Task) error
	// ListTasks returns a list of tasks matching the query parameters.
	ListTasks(ctx context.Context, query Query) ([]*Task, error)
	// DeleteTask removes a task by its ID.
	DeleteTask(ctx context.Context, id string) error

	// CreateFlowRun persists a new flow execution instance.
	CreateFlowRun(ctx context.Context, run *FlowRun) error
	// GetFlowRun retrieves a flow run by its ID.
	GetFlowRun(ctx context.Context, id string) (*FlowRun, error)
	// UpdateFlowRun updates an existing flow run.
	UpdateFlowRun(ctx context.Context, run *FlowRun) error
	// ListFlowRuns returns a list of flow runs matching the query.
	ListFlowRuns(ctx context.Context, query Query) ([]*FlowRun, error)
}

// LogRepository defines the interface for audit log persistence.
type LogRepository interface {
	// AddLog appends a new log entry.
	AddLog(ctx context.Context, entry *LogEntry) error
	// GetLogs retrieves all logs for a specific task, with specified order (asc/desc).
	GetLogs(ctx context.Context, taskID string, sortDirection string) ([]*LogEntry, error)
	// ListLogs returns a filtered list of logs across all tasks.
	ListLogs(ctx context.Context, query LogQuery) ([]*LogEntry, error)
}

type LogQuery struct {
	TaskID    string
	Action    string
	By        string
	Limit     int
	Offset    int
	SortDirection string // "asc" or "desc"
}

type TaskFilter struct {
	Status     []TaskStatus
	AssignedTo *string
	Tags       []string
	Search     string
	Offset     int
	Limit      int
}
