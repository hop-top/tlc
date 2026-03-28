package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"hop.top/tlc/internal/core"
	_ "modernc.org/sqlite"
)

// Compile-time check: SQLiteStorage implements core.TaskReader.
var _ core.TaskReader = (*SQLiteStorage)(nil)

const (
	sqlOpLike    = "LIKE"
	sqlOrderASC  = "ASC"
	sqlOrderDESC = "DESC"
	sqlLimit     = " LIMIT ?"
)

type SQLiteStorage struct {
	db        *sql.DB
	writeLock sync.Mutex
}

func NewSQLiteStorage(path string) (*SQLiteStorage, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}

	// Enable foreign keys
	if _, err := db.ExecContext(context.Background(), "PRAGMA foreign_keys = ON;"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Enable WAL mode and set busy timeout for better concurrency
	if _, err := db.ExecContext(context.Background(), "PRAGMA journal_mode = WAL;"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}
	if _, err := db.ExecContext(context.Background(), "PRAGMA busy_timeout = 5000;"); err != nil {
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	s := &SQLiteStorage{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate db: %w", err)
	}

	return s, nil
}

func (s *SQLiteStorage) withWriteTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback failed after error %v: %w", err, rbErr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) CreateTask(ctx context.Context, task *core.Task) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		metaJSON, _ := json.Marshal(task.Meta)
		tagsJSON, _ := json.Marshal(task.Tags)

		var lastSyncAt *string
		if task.LastSyncAt != nil {
			s := task.LastSyncAt.Format(time.RFC3339)
			lastSyncAt = &s
		}

		var staleTimeout *int64
		if task.StaleTimeout != nil {
			ns := int64(*task.StaleTimeout)
			staleTimeout = &ns
		}
		var staleFiredAt *string
		if task.StaleFiredAt != nil {
			s := task.StaleFiredAt.Format(time.RFC3339)
			staleFiredAt = &s
		}

		// Coalesce nil ProjectID to empty string to prevent NULL composite PK duplication
		var projectID string
		if task.ProjectID != nil {
			projectID = *task.ProjectID
		}

		_, err := tx.ExecContext(ctx, `
			INSERT INTO tasks (id, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id, effort, priority, stale_timeout, blocked_reason, stale_fired_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			task.ID, task.Title, task.Description, task.Status, task.AssignedTo, task.Reference,
			task.CreatedAt.Format(time.RFC3339), task.UpdatedAt.Format(time.RFC3339),
			string(metaJSON), string(tagsJSON), task.OriginSystem, lastSyncAt, task.Archived, projectID,
			string(task.Effort), string(task.Priority), staleTimeout, task.BlockedReason, staleFiredAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert task: %w", err)
		}
		return nil
	})
}

func (s *SQLiteStorage) GetTask(ctx context.Context, id string) (*core.Task, error) {
	proj := core.DetectProject()
	var row *sql.Row
	if proj != nil && proj.InProject && proj.ProjectID != "" {
		row = s.db.QueryRowContext(ctx, "SELECT id, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id, effort, priority, stale_timeout, blocked_reason, stale_fired_at FROM tasks WHERE id = ? AND project_id = ?", id, proj.ProjectID)
	} else {
		// Prefer the global bucket (project_id='') over project-scoped rows so that
		// tasks created outside any project context are consistently resolved even
		// when sync has duplicated them into project-specific rows.
		row = s.db.QueryRowContext(ctx, "SELECT id, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id, effort, priority, stale_timeout, blocked_reason, stale_fired_at FROM tasks WHERE id = ? ORDER BY CASE WHEN project_id = '' THEN 0 ELSE 1 END LIMIT 1", id)
	}

	var task core.Task
	var createdAtStr, updatedAtStr string
	var metaStr, tagsStr, originSystemStr, lastSyncAtStr, projectIDStr, effortStr, priorityStr sql.NullString
	var staleTimeoutNs sql.NullInt64
	var blockedReasonStr, staleFiredAtStr sql.NullString

	err := row.Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.AssignedTo, &task.Reference, &createdAtStr, &updatedAtStr, &metaStr, &tagsStr, &originSystemStr, &lastSyncAtStr, &task.Archived, &projectIDStr, &effortStr, &priorityStr, &staleTimeoutNs, &blockedReasonStr, &staleFiredAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan task row: %w", err)
	}

	task.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	task.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)

	if metaStr.Valid {
		if err := json.Unmarshal([]byte(metaStr.String), &task.Meta); err != nil {
			return nil, fmt.Errorf("failed to unmarshal meta: %w", err)
		}
	}
	if tagsStr.Valid {
		if err := json.Unmarshal([]byte(tagsStr.String), &task.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}
	if originSystemStr.Valid {
		task.OriginSystem = &originSystemStr.String
	}
	if lastSyncAtStr.Valid {
		t, _ := time.Parse(time.RFC3339, lastSyncAtStr.String)
		task.LastSyncAt = &t
	}
	if projectIDStr.Valid {
		task.ProjectID = &projectIDStr.String
	}
	if effortStr.Valid {
		task.Effort = core.Effort(effortStr.String)
	}
	if priorityStr.Valid {
		task.Priority = core.Priority(priorityStr.String)
	}
	if staleTimeoutNs.Valid {
		d := time.Duration(staleTimeoutNs.Int64)
		task.StaleTimeout = &d
	}
	if blockedReasonStr.Valid {
		task.BlockedReason = &blockedReasonStr.String
	}
	if staleFiredAtStr.Valid {
		t, _ := time.Parse(time.RFC3339, staleFiredAtStr.String)
		task.StaleFiredAt = &t
	}

	return &task, nil
}

func (s *SQLiteStorage) GetTaskInProject(ctx context.Context, id, projectID string) (*core.Task, error) {
	row := s.db.QueryRowContext(
		ctx,
		"SELECT id, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id, effort, priority, stale_timeout, blocked_reason, stale_fired_at FROM tasks WHERE id = ? AND project_id = ?",
		id, projectID,
	)

	var task core.Task
	var createdAtStr, updatedAtStr string
	var metaStr, tagsStr, originSystemStr, lastSyncAtStr, projectIDStr, effortStr, priorityStr sql.NullString
	var staleTimeoutNs sql.NullInt64
	var blockedReasonStr, staleFiredAtStr sql.NullString

	err := row.Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.AssignedTo, &task.Reference, &createdAtStr, &updatedAtStr, &metaStr, &tagsStr, &originSystemStr, &lastSyncAtStr, &task.Archived, &projectIDStr, &effortStr, &priorityStr, &staleTimeoutNs, &blockedReasonStr, &staleFiredAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan task row: %w", err)
	}

	task.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	task.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)

	if metaStr.Valid {
		if err := json.Unmarshal([]byte(metaStr.String), &task.Meta); err != nil {
			return nil, fmt.Errorf("failed to unmarshal meta: %w", err)
		}
	}
	if tagsStr.Valid {
		if err := json.Unmarshal([]byte(tagsStr.String), &task.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}
	if originSystemStr.Valid {
		task.OriginSystem = &originSystemStr.String
	}
	if lastSyncAtStr.Valid {
		t, _ := time.Parse(time.RFC3339, lastSyncAtStr.String)
		task.LastSyncAt = &t
	}
	if projectIDStr.Valid {
		task.ProjectID = &projectIDStr.String
	}
	if effortStr.Valid {
		task.Effort = core.Effort(effortStr.String)
	}
	if priorityStr.Valid {
		task.Priority = core.Priority(priorityStr.String)
	}
	if staleTimeoutNs.Valid {
		d := time.Duration(staleTimeoutNs.Int64)
		task.StaleTimeout = &d
	}
	if blockedReasonStr.Valid {
		task.BlockedReason = &blockedReasonStr.String
	}
	if staleFiredAtStr.Valid {
		t, _ := time.Parse(time.RFC3339, staleFiredAtStr.String)
		task.StaleFiredAt = &t
	}

	return &task, nil
}

func (s *SQLiteStorage) UpdateTask(ctx context.Context, task *core.Task) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		metaJSON, _ := json.Marshal(task.Meta)
		tagsJSON, _ := json.Marshal(task.Tags)

		var lastSyncAt *string
		if task.LastSyncAt != nil {
			s := task.LastSyncAt.Format(time.RFC3339)
			lastSyncAt = &s
		}

		var staleTimeout *int64
		if task.StaleTimeout != nil {
			ns := int64(*task.StaleTimeout)
			staleTimeout = &ns
		}
		var staleFiredAt *string
		if task.StaleFiredAt != nil {
			s := task.StaleFiredAt.Format(time.RFC3339)
			staleFiredAt = &s
		}

		projectID := ""
		if task.ProjectID != nil {
			projectID = *task.ProjectID
		}
		res, err := tx.ExecContext(ctx, `
			UPDATE tasks SET title = ?, description = ?, status = ?, assigned_to = ?, reference = ?, updated_at = ?, meta = ?, tags = ?, origin_system = ?, last_sync_at = ?, archived = ?, effort = ?, priority = ?, stale_timeout = ?, blocked_reason = ?, stale_fired_at = ?
			WHERE id = ? AND project_id = ?`,
			task.Title, task.Description, task.Status, task.AssignedTo, task.Reference,
			task.UpdatedAt.Format(time.RFC3339), string(metaJSON), string(tagsJSON),
			task.OriginSystem, lastSyncAt, task.Archived, string(task.Effort), string(task.Priority),
			staleTimeout, task.BlockedReason, staleFiredAt, task.ID, projectID,
		)
		if err != nil {
			return fmt.Errorf("failed to update task: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected: %w", err)
		}
		if n == 0 {
			var exists int
			err = tx.QueryRowContext(ctx, `SELECT 1 FROM tasks WHERE id = ? AND project_id = ? LIMIT 1`, task.ID, projectID).Scan(&exists)
			if err == sql.ErrNoRows {
				return fmt.Errorf("task %s not found (project_id=%q)", task.ID, projectID)
			}
			if err != nil {
				return fmt.Errorf("failed to verify task existence: %w", err)
			}
			// row exists, values were unchanged — not an error
		}
		return nil
	})
}

func (s *SQLiteStorage) UpdateTaskWithLog(ctx context.Context, task *core.Task, entry *core.LogEntry) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		// Update Task
		metaJSON, _ := json.Marshal(task.Meta)
		tagsJSON, _ := json.Marshal(task.Tags)
		var lastSyncAt *string
		if task.LastSyncAt != nil {
			s := task.LastSyncAt.Format(time.RFC3339)
			lastSyncAt = &s
		}

		var staleTimeout *int64
		if task.StaleTimeout != nil {
			ns := int64(*task.StaleTimeout)
			staleTimeout = &ns
		}
		var staleFiredAt *string
		if task.StaleFiredAt != nil {
			s := task.StaleFiredAt.Format(time.RFC3339)
			staleFiredAt = &s
		}

		projectID := ""
		if task.ProjectID != nil {
			projectID = *task.ProjectID
		}
		res, err := tx.ExecContext(ctx, `
			UPDATE tasks SET title = ?, description = ?, status = ?, assigned_to = ?, reference = ?, updated_at = ?, meta = ?, tags = ?, origin_system = ?, last_sync_at = ?, archived = ?, effort = ?, priority = ?, stale_timeout = ?, blocked_reason = ?, stale_fired_at = ?
			WHERE id = ? AND project_id = ?`,
			task.Title, task.Description, task.Status, task.AssignedTo, task.Reference,
			task.UpdatedAt.Format(time.RFC3339), string(metaJSON), string(tagsJSON),
			task.OriginSystem, lastSyncAt, task.Archived, string(task.Effort), string(task.Priority),
			staleTimeout, task.BlockedReason, staleFiredAt, task.ID, projectID,
		)
		if err != nil {
			return fmt.Errorf("failed to update task: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected: %w", err)
		}
		if n == 0 {
			var exists int
			err = tx.QueryRowContext(ctx, `SELECT 1 FROM tasks WHERE id = ? AND project_id = ? LIMIT 1`, task.ID, projectID).Scan(&exists)
			if err == sql.ErrNoRows {
				return fmt.Errorf("task %s not found (project_id=%q)", task.ID, projectID)
			}
			if err != nil {
				return fmt.Errorf("failed to verify task existence: %w", err)
			}
			// row exists, values were unchanged — not an error
		}

		// Add Log
		logMetaJSON, _ := json.Marshal(entry.Meta)

		_, err = tx.ExecContext(ctx, `
			INSERT INTO task_logs (project_id, task_id, timestamp, by, action, note, meta)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			projectID, entry.TaskID, entry.Timestamp.Format(time.RFC3339), entry.By, entry.Action, entry.Note, string(logMetaJSON),
		)
		if err != nil {
			return fmt.Errorf("failed to insert log entry: %w", err)
		}
		return nil
	})
}

