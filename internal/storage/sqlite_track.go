package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"hop.top/tlc/internal/core"
)

// Compile-time check: SQLiteStorage implements core.TrackRepository.
var _ core.TrackRepository = (*SQLiteStorage)(nil)

func (s *SQLiteStorage) CreateTrack(ctx context.Context, track *core.Track) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		metaJSON, _ := json.Marshal(track.Meta) //nolint:errcheck // marshalling known-valid struct

		projectID := ""
		if track.ProjectID != nil {
			projectID = *track.ProjectID
		}

		var planMappingSQL sql.NullString
		if len(track.PlanMapping) > 0 {
			pmJSON, _ := json.Marshal(track.PlanMapping) //nolint:errcheck // marshaling known-valid map
			planMappingSQL = sql.NullString{String: string(pmJSON), Valid: true}
		}

		_, err := tx.ExecContext(ctx, `
			INSERT INTO tracks (id, slug, title, type, status, assigned_to,
				created_at, updated_at, project_id, meta, plan_mapping)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			track.ID, track.Slug, track.Title, track.Type, track.Status,
			track.AssignedTo,
			track.CreatedAt.Format(time.RFC3339),
			track.UpdatedAt.Format(time.RFC3339),
			projectID,
			string(metaJSON),
			planMappingSQL,
		)
		if err != nil {
			return fmt.Errorf("failed to insert track: %w", err)
		}
		return nil
	})
}

func (s *SQLiteStorage) GetTrack(ctx context.Context, id string) (*core.Track, error) {
	var row *sql.Row
	if proj := core.DetectProject(); proj != nil && proj.InProject && proj.ProjectID != "" {
		row = s.db.QueryRowContext(ctx, `
			SELECT id, slug, title, type, status, assigned_to,
				created_at, updated_at, project_id, meta, plan_mapping
			FROM tracks WHERE id = ? AND project_id = ?`, id, proj.ProjectID)
	} else {
		row = s.db.QueryRowContext(ctx, `
			SELECT id, slug, title, type, status, assigned_to,
				created_at, updated_at, project_id, meta, plan_mapping
			FROM tracks WHERE id = ? ORDER BY CASE WHEN project_id = '' THEN 0 ELSE 1 END LIMIT 1`, id)
	}

	return scanTrackFromRow(row)
}

// getTrackInProject retrieves a track scoped to a specific project.
func (s *SQLiteStorage) getTrackInProject(ctx context.Context, id, projectID string) (*core.Track, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, title, type, status, assigned_to,
			created_at, updated_at, project_id, meta, plan_mapping
		FROM tracks WHERE id = ? AND project_id = ?`, id, projectID)
	return scanTrackFromRow(row)
}

func (s *SQLiteStorage) UpdateTrack(ctx context.Context, track *core.Track) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		metaJSON, _ := json.Marshal(track.Meta) //nolint:errcheck // marshalling known-valid struct

		projectID := ""
		if track.ProjectID != nil {
			projectID = *track.ProjectID
		}

		var planMappingSQL sql.NullString
		if len(track.PlanMapping) > 0 {
			pmJSON, _ := json.Marshal(track.PlanMapping) //nolint:errcheck // marshaling known-valid map
			planMappingSQL = sql.NullString{String: string(pmJSON), Valid: true}
		}

		res, err := tx.ExecContext(ctx, `
			UPDATE tracks
			SET title = ?, type = ?, status = ?, assigned_to = ?,
				updated_at = ?, project_id = ?, meta = ?, plan_mapping = ?
			WHERE id = ? AND project_id = ?`,
			track.Title, track.Type, track.Status, track.AssignedTo,
			track.UpdatedAt.Format(time.RFC3339),
			projectID,
			string(metaJSON),
			planMappingSQL,
			track.ID, projectID,
		)
		if err != nil {
			return fmt.Errorf("failed to update track: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected: %w", err)
		}
		if n == 0 {
			return fmt.Errorf(
				"track %q not found; run 'tlc track list' to see available tracks",
				track.ID,
			)
		}
		return nil
	})
}

func (s *SQLiteStorage) DeleteTrack(ctx context.Context, id string) error {
	return s.withWriteTransaction(ctx, func(tx *sql.Tx) error {
		// Check for linked tasks referencing this track.
		var count int
		err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM tasks WHERE track_id = ?", id,
		).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to check linked tasks: %w", err)
		}
		if count > 0 {
			return fmt.Errorf(
				"track %q has %d linked task(s); unlink them first with "+
					"'tlc task update <id> --track -'",
				id, count,
			)
		}

		proj := core.DetectProject()
		if proj != nil && proj.InProject && proj.ProjectID != "" {
			res, err := tx.ExecContext(ctx,
				"DELETE FROM tracks WHERE id = ? AND project_id = ?", id, proj.ProjectID,
			)
			if err != nil {
				return fmt.Errorf("failed to delete track: %w", err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return fmt.Errorf("failed to get rows affected: %w", err)
			}
			if n == 0 {
				return fmt.Errorf(
					"track %q not found; run 'tlc track list' to see available tracks",
					id,
				)
			}
			return nil
		}

		res, err := tx.ExecContext(ctx,
			"DELETE FROM tracks WHERE id = ?", id,
		)
		if err != nil {
			return fmt.Errorf("failed to delete track: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected: %w", err)
		}
		if n == 0 {
			return fmt.Errorf(
				"track %q not found; run 'tlc track list' to see available tracks",
				id,
			)
		}
		return nil
	})
}

