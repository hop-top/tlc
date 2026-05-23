package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"hop.top/kit/go/core/util"
	"hop.top/tlc/internal/core"
	_ "modernc.org/sqlite"
)

// parseRFC3339 parses an RFC3339 timestamp string, returning an error
// instead of silently producing a zero time on invalid input. Delegates
// to util.ParseStorageTime so the returned time.Time is always
// UTC-normalised regardless of any offset in the stored string.
func parseRFC3339(s string) (time.Time, error) {
	return util.ParseStorageTime(s)
}

// project calls projector.ProjectTask if a projector is set, logging errors.
func (s *SQLiteStorage) project(task *core.Task) {
	if s.projector != nil {
		if err := s.projector.ProjectTask(task); err != nil {
			log.Printf("projector: failed to project task %s: %v", task.ID, err)
		}
	}
}

// unproject calls projector.RemoveTask if a projector is set, logging errors.
func (s *SQLiteStorage) unproject(taskID string) {
	if s.projector != nil {
		if err := s.projector.RemoveTask(taskID); err != nil {
			log.Printf("projector: failed to remove task %s: %v", taskID, err)
		}
	}
}

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
	dbPath    string
	writeLock sync.Mutex
	projector Projector
}

// SetProjector attaches an optional Projector for filesystem projection.
// Nil projector = no-op (zero overhead when disabled).
func (s *SQLiteStorage) SetProjector(p Projector) {
	s.projector = p
}