func (s *SQLiteStorage) ListTasks(ctx context.Context, query core.Query) ([]*core.Task, error) {
	sqlQuery := "SELECT id, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id, effort, priority, stale_timeout, blocked_reason, stale_fired_at FROM tasks"
	var args []interface{}

	// Group filters by field to implement OR logic for same field
	fieldGroups := make(map[string][]core.FieldFilter)
	for _, f := range query.Filters {
		fieldGroups[f.Field] = append(fieldGroups[f.Field], f)
	}

	whereClauses := []string{}
	for field, filters := range fieldGroups {
		groupClauses := []string{}
		for _, f := range filters {
			// Tags are stored as JSON arrays; use json_each for exact element matching.
			if field == "tags" && f.Operator == core.OpContains {
				groupClauses = append(groupClauses, "EXISTS (SELECT 1 FROM json_each(tasks.tags) WHERE value = ?)")
				args = append(args, f.Value)
				continue
			}

			op := "="
			switch f.Operator {
			case core.OpEq, core.OpEqual:
				op = "="
			case core.OpNotEq:
				op = "!="
			case core.OpGt:
				op = ">"
			case core.OpGte:
				op = ">="
			case core.OpLt:
				op = "<"
			case core.OpLte:
				op = "<="
			case core.OpContains:
				op = sqlOpLike
				f.Value = "%" + fmt.Sprintf("%v", f.Value) + "%"
			case core.OpStart:
				op = sqlOpLike
				f.Value = fmt.Sprintf("%v", f.Value) + "%"
			case core.OpEnd:
				op = sqlOpLike
				f.Value = "%" + fmt.Sprintf("%v", f.Value)
			case core.OpIn:
				vals := strings.Split(fmt.Sprintf("%v", f.Value), ",")
				placeholders := make([]string, len(vals))
				for i, v := range vals {
					placeholders[i] = "?"
					args = append(args, strings.TrimSpace(v))
				}
				groupClauses = append(groupClauses, fmt.Sprintf("%s IN (%s)", field, strings.Join(placeholders, ",")))
				continue
			}

			if field == "assigned_to" && f.Value == nil {
				if op == "=" {
					groupClauses = append(groupClauses, "assigned_to IS NULL")
				} else {
					groupClauses = append(groupClauses, "assigned_to IS NOT NULL")
				}
			} else {
				groupClauses = append(groupClauses, fmt.Sprintf("%s %s ?", field, op))
				args = append(args, f.Value)
			}
		}
		if len(groupClauses) > 0 {
			whereClauses = append(whereClauses, "("+strings.Join(groupClauses, " OR ")+")")
		}
	}

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

	if len(whereClauses) > 0 {
		sqlQuery += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Sorting
	if query.SortBy != "" {
		order := sqlOrderASC
		if strings.ToLower(query.SortDirection) == "desc" {
			order = sqlOrderDESC
		}
		sqlQuery += fmt.Sprintf(" ORDER BY %s %s", query.SortBy, order) //nolint:gosec // G202: SortBy is validated against known columns
	} else {
		sqlQuery += " ORDER BY created_at DESC"
	}

	// Pagination
	if query.Limit > 0 {
		sqlQuery += sqlLimit
		args = append(args, query.Limit)
	}
	if query.Offset > 0 {
		sqlQuery += " OFFSET ?"
		args = append(args, query.Offset)
	}

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tasks []*core.Task
	for rows.Next() {
		task, err := scanTaskFromRow(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate task rows: %w", err)
	}
	return tasks, nil
}

func (s *SQLiteStorage) AddLog(ctx context.Context, entry *core.LogEntry) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		metaJSON, _ := json.Marshal(entry.Meta)

		// Get project_id from task
		var projectID sql.NullString
		proj := core.DetectProject()
		if proj != nil && proj.InProject && proj.ProjectID != "" {
			err := tx.QueryRowContext(ctx, "SELECT project_id FROM tasks WHERE id = ? AND project_id = ?", entry.TaskID, proj.ProjectID).Scan(&projectID)
			if err != nil {
				return fmt.Errorf("failed to get project_id: %w", err)
			}
		} else {
			err := tx.QueryRowContext(ctx, "SELECT project_id FROM tasks WHERE id = ?", entry.TaskID).Scan(&projectID)
			if err != nil {
				return fmt.Errorf("failed to get project_id: %w", err)
			}
		}

		_, err := tx.ExecContext(ctx, `
			INSERT INTO task_logs (project_id, task_id, timestamp, by, action, note, meta)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			projectID, entry.TaskID, entry.Timestamp.Format(time.RFC3339), entry.By, entry.Action, entry.Note, string(metaJSON),
		)
		if err != nil {
			return fmt.Errorf("failed to insert log entry: %w", err)
		}
		return nil
	})
}

func (s *SQLiteStorage) GetLogs(ctx context.Context, taskID string, sortDirection string) ([]*core.LogEntry, error) {
	order := "DESC"
	if strings.ToUpper(sortDirection) == sqlOrderASC {
		order = sqlOrderASC
	}
	proj := core.DetectProject()
	var query string
	var rows *sql.Rows
	var err error
	if proj != nil && proj.InProject && proj.ProjectID != "" {
		query = fmt.Sprintf("SELECT id, task_id, timestamp, by, action, note, meta FROM task_logs WHERE task_id = ? AND project_id = ? ORDER BY timestamp %s", order)
		rows, err = s.db.QueryContext(ctx, query, taskID, proj.ProjectID)
	} else {
		query = fmt.Sprintf("SELECT id, task_id, timestamp, by, action, note, meta FROM task_logs WHERE task_id = ? ORDER BY timestamp %s", order)
		rows, err = s.db.QueryContext(ctx, query, taskID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []*core.LogEntry
	for rows.Next() {
		var entry core.LogEntry
		var timestampStr string
		var metaStr sql.NullString
		if err := rows.Scan(&entry.ID, &entry.TaskID, &timestampStr, &entry.By, &entry.Action, &entry.Note, &metaStr); err != nil {
			return nil, fmt.Errorf("failed to scan log row: %w", err)
		}
		entry.Timestamp, _ = time.Parse(time.RFC3339, timestampStr)
		if metaStr.Valid {
			if err := json.Unmarshal([]byte(metaStr.String), &entry.Meta); err != nil {
				return nil, fmt.Errorf("failed to unmarshal meta: %w", err)
			}
		}
		entries = append(entries, &entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate log rows: %w", err)
	}
	return entries, nil
}

func (s *SQLiteStorage) DeleteTask(ctx context.Context, id string) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		proj := core.DetectProject()
		if proj != nil && proj.InProject && proj.ProjectID != "" {
			_, err := tx.ExecContext(ctx, "DELETE FROM tasks WHERE id = ? AND project_id = ?", id, proj.ProjectID)
			if err != nil {
				return fmt.Errorf("failed to delete task: %w", err)
			}
			return nil
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM tasks WHERE id = ?", id)
		if err != nil {
			return fmt.Errorf("failed to delete task: %w", err)
		}
		return nil
	})
}

func (s *SQLiteStorage) FindTaskByOrigin(ctx context.Context, system, originID string) (*core.Task, error) {
	sqlQuery := `
		SELECT id, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id, effort, priority, stale_timeout, blocked_reason, stale_fired_at
		FROM tasks
		WHERE origin_system = ? AND json_extract(meta, '$.origin_id') = ?
		LIMIT 1
	`
	row := s.db.QueryRowContext(ctx, sqlQuery, system, originID)

	var task core.Task
	var createdAtStr, updatedAtStr string
	var metaStr, tagsStr, originSystemStr, lastSyncAtStr, projectIDStr, effortStr, priorityStr sql.NullString
	var staleTimeoutNs sql.NullInt64
	var blockedReasonStr, staleFiredAtStr sql.NullString

	err := row.Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.AssignedTo, &task.Reference, &createdAtStr, &updatedAtStr, &metaStr, &tagsStr, &originSystemStr, &lastSyncAtStr, &task.Archived, &projectIDStr, &effortStr, &priorityStr, &staleTimeoutNs, &blockedReasonStr, &staleFiredAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan task by origin: %w", err)
	}

	task.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	task.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)

	if metaStr.Valid {
		if err := json.Unmarshal([]byte(metaStr.String), &task.Meta); err != nil {
			return nil, fmt.Errorf("failed to unmarshal meta: %w", err)
		}
	}
	if tagsStr.Valid {
		if err := json.Unmarshal([]byte(tagsStr.String), &task.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}
	if originSystemStr.Valid {
		task.OriginSystem = &originSystemStr.String
	}
	if lastSyncAtStr.Valid {
		t, _ := time.Parse(time.RFC3339, lastSyncAtStr.String)
		task.LastSyncAt = &t
	}
	if projectIDStr.Valid {
		task.ProjectID = &projectIDStr.String
	}
	if effortStr.Valid {
		task.Effort = core.Effort(effortStr.String)
	}
	if priorityStr.Valid {
		task.Priority = core.Priority(priorityStr.String)
	}
	if staleTimeoutNs.Valid {
		d := time.Duration(staleTimeoutNs.Int64)
		task.StaleTimeout = &d
	}
	if blockedReasonStr.Valid {
		task.BlockedReason = &blockedReasonStr.String
	}
	if staleFiredAtStr.Valid {
		t, _ := time.Parse(time.RFC3339, staleFiredAtStr.String)
		task.StaleFiredAt = &t
	}

	return &task, nil
}

func (s *SQLiteStorage) ArchiveTasks(ctx context.Context, threshold time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-threshold).Format(time.RFC3339)

	var count int64
	err := s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE tasks 
			SET archived = 1 
			WHERE archived = 0 
			AND (status = 'DONE' OR status = 'SKIPPED')
			AND updated_at < ?`,
			cutoff,
		)
		if err != nil {
			return fmt.Errorf("failed to archive tasks: %w", err)
		}
		count, _ = res.RowsAffected()
		return nil
	})

	return count, err
}

