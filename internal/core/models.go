package core

import (
	"time"
)

type TaskStatus string

const (
	StatusTodo       TaskStatus = "TODO"
	StatusInProgress TaskStatus = "IN_PROGRESS"
	StatusDone       TaskStatus = "DONE"
	StatusSkipped    TaskStatus = "SKIPPED"
)

// taskStatuses is the BUILT-IN set of task statuses in lifecycle order,
// used when the user's config declares no `task.statuses` of its own.
//
// It is no longer the whole story: `task.statuses` is a documented,
// validated config surface, and a user who declares IN_REVIEW there means
// it for the CLI too, not only for the workflow engine. Consumers that
// render or accept a status vocabulary — validation messages, the flag
// enums, fuzzy normalisation, shell completion — read
// ConfiguredTaskStatusStrings, which falls back to this slice. This slice
// remains the fallback and the compile-time home of the Status* constants
// the code refers to by name.
var taskStatuses = []TaskStatus{
	StatusTodo,
	StatusInProgress,
	StatusDone,
	StatusSkipped,
}

// TaskStatuses returns the closed set of task statuses in lifecycle order.
// The returned slice is a copy; mutating it does not affect the canon.
func TaskStatuses() []TaskStatus {
	return append([]TaskStatus(nil), taskStatuses...)
}

// TaskStatusStrings returns TaskStatuses as plain strings, for consumers
// that render or register the set (error messages, flag enums, completion).
func TaskStatusStrings() []string {
	return enumStrings(taskStatuses)
}

// ConfiguredTaskStatusStrings returns the effective task-status vocabulary:
// the names declared in the user's `task.statuses`, in declared order, or
// the built-in set when config declares none.
//
// Resolved lazily on every call rather than cached in a package-level var,
// because config is read long after package init: a var initialised at
// init time would pin the built-ins forever. It reads through the same
// taskConfigProvider hook DefaultWorkflow uses, so the vocabulary the CLI
// accepts and the vocabulary the workflow enforces cannot disagree —
// internal/core stays free of any dependency on viper or internal/cli.
func ConfiguredTaskStatusStrings() []string {
	cfg := resolveTaskConfig()
	if cfg == nil || len(cfg.Statuses) == 0 {
		return TaskStatusStrings()
	}
	out := make([]string, 0, len(cfg.Statuses))
	for _, s := range cfg.Statuses {
		if s.Name != "" {
			out = append(out, s.Name)
		}
	}
	if len(out) == 0 {
		return TaskStatusStrings()
	}
	return out
}

// ValidTaskStatus reports whether s is a recognized task status (or empty).
func ValidTaskStatus(s TaskStatus) bool {
	if s == "" {
		return true
	}
	for _, v := range taskStatuses {
		if s == v {
			return true
		}
	}
	return false
}

// Effort represents task size estimate: XS, S, M, L, XL.
type Effort string

const (
	EffortXS Effort = "XS"
	EffortS  Effort = "S"
	EffortM  Effort = "M"
	EffortL  Effort = "L"
	EffortXL Effort = "XL"
)

// efforts is the closed set of effort values in ascending size order.
// Same contract as taskStatuses: one declaration, every consumer reads it.
var efforts = []Effort{EffortXS, EffortS, EffortM, EffortL, EffortXL}

// Efforts returns the closed set of effort values in ascending size order.
// The returned slice is a copy.
func Efforts() []Effort {
	return append([]Effort(nil), efforts...)
}

// EffortStrings returns Efforts as plain strings.
func EffortStrings() []string {
	return enumStrings(efforts)
}

// ValidEffort returns true if e is a recognised effort value (or empty).
func ValidEffort(e Effort) bool {
	if e == "" {
		return true
	}
	for _, v := range efforts {
		if e == v {
			return true
		}
	}
	return false
}

// Priority represents task urgency level: P0 (critical) through P3 (low).
type Priority string

const (
	PriorityP0 Priority = "P0"
	PriorityP1 Priority = "P1"
	PriorityP2 Priority = "P2"
	PriorityP3 Priority = "P3"
)