func NewSQLiteStorage(path string) (*SQLiteStorage, error) {
	// One-time bootstrap: relocate any pre-existing db.*.bak files dropped
	// beside the live DB by older tlc versions into <dbDir>/.dbs/. Cheap
	// no-op when there are none.
	if path != "" {
		migrateOldBackups(filepath.Dir(path))
	}

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

	s := &SQLiteStorage{db: db, dbPath: path}
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
	if err := s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		metaJSON, _ := json.Marshal(task.Meta)
		tagsJSON, _ := json.Marshal(task.Tags)

		var lastSyncAt *string
		if task.LastSyncAt != nil {
			s := util.FormatStorageTime(*task.LastSyncAt)
			lastSyncAt = &s
		}

		var staleTimeout *int64
		if task.StaleTimeout != nil {
			ns := int64(*task.StaleTimeout)
			staleTimeout = &ns
		}
		var staleFiredAt *string
		if task.StaleFiredAt != nil {
			s := util.FormatStorageTime(*task.StaleFiredAt)
			staleFiredAt = &s
		}

		var dueAt *string
		if task.DueAt != nil {
			s := util.FormatStorageTime(*task.DueAt)
			dueAt = &s
		}
		var remindAt *string
		if task.RemindAt != nil {
			s := util.FormatStorageTime(*task.RemindAt)
			remindAt = &s
		}
		var rrule *string
		if task.RRule != "" {
			s := task.RRule
			rrule = &s
		}
		noAutoRemind := 0
		if task.NoAutoRemind {
			noAutoRemind = 1
		}

		// Coalesce nil ProjectID to empty string for consistent scoping.
		var projectID string
		if task.ProjectID != nil {
			projectID = *task.ProjectID
		}

		// Allocate a sequence number for the T-NNNN+ display alias if one
		// hasn't already been assigned. Allocation runs in the same write
		// transaction as the insert so concurrent creates can never collide
		// on (project_id, seq).
		if task.Seq == 0 {
			seq, err := allocSeqInTx(ctx, tx, projectID)
			if err != nil {
				return fmt.Errorf("failed to allocate task sequence: %w", err)
			}
			task.Seq = int64(seq)
		}

		_, err := tx.ExecContext(
			ctx, `
			INSERT INTO tasks (id, seq, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id, effort, priority, stale_timeout, blocked_reason, stale_fired_at, track_id, due_at, remind_at, rrule, no_auto_remind)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			task.ID, task.Seq, task.Title, task.Description, task.Status, task.AssignedTo, task.Reference,
			util.FormatStorageTime(task.CreatedAt), util.FormatStorageTime(task.UpdatedAt),
			string(metaJSON), string(tagsJSON), task.OriginSystem, lastSyncAt, task.Archived, projectID,
			string(task.Effort), string(task.Priority), staleTimeout, task.BlockedReason, staleFiredAt,
			task.TrackID, dueAt, remindAt, rrule, noAutoRemind,
		)
		if err != nil {
			return fmt.Errorf("failed to insert task: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	s.project(task)
	return nil
}

func (s *SQLiteStorage) GetTask(ctx context.Context, id string) (*core.Task, error) {
	proj := core.DetectProject()
	var row *sql.Row
	if proj != nil && proj.InProject && proj.ProjectID != "" {
		row = s.db.QueryRowContext(ctx, "SELECT id, seq, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id, effort, priority, stale_timeout, blocked_reason, stale_fired_at, track_id, due_at, remind_at, rrule, no_auto_remind FROM tasks WHERE id = ? AND project_id = ?", id, proj.ProjectID)
	} else {
		// Prefer the global bucket (project_id='') over project-scoped rows so that
		// tasks created outside any project context are consistently resolved even
		// when sync has duplicated them into project-specific rows.
		row = s.db.QueryRowContext(ctx, "SELECT id, seq, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id, effort, priority, stale_timeout, blocked_reason, stale_fired_at, track_id, due_at, remind_at, rrule, no_auto_remind FROM tasks WHERE id = ? ORDER BY CASE WHEN project_id = '' THEN 0 ELSE 1 END LIMIT 1", id)
	}

	var task core.Task
	var createdAtStr, updatedAtStr string
	var metaStr, tagsStr, originSystemStr, lastSyncAtStr, projectIDStr, effortStr, priorityStr sql.NullString
	var staleTimeoutNs sql.NullInt64
	var blockedReasonStr, staleFiredAtStr, trackIDStr sql.NullString
	var dueAtStr, remindAtStr, rruleStr sql.NullString
	var noAutoRemindInt sql.NullInt64

	err := row.Scan(&task.ID, &task.Seq, &task.Title, &task.Description, &task.Status, &task.AssignedTo, &task.Reference, &createdAtStr, &updatedAtStr, &metaStr, &tagsStr, &originSystemStr, &lastSyncAtStr, &task.Archived, &projectIDStr, &effortStr, &priorityStr, &staleTimeoutNs, &blockedReasonStr, &staleFiredAtStr, &trackIDStr, &dueAtStr, &remindAtStr, &rruleStr, &noAutoRemindInt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan task row: %w", err)
	}

	task.CreatedAt, _ = util.ParseStorageTime(createdAtStr)
	task.UpdatedAt, _ = util.ParseStorageTime(updatedAtStr)

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
		t, _ := util.ParseStorageTime(lastSyncAtStr.String)
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
		t, _ := util.ParseStorageTime(staleFiredAtStr.String)
		task.StaleFiredAt = &t
	}
	if trackIDStr.Valid {
		task.TrackID = &trackIDStr.String
	}
	if dueAtStr.Valid {
		t, _ := util.ParseStorageTime(dueAtStr.String)
		task.DueAt = &t
	}
	if remindAtStr.Valid {
		t, _ := util.ParseStorageTime(remindAtStr.String)
		task.RemindAt = &t
	}
	if rruleStr.Valid {
		task.RRule = rruleStr.String
	}
	if noAutoRemindInt.Valid && noAutoRemindInt.Int64 != 0 {
		task.NoAutoRemind = true
	}

	return &task, nil
}

// TaskIDExists reports whether any row exists with the given task ID,
// regardless of project_id. Useful for sync-projection paths that need
// to detect "id already known" without caring about which project bucket
// it lives in (the tasks.id column is a global PRIMARY KEY).
func (s *SQLiteStorage) TaskIDExists(ctx context.Context, id string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(
		ctx, "SELECT 1 FROM tasks WHERE id = ? LIMIT 1", id,
	).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to probe task id %q: %w", id, err)
	}
	return true, nil
}

func (s *SQLiteStorage) GetTaskInProject(ctx context.Context, id, projectID string) (*core.Task, error) {
	row := s.db.QueryRowContext(
		ctx,
		"SELECT id, seq, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id, effort, priority, stale_timeout, blocked_reason, stale_fired_at, track_id, due_at, remind_at, rrule, no_auto_remind FROM tasks WHERE id = ? AND project_id = ?",
		id, projectID,
	)

	var task core.Task
	var createdAtStr, updatedAtStr string
	var metaStr, tagsStr, originSystemStr, lastSyncAtStr, projectIDStr, effortStr, priorityStr sql.NullString
	var staleTimeoutNs sql.NullInt64
	var blockedReasonStr, staleFiredAtStr, trackIDStr sql.NullString
	var dueAtStr, remindAtStr, rruleStr sql.NullString
	var noAutoRemindInt sql.NullInt64

	err := row.Scan(&task.ID, &task.Seq, &task.Title, &task.Description, &task.Status, &task.AssignedTo, &task.Reference, &createdAtStr, &updatedAtStr, &metaStr, &tagsStr, &originSystemStr, &lastSyncAtStr, &task.Archived, &projectIDStr, &effortStr, &priorityStr, &staleTimeoutNs, &blockedReasonStr, &staleFiredAtStr, &trackIDStr, &dueAtStr, &remindAtStr, &rruleStr, &noAutoRemindInt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan task row: %w", err)
	}

	task.CreatedAt, _ = util.ParseStorageTime(createdAtStr)
	task.UpdatedAt, _ = util.ParseStorageTime(updatedAtStr)

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
		t, _ := util.ParseStorageTime(lastSyncAtStr.String)
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
		t, _ := util.ParseStorageTime(staleFiredAtStr.String)
		task.StaleFiredAt = &t
	}
	if trackIDStr.Valid {
		task.TrackID = &trackIDStr.String
	}
	if dueAtStr.Valid {
		t, _ := util.ParseStorageTime(dueAtStr.String)
		task.DueAt = &t
	}
	if remindAtStr.Valid {
		t, _ := util.ParseStorageTime(remindAtStr.String)
		task.RemindAt = &t
	}
	if rruleStr.Valid {
		task.RRule = rruleStr.String
	}
	if noAutoRemindInt.Valid && noAutoRemindInt.Int64 != 0 {
		task.NoAutoRemind = true
	}

	return &task, nil
}

// GetTaskBySeq retrieves a task by its (project_id, seq) display alias.
// Uses the same code path as GetTask once the typeid is resolved so
// query semantics, column scanning, and project scoping stay consistent.
//
// projectID is matched against tasks.project_id literally — empty string
// hits the global bucket; non-empty matches that exact project. Note that
// task_sequences uses "default" as the storage key for the empty-project
// bucket but tasks.project_id stores empty string, so seq lookup queries
// the latter directly.
func (s *SQLiteStorage) GetTaskBySeq(ctx context.Context, projectID string, seq int64) (*core.Task, error) {
	var id string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id FROM tasks WHERE project_id = ? AND seq = ?`,
		projectID, seq,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup task by seq: %w", err)
	}
	return s.GetTask(ctx, id)
}

