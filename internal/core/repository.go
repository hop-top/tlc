package core

import (
	"context"
	"time"
)

// Repository defines the interface for task persistence.
type Repository interface {
	// GetNextSequenceID returns the next monotonically-increasing task number for the
	// given project (empty string = global default). Each call increments the counter.
	GetNextSequenceID(ctx context.Context, projectID string) (int, error)
	// CreateTask persists a new task.
	CreateTask(ctx context.Context, task *Task) error
	// GetTask retrieves a task by its ID. Returns nil, nil if not found.
	GetTask(ctx context.Context, id string) (*Task, error)
	// GetTaskBySeq retrieves a task by its (project_id, seq) display alias.
	// projectID may be empty for the global bucket. Returns nil, nil if
	// not found.
	GetTaskBySeq(ctx context.Context, projectID string, seq int64) (*Task, error)
	// UpdateTask updates an existing task.
	UpdateTask(ctx context.Context, task *Task) error
	// UpdateTaskWithLog updates a task and adds a log entry atomically.
	UpdateTaskWithLog(ctx context.Context, task *Task, entry *LogEntry) error
	// ListTasks returns a list of tasks matching the query parameters.
	ListTasks(ctx context.Context, query Query) ([]*Task, error)
	// DeleteTask removes a task by its ID.
	DeleteTask(ctx context.Context, id string) error

	// GetTasksNeedingPush returns tasks that have been modified since their last sync.
	GetTasksNeedingPush(ctx context.Context) ([]*Task, error)

	// FindTaskByOrigin locates a task by its origin system and origin ID.
	FindTaskByOrigin(ctx context.Context, system, originID string) (*Task, error)

	// ArchiveTasks marks tasks as archived if they have been DONE or SKIPPED for longer than the threshold.
	ArchiveTasks(ctx context.Context, threshold time.Duration) (int64, error)

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
	TaskID        string
	Action        string
	By            string
	Limit         int
	Offset        int
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

// TrackRepository defines the interface for track persistence.
type TrackRepository interface {
	// CreateTrack persists a new track.
	CreateTrack(ctx context.Context, track *Track) error
	// GetTrack retrieves a track by its ID. Returns nil, nil if not found.
	GetTrack(ctx context.Context, id string) (*Track, error)
	// GetTrackBySlug retrieves a track by its (project_id, slug) display
	// alias. projectID may be empty. Returns nil, nil if not found.
	GetTrackBySlug(ctx context.Context, projectID, slug string) (*Track, error)
	// UpdateTrack updates an existing track.
	UpdateTrack(ctx context.Context, track *Track) error
	// DeleteTrack removes a track by its ID. Fails if tasks reference this track.
	DeleteTrack(ctx context.Context, id string) error
	// ListTracks returns a list of tracks matching the query parameters.
	ListTracks(ctx context.Context, query TrackQuery) ([]*Track, error)
}

// TrackQuery defines filters for listing tracks.
type TrackQuery struct {
	Status      []TrackStatus
	Type        string
	ProjectID   *string
	Limit       int
	Offset      int
	AllProjects bool // If true, don't filter by current project
}
