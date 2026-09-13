package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"hop.top/kit/go/core/util"
	"hop.top/tlc/internal/core"
)

// Compile-time check: SQLiteStorage is the recipe run ledger.
var _ core.RecipeRunStore = (*SQLiteStorage)(nil)

const recipeRunColumns = "id, project_id, recipe_id, version, hash, vars, subject_type, " +
	"subject_id, track_id, selection, dropped_deps, parent_run, created_by, created_at"

// CreateRecipeRun inserts one materialization record.
func (s *SQLiteStorage) CreateRecipeRun(ctx context.Context, run *core.RecipeRun) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			"INSERT INTO recipe_runs ("+recipeRunColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			run.ID, run.ProjectID, run.RecipeID, run.Version, run.Hash, jsonText(run.Vars),
			optString(run.SubjectType), optString(run.SubjectID), optString(run.TrackID),
			optString(run.Selection), jsonText(run.DroppedDeps), optString(run.ParentRun),
			run.CreatedBy, util.FormatStorageTime(run.CreatedAt),
		)
		if err != nil {
			return fmt.Errorf("failed to insert recipe run %s: %w", run.ID, err)
		}
		return nil
	})
}

// GetRecipeRun returns (nil, nil) when no run has that id.
func (s *SQLiteStorage) GetRecipeRun(ctx context.Context, id string) (*core.RecipeRun, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+recipeRunColumns+" FROM recipe_runs WHERE id = ?", id)
	run, err := scanRecipeRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return run, err
}

// ListRecipeRuns returns runs newest first. Scoping mirrors task
// queries: an explicit ProjectID filters on it; otherwise the detected
// project applies unless AllProjects is set.
func (s *SQLiteStorage) ListRecipeRuns(ctx context.Context, query core.RecipeRunQuery) ([]*core.RecipeRun, error) {
	where, args := recipeRunWhereClauses(query)
	// Constant column list plus placeholder clauses; every value is a bound arg.
	sqlQuery := "SELECT " + recipeRunColumns + " FROM recipe_runs" + taskWhereSuffix(where) + //nolint:gosec // see comment above
		" ORDER BY created_at DESC, id DESC"
	if query.Limit > 0 || query.Offset > 0 {
		// SQLite requires LIMIT before OFFSET; -1 means no limit.
		limit := query.Limit
		if limit <= 0 {
			limit = -1
		}
		sqlQuery += " LIMIT ? OFFSET ?"
		args = append(args, limit, query.Offset)
	}

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query recipe runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var runs []*core.RecipeRun
	for rows.Next() {
		run, err := scanRecipeRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate recipe run rows: %w", err)
	}
	return runs, nil
}

// recipeRunWhereClauses renders a RecipeRunQuery as placeholder clauses
// and bound args. Project scoping mirrors buildTaskWhereClauses.
func recipeRunWhereClauses(query core.RecipeRunQuery) (clauses []string, args []any) {
	add := func(clause string, arg any) {
		clauses = append(clauses, clause)
		args = append(args, arg)
	}
	if query.RecipeID != "" {
		add("recipe_id = ?", query.RecipeID)
	}
	if query.TrackID != "" {
		add("track_id = ?", query.TrackID)
	}
	if query.SubjectType != "" {
		add("subject_type = ?", query.SubjectType)
	}
	if query.SubjectID != "" {
		add("subject_id = ?", query.SubjectID)
	}
	switch {
	case query.ProjectID != "":
		add("project_id = ?", query.ProjectID)
	case !query.AllProjects:
		if proj := core.DetectProject(); proj != nil && proj.InProject && proj.ProjectID != "" {
			add("project_id = ?", proj.ProjectID)
		}
	}
	return clauses, args
}