func (s *SQLiteStorage) UpdateTask(ctx context.Context, task *core.Task) error {
	if err := s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		metaJSON, _ := json.Marshal(task.Meta)
		tagsJSON, _ := json.Marshal(task.Tags)

		var lastSyncAt *string
		if task.LastSyncAt != nil {
			s := util.FormatStorageTime(*task.LastSyncAt)
			lastSyncAt = &s
		}

		var staleTimeout *int64
		if task.StaleTimeout != nil {
			ns := int64(*task.StaleTimeout)
			staleTimeout = &ns
		}
		var staleFiredAt *string
		if task.StaleFiredAt != nil {
			s := util.FormatStorageTime(*task.StaleFiredAt)
			staleFiredAt = &s
		}

		var dueAt *string
		if task.DueAt != nil {
			s := util.FormatStorageTime(*task.DueAt)
			dueAt = &s
		}
		var remindAt *string
		if task.RemindAt != nil {
			s := util.FormatStorageTime(*task.RemindAt)
			remindAt = &s
		}
		var rrule *string
		if task.RRule != "" {
			s := task.RRule
			rrule = &s
		}
		noAutoRemind := 0
		if task.NoAutoRemind {
			noAutoRemind = 1
		}

		projectID := ""
		if task.ProjectID != nil {
			projectID = *task.ProjectID
		}
		res, err := tx.ExecContext(
			ctx, `
			UPDATE tasks SET title = ?, description = ?, status = ?, assigned_to = ?, reference = ?, updated_at = ?, meta = ?, tags = ?, origin_system = ?, last_sync_at = ?, archived = ?, effort = ?, priority = ?, stale_timeout = ?, blocked_reason = ?, stale_fired_at = ?, track_id = ?, due_at = ?, remind_at = ?, rrule = ?, no_auto_remind = ?
			WHERE id = ? AND project_id = ?`,
			task.Title, task.Description, task.Status, task.AssignedTo, task.Reference,
			util.FormatStorageTime(task.UpdatedAt), string(metaJSON), string(tagsJSON),
			task.OriginSystem, lastSyncAt, task.Archived, string(task.Effort), string(task.Priority),
			staleTimeout, task.BlockedReason, staleFiredAt, task.TrackID,
			dueAt, remindAt, rrule, noAutoRemind, task.ID, projectID,
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
	}); err != nil {
		return err
	}

	s.project(task)
	return nil
}

