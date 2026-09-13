package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"hop.top/kit/go/core/util"
	"hop.top/tlc/internal/core"
)

// taskColumns is the single column list for reading and inserting tasks.
// Every reader scans in exactly this order through scanTask and every
// writer binds it through taskRow, so a column added here is added
// everywhere at once. New columns go at the end so tests that pin column
// positions keep passing.
const taskColumns = "id, seq, title, description, status, assigned_to, reference, " +
	"created_at, updated_at, meta, tags, origin_system, last_sync_at, archived, " +
	"project_id, effort, priority, stale_timeout, blocked_reason, stale_fired_at, " +
	"track_id, due_at, remind_at, rrule, no_auto_remind, " +
	"kind, attempts, claimed_at, run_id, step_id, step_ordinal, spec, result"

// taskColumnCount is the number of entries in taskColumns. BindTask's
// test pins it so the bind and the select list cannot drift apart.
const taskColumnCount = 33

// taskPlaceholders is the VALUES list matching taskColumns.
var taskPlaceholders = strings.TrimSuffix(strings.Repeat("?, ", taskColumnCount), ", ")

// taskUpdateSet is the UPDATE assignment list: taskColumns minus the
// identity and creation columns (id, seq, created_at, project_id), which
// never change after insert.
const taskUpdateSet = "title = ?, description = ?, status = ?, assigned_to = ?, reference = ?, " +
	"updated_at = ?, meta = ?, tags = ?, origin_system = ?, last_sync_at = ?, archived = ?, " +
	"effort = ?, priority = ?, stale_timeout = ?, blocked_reason = ?, stale_fired_at = ?, " +
	"track_id = ?, due_at = ?, remind_at = ?, rrule = ?, no_auto_remind = ?, " +
	"kind = ?, attempts = ?, claimed_at = ?, run_id = ?, step_id = ?, step_ordinal = ?, " +
	"spec = ?, result = ?"

// scanner abstracts sql.Row and sql.Rows behind a single Scan method.
type scanner interface {
	Scan(dest ...any) error
}

// taskRow holds a task's column values encoded the way SQLite stores
// them: timestamps as UTC RFC3339 text, optional fields as NULL rather
// than "" or 0, JSON documents as text.
type taskRow struct {
	projectID    string
	metaJSON     string
	tagsJSON     string
	lastSyncAt   *string
	staleTimeout *int64
	staleFiredAt *string
	dueAt        *string
	remindAt     *string
	rrule        *string
	noAutoRemind int
	kind         string
	claimedAt    *string
	runID        *string
	stepID       *string
	stepOrdinal  *int64
	specJSON     *string
	resultJSON   *string
}

// encodeTask prepares every column value for task. Kind is made explicit
// here so the NOT NULL column never sees the struct's "" default.
func encodeTask(task *core.Task) taskRow {
	r := taskRow{
		metaJSON:     jsonText(task.Meta),
		tagsJSON:     jsonText(task.Tags),
		lastSyncAt:   optTime(task.LastSyncAt),
		staleFiredAt: optTime(task.StaleFiredAt),
		dueAt:        optTime(task.DueAt),
		remindAt:     optTime(task.RemindAt),
		rrule:        optString(task.RRule),
		kind:         string(task.EffectiveKind()),
		claimedAt:    optTime(task.ClaimedAt),
		runID:        optString(task.RunID),
		stepID:       optString(task.StepID),
	}
	if task.ProjectID != nil {
		r.projectID = *task.ProjectID
	}
	if task.StaleTimeout != nil {
		ns := int64(*task.StaleTimeout)
		r.staleTimeout = &ns
	}
	if task.NoAutoRemind {
		r.noAutoRemind = 1
	}
	if task.StepOrdinal != 0 {
		n := int64(task.StepOrdinal)
		r.stepOrdinal = &n
	}
	if task.Spec != nil {
		r.specJSON = optString(jsonText(task.Spec))
	}
	if len(task.Result) != 0 {
		r.resultJSON = optString(jsonText(task.Result))
	}
	return r
}

