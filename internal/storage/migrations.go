package storage

import (
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
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT,
			status TEXT NOT NULL,
			assigned_to TEXT,
			reference TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			meta TEXT,
			tags TEXT
		);
		CREATE TABLE IF NOT EXISTS task_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT NOT NULL,
			timestamp TEXT NOT NULL,
			by TEXT NOT NULL,
			action TEXT NOT NULL,
			note TEXT NOT NULL,
			meta TEXT,
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
		CREATE INDEX IF NOT EXISTS idx_tasks_assigned_to ON tasks(assigned_to);
		CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks(created_at);
		CREATE INDEX IF NOT EXISTS idx_task_logs_task_id ON task_logs(task_id);
		`,
	},
	{
		version: 2,
		query: `
		CREATE TABLE IF NOT EXISTS flow_runs (
			id TEXT PRIMARY KEY,
			flow_id TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			ended_at TEXT,
			results TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_flow_runs_flow_id ON flow_runs(flow_id);
		CREATE INDEX IF NOT EXISTS idx_flow_runs_status ON flow_runs(status);
		`,
	},
	{
		version: 3,
		query: `
		ALTER TABLE tasks ADD COLUMN origin_system TEXT;
		ALTER TABLE tasks ADD COLUMN last_sync_at TEXT;
		CREATE INDEX IF NOT EXISTS idx_tasks_origin_system ON tasks(origin_system);
		`,
	},
	{
		version: 4,
		query: `
		ALTER TABLE tasks ADD COLUMN archived INTEGER DEFAULT 0;
		CREATE INDEX IF NOT EXISTS idx_tasks_archived ON tasks(archived);
		`,
	},
}

func (s *SQLiteStorage) migrate() error {
	var currentVersion int
	err := s.db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&currentVersion)
	if err != nil {
		// Table might not exist yet
		currentVersion = 0
	}

	for _, m := range migrations {
		if m.version > currentVersion {
			if _, err := s.db.Exec(m.query); err != nil {
				return fmt.Errorf("migration to version %d failed: %w", m.version, err)
			}
			if _, err := s.db.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
				return fmt.Errorf("failed to record migration version %d: %w", m.version, err)
			}
		}
	}
	return nil
}