func (s *SQLiteStorage) ListTracks(
	ctx context.Context,
	query core.TrackQuery,
) ([]*core.Track, error) {
	sqlQuery := `SELECT id, slug, title, type, status, assigned_to,
		created_at, updated_at, project_id, meta, plan_mapping FROM tracks`
	var args []interface{}
	var whereClauses []string

	if len(query.Status) > 0 {
		placeholders := make([]string, len(query.Status))
		for i, st := range query.Status {
			placeholders[i] = "?"
			args = append(args, string(st))
		}
		whereClauses = append(whereClauses,
			fmt.Sprintf("status IN (%s)", strings.Join(placeholders, ",")))
	}

	if query.Type != "" {
		whereClauses = append(whereClauses, "type = ?")
		args = append(args, query.Type)
	}

	if query.ProjectID != nil {
		whereClauses = append(whereClauses, "project_id = ?")
		args = append(args, *query.ProjectID)
	}

	// Auto-filter by project if in a project context and not requesting all.
	if !query.AllProjects && query.ProjectID == nil {
		if proj := core.DetectProject(); proj != nil && proj.InProject && proj.ProjectID != "" {
			whereClauses = append(whereClauses, "project_id = ?")
			args = append(args, proj.ProjectID)
		}
	}

	if len(whereClauses) > 0 {
		sqlQuery += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	sqlQuery += " ORDER BY created_at DESC"

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
		return nil, fmt.Errorf("failed to query tracks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tracks []*core.Track
	for rows.Next() {
		track, err := scanTrackFromRows(rows)
		if err != nil {
			return nil, err
		}
		tracks = append(tracks, track)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate track rows: %w", err)
	}
	return tracks, nil
}

// scanTrackFromRow scans a single track from a *sql.Row.
func scanTrackFromRow(row *sql.Row) (*core.Track, error) {
	var track core.Track
	var createdAtStr, updatedAtStr string
	var assignedTo, projectID, metaStr, planMappingStr sql.NullString

	err := row.Scan(
		&track.ID, &track.Slug, &track.Title, &track.Type, &track.Status,
		&assignedTo, &createdAtStr, &updatedAtStr, &projectID, &metaStr,
		&planMappingStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan track row: %w", err)
	}

	if track.CreatedAt, err = parseRFC3339(createdAtStr); err != nil {
		return nil, err
	}
	if track.UpdatedAt, err = parseRFC3339(updatedAtStr); err != nil {
		return nil, err
	}

	if assignedTo.Valid {
		track.AssignedTo = &assignedTo.String
	}
	if projectID.Valid {
		track.ProjectID = &projectID.String
	}
	if metaStr.Valid && metaStr.String != "null" {
		if err := json.Unmarshal([]byte(metaStr.String), &track.Meta); err != nil {
			return nil, fmt.Errorf("failed to unmarshal track meta: %w", err)
		}
	}
	if planMappingStr.Valid && planMappingStr.String != "" {
		if err := json.Unmarshal([]byte(planMappingStr.String), &track.PlanMapping); err != nil {
			return nil, fmt.Errorf("failed to unmarshal plan_mapping: %w", err)
		}
	}

	return &track, nil
}

// scanTrackFromRows scans a single track from *sql.Rows (for use inside Next loop).
func scanTrackFromRows(rows *sql.Rows) (*core.Track, error) {
	var track core.Track
	var createdAtStr, updatedAtStr string
	var assignedTo, projectID, metaStr, planMappingStr sql.NullString

	err := rows.Scan(
		&track.ID, &track.Slug, &track.Title, &track.Type, &track.Status,
		&assignedTo, &createdAtStr, &updatedAtStr, &projectID, &metaStr,
		&planMappingStr,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan track row: %w", err)
	}

	if track.CreatedAt, err = parseRFC3339(createdAtStr); err != nil {
		return nil, err
	}
	if track.UpdatedAt, err = parseRFC3339(updatedAtStr); err != nil {
		return nil, err
	}

	if assignedTo.Valid {
		track.AssignedTo = &assignedTo.String
	}
	if projectID.Valid {
		track.ProjectID = &projectID.String
	}
	if metaStr.Valid && metaStr.String != "null" {
		if err := json.Unmarshal([]byte(metaStr.String), &track.Meta); err != nil {
			return nil, fmt.Errorf("failed to unmarshal track meta: %w", err)
		}
	}
	if planMappingStr.Valid && planMappingStr.String != "" {
		if err := json.Unmarshal([]byte(planMappingStr.String), &track.PlanMapping); err != nil {
			return nil, fmt.Errorf("failed to unmarshal plan_mapping: %w", err)
		}
	}

	return &track, nil
}
