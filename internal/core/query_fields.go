package core

import "errors"

// Sentinels for query inputs that name a field or direction the store
// does not offer. Callers match them with errors.Is to tell a rejected
// request from a failed query.
var (
	// ErrInvalidSortField marks a sort key outside the sortable set.
	ErrInvalidSortField = errors.New("invalid sort field")
	// ErrInvalidSortDirection marks a sort direction other than asc or desc.
	ErrInvalidSortDirection = errors.New("invalid sort direction")
	// ErrInvalidFilterField marks a filter on a field outside the
	// filterable set.
	ErrInvalidFilterField = errors.New("invalid filter field")
)