func (s *SQLiteStorage) UpdateTaskWithLog(ctx context.Context, task *core.Task, entry *core.LogEntry) error {
	if err := s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		// Update Task
		metaJSON, _ := json.Marshal(task.Meta)
		tagsJSON, _ := json.Marshal(task.Tags)
		var lastSyncAt *string
		if task.LastSyncAt != nil {
			s := util.FormatStorageTime(*task.LastSyncAt)
			lastSyncAt = &s
		}

		var staleTimeout *int64
		if task.StaleTimeout != nil {
			ns := int64(*task.StaleTimeout)
			staleTimeout = &ns
		}
		var staleFiredAt *string
		if task.StaleFiredAt != nil {
			s := util.FormatStorageTime(*task.StaleFiredAt)
			staleFiredAt = &s
		}

		var dueAt *string
		if task.DueAt != nil {
			s := util.FormatStorageTime(*task.DueAt)
			dueAt = &s
		}
		var remindAt *string
		if task.RemindAt != nil {
			s := util.FormatStorageTime(*task.RemindAt)
			remindAt = &s
		}
		var rrule *string
		if task.RRule != "" {
			s := task.RRule
			rrule = &s
		}
		noAutoRemind := 0
		if task.NoAutoRemind {
			noAutoRemind = 1
		}

		projectID := ""
		if task.ProjectID != nil {
			projectID = *task.ProjectID
		}
		res, err := tx.ExecContext(
			ctx, `
			UPDATE tasks SET title = ?, description = ?, status = ?, assigned_to = ?, reference = ?, updated_at = ?, meta = ?, tags = ?, origin_system = ?, last_sync_at = ?, archived = ?, effort = ?, priority = ?, stale_timeout = ?, blocked_reason = ?, stale_fired_at = ?, track_id = ?, due_at = ?, remind_at = ?, rrule = ?, no_auto_remind = ?
			WHERE id = ? AND project_id = ?`,
			task.Title, task.Description, task.Status, task.AssignedTo, task.Reference,
			util.FormatStorageTime(task.UpdatedAt), string(metaJSON), string(tagsJSON),
			task.OriginSystem, lastSyncAt, task.Archived, string(task.Effort), string(task.Priority),
			staleTimeout, task.BlockedReason, staleFiredAt, task.TrackID,
			dueAt, remindAt, rrule, noAutoRemind, task.ID, projectID,
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

		_, err = tx.ExecContext(
			ctx, `
			INSERT INTO task_logs (project_id, task_id, timestamp, by, action, note, meta)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			projectID, entry.TaskID, util.FormatStorageTime(entry.Timestamp), entry.By, entry.Action, entry.Note, string(logMetaJSON),
		)
		if err != nil {
			return fmt.Errorf("failed to insert log entry: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	s.project(task)
	return nil
}

// buildFilterClauses converts FieldFilter groups into SQL WHERE clauses and args.
func buildFilterClauses(filters []core.FieldFilter) ([]string, []any) {
	fieldGroups := make(map[string][]core.FieldFilter)
	for _, f := range filters {
		fieldGroups[f.Field] = append(fieldGroups[f.Field], f)
	}

	var whereClauses []string
	var args []any
	for field, group := range fieldGroups {
		var groupClauses []string
		for _, f := range group {
			if field == "tags" && f.Operator == core.OpContains {
				groupClauses = append(groupClauses,
					"EXISTS (SELECT 1 FROM json_each(tasks.tags) WHERE value = ?)")
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
				groupClauses = append(groupClauses,
					fmt.Sprintf("%s IN (%s)", field, strings.Join(placeholders, ",")))
				continue
			}

			if f.Value == nil {
				if op == "=" {
					groupClauses = append(groupClauses,
						fmt.Sprintf("(%s IS NULL OR %s = '')", field, field))
				} else {
					groupClauses = append(groupClauses,
						fmt.Sprintf("(%s IS NOT NULL AND %s != '')", field, field))
				}
			} else {
				groupClauses = append(groupClauses, fmt.Sprintf("%s %s ?", field, op))
				args = append(args, f.Value)
			}
		}
		if len(groupClauses) > 0 {
			whereClauses = append(whereClauses,
				"("+strings.Join(groupClauses, " OR ")+")")
		}
	}
	return whereClauses, args
}

// scanLogEntries iterates sql.Rows and returns parsed LogEntry slices.
func scanLogEntries(rows *sql.Rows) ([]*core.LogEntry, error) {
	var entries []*core.LogEntry
	for rows.Next() {
		var entry core.LogEntry
		var timestampStr string
		var metaStr sql.NullString
		if err := rows.Scan(
			&entry.ID, &entry.TaskID, &timestampStr,
			&entry.By, &entry.Action, &entry.Note, &metaStr,
		); err != nil {
			return nil, fmt.Errorf("failed to scan log row: %w", err)
		}
		entry.Timestamp, _ = util.ParseStorageTime(timestampStr)
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

func (s *SQLiteStorage) ListTasks(ctx context.Context, query core.Query) ([]*core.Task, error) {
	sqlQuery := "SELECT id, seq, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id, effort, priority, stale_timeout, blocked_reason, stale_fired_at, track_id, due_at, remind_at, rrule, no_auto_remind FROM tasks"

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

	// Temporal filters (T-0908). due_at is stored as RFC3339 UTC TEXT
	// per docs/temporal-spec-0.1.md §4. RFC3339's lexicographic byte
	// ordering matches chronological ordering when all values share the
	// same offset (writes use UTC with the `Z` suffix uniformly — see
	// the INSERT/UPDATE paths above), so a string `<` / `>` against an
	// RFC3339 literal is a correct chronological compare.
	if query.DueBefore != nil {
		whereClauses = append(whereClauses, "due_at IS NOT NULL AND due_at < ?")
		args = append(args, query.DueBefore.UTC().Format(time.RFC3339))
	}
	if query.DueAfter != nil {
		whereClauses = append(whereClauses, "due_at IS NOT NULL AND due_at > ?")
		args = append(args, query.DueAfter.UTC().Format(time.RFC3339))
	}
	if query.HasDue != nil {
		if *query.HasDue {
			whereClauses = append(whereClauses, "due_at IS NOT NULL AND due_at != ''")
		} else {
			whereClauses = append(whereClauses, "(due_at IS NULL OR due_at = '')")
		}
	}

	if len(whereClauses) > 0 {
		sqlQuery += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Sorting
	var orderParts []string
	if query.StatusPriority != "" {
		orderParts = append(orderParts,
			"CASE WHEN status = ? THEN 0 ELSE 1 END")
		args = append(args, query.StatusPriority)
	}
	if query.SortBy != "" {
		order := sqlOrderASC
		if strings.ToLower(query.SortDirection) == "desc" {
			order = sqlOrderDESC
		}
		orderParts = append(orderParts,
			fmt.Sprintf("%s %s", query.SortBy, order)) //nolint:gosec // G202: SortBy is validated against known columns
	} else {
		orderParts = append(orderParts, "created_at DESC")
	}
	sqlQuery += " ORDER BY " + strings.Join(orderParts, ", ")

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

		_, err := tx.ExecContext(
			ctx, `
			INSERT INTO task_logs (project_id, task_id, timestamp, by, action, note, meta)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			projectID, entry.TaskID, util.FormatStorageTime(entry.Timestamp), entry.By, entry.Action, entry.Note, string(metaJSON),
		)
		if err != nil {
			return fmt.Errorf("failed to insert log entry: %w", err)
		}
		return nil
	})
}

