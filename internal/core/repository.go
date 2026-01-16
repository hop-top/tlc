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
}

// LogRepository defines the interface for audit log persistence.
type LogRepository interface {
	// AddLog appends a new log entry.
	AddLog(ctx context.Context, entry *LogEntry) error
	// GetLogs retrieves all logs for a specific task, sorted by timestamp DESC.
	GetLogs(ctx context.Context, taskID string) ([]*LogEntry, error)
	// ListLogs returns a filtered list of logs across all tasks.
	ListLogs(ctx context.Context, query LogQuery) ([]*LogEntry, error)
}

type LogQuery struct {
	TaskID    string
	Action    string
	By        string
	Limit     int
	Offset    int
	SortOrder string // "asc" or "desc"
}

type TaskFilter struct {
	Status     []TaskStatus
	AssignedTo *string
	Tags       []string
	Search     string
	Offset     int
	Limit      int
}
