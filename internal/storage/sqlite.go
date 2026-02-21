package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/IdeaCraftersLabs/oss-tlc-cli/internal/core"
	_ "modernc.org/sqlite"
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
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Enable WAL mode and set busy timeout for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode = WAL;"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout = 5000;"); err != nil {
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
		return err
	}
	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
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

		// Coalesce nil ProjectID to empty string to prevent NULL composite PK duplication
		var projectID string
		if task.ProjectID != nil {
			projectID = *task.ProjectID
		}

		_, err := tx.ExecContext(ctx, `
			INSERT INTO tasks (id, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			task.ID, task.Title, task.Description, task.Status, task.AssignedTo, task.Reference,
			task.CreatedAt.Format(time.RFC3339), task.UpdatedAt.Format(time.RFC3339),
			string(metaJSON), string(tagsJSON), task.OriginSystem, lastSyncAt, task.Archived, projectID,
		)
		return err
	})
}

func (s *SQLiteStorage) GetTask(ctx context.Context, id string) (*core.Task, error) {
	proj := core.DetectProject()
	var row *sql.Row
	if proj != nil && proj.InProject && proj.ProjectID != "" {
		row = s.db.QueryRowContext(ctx, "SELECT id, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id FROM tasks WHERE id = ? AND project_id = ?", id, proj.ProjectID)
	} else {
		row = s.db.QueryRowContext(ctx, "SELECT id, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id FROM tasks WHERE id = ?", id)
	}

	var task core.Task
	var createdAtStr, updatedAtStr string
	var metaStr, tagsStr, originSystemStr, lastSyncAtStr, projectIDStr sql.NullString

	err := row.Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.AssignedTo, &task.Reference, &createdAtStr, &updatedAtStr, &metaStr, &tagsStr, &originSystemStr, &lastSyncAtStr, &task.Archived, &projectIDStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	task.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	task.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)

	if metaStr.Valid {
		json.Unmarshal([]byte(metaStr.String), &task.Meta)
	}
	if tagsStr.Valid {
		json.Unmarshal([]byte(tagsStr.String), &task.Tags)
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

		var err error
		if task.ProjectID != nil {
			_, err = tx.ExecContext(ctx, `
				UPDATE tasks SET title = ?, description = ?, status = ?, assigned_to = ?, reference = ?, updated_at = ?, meta = ?, tags = ?, origin_system = ?, last_sync_at = ?, archived = ?
				WHERE id = ? AND project_id = ?`,
				task.Title, task.Description, task.Status, task.AssignedTo, task.Reference,
				task.UpdatedAt.Format(time.RFC3339), string(metaJSON), string(tagsJSON),
				task.OriginSystem, lastSyncAt, task.Archived, task.ID, task.ProjectID,
			)
		} else {
			_, err = tx.ExecContext(ctx, `
				UPDATE tasks SET title = ?, description = ?, status = ?, assigned_to = ?, reference = ?, updated_at = ?, meta = ?, tags = ?, origin_system = ?, last_sync_at = ?, archived = ?
				WHERE id = ? AND project_id IS NULL`,
				task.Title, task.Description, task.Status, task.AssignedTo, task.Reference,
				task.UpdatedAt.Format(time.RFC3339), string(metaJSON), string(tagsJSON),
				task.OriginSystem, lastSyncAt, task.Archived, task.ID,
			)
		}
		return err
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

		var err error
		if task.ProjectID != nil {
			_, err = tx.ExecContext(ctx, `
				UPDATE tasks SET title = ?, description = ?, status = ?, assigned_to = ?, reference = ?, updated_at = ?, meta = ?, tags = ?, origin_system = ?, last_sync_at = ?, archived = ?
				WHERE id = ? AND project_id = ?`,
				task.Title, task.Description, task.Status, task.AssignedTo, task.Reference,
				task.UpdatedAt.Format(time.RFC3339), string(metaJSON), string(tagsJSON),
				task.OriginSystem, lastSyncAt, task.Archived, task.ID, task.ProjectID,
			)
		} else {
			_, err = tx.ExecContext(ctx, `
				UPDATE tasks SET title = ?, description = ?, status = ?, assigned_to = ?, reference = ?, updated_at = ?, meta = ?, tags = ?, origin_system = ?, last_sync_at = ?, archived = ?
				WHERE id = ? AND project_id IS NULL`,
				task.Title, task.Description, task.Status, task.AssignedTo, task.Reference,
				task.UpdatedAt.Format(time.RFC3339), string(metaJSON), string(tagsJSON),
				task.OriginSystem, lastSyncAt, task.Archived, task.ID,
			)
		}
		if err != nil {
			return err
		}

		// Add Log
		logMetaJSON, _ := json.Marshal(entry.Meta)

		// Get project_id from task
		var projectID sql.NullString
		projectID = sql.NullString{String: *task.ProjectID, Valid: task.ProjectID != nil}
		if task.ProjectID == nil {
			projectID = sql.NullString{String: "default", Valid: true}
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO task_logs (project_id, task_id, timestamp, by, action, note, meta)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			projectID, entry.TaskID, entry.Timestamp.Format(time.RFC3339), entry.By, entry.Action, entry.Note, string(logMetaJSON),
		)
		return err
	})
}

func (s *SQLiteStorage) ListTasks(ctx context.Context, query core.Query) ([]*core.Task, error) {
	sqlQuery := "SELECT id, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id FROM tasks"
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
				op = "LIKE"
				f.Value = "%" + fmt.Sprintf("%v", f.Value) + "%"
			case core.OpStart:
				op = "LIKE"
				f.Value = fmt.Sprintf("%v", f.Value) + "%"
			case core.OpEnd:
				op = "LIKE"
				f.Value = "%" + fmt.Sprintf("%v", f.Value)
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
		order := "ASC"
		if strings.ToLower(query.SortDirection) == "desc" {
			order = "DESC"
		}
		sqlQuery += fmt.Sprintf(" ORDER BY %s %s", query.SortBy, order)
	} else {
		sqlQuery += " ORDER BY created_at DESC"
	}

	// Pagination
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
		return nil, err
	}
	defer rows.Close()

	var tasks []*core.Task
	for rows.Next() {
		var task core.Task
		var createdAtStr, updatedAtStr string
		var metaStr, tagsStr, originSystemStr, lastSyncAtStr, projectIDStr sql.NullString
		if err := rows.Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.AssignedTo, &task.Reference, &createdAtStr, &updatedAtStr, &metaStr, &tagsStr, &originSystemStr, &lastSyncAtStr, &task.Archived, &projectIDStr); err != nil {
			return nil, err
		}
		task.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		task.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
		if metaStr.Valid {
			json.Unmarshal([]byte(metaStr.String), &task.Meta)
		}
		if tagsStr.Valid {
			json.Unmarshal([]byte(tagsStr.String), &task.Tags)
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
		tasks = append(tasks, &task)
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
				return err
			}
		} else {
			err := tx.QueryRowContext(ctx, "SELECT project_id FROM tasks WHERE id = ?", entry.TaskID).Scan(&projectID)
			if err != nil {
				return err
			}
		}

		_, err := tx.ExecContext(ctx, `
			INSERT INTO task_logs (project_id, task_id, timestamp, by, action, note, meta)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			projectID, entry.TaskID, entry.Timestamp.Format(time.RFC3339), entry.By, entry.Action, entry.Note, string(metaJSON),
		)
		return err
	})
}