func (s *SQLiteStorage) GetTasksNeedingPush(ctx context.Context) ([]*core.Task, error) {
	// A task needs push if it has an origin system AND (it has never been synced OR updated_at > last_sync_at)
	// AND it is NOT archived.
	sqlQuery := `
		SELECT id, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id, effort, priority, stale_timeout, blocked_reason, stale_fired_at
		FROM tasks
		WHERE origin_system IS NOT NULL AND origin_system != ''
		AND (last_sync_at IS NULL OR updated_at > last_sync_at)
		AND archived = 0
	`
	rows, err := s.db.QueryContext(ctx, sqlQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query tasks needing push: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tasks []*core.Task
	for rows.Next() {
		task, err := scanTaskFromRow(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate tasks needing push: %w", err)
	}
	return tasks, nil
}

func (s *SQLiteStorage) CreateFlowRun(ctx context.Context, run *core.FlowRun) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		resultsJSON, _ := json.Marshal(run.Results)
		_, err := tx.ExecContext(ctx, `
			INSERT INTO flow_runs (id, flow_id, status, started_at, ended_at, results)
			VALUES (?, ?, ?, ?, ?, ?)`,
			run.ID, run.FlowID, run.Status, run.StartedAt.Format(time.RFC3339),
			nil, string(resultsJSON),
		)
		if err != nil {
			return fmt.Errorf("failed to insert flow run: %w", err)
		}
		return nil
	})
}