// UpdateLogNote rewrites the note and meta of an existing log entry.
// Used by `tlc task update --amend` to edit the most recent log in
// place. Returns an error if the log row does not exist.
func (s *SQLiteStorage) UpdateLogNote(ctx context.Context, logID int64, note string, meta map[string]any) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		metaJSON, err := json.Marshal(meta)
		if err != nil {
			return fmt.Errorf("failed to marshal log meta: %w", err)
		}
		res, err := tx.ExecContext(
			ctx, `
			UPDATE task_logs SET note = ?, meta = ? WHERE id = ?`,
			note, string(metaJSON), logID,
		)
		if err != nil {
			return fmt.Errorf("failed to update log entry: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to read rows affected: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("log entry %d not found", logID)
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

	return scanLogEntries(rows)
}

func (s *SQLiteStorage) DeleteTask(ctx context.Context, id string) error {
	if err := s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
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
	}); err != nil {
		return err
	}

	s.unproject(id)
	return nil
}

func (s *SQLiteStorage) FindTaskByOrigin(ctx context.Context, system, originID string) (*core.Task, error) {
	sqlQuery := `
		SELECT id, seq, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id, effort, priority, stale_timeout, blocked_reason, stale_fired_at, track_id, due_at, remind_at, rrule, no_auto_remind
		FROM tasks
		WHERE origin_system = ? AND json_extract(meta, '$.origin_id') = ?
		LIMIT 1
	`
	row := s.db.QueryRowContext(ctx, sqlQuery, system, originID)

	var task core.Task
	var createdAtStr, updatedAtStr string
	var metaStr, tagsStr, originSystemStr, lastSyncAtStr, projectIDStr, effortStr, priorityStr sql.NullString
	var staleTimeoutNs sql.NullInt64
	var blockedReasonStr, staleFiredAtStr, trackIDStr sql.NullString
	var dueAtStr, remindAtStr, rruleStr sql.NullString
	var noAutoRemindInt sql.NullInt64

	err := row.Scan(&task.ID, &task.Seq, &task.Title, &task.Description, &task.Status, &task.AssignedTo, &task.Reference, &createdAtStr, &updatedAtStr, &metaStr, &tagsStr, &originSystemStr, &lastSyncAtStr, &task.Archived, &projectIDStr, &effortStr, &priorityStr, &staleTimeoutNs, &blockedReasonStr, &staleFiredAtStr, &trackIDStr, &dueAtStr, &remindAtStr, &rruleStr, &noAutoRemindInt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan task by origin: %w", err)
	}

	task.CreatedAt, _ = util.ParseStorageTime(createdAtStr)
	task.UpdatedAt, _ = util.ParseStorageTime(updatedAtStr)

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
		t, _ := util.ParseStorageTime(lastSyncAtStr.String)
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
		t, _ := util.ParseStorageTime(staleFiredAtStr.String)
		task.StaleFiredAt = &t
	}
	if trackIDStr.Valid {
		task.TrackID = &trackIDStr.String
	}
	if dueAtStr.Valid {
		t, _ := util.ParseStorageTime(dueAtStr.String)
		task.DueAt = &t
	}
	if remindAtStr.Valid {
		t, _ := util.ParseStorageTime(remindAtStr.String)
		task.RemindAt = &t
	}
	if rruleStr.Valid {
		task.RRule = rruleStr.String
	}
	if noAutoRemindInt.Valid && noAutoRemindInt.Int64 != 0 {
		task.NoAutoRemind = true
	}

	return &task, nil
}

