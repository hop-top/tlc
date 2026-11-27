package storage

import (
	"fmt"
	"strings"

	"hop.top/tlc/internal/core"
)

// taskFieldNames lists the task columns a caller may sort or filter on,
// in the order help text and rejection messages render them. Every other
// string is rejected before it can reach a statement: identifiers cannot
// be bound as parameters, so an allowlist is the only safe gate.
//
// meta, spec and result are left out on purpose: they hold JSON documents,
// and addressing keys inside them is a separate, validated path (see
// core.ValidateMetaFilterKey), not a column name.
var taskFieldNames = strings.Fields(
	"id seq title description status assigned_to " +
		"reference created_at updated_at tags origin_system " +
		"last_sync_at archived project_id effort priority " +
		"stale_timeout blocked_reason stale_fired_at track_id " +
		"due_at remind_at rrule no_auto_remind kind attempts " +
		"claimed_at run_id step_id step_ordinal")

// taskFieldExprs maps each user-facing field name to the SQL expression it
// stands for. Today every entry is its own column; the indirection is the
// seam a computed field (a key inside meta) plugs into without reopening
// raw interpolation: such a branch emits an expression built from a
// validated key, never the caller's text.
var taskFieldExprs = func() map[string]string {
	m := make(map[string]string, len(taskFieldNames))
	for _, n := range taskFieldNames {
		m[n] = n
	}
	return m
}()

// TaskFieldNames returns the sortable and filterable task fields in
// display order. The returned slice is a copy.
func TaskFieldNames() []string {
	return append([]string(nil), taskFieldNames...)
}

// resolveTaskField maps a caller-supplied field name to its canonical
// name; the SQL expression is taskFieldExprs[canonical]. Matching ignores
// case and surrounding space; ok is false for anything outside the
// allowlist.
func resolveTaskField(name string) (canonical string, ok bool) {
	canonical = strings.ToLower(strings.TrimSpace(name))
	_, ok = taskFieldExprs[canonical]
	return canonical, ok
}

func invalidFieldError(sentinel error, name string) error {
	return fmt.Errorf("%w %q; allowed fields: %s",
		sentinel, name, core.VocabularyList(taskFieldNames))
}

// sortOrderKeyword resolves a sort direction to its SQL keyword. Empty
// means ascending, as it always has; anything but asc or desc is rejected.
func sortOrderKeyword(direction string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "", "asc":
		return sqlOrderASC, nil
	case "desc":
		return sqlOrderDESC, nil
	}
	return "", fmt.Errorf("%w %q; allowed: asc, desc",
		core.ErrInvalidSortDirection, direction)
}

// sortKeys splits a sort spec into its allowlisted keys. A spec is one
// field or a comma-separated list; each is resolved independently and the
// direction applies to every key.
func sortKeys(spec string) ([]string, error) {
	parts := strings.Split(spec, ",")
	keys := make([]string, 0, len(parts))
	for _, p := range parts {
		canonical, ok := resolveTaskField(p)
		if !ok {
			return nil, invalidFieldError(core.ErrInvalidSortField, p)
		}
		keys = append(keys, canonical)
	}
	return keys, nil
}

// ValidateTaskSort reports whether sortBy and direction would be accepted
// by ListTasks. Callers that fan a query out (workspace queries) or open
// resources first use it to reject a bad request up front rather than
// per backend.
func ValidateTaskSort(sortBy, direction string) error {
	if _, err := sortOrderKeyword(direction); err != nil {
		return err
	}
	if sortBy == "" {
		return nil
	}
	_, err := sortKeys(sortBy)
	return err
}

// ValidateTaskQuery reports whether a query's sort key, sort direction and
// filter fields would be accepted by the task reads (ListTasks and the
// counts). It exists for callers that fan one query out to several stores
// and tolerate per-store failures: without an up-front check, a rejected
// field would be reported as every store being unreadable.
func ValidateTaskQuery(query core.Query) error {
	if err := ValidateTaskSort(query.SortBy, query.SortDirection); err != nil {
		return err
	}
	for _, f := range query.Filters {
		if _, ok := resolveTaskField(f.Field); !ok {
			return invalidFieldError(core.ErrInvalidFilterField, f.Field)
		}
	}
	return nil
}

// taskOrderTerms builds the ORDER BY terms for a query: the sort keys
// resolved through the allowlist, each with its bound args. Identifiers
// in the result come only from taskFieldExprs and the status/vocabulary
// helpers, never from the caller's string.
func taskOrderTerms(query core.Query) ([]string, []any, error) {
	order, err := sortOrderKeyword(query.SortDirection)
	if err != nil {
		return nil, nil, err
	}
	if query.SortBy == "" {
		return []string{"created_at DESC"}, nil, nil
	}
	keys, err := sortKeys(query.SortBy)
	if err != nil {
		return nil, nil, err
	}

	var terms []string
	var args []any
	for _, key := range keys {
		if expr, exprArgs := vocabRankOrder(key, query, order); expr != "" {
			terms = append(terms, expr)
			args = append(args, exprArgs...)
		} else if key == "status" {
			terms = append(terms, statusSortExpr()+" "+order)
		} else {
			terms = append(terms, taskFieldExprs[key]+" "+order)
		}
	}
	return terms, args, nil
}