func (s *SQLiteStorage) GetFlowRun(ctx context.Context, id string) (*core.FlowRun, error) {
	row := s.db.QueryRowContext(ctx, "SELECT id, flow_id, status, started_at, ended_at, results FROM flow_runs WHERE id = ?", id)

	var run core.FlowRun
	var startedAtStr string
	var endedAtStr, resultsStr sql.NullString

	err := row.Scan(&run.ID, &run.FlowID, &run.Status, &startedAtStr, &endedAtStr, &resultsStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan flow run: %w", err)
	}

	run.StartedAt, _ = time.Parse(time.RFC3339, startedAtStr)
	if endedAtStr.Valid {
		t, _ := time.Parse(time.RFC3339, endedAtStr.String)
		run.EndedAt = &t
	}
	if resultsStr.Valid {
		if err := json.Unmarshal([]byte(resultsStr.String), &run.Results); err != nil {
			return nil, fmt.Errorf("failed to unmarshal results: %w", err)
		}
	}

	return &run, nil
}

func (s *SQLiteStorage) UpdateFlowRun(ctx context.Context, run *core.FlowRun) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		resultsJSON, _ := json.Marshal(run.Results)
		var endedAt interface{}
		if run.EndedAt != nil {
			endedAt = run.EndedAt.Format(time.RFC3339)
		}

		_, err := tx.ExecContext(ctx, `
			UPDATE flow_runs SET status = ?, ended_at = ?, results = ?
			WHERE id = ?`,
			run.Status, endedAt, string(resultsJSON), run.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to update flow run: %w", err)
		}
		return nil
	})
}

