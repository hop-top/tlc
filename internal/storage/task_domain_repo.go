package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"hop.top/kit/go/runtime/domain"
	"hop.top/tlc/internal/core"
)

// TaskDomainRepo adapts SQLiteStorage to domain.Repository[core.Task].
type TaskDomainRepo struct {
	store *SQLiteStorage
}

// Compile-time assertion: TaskDomainRepo implements domain.Repository[core.Task].
var _ domain.Repository[core.Task] = (*TaskDomainRepo)(nil)

// NewTaskDomainRepo wraps a SQLiteStorage as a domain.Repository[core.Task].
func NewTaskDomainRepo(store *SQLiteStorage) *TaskDomainRepo {
	return &TaskDomainRepo{store: store}
}

// scanner abstracts sql.Row and sql.Rows behind a single Scan method.
type scanner interface {
	Scan(dest ...any) error
}

// scanTaskFields scans a task row using any scanner (sql.Row or sql.Rows).
func scanTaskFields(s scanner) (core.Task, error) {
	var t core.Task
	var createdAt, updatedAt string
	var metaStr, tagsStr, originSys, lastSync sql.NullString
	var projectID, effortStr, priorityStr sql.NullString
	var staleNs sql.NullInt64
	var blockedReason, staleFired, trackID sql.NullString

	err := s.Scan(
		&t.ID, &t.Seq, &t.Title, &t.Description, &t.Status,
		&t.AssignedTo, &t.Reference, &createdAt, &updatedAt,
		&metaStr, &tagsStr, &originSys, &lastSync,
		&t.Archived, &projectID, &effortStr, &priorityStr,
		&staleNs, &blockedReason, &staleFired, &trackID,
	)
	if err != nil {
		return t, err
	}
	populateTask(&t, createdAt, updatedAt, metaStr, tagsStr, originSys,
		lastSync, projectID, effortStr, priorityStr, staleNs,
		blockedReason, staleFired, trackID)
	return t, nil
}

// ScanTask scans a single sql.Row into a core.Task.
func ScanTask(row *sql.Row) (core.Task, error) {
	return scanTaskFields(row)
}

// ScanTaskRows scans a sql.Rows cursor into a core.Task.
func ScanTaskRows(rows *sql.Rows) (core.Task, error) {
	return scanTaskFields(rows)
}

// BindTask returns column names and values for a Task, suitable for
// INSERT/UPDATE statements.
func BindTask(t core.Task) (cols []string, vals []any) {
	metaJSON, _ := json.Marshal(t.Meta)
	tagsJSON, _ := json.Marshal(t.Tags)

	var lastSync *string
	if t.LastSyncAt != nil {
		s := t.LastSyncAt.Format(time.RFC3339)
		lastSync = &s
	}
	var staleNs *int64
	if t.StaleTimeout != nil {
		ns := int64(*t.StaleTimeout)
		staleNs = &ns
	}
	var staleFired *string
	if t.StaleFiredAt != nil {
		s := t.StaleFiredAt.Format(time.RFC3339)
		staleFired = &s
	}
	var pid string
	if t.ProjectID != nil {
		pid = *t.ProjectID
	}

	cols = []string{
		"id", "seq", "title", "description", "status", "assigned_to", "reference",
		"created_at", "updated_at", "meta", "tags", "origin_system",
		"last_sync_at", "archived", "project_id", "effort", "priority",
		"stale_timeout", "blocked_reason", "stale_fired_at", "track_id",
	}
	vals = []any{
		t.ID, t.Seq, t.Title, t.Description, string(t.Status), t.AssignedTo,
		t.Reference, t.CreatedAt.Format(time.RFC3339),
		t.UpdatedAt.Format(time.RFC3339), string(metaJSON), string(tagsJSON),
		t.OriginSystem, lastSync, t.Archived, pid, string(t.Effort),
		string(t.Priority), staleNs, t.BlockedReason, staleFired, t.TrackID,
	}
	return cols, vals
}

// populateTask fills computed fields from scanned nullable values.
func populateTask(
	t *core.Task,
	createdAt, updatedAt string,
	metaStr, tagsStr, originSys, lastSync sql.NullString,
	projectID, effortStr, priorityStr sql.NullString,
	staleNs sql.NullInt64,
	blockedReason, staleFired, trackID sql.NullString,
) {
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if metaStr.Valid {
		_ = json.Unmarshal([]byte(metaStr.String), &t.Meta)
	}
	if tagsStr.Valid {
		_ = json.Unmarshal([]byte(tagsStr.String), &t.Tags)
	}
	if originSys.Valid {
		t.OriginSystem = &originSys.String
	}
	if lastSync.Valid {
		ts, _ := time.Parse(time.RFC3339, lastSync.String)
		t.LastSyncAt = &ts
	}
	if projectID.Valid {
		t.ProjectID = &projectID.String
	}
	if effortStr.Valid {
		t.Effort = core.Effort(effortStr.String)
	}
	if priorityStr.Valid {
		t.Priority = core.Priority(priorityStr.String)
	}
	if staleNs.Valid {
		d := time.Duration(staleNs.Int64)
		t.StaleTimeout = &d
	}
	if blockedReason.Valid {
		t.BlockedReason = &blockedReason.String
	}
	if staleFired.Valid {
		ts, _ := time.Parse(time.RFC3339, staleFired.String)
		t.StaleFiredAt = &ts
	}
	if trackID.Valid {
		t.TrackID = &trackID.String
	}
}

// Create implements domain.Repository[core.Task].
func (r *TaskDomainRepo) Create(ctx context.Context, task *core.Task) error {
	return r.store.CreateTask(ctx, task)
}

// Get implements domain.Repository[core.Task].
func (r *TaskDomainRepo) Get(ctx context.Context, id string) (*core.Task, error) {
	return r.store.GetTask(ctx, id)
}

// List implements domain.Repository[core.Task] with basic Query support.
// For rich filtering (status, tags, assignee), use ListTasks directly.
func (r *TaskDomainRepo) List(ctx context.Context, q domain.Query) ([]core.Task, error) {
	query := core.Query{
		Limit:  q.Limit,
		Offset: q.Offset,
	}
	if q.Search != "" {
		query.Search = q.Search
	}
	if q.Sort != "" {
		query.SortBy = q.Sort
	}
	tasks, err := r.store.ListTasks(ctx, query)
	if err != nil {
		return nil, err
	}
	result := make([]core.Task, len(tasks))
	for i, t := range tasks {
		result[i] = *t
	}
	return result, nil
}

// Update implements domain.Repository[core.Task].
func (r *TaskDomainRepo) Update(ctx context.Context, task *core.Task) error {
	return r.store.UpdateTask(ctx, task)
}

// Delete implements domain.Repository[core.Task].
func (r *TaskDomainRepo) Delete(ctx context.Context, id string) error {
	return r.store.DeleteTask(ctx, id)
}