// DeleteRecipeRun removes a run and its step→task rows. The rows are
// deleted explicitly rather than through the declared cascade, so the
// result does not depend on the connection's foreign_keys pragma.
func (s *SQLiteStorage) DeleteRecipeRun(ctx context.Context, id string) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM recipe_run_tasks WHERE run_id = ?", id); err != nil {
			return fmt.Errorf("failed to delete recipe run tasks of %s: %w", id, err)
		}
		res, err := tx.ExecContext(ctx, "DELETE FROM recipe_runs WHERE id = ?", id)
		if err != nil {
			return fmt.Errorf("failed to delete recipe run %s: %w", id, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to read delete result for recipe run %s: %w", id, err)
		}
		if n == 0 {
			return fmt.Errorf("recipe run %s not found", id)
		}
		return nil
	})
}

// AddRecipeRunTasks records the tasks a run created. (run_id, step_id)
// is the primary key, so recording a step twice fails.
func (s *SQLiteStorage) AddRecipeRunTasks(ctx context.Context, runID string, tasks []core.RecipeRunTask) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		for _, t := range tasks {
			if _, err := tx.ExecContext(
				ctx,
				"INSERT INTO recipe_run_tasks (run_id, step_id, task_id) VALUES (?, ?, ?)",
				runID, t.StepID, t.TaskID,
			); err != nil {
				return fmt.Errorf("failed to record step %s of recipe run %s: %w", t.StepID, runID, err)
			}
		}
		return nil
	})
}

// ListRecipeRunTasks returns a run's rows in insertion order.
func (s *SQLiteStorage) ListRecipeRunTasks(ctx context.Context, runID string) ([]core.RecipeRunTask, error) {
	return s.queryRunTasks(ctx,
		"SELECT run_id, step_id, task_id FROM recipe_run_tasks WHERE run_id = ? ORDER BY rowid", runID)
}

// ListRecipeRunTasksByTrack returns the rows of every run on a track,
// runs oldest first, rows in insertion order.
func (s *SQLiteStorage) ListRecipeRunTasksByTrack(ctx context.Context, trackID string) ([]core.RecipeRunTask, error) {
	return s.queryRunTasks(ctx, `
		SELECT t.run_id, t.step_id, t.task_id
		FROM recipe_run_tasks t
		JOIN recipe_runs r ON r.id = t.run_id
		WHERE r.track_id = ?
		ORDER BY r.created_at, r.id, t.rowid`, trackID)
}

func (s *SQLiteStorage) queryRunTasks(ctx context.Context, sqlQuery string, args ...any) ([]core.RecipeRunTask, error) {
	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query recipe run tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []core.RecipeRunTask
	for rows.Next() {
		var t core.RecipeRunTask
		if err := rows.Scan(&t.RunID, &t.StepID, &t.TaskID); err != nil {
			return nil, fmt.Errorf("failed to scan recipe run task row: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate recipe run task rows: %w", err)
	}
	return out, nil
}

// scanRecipeRun reads one row selected with recipeRunColumns.
// sql.ErrNoRows passes through unwrapped so callers can map it to (nil, nil).
func scanRecipeRun(sc scanner) (*core.RecipeRun, error) {
	var run core.RecipeRun
	var createdAt string
	var vars, subjectType, subjectID, trackID, selection, droppedDeps, parentRun sql.NullString
	err := sc.Scan(
		&run.ID, &run.ProjectID, &run.RecipeID, &run.Version, &run.Hash, &vars,
		&subjectType, &subjectID, &trackID, &selection, &droppedDeps, &parentRun,
		&run.CreatedBy, &createdAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err //nolint:wrapcheck // sentinel passes through so callers map it to (nil, nil)
		}
		return nil, fmt.Errorf("failed to scan recipe run row: %w", err)
	}
	run.CreatedAt = lenientTime(createdAt)
	run.SubjectType = subjectType.String
	run.SubjectID = subjectID.String
	run.TrackID = trackID.String
	run.Selection = selection.String
	run.ParentRun = parentRun.String
	if vars.Valid {
		if err := json.Unmarshal([]byte(vars.String), &run.Vars); err != nil {
			return nil, fmt.Errorf("failed to unmarshal recipe run vars: %w", err)
		}
	}
	if droppedDeps.Valid {
		if err := json.Unmarshal([]byte(droppedDeps.String), &run.DroppedDeps); err != nil {
			return nil, fmt.Errorf("failed to unmarshal recipe run dropped deps: %w", err)
		}
	}
	return &run, nil
}
