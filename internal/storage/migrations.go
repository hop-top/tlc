package storage

import (
	"context"
	"fmt"
)

type migration struct {
	version int
	query   string
}

var migrations = []migration{
	{
		version: 1,
		query: `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY
		);

		CREATE TABLE IF NOT EXISTS tasks (
			project_id TEXT NOT NULL DEFAULT '',
			id TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT,
			status TEXT NOT NULL,
			assigned_to TEXT,
			reference TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			meta TEXT,
			tags TEXT,
			origin_system TEXT,
			last_sync_at TEXT,
			archived INTEGER DEFAULT 0,
			PRIMARY KEY (project_id, id)
		);

		CREATE TABLE IF NOT EXISTS task_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id TEXT,
			task_id TEXT NOT NULL,
			timestamp TEXT NOT NULL,
			by TEXT NOT NULL,
			action TEXT NOT NULL,
			note TEXT NOT NULL,
			meta TEXT,
			FOREIGN KEY (project_id, task_id) REFERENCES tasks(project_id, id) ON DELETE CASCADE
		);

		CREATE TABLE IF NOT EXISTS flow_runs (
			id TEXT PRIMARY KEY,
			flow_id TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			ended_at TEXT,
			results TEXT
		);

		CREATE TABLE IF NOT EXISTS task_sequences (
			project_id TEXT PRIMARY KEY,
			next_id INTEGER DEFAULT 1
		);

		CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
		CREATE INDEX IF NOT EXISTS idx_tasks_assigned_to ON tasks(assigned_to);
		CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks(created_at);
		CREATE INDEX IF NOT EXISTS idx_tasks_origin_system ON tasks(origin_system);
		CREATE INDEX IF NOT EXISTS idx_tasks_archived ON tasks(archived);
		CREATE INDEX IF NOT EXISTS idx_tasks_project_id ON tasks(project_id);
		CREATE INDEX IF NOT EXISTS idx_task_logs_task_id ON task_logs(task_id);
		CREATE INDEX IF NOT EXISTS idx_task_logs_project_id ON task_logs(project_id);
		CREATE INDEX IF NOT EXISTS idx_flow_runs_flow_id ON flow_runs(flow_id);
		CREATE INDEX IF NOT EXISTS idx_flow_runs_status ON flow_runs(status);
		`,
	},
	{
		version: 2,
		query: `
		CREATE TABLE IF NOT EXISTS projects (
			project_id    TEXT PRIMARY KEY,
			db_path       TEXT NOT NULL,
			space_uri     TEXT,
			label         TEXT,
			registered_at TEXT NOT NULL,
			last_seen_at  TEXT NOT NULL,
			status        TEXT NOT NULL DEFAULT 'active'
		);

		CREATE INDEX IF NOT EXISTS idx_projects_space_uri ON projects(space_uri);
		CREATE INDEX IF NOT EXISTS idx_projects_status ON projects(status);
		`,
	},
	{
		version: 3,
		query:   `ALTER TABLE tasks ADD COLUMN effort TEXT NOT NULL DEFAULT '';`,
	},
}

// LatestMigrationVersion is the highest migration version in the schema.
const LatestMigrationVersion = 3

// SchemaVersion returns the current schema version from the database.
func (s *SQLiteStorage) SchemaVersion() (int, error) {
	var version int
	err := s.db.QueryRowContext(context.Background(), "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("failed to query schema version: %w", err)
	}
	return version, nil
}

func (s *SQLiteStorage) migrate() error {
	var currentVersion int
	err := s.db.QueryRowContext(context.Background(), "SELECT MAX(version) FROM schema_migrations").Scan(&currentVersion)
	if err != nil {
		// Table might not exist yet
		currentVersion = 0
	}

	for _, m := range migrations {
		if m.version > currentVersion {
			if _, err := s.db.ExecContext(context.Background(), m.query); err != nil {
				return fmt.Errorf("migration to version %d failed: %w", m.version, err)
			}
			if _, err := s.db.ExecContext(context.Background(), "INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
				return fmt.Errorf("failed to record migration version %d: %w", m.version, err)
			}
		}
	}
	return nil
}
