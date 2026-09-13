package storage

import (
	"context"
	"fmt"
	"time"

	"hop.top/kit/go/core/util"
	"hop.top/tlc/internal/core"
)

// Compile-time check: SQLiteStorage provides the executor's claim.
var _ core.TaskClaimer = (*SQLiteStorage)(nil)

// ClaimTask is a compare-and-set: the UPDATE only lands when the row is
// still in `from`, in the given project, and unassigned or already
// assigned to actor. The write lock serializes claimants in this process;
// the WHERE predicate serializes them across processes sharing the file.
// Callers log the claim themselves (see core.ActionClaimed).
func (s *SQLiteStorage) ClaimTask(
	ctx context.Context, id, projectID string, from, to core.TaskStatus, actor string, now time.Time,
) (bool, error) {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()

	stamp := util.FormatStorageTime(now)
	res, err := s.db.ExecContext(
		ctx, `
		UPDATE tasks
		SET status = ?, assigned_to = ?, claimed_at = ?, updated_at = ?
		WHERE id = ? AND project_id = ? AND status = ?
		  AND (assigned_to IS NULL OR assigned_to = '' OR assigned_to = ?)`,
		string(to), actor, stamp, stamp,
		id, projectID, string(from), actor,
	)
	if err != nil {
		return false, fmt.Errorf("failed to claim task %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read claim result for task %s: %w", id, err)
	}
	if n != 1 {
		return false, nil
	}
	if s.projector != nil {
		if task, err := s.GetTaskInProject(ctx, id, projectID); err == nil && task != nil {
			s.project(task)
		}
	}
	return true, nil
}
