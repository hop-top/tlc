package storage

import (
	"context"
	"database/sql"
	"strings"

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

// ScanTask scans a single sql.Row selected with taskColumns into a core.Task.
func ScanTask(row *sql.Row) (core.Task, error) {
	return derefTask(scanTask(row))
}

// ScanTaskRows scans a sql.Rows cursor selected with taskColumns into a core.Task.
func ScanTaskRows(rows *sql.Rows) (core.Task, error) {
	return derefTask(scanTask(rows))
}

func derefTask(t *core.Task, err error) (core.Task, error) {
	if err != nil {
		return core.Task{}, err
	}
	return *t, nil
}

// BindTask returns column names and values for a Task, suitable for
// INSERT statements. It covers every column in taskColumns, in order.
func BindTask(t core.Task) (cols []string, vals []any) {
	cols = strings.Split(taskColumns, ", ")
	vals = encodeTask(&t).insertArgs(&t)
	return cols, vals
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