// insertArgs returns the values in taskColumns order. It reads task.Seq
// at call time so a sequence allocated after encodeTask is picked up.
func (r taskRow) insertArgs(task *core.Task) []any {
	return []any{
		task.ID, task.Seq, task.Title, task.Description, task.Status, task.AssignedTo, task.Reference,
		util.FormatStorageTime(task.CreatedAt), util.FormatStorageTime(task.UpdatedAt),
		r.metaJSON, r.tagsJSON, task.OriginSystem, r.lastSyncAt, task.Archived,
		r.projectID, string(task.Effort), string(task.Priority), r.staleTimeout, task.BlockedReason, r.staleFiredAt,
		task.TrackID, r.dueAt, r.remindAt, r.rrule, r.noAutoRemind,
		r.kind, task.Attempts, r.claimedAt, r.runID, r.stepID, r.stepOrdinal, r.specJSON, r.resultJSON,
	}
}

// updateArgs returns the values in taskUpdateSet order followed by the
// WHERE keys (id, project_id).
func (r taskRow) updateArgs(task *core.Task) []any {
	return []any{
		task.Title, task.Description, task.Status, task.AssignedTo, task.Reference,
		util.FormatStorageTime(task.UpdatedAt), r.metaJSON, r.tagsJSON,
		task.OriginSystem, r.lastSyncAt, task.Archived,
		string(task.Effort), string(task.Priority), r.staleTimeout, task.BlockedReason, r.staleFiredAt,
		task.TrackID, r.dueAt, r.remindAt, r.rrule, r.noAutoRemind,
		r.kind, task.Attempts, r.claimedAt, r.runID, r.stepID, r.stepOrdinal,
		r.specJSON, r.resultJSON,
		task.ID, r.projectID,
	}
}

// jsonText encodes v for a TEXT column. The task documents (Meta, Tags,
// Spec, Result) are decoded JSON and plain structs, which cannot fail to
// encode; the writers this replaces discarded the error the same way.
func jsonText(v any) string {
	b, _ := json.Marshal(v) //nolint:errcheck // see doc comment
	return string(b)
}

func optTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := util.FormatStorageTime(*t)
	return &s
}

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// updateTaskInTx writes every mutable task column and confirms the row
// exists; an unchanged row is not an error. Shared by UpdateTask and
// UpdateTaskWithLog so the two write paths cannot drift.
func updateTaskInTx(ctx context.Context, tx *sql.Tx, task *core.Task) error {
	row := encodeTask(task)
	res, err := tx.ExecContext(
		ctx, "UPDATE tasks SET "+taskUpdateSet+" WHERE id = ? AND project_id = ?",
		row.updateArgs(task)...,
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
		err = tx.QueryRowContext(
			ctx,
			`SELECT 1 FROM tasks WHERE id = ? AND project_id = ? LIMIT 1`, task.ID, row.projectID,
		).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task %s not found (project_id=%q)", task.ID, row.projectID)
		}
		if err != nil {
			return fmt.Errorf("failed to verify task existence: %w", err)
		}
		// row exists, values were unchanged — not an error
	}
	return nil
}

// taskScanCols receives the raw column values of one tasks row, in
// taskColumns order, before they are decoded onto a core.Task.
type taskScanCols struct {
	createdAt, updatedAt, kind                                        string
	attempts                                                          int64
	meta, tags, originSystem, lastSyncAt, projectID, effort, priority sql.NullString
	staleTimeout                                                      sql.NullInt64
	blockedReason, staleFiredAt, trackID                              sql.NullString
	dueAt, remindAt, rrule                                            sql.NullString
	noAutoRemind                                                      sql.NullInt64
	claimedAt, runID, stepID, spec, result                            sql.NullString
	stepOrdinal                                                       sql.NullInt64
}

// dests returns the Scan targets in taskColumns order; the non-nullable
// scalars land directly on task.
func (c *taskScanCols) dests(task *core.Task) []any {
	return []any{
		&task.ID, &task.Seq, &task.Title, &task.Description, &task.Status, &task.AssignedTo, &task.Reference,
		&c.createdAt, &c.updatedAt, &c.meta, &c.tags, &c.originSystem, &c.lastSyncAt, &task.Archived,
		&c.projectID, &c.effort, &c.priority, &c.staleTimeout, &c.blockedReason, &c.staleFiredAt,
		&c.trackID, &c.dueAt, &c.remindAt, &c.rrule, &c.noAutoRemind,
		&c.kind, &c.attempts, &c.claimedAt, &c.runID, &c.stepID, &c.stepOrdinal, &c.spec, &c.result,
	}
}