func (s *SQLiteStorage) ListFlowRuns(ctx context.Context, query core.Query) ([]*core.FlowRun, error) {
	sqlQuery := "SELECT id, flow_id, status, started_at, ended_at, results FROM flow_runs"
	var args []interface{}

	// Basic implementation, can be extended like ListTasks
	sqlQuery += " ORDER BY started_at DESC"

	if query.Limit > 0 {
		sqlQuery += " LIMIT ?"
		args = append(args, query.Limit)
	}

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query flow runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var runs []*core.FlowRun
	for rows.Next() {
		var run core.FlowRun
		var startedAtStr string
		var endedAtStr, resultsStr sql.NullString
		if err := rows.Scan(&run.ID, &run.FlowID, &run.Status, &startedAtStr, &endedAtStr, &resultsStr); err != nil {
			return nil, fmt.Errorf("failed to scan flow run row: %w", err)
		}
		run.StartedAt, _ = time.Parse(time.RFC3339, startedAtStr)
		if endedAtStr.Valid {
			t, _ := time.Parse(time.RFC3339, endedAtStr.String)
			run.EndedAt = &t
		}
		if resultsStr.Valid {
			if err := json.Unmarshal([]byte(resultsStr.String), &run.Results); err != nil {
				return nil, fmt.Errorf("failed to unmarshal results: %w", err)
			}
		}
		runs = append(runs, &run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate flow run rows: %w", err)
	}
	return runs, nil
}

func (s *SQLiteStorage) ListLogs(ctx context.Context, query core.LogQuery) ([]*core.LogEntry, error) {
	sqlQuery := "SELECT id, task_id, timestamp, by, action, note, meta FROM task_logs"
	var args []interface{}
	whereClauses := []string{}

	if query.TaskID != "" {
		whereClauses = append(whereClauses, "task_id = ?")
		args = append(args, query.TaskID)
	}
	if query.Action != "" {
		whereClauses = append(whereClauses, "action = ?")
		args = append(args, query.Action)
	}
	if query.By != "" {
		whereClauses = append(whereClauses, "by = ?")
		args = append(args, query.By)
	}

	if len(whereClauses) > 0 {
		sqlQuery += " WHERE " + strings.Join(whereClauses, " AND ") //nolint:gosec // G202: whereClauses built from validated field names
	}

	order := "DESC"
	if strings.ToLower(query.SortDirection) == "asc" {
		order = "ASC"
	}
	sqlQuery += " ORDER BY timestamp " + order

	if query.Limit > 0 {
		sqlQuery += " LIMIT ?"
		args = append(args, query.Limit)
	}
	if query.Offset > 0 {
		sqlQuery += " OFFSET ?"
		args = append(args, query.Offset)
	}

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []*core.LogEntry
	for rows.Next() {
		var entry core.LogEntry
		var timestampStr string
		var metaStr sql.NullString
		if err := rows.Scan(&entry.ID, &entry.TaskID, &timestampStr, &entry.By, &entry.Action, &entry.Note, &metaStr); err != nil {
			return nil, fmt.Errorf("failed to scan log row: %w", err)
		}
		entry.Timestamp, _ = time.Parse(time.RFC3339, timestampStr)
		if metaStr.Valid {
			if err := json.Unmarshal([]byte(metaStr.String), &entry.Meta); err != nil {
				return nil, fmt.Errorf("failed to unmarshal meta: %w", err)
			}
		}
		entries = append(entries, &entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate log rows: %w", err)
	}
	return entries, nil
}

func (s *SQLiteStorage) GetNextSequenceID(ctx context.Context, projectID string) (int, error) {
	var nextID int

	if projectID == "" {
		projectID = "default"
	}

	err := s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO task_sequences (project_id, next_id)
			VALUES (?, 1)
		`, projectID)
		if err != nil {
			return fmt.Errorf("failed to insert sequence: %w", err)
		}

		err = tx.QueryRowContext(ctx, "SELECT next_id FROM task_sequences WHERE project_id = ?", projectID).Scan(&nextID)
		if err != nil {
			return fmt.Errorf("failed to get next sequence id: %w", err)
		}

		_, err = tx.ExecContext(ctx, "UPDATE task_sequences SET next_id = next_id + 1 WHERE project_id = ?", projectID)
		if err != nil {
			return fmt.Errorf("failed to update sequence: %w", err)
		}
		return nil
	})

	return nextID, err
}

func scanTaskFromRow(rows *sql.Rows) (*core.Task, error) {
	var task core.Task
	var createdAtStr, updatedAtStr string
	var metaStr, tagsStr, originSystemStr, lastSyncAtStr, projectIDStr, effortStr, priorityStr sql.NullString
	var staleTimeoutNs sql.NullInt64
	var blockedReasonStr, staleFiredAtStr sql.NullString
	if err := rows.Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.AssignedTo, &task.Reference, &createdAtStr, &updatedAtStr, &metaStr, &tagsStr, &originSystemStr, &lastSyncAtStr, &task.Archived, &projectIDStr, &effortStr, &priorityStr, &staleTimeoutNs, &blockedReasonStr, &staleFiredAtStr); err != nil {
		return nil, fmt.Errorf("failed to scan task row: %w", err)
	}
	task.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	task.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
	if metaStr.Valid {
		if err := json.Unmarshal([]byte(metaStr.String), &task.Meta); err != nil {
			return nil, fmt.Errorf("failed to unmarshal meta: %w", err)
		}
	}
	if tagsStr.Valid {
		if err := json.Unmarshal([]byte(tagsStr.String), &task.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}
	if originSystemStr.Valid {
		task.OriginSystem = &originSystemStr.String
	}
	if lastSyncAtStr.Valid {
		t, _ := time.Parse(time.RFC3339, lastSyncAtStr.String)
		task.LastSyncAt = &t
	}
	if projectIDStr.Valid {
		task.ProjectID = &projectIDStr.String
	}
	if effortStr.Valid {
		task.Effort = core.Effort(effortStr.String)
	}
	if priorityStr.Valid {
		task.Priority = core.Priority(priorityStr.String)
	}
	if staleTimeoutNs.Valid {
		d := time.Duration(staleTimeoutNs.Int64)
		task.StaleTimeout = &d
	}
	if blockedReasonStr.Valid {
		task.BlockedReason = &blockedReasonStr.String
	}
	if staleFiredAtStr.Valid {
		t, _ := time.Parse(time.RFC3339, staleFiredAtStr.String)
		task.StaleFiredAt = &t
	}
	return &task, nil
}

func (s *SQLiteStorage) RegisterProject(ctx context.Context, projectID, dbPath, spaceURI, label string) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		_, err := tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO projects (project_id, db_path, space_uri, label, registered_at, last_seen_at, status)
			VALUES (?, ?, ?, ?, ?, ?, 'active')`,
			projectID, dbPath, spaceURI, label, now, now,
		)
		if err != nil {
			return fmt.Errorf("failed to register project: %w", err)
		}
		return nil
	})
}