// priorities is the closed set of priorities in descending urgency order.
// Same contract as taskStatuses: one declaration, every consumer reads it.
var priorities = []Priority{PriorityP0, PriorityP1, PriorityP2, PriorityP3}

// Priorities returns the closed set of priorities in descending urgency
// order. The returned slice is a copy.
func Priorities() []Priority {
	return append([]Priority(nil), priorities...)
}

// PriorityStrings returns Priorities as plain strings.
func PriorityStrings() []string {
	return enumStrings(priorities)
}

// ValidPriority returns true if p is a recognised priority value (or empty).
func ValidPriority(p Priority) bool {
	if p == "" {
		return true
	}
	for _, v := range priorities {
		if p == v {
			return true
		}
	}
	return false
}

// enumStrings renders a canonical enum slice as plain strings, preserving
// declaration order. One helper so every enum's string form is derived the
// same way rather than re-typed beside its constants.
func enumStrings[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}

type Task struct {
	ID            string                 `json:"id" yaml:"id" table:"ID"`
	Seq           int64                  `json:"seq" yaml:"seq"`
	Title         string                 `json:"title" yaml:"title" table:"Title"`
	Description   string                 `json:"description,omitempty" yaml:"description,omitempty"`
	Status        TaskStatus             `json:"status" yaml:"status" table:"Status"`
	AssignedTo    *string                `json:"assigned_to" yaml:"assigned_to" table:"Assigned"`
	Tags          []string               `json:"tags,omitempty" yaml:"tags,omitempty"`
	Reference     string                 `json:"reference" yaml:"reference"`
	Effort        Effort                 `json:"effort,omitempty" yaml:"effort,omitempty" table:"Effort"`
	Priority      Priority               `json:"priority,omitempty" yaml:"priority,omitempty" table:"Priority"`
	CreatedAt     time.Time              `json:"created_at" yaml:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at" yaml:"updated_at"`
	OriginSystem  *string                `json:"origin_system,omitempty" yaml:"origin_system,omitempty"`
	LastSyncAt    *time.Time             `json:"last_sync_at,omitempty" yaml:"last_sync_at,omitempty"`
	Archived      bool                   `json:"archived" yaml:"archived"`
	ProjectID     *string                `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	Meta          map[string]interface{} `json:"meta,omitempty" yaml:"meta,omitempty"`
	StaleTimeout  *time.Duration         `json:"stale_timeout,omitempty" yaml:"stale_timeout,omitempty"`
	BlockedReason *string                `json:"blocked_reason,omitempty" yaml:"blocked_reason,omitempty"`
	StaleFiredAt  *time.Time             `json:"stale_fired_at,omitempty" yaml:"stale_fired_at,omitempty"`
	TrackID       *string                `json:"track_id,omitempty" yaml:"track_id,omitempty" table:"Track"`
	DueAt         *time.Time             `json:"due_at,omitempty" yaml:"due_at,omitempty"`
	RemindAt      *time.Time             `json:"remind_at,omitempty" yaml:"remind_at,omitempty"`
	RRule         string                 `json:"rrule,omitempty" yaml:"rrule,omitempty"`
	NoAutoRemind  bool                   `json:"no_auto_remind,omitempty" yaml:"no_auto_remind,omitempty"`
}

type RegisteredProject struct {
	ProjectID    string    `json:"project_id"`
	DBPath       string    `json:"db_path"`
	SpaceURI     string    `json:"space_uri,omitempty"`
	Label        string    `json:"label,omitempty"`
	RegisteredAt time.Time `json:"registered_at"`
	LastSeenAt   time.Time `json:"last_seen_at"`
	Status       string    `json:"status"`
}

type LogEntry struct {
	ID        int64                  `json:"id,omitempty" yaml:"id,omitempty"`
	TaskID    string                 `json:"task_id" yaml:"task_id"`
	Timestamp time.Time              `json:"timestamp" yaml:"timestamp"`
	By        string                 `json:"by" yaml:"by"`
	Action    string                 `json:"action" yaml:"action"`
	Note      string                 `json:"note" yaml:"note"`
	Meta      map[string]interface{} `json:"meta,omitempty" yaml:"meta,omitempty"`
}