func (s *SQLiteStorage) ArchiveTasks(ctx context.Context, threshold time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-threshold).Format(time.RFC3339)

	var count int64
	err := s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(
			ctx, `
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
		SELECT id, seq, title, description, status, assigned_to, reference, created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, project_id, effort, priority, stale_timeout, blocked_reason, stale_fired_at, track_id, due_at, remind_at, rrule, no_auto_remind
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
		_, err := tx.ExecContext(
			ctx, `
			INSERT INTO flow_runs (id, flow_id, status, started_at, ended_at, results)
			VALUES (?, ?, ?, ?, ?, ?)`,
			run.ID, run.FlowID, run.Status, util.FormatStorageTime(run.StartedAt),
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

	run.StartedAt, _ = util.ParseStorageTime(startedAtStr)
	if endedAtStr.Valid {
		t, _ := util.ParseStorageTime(endedAtStr.String)
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
			endedAt = util.FormatStorageTime(*run.EndedAt)
		}

		_, err := tx.ExecContext(
			ctx, `
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
		run.StartedAt, _ = util.ParseStorageTime(startedAtStr)
		if endedAtStr.Valid {
			t, _ := util.ParseStorageTime(endedAtStr.String)
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

// deleteFlowRun removes a flow run by ID.
func (s *SQLiteStorage) deleteFlowRun(ctx context.Context, id string) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "DELETE FROM flow_runs WHERE id = ?", id)
		if err != nil {
			return fmt.Errorf("failed to delete flow run: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return fmt.Errorf("flow run %s not found", id)
		}
		return nil
	})
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

	// Temporal filters (T-1381). task_logs.timestamp is stored as
	// RFC3339 TEXT (see AddLog and UpdateTaskWithLog write paths).
	// util.FormatStorageTime emits UTC RFC3339, which — combined with
	// T-1383's sweep that routes every write through the same primitive
	// — yields a lexicographically comparable boundary value safe for
	// `>=` / `<=` against the column.
	if query.Since != nil {
		whereClauses = append(whereClauses, "timestamp >= ?")
		args = append(args, util.FormatStorageTime(*query.Since))
	}
	if query.Until != nil {
		whereClauses = append(whereClauses, "timestamp <= ?")
		args = append(args, util.FormatStorageTime(*query.Until))
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

	return scanLogEntries(rows)
}

