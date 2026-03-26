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

// Effort represents task size estimate: XS, S, M, L, XL.
type Effort string

const (
	EffortXS Effort = "XS"
	EffortS  Effort = "S"
	EffortM  Effort = "M"
	EffortL  Effort = "L"
	EffortXL Effort = "XL"
)

// ValidEffort returns true if e is a recognised effort value (or empty).
func ValidEffort(e Effort) bool {
	switch e {
	case "", EffortXS, EffortS, EffortM, EffortL, EffortXL:
		return true
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

// ValidPriority returns true if p is a recognised priority value (or empty).
func ValidPriority(p Priority) bool {
	switch p {
	case "", PriorityP0, PriorityP1, PriorityP2, PriorityP3:
		return true
	}
	return false
}

type Task struct {
	ID           string                 `json:"id" yaml:"id"`
	Title        string                 `json:"title" yaml:"title"`
	Description  string                 `json:"description,omitempty" yaml:"description,omitempty"`
	Status       TaskStatus             `json:"status" yaml:"status"`
	AssignedTo   *string                `json:"assigned_to" yaml:"assigned_to"`
	Tags         []string               `json:"tags,omitempty" yaml:"tags,omitempty"`
	Reference    string                 `json:"reference" yaml:"reference"`
	Effort       Effort                 `json:"effort,omitempty" yaml:"effort,omitempty"`
	Priority     Priority               `json:"priority,omitempty" yaml:"priority,omitempty"`
	CreatedAt    time.Time              `json:"created_at" yaml:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at" yaml:"updated_at"`
	OriginSystem *string                `json:"origin_system,omitempty" yaml:"origin_system,omitempty"`
	LastSyncAt   *time.Time             `json:"last_sync_at,omitempty" yaml:"last_sync_at,omitempty"`
	Archived     bool                   `json:"archived" yaml:"archived"`
	ProjectID    *string                `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	Meta         map[string]interface{} `json:"meta,omitempty" yaml:"meta,omitempty"`
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
