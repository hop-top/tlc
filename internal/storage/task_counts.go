package storage

// Task WHERE-fragment construction and the aggregate COUNT queries built on
// it. One fragment, one definition of every predicate: the list query, the
// per-status count, the per-project count, and the scalar count all pass
// through buildTaskWhereClauses, so a filter added here reaches all four and
// cannot drift between them.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"hop.top/tlc/internal/core"
)

// closedStatuses returns the statuses that take a task out of the
// overdue population: a finished task is not late.
//
// Derived from the CONFIGURED vocabulary, not from the built-in
// DONE/SKIPPED constants. The literal it replaces answered for one
// vocabulary only — a config declaring SHIPPED and CANCELED as its
// terminal statuses matched neither, so finished work stayed overdue
// forever, and in all four consumers of the shared WHERE fragment at
// once.
//
// A VALUE rather than a per-row IsTerminal call, because this feeds a
// SQL `status NOT IN (...)` predicate: the exclusion has to be a set of
// names bound as args before any row is read. is_terminal is the same
// property WorkflowManager.IsTerminal reports, read off the same
// definitions, so the SQL and the in-process checks cannot disagree.
//
// Resolved per call rather than cached in a package var, for the reason
// spelled out on core.ConfiguredTaskStatusStrings: a var initialized at
// init time would pin the built-ins before config — and any
// `-c key=value` override — had loaded.
func closedStatuses() []string {
	defs := core.ConfiguredTaskStatusDefinitions()
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		if d.Name != "" && d.IsTerminal {
			out = append(out, d.Name)
		}
	}
	return out
}

// buildTaskWhereClauses assembles the WHERE fragment shared by the task
// list and task count queries: caller filters, full-text search, implicit
// project scoping, archive exclusion, the due_at temporal predicates, the
// overdue compound predicate, and the blocked column predicate.
//
// Pagination (LIMIT/OFFSET) and ORDER BY are deliberately excluded so
// aggregate callers can count the full match set independently of page
// size — see CountTasksByStatus.
func buildTaskWhereClauses(query core.Query) ([]string, []any) {
	whereClauses, args := buildFilterClauses(query.Filters)

	if query.Search != "" {
		whereClauses = append(whereClauses, "(title LIKE ? OR description LIKE ?)")
		args = append(args, "%"+query.Search+"%", "%"+query.Search+"%")
	}

	// Auto-filter by project if in a project context and not explicitly requesting all projects
	if !query.AllProjects {
		if proj := core.DetectProject(); proj != nil && proj.InProject && proj.ProjectID != "" {
			whereClauses = append(whereClauses, "project_id = ?")
			args = append(args, proj.ProjectID)
		}
	}

	if !query.IncludeArchived {
		whereClauses = append(whereClauses, "archived = 0")
	}

	tClauses, tArgs := buildTaskTimeClauses(query)
	whereClauses = append(whereClauses, tClauses...)
	args = append(args, tArgs...)

	rClauses, rArgs := buildTaskRecipeClauses(query)
	whereClauses = append(whereClauses, rClauses...)
	args = append(args, rArgs...)

	return whereClauses, args
}

// buildTaskRecipeClauses renders the recipe-era column predicates: run
// id presence (the executor's strict mode) and claim age (its reclaim
// query). claimed_at is RFC3339 UTC text like due_at, so the string
// compare is chronological — see buildTaskTimeClauses.
func buildTaskRecipeClauses(query core.Query) (clauses []string, args []any) {
	if query.HasRunID != nil {
		if *query.HasRunID {
			clauses = append(clauses, "run_id IS NOT NULL AND run_id != ''")
		} else {
			clauses = append(clauses, "(run_id IS NULL OR run_id = '')")
		}
	}
	if query.ClaimedBefore != nil {
		clauses = append(clauses, "claimed_at IS NOT NULL AND claimed_at < ?")
		args = append(args, query.ClaimedBefore.UTC().Format(time.RFC3339))
	}
	return clauses, args
}

