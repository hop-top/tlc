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

type Task struct {
	ID          string                 `json:"id" yaml:"id"`
	Title       string                 `json:"title" yaml:"title"`
	Description string                 `json:"description,omitempty" yaml:"description,omitempty"`
	Status      TaskStatus             `json:"status" yaml:"status"`
	AssignedTo  *string                `json:"assigned_to" yaml:"assigned_to"`
	Tags        []string               `json:"tags,omitempty" yaml:"tags,omitempty"`
	Reference   string                 `json:"reference" yaml:"reference"`
	CreatedAt   time.Time              `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" yaml:"updated_at"`
	Meta        map[string]interface{} `json:"meta,omitempty" yaml:"meta,omitempty"`
}

type LogEntry struct {
	ID         int64                  `json:"id,omitempty" yaml:"id,omitempty"`
	TaskID     string                 `json:"task_id" yaml:"task_id"`
	Timestamp  time.Time              `json:"timestamp" yaml:"timestamp"`
	By         string                 `json:"by" yaml:"by"`
	Action     string                 `json:"action" yaml:"action"`
	Note       string                 `json:"note" yaml:"note"`
	Meta       map[string]interface{} `json:"meta,omitempty" yaml:"meta,omitempty"`
}
