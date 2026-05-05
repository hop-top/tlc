package storage

import (
	"context"
	"fmt"
	"log"

	"hop.top/kit/go/storage/sqlstore"
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
	{
		version: 4,
		query:   `ALTER TABLE tasks ADD COLUMN priority TEXT NOT NULL DEFAULT '';`,
	},
	{
		version: 5,
		query: `
		ALTER TABLE tasks ADD COLUMN stale_timeout INTEGER;
		ALTER TABLE tasks ADD COLUMN blocked_reason TEXT;
		ALTER TABLE tasks ADD COLUMN stale_fired_at TEXT;
		`,
	},
	{
		version: 6,
		query: `
		CREATE TABLE IF NOT EXISTS tracks (
			id          TEXT NOT NULL,
			title       TEXT NOT NULL,
			type        TEXT NOT NULL,
			status      TEXT NOT NULL DEFAULT 'pending',
			assigned_to TEXT,
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL,
			project_id  TEXT NOT NULL DEFAULT '',
			meta        TEXT,
			PRIMARY KEY (project_id, id)
		);

		ALTER TABLE tasks ADD COLUMN track_id TEXT;

		CREATE INDEX IF NOT EXISTS idx_tracks_status ON tracks(status);
		CREATE INDEX IF NOT EXISTS idx_tracks_project_id ON tracks(project_id);
		CREATE INDEX IF NOT EXISTS idx_tasks_track_id ON tasks(track_id);
		`,
	},
	{
		version: 7,
		query: `
		-- Rebuild tracks table with composite PK (project_id, id) so the
		-- same track ID can exist in different projects.  The original v6
		-- migration used CREATE TABLE IF NOT EXISTS which was a no-op for
		-- databases that already had a tracks table with id TEXT PRIMARY KEY.

		-- Disable FK checks — tasks.track_id references tracks(id) and
		-- would block the DROP.  Re-enabled after the swap.
		PRAGMA foreign_keys = OFF;

		-- Clean up leftover from a previously failed run.
		DROP TABLE IF EXISTS tracks_new;

		CREATE TABLE tracks_new (
			id          TEXT NOT NULL,
			title       TEXT NOT NULL,
			type        TEXT NOT NULL,
			status      TEXT NOT NULL DEFAULT 'pending',
			assigned_to TEXT,
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL,
			project_id  TEXT NOT NULL DEFAULT '',
			meta        TEXT,
			PRIMARY KEY (project_id, id)
		);

		INSERT INTO tracks_new
			(id, title, type, status, assigned_to,
			 created_at, updated_at, project_id, meta)
		SELECT id, title, type, status, assigned_to,
			   created_at, updated_at,
			   COALESCE(project_id, ''),
			   meta
		FROM tracks;

		DROP TABLE tracks;
		ALTER TABLE tracks_new RENAME TO tracks;

		CREATE INDEX IF NOT EXISTS idx_tracks_status ON tracks(status);
		CREATE INDEX IF NOT EXISTS idx_tracks_project_id ON tracks(project_id);

		PRAGMA foreign_keys = ON;
		`,
	},
	{
		version: 8,
		query: `
		-- Rebuild tasks table to drop the FK on track_id that referenced
		-- tracks(id) — no longer valid after v7 changed the tracks PK to
		-- (project_id, id).  Track integrity is enforced at app level.
		PRAGMA foreign_keys = OFF;

		DROP TABLE IF EXISTS tasks_new;

		CREATE TABLE tasks_new (
			project_id    TEXT NOT NULL DEFAULT '',
			id            TEXT NOT NULL,
			title         TEXT NOT NULL,
			description   TEXT,
			status        TEXT NOT NULL,
			assigned_to   TEXT,
			reference     TEXT NOT NULL DEFAULT '',
			created_at    TEXT NOT NULL,
			updated_at    TEXT NOT NULL,
			meta          TEXT,
			tags          TEXT,
			origin_system TEXT,
			last_sync_at  TEXT,
			archived      INTEGER DEFAULT 0,
			effort        TEXT NOT NULL DEFAULT '',
			priority      TEXT NOT NULL DEFAULT '',
			stale_timeout INTEGER,
			blocked_reason TEXT,
			stale_fired_at TEXT,
			track_id      TEXT,
			PRIMARY KEY (project_id, id)
		);

		INSERT INTO tasks_new
			(project_id, id, title, description, status,
			 assigned_to, reference, created_at, updated_at,
			 meta, tags, origin_system, last_sync_at, archived,
			 effort, priority, stale_timeout, blocked_reason,
			 stale_fired_at, track_id)
		SELECT project_id, id, title, description, status,
			   assigned_to, reference, created_at, updated_at,
			   meta, tags, origin_system, last_sync_at, archived,
			   effort, priority, stale_timeout, blocked_reason,
			   stale_fired_at, track_id
		FROM tasks;

		DROP TABLE tasks;
		ALTER TABLE tasks_new RENAME TO tasks;

		CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
		CREATE INDEX IF NOT EXISTS idx_tasks_assigned_to ON tasks(assigned_to);
		CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks(created_at);
		CREATE INDEX IF NOT EXISTS idx_tasks_origin_system ON tasks(origin_system);
		CREATE INDEX IF NOT EXISTS idx_tasks_archived ON tasks(archived);
		CREATE INDEX IF NOT EXISTS idx_tasks_project_id ON tasks(project_id);
		CREATE INDEX IF NOT EXISTS idx_tasks_track_id ON tasks(track_id);

		PRAGMA foreign_keys = ON;
		`,
	},
	{
		version: 9,
		query: `
		CREATE TABLE IF NOT EXISTS jobs (
			id         TEXT PRIMARY KEY,
			queue      TEXT NOT NULL,
			type       TEXT NOT NULL,
			status     TEXT NOT NULL DEFAULT 'queued',
			payload    TEXT NOT NULL,
			result     TEXT,
			error      TEXT,
			created_at TEXT NOT NULL,
			started_at TEXT,
			ended_at   TEXT,
			created_by TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_jobs_queue_status ON jobs(queue, status);
		CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
		CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs(created_at);
		`,
	},
	{
		version: 10,
		query: `
		CREATE TABLE IF NOT EXISTS agent_runs (
			id           TEXT PRIMARY KEY,
			agent        TEXT NOT NULL,
			target_type  TEXT NOT NULL,
			target_id    TEXT NOT NULL,
			job_id       TEXT,
			container_id TEXT,
			status       TEXT NOT NULL,
			exit_code    INTEGER,
			error        TEXT,
			result_path  TEXT,
			started_at   TEXT NOT NULL,
			ended_at     TEXT,
			created_by   TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_agent_runs_agent ON agent_runs(agent);
		CREATE INDEX IF NOT EXISTS idx_agent_runs_status ON agent_runs(status);
		CREATE INDEX IF NOT EXISTS idx_agent_runs_started_at ON agent_runs(started_at);
		CREATE INDEX IF NOT EXISTS idx_agent_runs_target ON agent_runs(target_type, target_id);
		`,
	},
	{
		version: 11,
		query:   `ALTER TABLE tracks ADD COLUMN plan_mapping TEXT;`,
	},
	{
		version: 12,
		query: `
		ALTER TABLE tasks ADD COLUMN due_at TEXT;
		ALTER TABLE tasks ADD COLUMN remind_at TEXT;
		ALTER TABLE tasks ADD COLUMN remind_every INTEGER;
		ALTER TABLE tasks ADD COLUMN no_auto_remind INTEGER DEFAULT 0;
		CREATE INDEX IF NOT EXISTS idx_tasks_due_at ON tasks(due_at);
		`,
	},
	{
		// Migrate task and track IDs to TypeID format (task_<26char>,
		// track_<26char>) and add per-project sequence/slug aliases for
		// human-facing display. Existing rows are dropped — typeid-ids
		// design explicitly disposes of pre-migration data (T-0812, T-0813).
		version: 13,
		query: `
		PRAGMA foreign_keys = OFF;

		DROP TABLE IF EXISTS task_logs;
		DROP TABLE IF EXISTS tasks;
		DROP TABLE IF EXISTS tracks;

		CREATE TABLE tasks (
			id              TEXT PRIMARY KEY,
			seq             INTEGER NOT NULL,
			project_id      TEXT NOT NULL DEFAULT '',
			title           TEXT NOT NULL,
			description     TEXT,
			status          TEXT NOT NULL,
			assigned_to     TEXT,
			reference       TEXT NOT NULL DEFAULT '',
			created_at      TEXT NOT NULL,
			updated_at      TEXT NOT NULL,
			meta            TEXT,
			tags            TEXT,
			origin_system   TEXT,
			last_sync_at    TEXT,
			archived        INTEGER DEFAULT 0,
			effort          TEXT NOT NULL DEFAULT '',
			priority        TEXT NOT NULL DEFAULT '',
			stale_timeout   INTEGER,
			blocked_reason  TEXT,
			stale_fired_at  TEXT,
			track_id        TEXT,
			due_at          TEXT,
			remind_at       TEXT,
			remind_every    INTEGER,
			no_auto_remind  INTEGER DEFAULT 0,
			UNIQUE (project_id, seq)
		);

		CREATE TABLE tracks (
			id          TEXT PRIMARY KEY,
			slug        TEXT NOT NULL,
			project_id  TEXT NOT NULL DEFAULT '',
			title       TEXT NOT NULL,
			type        TEXT NOT NULL,
			status      TEXT NOT NULL DEFAULT 'pending',
			assigned_to TEXT,
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL,
			meta        TEXT,
			plan_mapping TEXT,
			UNIQUE (project_id, slug)
		);

		CREATE TABLE task_logs (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id    TEXT NOT NULL,
			project_id TEXT,
			timestamp  TEXT NOT NULL,
			by         TEXT NOT NULL,
			action     TEXT NOT NULL,
			note       TEXT NOT NULL,
			meta       TEXT,
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		);

		CREATE INDEX idx_tasks_status        ON tasks(status);
		CREATE INDEX idx_tasks_assigned_to   ON tasks(assigned_to);
		CREATE INDEX idx_tasks_created_at    ON tasks(created_at);
		CREATE INDEX idx_tasks_origin_system ON tasks(origin_system);
		CREATE INDEX idx_tasks_archived      ON tasks(archived);
		CREATE INDEX idx_tasks_project_id    ON tasks(project_id);
		CREATE INDEX idx_tasks_track_id      ON tasks(track_id);
		CREATE INDEX idx_tasks_due_at        ON tasks(due_at);
		CREATE INDEX idx_tasks_seq           ON tasks(project_id, seq);

		CREATE INDEX idx_tracks_status     ON tracks(status);
		CREATE INDEX idx_tracks_project_id ON tracks(project_id);
		CREATE INDEX idx_tracks_slug       ON tracks(project_id, slug);

		CREATE INDEX idx_task_logs_task_id    ON task_logs(task_id);
		CREATE INDEX idx_task_logs_project_id ON task_logs(project_id);

		PRAGMA foreign_keys = ON;
		`,
	},
	{
		// Replace remind_every (fixed Duration) with rrule (RFC 5545
		// recurrence string) so reminders can express richer schedules
		// like "every weekday until June" or "first Monday of month".
		// Existing remind_every data is dropped per typeid-ids design's
		// disposability rule.
		version: 14,
		query: `
		ALTER TABLE tasks DROP COLUMN remind_every;
		ALTER TABLE tasks ADD COLUMN rrule TEXT;
		`,
	},
	{
		// Drop ON DELETE CASCADE on task_logs.task_id so log entries
		// (including the --note recorded at delete time, T-1178) survive
		// task deletion. Required for kit/runtime/policy adoption
		// (T-1192, "delete-requires-note") to not be theater: capturing
		// a delete reason has no audit value if the cascade wipes it.
		//
		// SQLite cannot ALTER a foreign key in place; rebuild the table
		// without the cascade and copy rows over.  The FK is preserved
		// as a referential hint but with no action on parent delete —
		// task_id may legitimately reference a deleted task after this.
		version: 15,
		query: `
		PRAGMA foreign_keys = OFF;

		DROP TABLE IF EXISTS task_logs_new;

		CREATE TABLE task_logs_new (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id    TEXT NOT NULL,
			project_id TEXT,
			timestamp  TEXT NOT NULL,
			by         TEXT NOT NULL,
			action     TEXT NOT NULL,
			note       TEXT NOT NULL,
			meta       TEXT
		);

		INSERT INTO task_logs_new
			(id, task_id, project_id, timestamp, by, action, note, meta)
		SELECT id, task_id, project_id, timestamp, by, action, note, meta
		FROM task_logs;

		DROP TABLE task_logs;
		ALTER TABLE task_logs_new RENAME TO task_logs;

		CREATE INDEX IF NOT EXISTS idx_task_logs_task_id    ON task_logs(task_id);
		CREATE INDEX IF NOT EXISTS idx_task_logs_project_id ON task_logs(project_id);

		PRAGMA foreign_keys = ON;
		`,
	},
}

// LatestMigrationVersion is the highest migration version in the schema.
const LatestMigrationVersion = 15

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

	// Check if any migrations are pending.
	hasPending := false
	for _, m := range migrations {
		if m.version > currentVersion {
			hasPending = true
			break
		}
	}

	// Backup before applying any pending migrations.
	if hasPending && s.dbPath != "" {
		nextVersion := currentVersion + 1
		if bp, err := sqlstore.BackupBeforeMigrate(s.dbPath, nextVersion); err != nil {
			return fmt.Errorf("pre-migration backup failed: %w", err)
		} else if bp != "" {
			log.Printf("backed up database before migration: %s", bp)
		}
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

	// Data migrations run after schema is at HEAD. Each is independently
	// idempotent via its own marker; safe to invoke on every open.
	if err := s.runStatusDataMigration(); err != nil {
		return err
	}
	return nil
}