func (s *SQLiteStorage) LookupProject(ctx context.Context, projectID string) (*core.RegisteredProject, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT project_id, db_path, space_uri, label, registered_at, last_seen_at, status FROM projects WHERE project_id = ?",
		projectID,
	)

	var p core.RegisteredProject
	var registeredAtStr, lastSeenAtStr string
	var spaceURI, label sql.NullString

	err := row.Scan(&p.ProjectID, &p.DBPath, &spaceURI, &label, &registeredAtStr, &lastSeenAtStr, &p.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan project row: %w", err)
	}

	p.RegisteredAt, _ = time.Parse(time.RFC3339, registeredAtStr)
	p.LastSeenAt, _ = time.Parse(time.RFC3339, lastSeenAtStr)
	if spaceURI.Valid {
		p.SpaceURI = spaceURI.String
	}
	if label.Valid {
		p.Label = label.String
	}

	return &p, nil
}

func (s *SQLiteStorage) ListProjectsBySpace(ctx context.Context, spaceURI string) ([]core.RegisteredProject, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT project_id, db_path, space_uri, label, registered_at, last_seen_at, status FROM projects WHERE space_uri = ? AND status = 'active'",
		spaceURI,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query projects by space: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanProjects(rows)
}

func (s *SQLiteStorage) ListAllProjects(ctx context.Context) ([]core.RegisteredProject, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT project_id, db_path, space_uri, label, registered_at, last_seen_at, status FROM projects",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query all projects: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanProjects(rows)
}

