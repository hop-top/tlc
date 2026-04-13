package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"hop.top/kit/domain"
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

// ScanTask scans a single sql.Row into a core.Task.
func ScanTask(row *sql.Row) (core.Task, error) {
	var t core.Task
	var createdAt, updatedAt string
	var metaStr, tagsStr, originSys, lastSync sql.NullString
	var projectID, effortStr, priorityStr sql.NullString
	var staleNs sql.NullInt64
	var blockedReason, staleFired, trackID sql.NullString

	err := row.Scan(
		&t.ID, &t.Title, &t.Description, &t.Status,
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

// ScanTaskRows scans a sql.Rows cursor into a core.Task.
func ScanTaskRows(rows *sql.Rows) (core.Task, error) {
	var t core.Task
	var createdAt, updatedAt string
	var metaStr, tagsStr, originSys, lastSync sql.NullString
	var projectID, effortStr, priorityStr sql.NullString
	var staleNs sql.NullInt64
	var blockedReason, staleFired, trackID sql.NullString

	err := rows.Scan(
		&t.ID, &t.Title, &t.Description, &t.Status,
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
		"id", "title", "description", "status", "assigned_to", "reference",
		"created_at", "updated_at", "meta", "tags", "origin_system",
		"last_sync_at", "archived", "project_id", "effort", "priority",
		"stale_timeout", "blocked_reason", "stale_fired_at", "track_id",
	}
	vals = []any{
		t.ID, t.Title, t.Description, string(t.Status), t.AssignedTo,
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
// The id parameter is a compound key (projectID:entityID) or plain entityID.
func (r *TaskDomainRepo) Get(ctx context.Context, id string) (*core.Task, error) {
	projectID, entityID := splitCompoundKey(id)
	if projectID != "" {
		return r.store.GetTaskInProject(ctx, entityID, projectID)
	}
	return r.store.GetTask(ctx, entityID)
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
	_, entityID := splitCompoundKey(id)
	return r.store.DeleteTask(ctx, entityID)
}

// splitCompoundKey splits "projectID:entityID" into parts.
// If no colon is present, returns ("", id).
func splitCompoundKey(id string) (projectID, entityID string) {
	if i := strings.Index(id, ":"); i >= 0 {
		return id[:i], id[i+1:]
	}
	return "", id
}