func (s *SQLiteStorage) GetNextSequenceID(ctx context.Context, projectID string) (int, error) {
	var nextID int
	err := s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		var err error
		nextID, err = allocSeqInTx(ctx, tx, projectID)
		return err
	})
	return nextID, err
}

// allocSeqInTx reserves the next monotonically increasing sequence number
// for projectID inside the given transaction. The seq → display-alias mapping
// is "T-<seq>" with at-least-4-digit zero-padding (e.g. T-0042, T-12345).
//
// Empty projectID is normalized to "default" so the global bucket has its
// own counter row. The increment must happen in the same tx as the INSERT
// that consumes the value to avoid (project_id, seq) collisions under
// concurrent creates.
func allocSeqInTx(ctx context.Context, tx *sql.Tx, projectID string) (int, error) {
	if projectID == "" {
		projectID = "default"
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO task_sequences (project_id, next_id)
		VALUES (?, 1)
	`, projectID); err != nil {
		return 0, fmt.Errorf("ensure sequence row: %w", err)
	}

	var nextID int
	if err := tx.QueryRowContext(
		ctx,
		`SELECT next_id FROM task_sequences WHERE project_id = ?`, projectID,
	).Scan(&nextID); err != nil {
		return 0, fmt.Errorf("read sequence: %w", err)
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE task_sequences SET next_id = next_id + 1 WHERE project_id = ?`, projectID,
	); err != nil {
		return 0, fmt.Errorf("bump sequence: %w", err)
	}

	return nextID, nil
}

