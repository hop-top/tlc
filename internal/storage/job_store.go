package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"hop.top/tlc/internal/core"
)

// Compile-time check.
var _ core.JobStore = (*SQLiteStorage)(nil)

// CreateJob inserts a new job record.
func (s *SQLiteStorage) CreateJob(ctx context.Context, job *core.Job) error {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO jobs (id, queue, type, status, payload, created_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Queue, string(job.Type), string(job.Status),
		job.Payload, job.CreatedAt.Format(time.RFC3339),
		job.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("create job: %w", err)
	}
	return nil
}

// UpdateJob persists changes to an existing job.
func (s *SQLiteStorage) UpdateJob(ctx context.Context, job *core.Job) error {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()

	var startedAt, endedAt *string
	if job.StartedAt != nil {
		v := job.StartedAt.Format(time.RFC3339)
		startedAt = &v
	}
	if job.EndedAt != nil {
		v := job.EndedAt.Format(time.RFC3339)
		endedAt = &v
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE jobs SET status = ?, result = ?, error = ?,
			started_at = ?, ended_at = ?
		WHERE id = ?`,
		string(job.Status), job.Result, job.Error,
		startedAt, endedAt, job.ID,
	)
	if err != nil {
		return fmt.Errorf("update job: %w", err)
	}
	return nil
}

// GetJob retrieves a job by ID.
func (s *SQLiteStorage) GetJob(ctx context.Context, id string) (*core.Job, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, queue, type, status, payload, result, error,
			created_at, started_at, ended_at, created_by
		FROM jobs WHERE id = ?`, id)

	return scanJob(row)
}

// ListJobs returns jobs filtered by queue and status.
func (s *SQLiteStorage) ListJobs(
	ctx context.Context, queue string, status core.JobStatus, limit int,
) ([]*core.Job, error) {
	query := "SELECT id, queue, type, status, payload, result, error, created_at, started_at, ended_at, created_by FROM jobs WHERE 1=1"
	var args []any

	if queue != "" {
		query += " AND queue = ?"
		args = append(args, queue)
	}
	if status != "" {
		query += " AND status = ?"
		args = append(args, string(status))
	}
	query += " ORDER BY created_at DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var jobs []*core.Job
	for rows.Next() {
		job, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// ClaimNextJob atomically picks the oldest queued job and marks it running.
func (s *SQLiteStorage) ClaimNextJob(ctx context.Context, queue string) (*core.Job, error) {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `
		UPDATE jobs SET status = 'running', started_at = ?
		WHERE id = (
			SELECT id FROM jobs
			WHERE queue = ? AND status = 'queued'
			ORDER BY created_at ASC
			LIMIT 1
		)`, now, queue)
	if err != nil {
		return nil, fmt.Errorf("claim job: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return nil, nil
	}

	// Fetch the claimed job.
	row := s.db.QueryRowContext(ctx, `
		SELECT id, queue, type, status, payload, result, error,
			created_at, started_at, ended_at, created_by
		FROM jobs WHERE queue = ? AND status = 'running'
		ORDER BY started_at DESC LIMIT 1`, queue)
	return scanJob(row)
}

// CancelJob marks a queued job as canceled.
func (s *SQLiteStorage) CancelJob(ctx context.Context, id string) error {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()

	result, err := s.db.ExecContext(ctx, `
		UPDATE jobs SET status = 'canceled', ended_at = ?
		WHERE id = ? AND status = 'queued'`,
		time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("cancel job: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf(
			"job %s not found or not in queued state; "+
				"only queued jobs can be canceled",
			id,
		)
	}
	return nil
}

func scanJob(row *sql.Row) (*core.Job, error) {
	var j core.Job
	var typ, status string
	var createdAt string
	var startedAt, endedAt, result, errStr, createdBy sql.NullString

	err := row.Scan(
		&j.ID, &j.Queue, &typ, &status, &j.Payload,
		&result, &errStr,
		&createdAt, &startedAt, &endedAt, &createdBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan job: %w", err)
	}

	j.Type = core.JobType(typ)
	j.Status = core.JobStatus(status)
	j.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if result.Valid {
		j.Result = result.String
	}
	if errStr.Valid {
		j.Error = errStr.String
	}
	if createdBy.Valid {
		j.CreatedBy = createdBy.String
	}
	if startedAt.Valid {
		t, _ := time.Parse(time.RFC3339, startedAt.String)
		j.StartedAt = &t
	}
	if endedAt.Valid {
		t, _ := time.Parse(time.RFC3339, endedAt.String)
		j.EndedAt = &t
	}
	return &j, nil
}

func scanJobRow(rows *sql.Rows) (*core.Job, error) {
	var j core.Job
	var typ, status string
	var createdAt string
	var startedAt, endedAt, result, errStr, createdBy sql.NullString

	err := rows.Scan(
		&j.ID, &j.Queue, &typ, &status, &j.Payload,
		&result, &errStr,
		&createdAt, &startedAt, &endedAt, &createdBy,
	)
	if err != nil {
		return nil, fmt.Errorf("scan job row: %w", err)
	}

	j.Type = core.JobType(typ)
	j.Status = core.JobStatus(status)
	j.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if result.Valid {
		j.Result = result.String
	}
	if errStr.Valid {
		j.Error = errStr.String
	}
	if createdBy.Valid {
		j.CreatedBy = createdBy.String
	}
	if startedAt.Valid {
		t, _ := time.Parse(time.RFC3339, startedAt.String)
		j.StartedAt = &t
	}
	if endedAt.Valid {
		t, _ := time.Parse(time.RFC3339, endedAt.String)
		j.EndedAt = &t
	}
	return &j, nil
}