func (s *SQLiteStorage) UpdateProjectPath(ctx context.Context, projectID, newDBPath string) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		_, err := tx.ExecContext(ctx,
			"UPDATE projects SET db_path = ?, last_seen_at = ? WHERE project_id = ?",
			newDBPath, now, projectID,
		)
		if err != nil {
			return fmt.Errorf("failed to update project path: %w", err)
		}
		return nil
	})
}

func (s *SQLiteStorage) TouchProject(ctx context.Context, projectID string) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		_, err := tx.ExecContext(ctx,
			"UPDATE projects SET last_seen_at = ? WHERE project_id = ?",
			now, projectID,
		)
		if err != nil {
			return fmt.Errorf("failed to touch project: %w", err)
		}
		return nil
	})
}

func scanProjects(rows *sql.Rows) ([]core.RegisteredProject, error) {
	var projects []core.RegisteredProject
	for rows.Next() {
		var p core.RegisteredProject
		var registeredAtStr, lastSeenAtStr string
		var spaceURI, label sql.NullString

		if err := rows.Scan(&p.ProjectID, &p.DBPath, &spaceURI, &label, &registeredAtStr, &lastSeenAtStr, &p.Status); err != nil {
			return nil, fmt.Errorf("failed to scan project row: %w", err)
		}

		p.RegisteredAt, _ = time.Parse(time.RFC3339, registeredAtStr)
		p.LastSeenAt, _ = time.Parse(time.RFC3339, lastSeenAtStr)
		if spaceURI.Valid {
			p.SpaceURI = spaceURI.String
		}
		if label.Valid {
			p.Label = label.String
		}

		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate project rows: %w", err)
	}
	return projects, nil
}

