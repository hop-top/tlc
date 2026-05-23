package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"hop.top/tlc/internal/core"
)

// Compile-time check.
var _ core.AgentRunStore = (*SQLiteStorage)(nil)

// CreateAgentRun inserts a new agent run audit record.
func (s *SQLiteStorage) CreateAgentRun(
	ctx context.Context, r *core.AgentRunRecord,
) error {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()

	_, err := s.db.ExecContext(
		ctx, `
		INSERT INTO agent_runs
			(id, agent, target_type, target_id, job_id, container_id,
			 status, exit_code, error, result_path, started_at, ended_at,
			 created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Agent, r.TargetType, r.TargetID,
		r.JobID, r.ContainerID,
		r.Status, r.ExitCode, r.Error, r.ResultPath,
		r.StartedAt.Format(time.RFC3339),
		formatNullableTime(r.EndedAt),
		r.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("create agent run: %w", err)
	}
	return nil
}

// UpdateAgentRun updates an existing agent run record.
func (s *SQLiteStorage) UpdateAgentRun(
	ctx context.Context, r *core.AgentRunRecord,
) error {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()

	_, err := s.db.ExecContext(
		ctx, `
		UPDATE agent_runs
		SET status = ?, exit_code = ?, error = ?, result_path = ?,
			ended_at = ?, container_id = ?
		WHERE id = ?`,
		r.Status, r.ExitCode, r.Error, r.ResultPath,
		formatNullableTime(r.EndedAt),
		r.ContainerID,
		r.ID,
	)
	if err != nil {
		return fmt.Errorf("update agent run: %w", err)
	}
	return nil
}

// GetAgentRun retrieves an agent run by ID.
func (s *SQLiteStorage) GetAgentRun(
	ctx context.Context, id string,
) (*core.AgentRunRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, agent, target_type, target_id, job_id, container_id,
			status, exit_code, error, result_path, started_at, ended_at,
			created_by
		FROM agent_runs WHERE id = ?`, id)
	return scanAgentRun(row)
}

// ListAgentRuns returns agent runs with optional filters.
func (s *SQLiteStorage) ListAgentRuns(
	ctx context.Context, status string, limit int,
) ([]*core.AgentRunRecord, error) {
	query := `SELECT id, agent, target_type, target_id, job_id,
		container_id, status, exit_code, error, result_path,
		started_at, ended_at, created_by
		FROM agent_runs WHERE 1=1`
	var args []any

	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	query += " ORDER BY started_at DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list agent runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []*core.AgentRunRecord
	for rows.Next() {
		r, err := scanAgentRunRow(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func scanAgentRun(row *sql.Row) (*core.AgentRunRecord, error) {
	var r core.AgentRunRecord
	var jobID, containerID, errStr, resultPath, createdBy sql.NullString
	var endedAt sql.NullString
	var startedAt string

	err := row.Scan(
		&r.ID, &r.Agent, &r.TargetType, &r.TargetID,
		&jobID, &containerID,
		&r.Status, &r.ExitCode, &errStr, &resultPath,
		&startedAt, &endedAt, &createdBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan agent run: %w", err)
	}

	r.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	if jobID.Valid {
		r.JobID = jobID.String
	}
	if containerID.Valid {
		r.ContainerID = containerID.String
	}
	if errStr.Valid {
		r.Error = errStr.String
	}
	if resultPath.Valid {
		r.ResultPath = resultPath.String
	}
	if createdBy.Valid {
		r.CreatedBy = createdBy.String
	}
	if endedAt.Valid {
		t, _ := time.Parse(time.RFC3339, endedAt.String)
		r.EndedAt = &t
	}
	return &r, nil
}

func scanAgentRunRow(rows *sql.Rows) (*core.AgentRunRecord, error) {
	var r core.AgentRunRecord
	var jobID, containerID, errStr, resultPath, createdBy sql.NullString
	var endedAt sql.NullString
	var startedAt string

	err := rows.Scan(
		&r.ID, &r.Agent, &r.TargetType, &r.TargetID,
		&jobID, &containerID,
		&r.Status, &r.ExitCode, &errStr, &resultPath,
		&startedAt, &endedAt, &createdBy,
	)
	if err != nil {
		return nil, fmt.Errorf("scan agent run row: %w", err)
	}

	r.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	if jobID.Valid {
		r.JobID = jobID.String
	}
	if containerID.Valid {
		r.ContainerID = containerID.String
	}
	if errStr.Valid {
		r.Error = errStr.String
	}
	if resultPath.Valid {
		r.ResultPath = resultPath.String
	}
	if createdBy.Valid {
		r.CreatedBy = createdBy.String
	}
	if endedAt.Valid {
		t, _ := time.Parse(time.RFC3339, endedAt.String)
		r.EndedAt = &t
	}
	return &r, nil
}

func formatNullableTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}