func scanTaskFromRow(rows *sql.Rows) (*core.Task, error) {
	var task core.Task
	var createdAtStr, updatedAtStr string
	var metaStr, tagsStr, originSystemStr, lastSyncAtStr, projectIDStr, effortStr, priorityStr sql.NullString
	var staleTimeoutNs sql.NullInt64
	var blockedReasonStr, staleFiredAtStr, trackIDStr sql.NullString
	var dueAtStr, remindAtStr, rruleStr sql.NullString
	var noAutoRemindInt sql.NullInt64
	if err := rows.Scan(&task.ID, &task.Seq, &task.Title, &task.Description, &task.Status, &task.AssignedTo, &task.Reference, &createdAtStr, &updatedAtStr, &metaStr, &tagsStr, &originSystemStr, &lastSyncAtStr, &task.Archived, &projectIDStr, &effortStr, &priorityStr, &staleTimeoutNs, &blockedReasonStr, &staleFiredAtStr, &trackIDStr, &dueAtStr, &remindAtStr, &rruleStr, &noAutoRemindInt); err != nil {
		return nil, fmt.Errorf("failed to scan task row: %w", err)
	}
	task.CreatedAt, _ = util.ParseStorageTime(createdAtStr)
	task.UpdatedAt, _ = util.ParseStorageTime(updatedAtStr)
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
		t, _ := util.ParseStorageTime(lastSyncAtStr.String)
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
		t, _ := util.ParseStorageTime(staleFiredAtStr.String)
		task.StaleFiredAt = &t
	}
	if trackIDStr.Valid {
		task.TrackID = &trackIDStr.String
	}
	if dueAtStr.Valid {
		t, _ := util.ParseStorageTime(dueAtStr.String)
		task.DueAt = &t
	}
	if remindAtStr.Valid {
		t, _ := util.ParseStorageTime(remindAtStr.String)
		task.RemindAt = &t
	}
	if rruleStr.Valid {
		task.RRule = rruleStr.String
	}
	if noAutoRemindInt.Valid && noAutoRemindInt.Int64 != 0 {
		task.NoAutoRemind = true
	}
	return &task, nil
}

func (s *SQLiteStorage) RegisterProject(ctx context.Context, projectID, dbPath, spaceURI, label string) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		_, err := tx.ExecContext(
			ctx, `
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
	row := s.db.QueryRowContext(
		ctx,
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

	p.RegisteredAt, _ = util.ParseStorageTime(registeredAtStr)
	p.LastSeenAt, _ = util.ParseStorageTime(lastSeenAtStr)
	if spaceURI.Valid {
		p.SpaceURI = spaceURI.String
	}
	if label.Valid {
		p.Label = label.String
	}

	return &p, nil
}

func (s *SQLiteStorage) ListProjectsBySpace(ctx context.Context, spaceURI string) ([]core.RegisteredProject, error) {
	rows, err := s.db.QueryContext(
		ctx,
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
	rows, err := s.db.QueryContext(
		ctx,
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
		_, err := tx.ExecContext(
			ctx,
			"UPDATE projects SET db_path = ?, last_seen_at = ? WHERE project_id = ?",
			newDBPath, now, projectID,
		)
		if err != nil {
			return fmt.Errorf("failed to update project path: %w", err)
		}
		return nil
	})
}

func (s *SQLiteStorage) DeleteProject(ctx context.Context, projectID string) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM projects WHERE project_id = ?", projectID)
		if err != nil {
			return fmt.Errorf("failed to delete project: %w", err)
		}
		return nil
	})
}

// ResolveProjectByShortname finds a project by shortname. It first
// tries an exact match on project_id, then falls back to matching
// the last path segment (e.g. "tlc" matches "hop-top/tlc"). If
// multiple projects match the suffix, returns an ambiguity error.
func (s *SQLiteStorage) ResolveProjectByShortname(ctx context.Context, shortname string) (*core.RegisteredProject, error) {
	// 1. Exact match
	p, err := s.LookupProject(ctx, shortname)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return p, nil
	}

	// 2. Suffix match: project_id ends with "/<shortname>"
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT project_id, db_path, space_uri, label, registered_at, last_seen_at, status
		 FROM projects
		 WHERE project_id LIKE '%/' || ? AND status = 'active'`,
		shortname,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query projects by shortname: %w", err)
	}
	defer func() { _ = rows.Close() }()

	matches, err := scanProjects(rows)
	if err != nil {
		return nil, err
	}

	switch len(matches) {
	case 0:
		return nil, nil
	case 1:
		return &matches[0], nil
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ProjectID
		}
		return nil, fmt.Errorf(
			"ambiguous shortname %q matches %d projects: %s; use full project ID",
			shortname, len(matches), strings.Join(ids, ", "),
		)
	}
}

func (s *SQLiteStorage) TouchProject(ctx context.Context, projectID string) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		_, err := tx.ExecContext(
			ctx,
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

		p.RegisteredAt, _ = util.ParseStorageTime(registeredAtStr)
		p.LastSeenAt, _ = util.ParseStorageTime(lastSeenAtStr)
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