func (s *SQLiteStorage) ListAllTags(ctx context.Context) ([]string, error) {
	// SQLite 3.38+ supports JSON_EACH.
	// If older, we'd need to fetch all and parse in Go.
	query := `
		SELECT DISTINCT value
		FROM tasks, json_each(tasks.tags)
		WHERE tasks.archived = 0
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tags: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("failed to scan tag: %w", err)
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

func (s *SQLiteStorage) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}
	return nil
}

// GetTaskLogs returns log entries for a task, sorted newest-first.
// Satisfies core.TaskReader.
func (s *SQLiteStorage) GetTaskLogs(ctx context.Context, taskID string) ([]*core.LogEntry, error) {
	return s.GetLogs(ctx, taskID, "desc")
}

// CountTasks returns the number of tasks matching the query.
// Satisfies core.TaskReader.
func (s *SQLiteStorage) CountTasks(ctx context.Context, query core.Query) (int, error) {
	sqlQuery := "SELECT COUNT(*) FROM tasks"
	var args []interface{}

	// Group filters by field to implement OR logic for same field
	fieldGroups := make(map[string][]core.FieldFilter)
	for _, f := range query.Filters {
		fieldGroups[f.Field] = append(fieldGroups[f.Field], f)
	}

	whereClauses := []string{}
	for field, filters := range fieldGroups {
		groupClauses := []string{}
		for _, f := range filters {
			// Tags are stored as JSON arrays; use json_each for exact element matching.
			if field == "tags" && f.Operator == core.OpContains {
				groupClauses = append(groupClauses, "EXISTS (SELECT 1 FROM json_each(tasks.tags) WHERE value = ?)")
				args = append(args, f.Value)
				continue
			}

			op := "="
			switch f.Operator {
			case core.OpEq, core.OpEqual:
				op = "="
			case core.OpNotEq:
				op = "!="
			case core.OpGt:
				op = ">"
			case core.OpGte:
				op = ">="
			case core.OpLt:
				op = "<"
			case core.OpLte:
				op = "<="
			case core.OpContains:
				op = sqlOpLike
				f.Value = "%" + fmt.Sprintf("%v", f.Value) + "%"
			case core.OpStart:
				op = sqlOpLike
				f.Value = fmt.Sprintf("%v", f.Value) + "%"
			case core.OpEnd:
				op = sqlOpLike
				f.Value = "%" + fmt.Sprintf("%v", f.Value)
			case core.OpIn:
				vals := strings.Split(fmt.Sprintf("%v", f.Value), ",")
				placeholders := make([]string, len(vals))
				for i, v := range vals {
					placeholders[i] = "?"
					args = append(args, strings.TrimSpace(v))
				}
				groupClauses = append(groupClauses, fmt.Sprintf("%s IN (%s)", field, strings.Join(placeholders, ",")))
				continue
			}

			if field == "assigned_to" && f.Value == nil {
				if op == "=" {
					groupClauses = append(groupClauses, "assigned_to IS NULL")
				} else {
					groupClauses = append(groupClauses, "assigned_to IS NOT NULL")
				}
			} else {
				groupClauses = append(groupClauses, fmt.Sprintf("%s %s ?", field, op))
				args = append(args, f.Value)
			}
		}
		if len(groupClauses) > 0 {
			whereClauses = append(whereClauses, "("+strings.Join(groupClauses, " OR ")+")")
		}
	}

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

	if len(whereClauses) > 0 {
		sqlQuery += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	var count int
	err := s.db.QueryRowContext(ctx, sqlQuery, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count tasks: %w", err)
	}
	return count, nil
}
