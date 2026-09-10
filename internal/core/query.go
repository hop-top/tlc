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

	// Temporal filters on tasks.due_at. See TaskFilter docs and
	// internal/storage/task_counts.go::buildTaskWhereClauses for SQL
	// pushdown semantics.
	DueBefore *time.Time
	DueAfter  *time.Time
	HasDue    *bool

	// Overdue selects tasks past their due date that are still open:
	// due_at < Overdue AND status NOT IN (DONE, SKIPPED). One definition
	// of overdue, expressed in SQL, shared by every caller — the status
	// exclusion cannot merge into the OR-joined Filters status group.
	Overdue *time.Time

	// Blocked, when set, selects tasks by whether blocked_reason is a
	// non-empty string. A column predicate, so it pushes down to SQL.
	Blocked *bool

	// PriorityOrder is the priority vocabulary in rank order, most
	// urgent first, used to make SortBy=="priority" mean urgency
	// instead of alphabet.
	//
	// Carried on the query rather than read by the store because the
	// vocabulary is a CONFIG fact and internal/storage must not depend
	// on config or the CLI. The caller that already knows the
	// configured order passes it down; an empty slice leaves the store
	// on its previous plain column sort, which is what library
	// consumers and the built-in P0..P3 (where alphabet and rank
	// coincide) need.
	//
	// Tasks with no priority always sort last regardless of direction:
	// "not triaged" is not a rank, and a task the user never
	// prioritised must not outrank one they deliberately marked lowest.
	PriorityOrder []string

	// EffortOrder is the effort vocabulary in rank order, smallest
	// first, used to make SortBy=="effort" mean size instead of
	// alphabet.
	//
	// PriorityOrder's counterpart, carried for the same reason: the
	// vocabulary is a CONFIG fact and internal/storage must not depend
	// on config or the CLI. An empty slice leaves the store on its
	// previous plain column sort.
	//
	// Where priority's text sort was right for P0..P3 by accident, this
	// one never was: the built-in XS, S, M, L, XL sorts to L, M, S, XL,
	// XS as text, so effort-ordered listing was wrong even with no
	// config at all.
	//
	// Tasks with no effort always sort last regardless of direction:
	// "not estimated" is not a size, and an unestimated task must not
	// sort ahead of one the user deliberately sized XS.
	EffortOrder []string
}
