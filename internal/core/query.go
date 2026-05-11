package core

import "time"

type Operator string

const (
	OpEq       Operator = ":"
	OpEqual    Operator = "="
	OpNotEq    Operator = "!="
	OpGt       Operator = ">"
	OpGte      Operator = ">="
	OpLt       Operator = "<"
	OpLte      Operator = "<="
	OpContains Operator = "~"
	OpNotCont  Operator = "!~"
	OpStart    Operator = "^"
	OpEnd      Operator = "$"
	OpIn       Operator = "IN"
)

type FieldFilter struct {
	Field    string
	Operator Operator
	Value    interface{}
}

type Query struct {
	Filters         []FieldFilter
	Search          string // Full-text search term
	SortBy          string
	SortDirection   string // "asc" or "desc"
	Limit           int
	Offset          int
	IncludeArchived bool
	AllProjects     bool   // If true, don't filter by current project
	StatusPriority  string // Status value to sort first (e.g. "IN_PROGRESS")

	// Temporal filters on tasks.due_at (T-0908). See TaskFilter docs and
	// internal/storage/sqlite.go::ListTasks for SQL pushdown semantics.
	DueBefore *time.Time
	DueAfter  *time.Time
	HasDue    *bool
}
