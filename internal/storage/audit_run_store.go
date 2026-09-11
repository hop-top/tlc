package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"hop.top/tlc/internal/core"
)

// Compile-time check.
var _ core.AuditRunStore = (*SQLiteStorage)(nil)

// UpsertAuditRun creates or replaces an external-tool audit run keyed
// by (project_id, tool, run_id). Steps are replaced wholesale so a
// re-record of the same run converges on the latest report.
func (s *SQLiteStorage) UpsertAuditRun(ctx context.Context, r *core.AuditRun) error {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()

	var metrics *string
	if len(r.Metrics) > 0 {
		b, err := json.Marshal(r.Metrics)
		if err != nil {
			return fmt.Errorf("marshal audit metrics: %w", err)
		}
		ms := string(b)
		metrics = &ms
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin audit upsert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO audit_runs
			(project_id, tool, run_id, subject, started_at, finished_at,
			 outcome, metrics, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (project_id, tool, run_id) DO UPDATE SET
			subject = excluded.subject,
			started_at = excluded.started_at,
			finished_at = excluded.finished_at,
			outcome = excluded.outcome,
			metrics = excluded.metrics,
			updated_at = excluded.updated_at`,
		r.ProjectID, r.Tool, r.RunID, nullableString(r.Subject),
		r.StartedAt.UTC().Format(time.RFC3339),
		formatNullableTime(r.FinishedAt),
		nullableString(r.Outcome), metrics, now, now,
	)
	if err != nil {
		return fmt.Errorf("upsert audit run: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		DELETE FROM audit_run_steps
		WHERE project_id = ? AND tool = ? AND run_id = ?`,
		r.ProjectID, r.Tool, r.RunID,
	)
	if err != nil {
		return fmt.Errorf("clear audit steps: %w", err)
	}

	for _, st := range r.Steps {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO audit_run_steps
				(project_id, tool, run_id, seq, name, status, detail)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			r.ProjectID, r.Tool, r.RunID,
			st.Seq, st.Name, st.Status, nullableString(st.Detail),
		)
		if err != nil {
			return fmt.Errorf("insert audit step %d: %w", st.Seq, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit audit upsert: %w", err)
	}
	return nil
}

// ListAuditRuns returns audit runs newest-first, steps omitted.
func (s *SQLiteStorage) ListAuditRuns(
	ctx context.Context, q core.AuditRunQuery,
) ([]*core.AuditRun, error) {
	query := `SELECT project_id, tool, run_id, subject, started_at,
		finished_at, outcome, metrics, created_at, updated_at
		FROM audit_runs WHERE 1=1`
	var args []any

	if q.ProjectID != "" {
		query += " AND project_id = ?"
		args = append(args, q.ProjectID)
	}
	if q.Tool != "" {
		query += " AND tool = ?"
		args = append(args, q.Tool)
	}
	if q.Subject != "" {
		query += " AND subject = ?"
		args = append(args, q.Subject)
	}
	query += " ORDER BY started_at DESC, tool, run_id"
	if q.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", q.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list audit runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var runs []*core.AuditRun
	for rows.Next() {
		r, err := scanAuditRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// GetAuditRuns returns every run matching runID, optionally narrowed
// by tool and project, with steps loaded. More than one row means the
// id is ambiguous across tools or projects.
func (s *SQLiteStorage) GetAuditRuns(
	ctx context.Context, runID, tool, projectID string,
) ([]*core.AuditRun, error) {
	query := `SELECT project_id, tool, run_id, subject, started_at,
		finished_at, outcome, metrics, created_at, updated_at
		FROM audit_runs WHERE run_id = ?`
	args := []any{runID}
	if tool != "" {
		query += " AND tool = ?"
		args = append(args, tool)
	}
	if projectID != "" {
		query += " AND project_id = ?"
		args = append(args, projectID)
	}
	query += " ORDER BY tool, project_id"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get audit runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var runs []*core.AuditRun
	for rows.Next() {
		r, err := scanAuditRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, r := range runs {
		steps, err := s.listAuditSteps(ctx, r.ProjectID, r.Tool, r.RunID)
		if err != nil {
			return nil, err
		}
		r.Steps = steps
	}
	return runs, nil
}

func (s *SQLiteStorage) listAuditSteps(
	ctx context.Context, projectID, tool, runID string,
) ([]core.AuditStep, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT seq, name, status, detail
		FROM audit_run_steps
		WHERE project_id = ? AND tool = ? AND run_id = ?
		ORDER BY seq`, projectID, tool, runID)
	if err != nil {
		return nil, fmt.Errorf("list audit steps: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var steps []core.AuditStep
	for rows.Next() {
		var st core.AuditStep
		var detail sql.NullString
		if err := rows.Scan(&st.Seq, &st.Name, &st.Status, &detail); err != nil {
			return nil, fmt.Errorf("scan audit step: %w", err)
		}
		if detail.Valid {
			st.Detail = detail.String
		}
		steps = append(steps, st)
	}
	return steps, rows.Err()
}

func scanAuditRun(rows *sql.Rows) (*core.AuditRun, error) {
	var r core.AuditRun
	var subject, finishedAt, outcome, metrics sql.NullString
	var startedAt, createdAt, updatedAt string

	err := rows.Scan(
		&r.ProjectID, &r.Tool, &r.RunID, &subject, &startedAt,
		&finishedAt, &outcome, &metrics, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan audit run: %w", err)
	}

	r.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if subject.Valid {
		r.Subject = subject.String
	}
	if outcome.Valid {
		r.Outcome = outcome.String
	}
	if finishedAt.Valid {
		t, _ := time.Parse(time.RFC3339, finishedAt.String)
		r.FinishedAt = &t
	}
	if metrics.Valid && metrics.String != "" {
		_ = json.Unmarshal([]byte(metrics.String), &r.Metrics)
	}
	return &r, nil
}

func nullableString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
