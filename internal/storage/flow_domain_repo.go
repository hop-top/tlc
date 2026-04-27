package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"hop.top/kit/go/runtime/domain"
	"hop.top/tlc/internal/core"
)

// FlowRunDomainRepo adapts SQLiteStorage to domain.Repository[core.FlowRun].
type FlowRunDomainRepo struct {
	store *SQLiteStorage
}

// Compile-time assertion.
var _ domain.Repository[core.FlowRun] = (*FlowRunDomainRepo)(nil)

// NewFlowRunDomainRepo wraps a SQLiteStorage as a domain.Repository[core.FlowRun].
func NewFlowRunDomainRepo(store *SQLiteStorage) *FlowRunDomainRepo {
	return &FlowRunDomainRepo{store: store}
}

// ScanFlowRun scans a single sql.Row into a core.FlowRun.
func ScanFlowRun(row *sql.Row) (core.FlowRun, error) {
	var r core.FlowRun
	var startedAt string
	var endedAt, resultsStr sql.NullString

	err := row.Scan(&r.ID, &r.FlowID, &r.Status, &startedAt,
		&endedAt, &resultsStr)
	if err != nil {
		return r, err
	}
	if err := populateFlowRun(&r, startedAt, endedAt, resultsStr); err != nil {
		return r, err
	}
	return r, nil
}

// ScanFlowRunRows scans a sql.Rows cursor into a core.FlowRun.
func ScanFlowRunRows(rows *sql.Rows) (core.FlowRun, error) {
	var r core.FlowRun
	var startedAt string
	var endedAt, resultsStr sql.NullString

	err := rows.Scan(&r.ID, &r.FlowID, &r.Status, &startedAt,
		&endedAt, &resultsStr)
	if err != nil {
		return r, err
	}
	if err := populateFlowRun(&r, startedAt, endedAt, resultsStr); err != nil {
		return r, err
	}
	return r, nil
}

// BindFlowRun returns column names and values for a FlowRun.
func BindFlowRun(r core.FlowRun) (cols []string, vals []any) {
	resultsJSON, _ := json.Marshal(r.Results) //nolint:errcheck // marshalling known-valid struct
	var endedAt *string
	if r.EndedAt != nil {
		s := r.EndedAt.Format(time.RFC3339)
		endedAt = &s
	}

	cols = []string{"id", "flow_id", "status", "started_at", "ended_at", "results"}
	vals = []any{
		r.ID, r.FlowID, string(r.Status),
		r.StartedAt.Format(time.RFC3339), endedAt, string(resultsJSON),
	}
	return cols, vals
}

// populateFlowRun fills computed fields from scanned nullable values.
func populateFlowRun(
	r *core.FlowRun,
	startedAt string,
	endedAt, resultsStr sql.NullString,
) error {
	var err error
	if r.StartedAt, err = parseRFC3339(startedAt); err != nil {
		return err
	}
	if endedAt.Valid {
		t, err := parseRFC3339(endedAt.String)
		if err != nil {
			return err
		}
		r.EndedAt = &t
	}
	if resultsStr.Valid {
		if err := json.Unmarshal([]byte(resultsStr.String), &r.Results); err != nil {
			return fmt.Errorf("failed to unmarshal flow run results: %w", err)
		}
	}
	return nil
}

// Create implements domain.Repository[core.FlowRun].
func (r *FlowRunDomainRepo) Create(ctx context.Context, run *core.FlowRun) error {
	return r.store.CreateFlowRun(ctx, run)
}

// Get implements domain.Repository[core.FlowRun].
func (r *FlowRunDomainRepo) Get(ctx context.Context, id string) (*core.FlowRun, error) {
	return r.store.GetFlowRun(ctx, id)
}

// List implements domain.Repository[core.FlowRun] with basic Query support.
func (r *FlowRunDomainRepo) List(ctx context.Context, q domain.Query) ([]core.FlowRun, error) {
	coreQuery := core.Query{
		Limit:  q.Limit,
		Offset: q.Offset,
	}
	runs, err := r.store.ListFlowRuns(ctx, coreQuery)
	if err != nil {
		return nil, err
	}
	result := make([]core.FlowRun, len(runs))
	for i, run := range runs {
		result[i] = *run
	}
	return result, nil
}

// Update implements domain.Repository[core.FlowRun].
func (r *FlowRunDomainRepo) Update(ctx context.Context, run *core.FlowRun) error {
	return r.store.UpdateFlowRun(ctx, run)
}

// Delete implements domain.Repository[core.FlowRun].
func (r *FlowRunDomainRepo) Delete(ctx context.Context, id string) error {
	return r.store.deleteFlowRun(ctx, id)
}
