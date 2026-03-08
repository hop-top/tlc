package core

import "context"

// TaskReader provides read-only access to task data.
// Used for cross-workspace queries where databases are opened read-only.
type TaskReader interface {
	// ListTasks returns tasks matching the query.
	ListTasks(ctx context.Context, query Query) ([]*Task, error)
	// GetTask returns a single task by ID.
	GetTask(ctx context.Context, id string) (*Task, error)
	// GetTaskLogs returns log entries for a task.
	GetTaskLogs(ctx context.Context, taskID string) ([]*LogEntry, error)
	// CountTasks returns the number of tasks matching the query.
	CountTasks(ctx context.Context, query Query) (int, error)
}