// buildTaskTimeClauses assembles the due_at temporal predicates plus the two
// compound/column predicates that share their shape: overdue and blocked.
// Split from buildTaskWhereClauses to keep each function's branching readable.
func buildTaskTimeClauses(query core.Query) (clauses []string, args []any) {
	// Temporal filters. due_at is stored as RFC3339 UTC TEXT per
	// docs/temporal-spec-0.1.md §4. RFC3339's lexicographic byte
	// ordering matches chronological ordering when all values share the
	// same offset (writes use UTC with the `Z` suffix uniformly — see
	// the INSERT/UPDATE paths in sqlite.go), so a string `<` / `>`
	// against an RFC3339 literal is a correct chronological compare.
	if query.DueBefore != nil {
		clauses = append(clauses, "due_at IS NOT NULL AND due_at < ?")
		args = append(args, query.DueBefore.UTC().Format(time.RFC3339))
	}
	if query.DueAfter != nil {
		clauses = append(clauses, "due_at IS NOT NULL AND due_at > ?")
		args = append(args, query.DueAfter.UTC().Format(time.RFC3339))
	}
	if query.HasDue != nil {
		if *query.HasDue {
			clauses = append(clauses, "due_at IS NOT NULL AND due_at != ''")
		} else {
			clauses = append(clauses, "(due_at IS NULL OR due_at = '')")
		}
	}

	// Overdue is one predicate, not two: the status exclusion travels in
	// its own AND-joined NOT IN rather than through Filters, where it
	// would merge into the OR-joined status group and widen the result
	// set instead of narrowing it.
	if query.Overdue != nil {
		// The terminal set is config-derived, so its length varies:
		// the placeholder count and the args appended below are both
		// driven off this one slice, never off a remembered count.
		closed := closedStatuses()
		placeholders := make([]string, len(closed))
		for i := range closed {
			placeholders[i] = "?"
		}
		// A vocabulary declaring no terminal status excludes nothing.
		// `NOT IN ()` is not valid SQLite, so the exclusion is dropped
		// rather than rendered empty — which is also the right answer:
		// nothing is finished, so nothing leaves the overdue set.
		statusExclusion := ""
		if len(closed) > 0 {
			statusExclusion = fmt.Sprintf(" AND status NOT IN (%s)",
				strings.Join(placeholders, ","))
		}
		clauses = append(clauses,
			"due_at IS NOT NULL AND due_at < ?"+statusExclusion)
		// Args follow placeholder order within the clause: the due_at
		// bound first, then the excluded statuses. Appending the
		// statuses first would bind the timestamp as a status name and
		// a status name as the due_at bound, silently changing which
		// rows match rather than erroring.
		args = append(args, query.Overdue.UTC().Format(time.RFC3339))
		for _, st := range closed {
			args = append(args, st)
		}
	}

	if query.Blocked != nil {
		if *query.Blocked {
			clauses = append(clauses,
				"blocked_reason IS NOT NULL AND blocked_reason != ''")
		} else {
			clauses = append(clauses,
				"(blocked_reason IS NULL OR blocked_reason = '')")
		}
	}

	return clauses, args
}

// taskWhereSuffix renders the WHERE fragment for a task query, or "" when
// the query matches every row.
func taskWhereSuffix(clauses []string) string {
	if len(clauses) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(clauses, " AND ")
}

// CountTasksByStatus returns per-status row counts over the query's full
// match set, ignoring Limit and Offset.
//
// Aggregates must not depend on page size. Counting client-side over the
// slice ListTasks returns yields the count of the page, which is silently
// wrong whenever the match set exceeds the limit. This computes the counts
// in SQL over the same WHERE fragment ListTasks uses, so pagination cannot
// influence the result and no rows are materialized.
//
// Callers applying post-query filters this fragment cannot express
// (--stale, --blocked-by, qualified track IDs) must not use this; count
// the fully filtered slice instead, fetched without a limit.
func (s *SQLiteStorage) CountTasksByStatus(ctx context.Context, query core.Query) (map[string]int, error) {
	whereClauses, args := buildTaskWhereClauses(query)

	//nolint:gosec // G202: whereClauses built from validated field names; values travel as ? args
	sqlQuery := "SELECT status, COUNT(*) FROM tasks" +
		taskWhereSuffix(whereClauses) + " GROUP BY status"

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to count tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, fmt.Errorf("failed to scan task count row: %w", err)
		}
		counts[status] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate task count rows: %w", err)
	}
	return counts, nil
}

// CountTasksByProjectAndStatus returns row counts grouped by project and
// then by status, over the query's full match set, ignoring Limit and
// Offset.
//
// The project dimension is what a grouped summary renders, and it is a
// column, so grouping by it in SQL keeps a cross-project summary on the
// counting path instead of forcing every row to be materialized just to
// bucket it in Go. Tasks with no project land under the empty-string key;
// callers map that to their own display label.
func (s *SQLiteStorage) CountTasksByProjectAndStatus(
	ctx context.Context, query core.Query,
) (map[string]map[string]int, error) {
	whereClauses, args := buildTaskWhereClauses(query)

	//nolint:gosec // G202: whereClauses built from validated field names; values travel as ? args
	sqlQuery := "SELECT COALESCE(project_id, ''), status, COUNT(*) FROM tasks" +
		taskWhereSuffix(whereClauses) + " GROUP BY project_id, status"

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to count tasks by project: %w", err)
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[string]map[string]int)
	for rows.Next() {
		var project, status string
		var n int
		if err := rows.Scan(&project, &status, &n); err != nil {
			return nil, fmt.Errorf("failed to scan task count row: %w", err)
		}
		if counts[project] == nil {
			counts[project] = make(map[string]int)
		}
		counts[project][status] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate task count rows: %w", err)
	}
	return counts, nil
}

// CountTasks returns the number of tasks matching the query.
// Satisfies core.TaskReader.
//
// Built on buildTaskWhereClauses rather than a hand-rolled filter copy, so
// every predicate the list query honors — the temporal ones included —
// narrows this count too.
func (s *SQLiteStorage) CountTasks(ctx context.Context, query core.Query) (int, error) {
	whereClauses, args := buildTaskWhereClauses(query)

	// whereClauses are built from validated field names; values travel as ? args.
	sqlQuery := "SELECT COUNT(*) FROM tasks" + taskWhereSuffix(whereClauses)

	var count int
	if err := s.db.QueryRowContext(ctx, sqlQuery, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count tasks: %w", err)
	}
	return count, nil
}
