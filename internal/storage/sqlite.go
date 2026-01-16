package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/oss-tlc-cli/internal/core"
	_ "modernc.org/sqlite"
)

type SQLiteStorage struct {
	db *sql.DB
}

func NewSQLiteStorage(path string) (*SQLiteStorage, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}

	s := &SQLiteStorage{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate db: %w", err)
	}

	return s, nil
}

func (s *SQLiteStorage) CreateTask(ctx context.Context, task *core.Task) error {
	metaJSON, _ := json.Marshal(task.Meta)
	tagsJSON, _ := json.Marshal(task.Tags)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.Title, task.Description, task.Status, task.AssignedTo, task.Reference,
		task.CreatedAt.Format(time.RFC3339), task.UpdatedAt.Format(time.RFC3339),
		string(metaJSON), string(tagsJSON),
	)
	return err
}

func (s *SQLiteStorage) GetTask(ctx context.Context, id string) (*core.Task, error) {
	row := s.db.QueryRowContext(ctx, "SELECT id, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags FROM tasks WHERE id = ?", id)

	var task core.Task
	var createdAtStr, updatedAtStr string
	var metaStr, tagsStr sql.NullString

	err := row.Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.AssignedTo, &task.Reference, &createdAtStr, &updatedAtStr, &metaStr, &tagsStr)
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

	return &task, nil
}

func (s *SQLiteStorage) UpdateTask(ctx context.Context, task *core.Task) error {
	metaJSON, _ := json.Marshal(task.Meta)
	tagsJSON, _ := json.Marshal(task.Tags)

	_, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET title = ?, description = ?, status = ?, assigned_to = ?, reference = ?, updated_at = ?, meta = ?, tags = ?
		WHERE id = ?`,
		task.Title, task.Description, task.Status, task.AssignedTo, task.Reference,
		task.UpdatedAt.Format(time.RFC3339), string(metaJSON), string(tagsJSON), task.ID,
	)
	return err
}

func (s *SQLiteStorage) ListTasks(ctx context.Context, query core.Query) ([]*core.Task, error) {
	sqlQuery := "SELECT id, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags FROM tasks"
	var args []interface{}
	whereClauses := []string{}

	for _, f := range query.Filters {
		field := f.Field
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
				whereClauses = append(whereClauses, "assigned_to IS NULL")
			} else {
				whereClauses = append(whereClauses, "assigned_to IS NOT NULL")
			}
		} else {
			whereClauses = append(whereClauses, fmt.Sprintf("%s %s ?", field, op))
			args = append(args, f.Value)
		}
	}

	if query.Search != "" {
		whereClauses = append(whereClauses, "(title LIKE ? OR description LIKE ?)")
		args = append(args, "%"+query.Search+"%", "%"+query.Search+"%")
	}

	if len(whereClauses) > 0 {
		sqlQuery += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Sorting
	if query.SortBy != "" {
		order := "ASC"
		if strings.ToLower(query.Order) == "desc" {
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
		var metaStr, tagsStr sql.NullString
		if err := rows.Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.AssignedTo, &task.Reference, &createdAtStr, &updatedAtStr, &metaStr, &tagsStr); err != nil {
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
		tasks = append(tasks, &task)
	}
	return tasks, nil
}

func (s *SQLiteStorage) AddLog(ctx context.Context, entry *core.LogEntry) error {
	metaJSON, _ := json.Marshal(entry.Meta)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO task_logs (task_id, timestamp, by, action, note, meta)
		VALUES (?, ?, ?, ?, ?, ?)`,
		entry.TaskID, entry.Timestamp.Format(time.RFC3339), entry.By, entry.Action, entry.Note, string(metaJSON),
	)
	return err
}

func (s *SQLiteStorage) GetLogs(ctx context.Context, taskID string) ([]*core.LogEntry, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, task_id, timestamp, by, action, note, meta FROM task_logs WHERE task_id = ? ORDER BY timestamp DESC", taskID)
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
	_, err := s.db.ExecContext(ctx, "DELETE FROM tasks WHERE id = ?", id)
	return err
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
	if strings.ToLower(query.SortOrder) == "asc" {
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

func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}
