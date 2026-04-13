package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"hop.top/kit/domain"
	"hop.top/tlc/internal/core"
)

// TrackDomainRepo adapts SQLiteStorage to domain.Repository[core.Track].
type TrackDomainRepo struct {
	store *SQLiteStorage
}

// Compile-time assertion: TrackDomainRepo implements domain.Repository[core.Track].
var _ domain.Repository[core.Track] = (*TrackDomainRepo)(nil)

// NewTrackDomainRepo wraps a SQLiteStorage as a domain.Repository[core.Track].
func NewTrackDomainRepo(store *SQLiteStorage) *TrackDomainRepo {
	return &TrackDomainRepo{store: store}
}

// ScanTrack scans a single sql.Row into a core.Track.
func ScanTrack(row *sql.Row) (core.Track, error) {
	var t core.Track
	var createdAt, updatedAt string
	var assignedTo, projectID, metaStr sql.NullString

	err := row.Scan(
		&t.ID, &t.Title, &t.Type, &t.Status,
		&assignedTo, &createdAt, &updatedAt, &projectID, &metaStr,
	)
	if err != nil {
		return t, err
	}
	populateTrack(&t, createdAt, updatedAt, assignedTo, projectID, metaStr)
	return t, nil
}

// ScanTrackRows scans a sql.Rows cursor into a core.Track.
func ScanTrackRows(rows *sql.Rows) (core.Track, error) {
	var t core.Track
	var createdAt, updatedAt string
	var assignedTo, projectID, metaStr sql.NullString

	err := rows.Scan(
		&t.ID, &t.Title, &t.Type, &t.Status,
		&assignedTo, &createdAt, &updatedAt, &projectID, &metaStr,
	)
	if err != nil {
		return t, err
	}
	populateTrack(&t, createdAt, updatedAt, assignedTo, projectID, metaStr)
	return t, nil
}

// BindTrack returns column names and values for a Track.
func BindTrack(t core.Track) (cols []string, vals []any) {
	metaJSON, _ := json.Marshal(t.Meta)
	var pid string
	if t.ProjectID != nil {
		pid = *t.ProjectID
	}

	cols = []string{
		"id", "title", "type", "status", "assigned_to",
		"created_at", "updated_at", "project_id", "meta",
	}
	vals = []any{
		t.ID, t.Title, t.Type, string(t.Status), t.AssignedTo,
		t.CreatedAt.Format(time.RFC3339),
		t.UpdatedAt.Format(time.RFC3339),
		pid, string(metaJSON),
	}
	return cols, vals
}

// populateTrack fills computed fields from scanned nullable values.
func populateTrack(
	t *core.Track,
	createdAt, updatedAt string,
	assignedTo, projectID, metaStr sql.NullString,
) {
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if assignedTo.Valid {
		t.AssignedTo = &assignedTo.String
	}
	if projectID.Valid {
		t.ProjectID = &projectID.String
	}
	if metaStr.Valid && metaStr.String != "null" {
		_ = json.Unmarshal([]byte(metaStr.String), &t.Meta)
	}
}

// Create implements domain.Repository[core.Track].
func (r *TrackDomainRepo) Create(ctx context.Context, track *core.Track) error {
	return r.store.CreateTrack(ctx, track)
}

// Get implements domain.Repository[core.Track].
func (r *TrackDomainRepo) Get(ctx context.Context, id string) (*core.Track, error) {
	return r.store.GetTrack(ctx, id)
}

// List implements domain.Repository[core.Track] with basic Query support.
// For rich filtering (status, type), use ListTracks directly.
func (r *TrackDomainRepo) List(ctx context.Context, q domain.Query) ([]core.Track, error) {
	query := core.TrackQuery{
		Limit:  q.Limit,
		Offset: q.Offset,
	}
	tracks, err := r.store.ListTracks(ctx, query)
	if err != nil {
		return nil, err
	}
	result := make([]core.Track, len(tracks))
	for i, t := range tracks {
		result[i] = *t
	}
	return result, nil
}

// Update implements domain.Repository[core.Track].
func (r *TrackDomainRepo) Update(ctx context.Context, track *core.Track) error {
	return r.store.UpdateTrack(ctx, track)
}

// Delete implements domain.Repository[core.Track].
func (r *TrackDomainRepo) Delete(ctx context.Context, id string) error {
	return r.store.DeleteTrack(ctx, id)
}