func (s *SQLiteStorage) GetLogs(ctx context.Context, taskID string, sortDirection string) ([]*core.LogEntry, error) {
	order := "DESC"
	if strings.ToUpper(sortDirection) == "ASC" {
		order = "ASC"
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
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*core.LogEntry
	for rows.Next() {
		var entry core.LogEntry
		var timestampStr string
		var metaStr sql.NullString
		if err := rows.Scan(&entry.ID, &entry.TaskID, &timestampStr, &entry.By, &entry.Action, &entry.Note, &metaStr); err != nil {
			return nil, err
		}
		entry.Timestamp, _ = time.Parse(time.RFC3339, timestampStr)
		if metaStr.Valid {
			json.Unmarshal([]byte(metaStr.String), &entry.Meta)
		}
		entries = append(entries, &entry)
	}
	return entries, nil
}

func (s *SQLiteStorage) DeleteTask(ctx context.Context, id string) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		proj := core.DetectProject()
		if proj != nil && proj.InProject && proj.ProjectID != "" {
			_, err := tx.ExecContext(ctx, "DELETE FROM tasks WHERE id = ? AND project_id = ?", id, proj.ProjectID)
			return err
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM tasks WHERE id = ?", id)
		return err
	})
}

func (s *SQLiteStorage) FindTaskByOrigin(ctx context.Context, system, originID string) (*core.Task, error) {
	sqlQuery := `
		SELECT id, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id
		FROM tasks
		WHERE origin_system = ? AND json_extract(meta, '$.origin_id') = ?
		LIMIT 1
	`
	row := s.db.QueryRowContext(ctx, sqlQuery, system, originID)

	var task core.Task
	var createdAtStr, updatedAtStr string
	var metaStr, tagsStr, originSystemStr, lastSyncAtStr, projectIDStr sql.NullString

	err := row.Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.AssignedTo, &task.Reference, &createdAtStr, &updatedAtStr, &metaStr, &tagsStr, &originSystemStr, &lastSyncAtStr, &task.Archived, &projectIDStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	task.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	task.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)

	if metaStr.Valid {
		json.Unmarshal([]byte(metaStr.String), &task.Meta)
	}
	if tagsStr.Valid {
		json.Unmarshal([]byte(tagsStr.String), &task.Tags)
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
			return err
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
		SELECT id, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id
		FROM tasks
		WHERE origin_system IS NOT NULL AND origin_system != ''
		AND (last_sync_at IS NULL OR updated_at > last_sync_at)
		AND archived = 0
	`
	rows, err := s.db.QueryContext(ctx, sqlQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*core.Task
	for rows.Next() {
		var task core.Task
		var createdAtStr, updatedAtStr string
		var metaStr, tagsStr, originSystemStr, lastSyncAtStr, projectIDStr sql.NullString
		if err := rows.Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.AssignedTo, &task.Reference, &createdAtStr, &updatedAtStr, &metaStr, &tagsStr, &originSystemStr, &lastSyncAtStr, &task.Archived, &projectIDStr); err != nil {
			return nil, err
		}
		task.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		task.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
		if metaStr.Valid {
			json.Unmarshal([]byte(metaStr.String), &task.Meta)
		}
		if tagsStr.Valid {
			json.Unmarshal([]byte(tagsStr.String), &task.Tags)
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
		tasks = append(tasks, &task)
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
		return err
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
		return nil, err
	}

	run.StartedAt, _ = time.Parse(time.RFC3339, startedAtStr)
	if endedAtStr.Valid {
		t, _ := time.Parse(time.RFC3339, endedAtStr.String)
		run.EndedAt = &t
	}
	if resultsStr.Valid {
		json.Unmarshal([]byte(resultsStr.String), &run.Results)
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
		return err
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
		return nil, err
	}
	defer rows.Close()

	var runs []*core.FlowRun
	for rows.Next() {
		var run core.FlowRun
		var startedAtStr string
		var endedAtStr, resultsStr sql.NullString
		if err := rows.Scan(&run.ID, &run.FlowID, &run.Status, &startedAtStr, &endedAtStr, &resultsStr); err != nil {
			return nil, err
		}
		run.StartedAt, _ = time.Parse(time.RFC3339, startedAtStr)
		if endedAtStr.Valid {
			t, _ := time.Parse(time.RFC3339, endedAtStr.String)
			run.EndedAt = &t
		}
		if resultsStr.Valid {
			json.Unmarshal([]byte(resultsStr.String), &run.Results)
		}
		runs = append(runs, &run)
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
		sqlQuery += " WHERE " + strings.Join(whereClauses, " AND ")
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
		return nil, err
	}
	defer rows.Close()

	var entries []*core.LogEntry
	for rows.Next() {
		var entry core.LogEntry
		var timestampStr string
		var metaStr sql.NullString
		if err := rows.Scan(&entry.ID, &entry.TaskID, &timestampStr, &entry.By, &entry.Action, &entry.Note, &metaStr); err != nil {
			return nil, err
		}
		entry.Timestamp, _ = time.Parse(time.RFC3339, timestampStr)
		if metaStr.Valid {
			json.Unmarshal([]byte(metaStr.String), &entry.Meta)
		}
		entries = append(entries, &entry)
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
			return err
		}

		err = tx.QueryRowContext(ctx, "SELECT next_id FROM task_sequences WHERE project_id = ?", projectID).Scan(&nextID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, "UPDATE task_sequences SET next_id = next_id + 1 WHERE project_id = ?", projectID)
		return err
	})

	return nextID, err
}

func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}