// applyDocuments decodes the JSON columns. These are the only decode
// steps that can fail, and they fail the whole read.
func (c *taskScanCols) applyDocuments(task *core.Task) error {
	if c.meta.Valid {
		if err := json.Unmarshal([]byte(c.meta.String), &task.Meta); err != nil {
			return fmt.Errorf("failed to unmarshal meta: %w", err)
		}
	}
	if c.tags.Valid {
		if err := json.Unmarshal([]byte(c.tags.String), &task.Tags); err != nil {
			return fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}
	if c.spec.Valid {
		var spec core.TaskSpec
		if err := json.Unmarshal([]byte(c.spec.String), &spec); err != nil {
			return fmt.Errorf("failed to unmarshal spec: %w", err)
		}
		task.Spec = &spec
	}
	if c.result.Valid {
		if err := json.Unmarshal([]byte(c.result.String), &task.Result); err != nil {
			return fmt.Errorf("failed to unmarshal result: %w", err)
		}
	}
	return nil
}

// applyScalars copies the remaining columns. A NULL string column reads
// as nil/"" and a NULL integer as 0, which is what each field's zero
// value already means.
func (c *taskScanCols) applyScalars(task *core.Task) {
	task.CreatedAt = lenientTime(c.createdAt)
	task.UpdatedAt = lenientTime(c.updatedAt)
	task.OriginSystem = nullStr(c.originSystem)
	task.LastSyncAt = nullTime(c.lastSyncAt)
	task.ProjectID = nullStr(c.projectID)
	task.Effort = core.Effort(c.effort.String)
	task.Priority = core.Priority(c.priority.String)
	if c.staleTimeout.Valid {
		d := time.Duration(c.staleTimeout.Int64)
		task.StaleTimeout = &d
	}
	task.BlockedReason = nullStr(c.blockedReason)
	task.StaleFiredAt = nullTime(c.staleFiredAt)
	task.TrackID = nullStr(c.trackID)
	task.DueAt = nullTime(c.dueAt)
	task.RemindAt = nullTime(c.remindAt)
	task.RRule = c.rrule.String
	task.NoAutoRemind = c.noAutoRemind.Int64 != 0

	task.Kind = core.TaskKind(c.kind)
	task.Attempts = int(c.attempts)
	task.ClaimedAt = nullTime(c.claimedAt)
	task.RunID = c.runID.String
	task.StepID = c.stepID.String
	task.StepOrdinal = int(c.stepOrdinal.Int64)
}

// scanTask reads one row selected with taskColumns. sql.ErrNoRows passes
// through unwrapped so callers can map it to (nil, nil).
func scanTask(sc scanner) (*core.Task, error) {
	var task core.Task
	var cols taskScanCols
	if err := sc.Scan(cols.dests(&task)...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err //nolint:wrapcheck // sentinel passes through so callers map it to (nil, nil)
		}
		return nil, fmt.Errorf("failed to scan task row: %w", err)
	}
	if err := cols.applyDocuments(&task); err != nil {
		return nil, err
	}
	cols.applyScalars(&task)
	return &task, nil
}

// lenientTime parses a stored timestamp, yielding the zero time for an
// unparsable value — the readers this replaces never surfaced that error,
// and a task must still load when one optional timestamp is corrupt.
func lenientTime(s string) time.Time {
	t, _ := util.ParseStorageTime(s) //nolint:errcheck // see doc comment
	return t
}

// nullTime reads an optional stored timestamp: NULL is nil, anything else
// goes through lenientTime.
func nullTime(s sql.NullString) *time.Time {
	if !s.Valid {
		return nil
	}
	t := lenientTime(s.String)
	return &t
}

func nullStr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	v := s.String
	return &v
}
