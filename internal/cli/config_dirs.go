package cli

import (
	"path/filepath"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/uri"
)

// assigneesDirFromConfig returns the assignees directory from config
// or the default "examples/assignees".
func assigneesDirFromConfig() string {
	if d := viper.GetString("flow.assignees_dir"); d != "" {
		return d
	}
	return filepath.Join("examples", "assignees")
}

// flowsDirFromConfig returns the flows directory from config
// or the default "examples/flows".
func flowsDirFromConfig() string {
	if d := viper.GetString("flow.dir"); d != "" {
		return d
	}
	return filepath.Join("examples", "flows")
}

// projectionDirFromConfig returns the projection directory name from
// config or the default "tasks".
func projectionDirFromConfig() string {
	if d := viper.GetString("task.projection_dir"); d != "" {
		return d
	}
	return "tasks"
}

// tracksDir returns the configured tracks directory name or the default.
func tracksDir() string {
	if d := viper.GetString("tracks.dir"); d != "" {
		return d
	}
	return "tracks"
}

// uriDirsConfig returns a TypesDirConfig populated from viper settings.
func uriDirsConfig() *uri.TypesDirConfig {
	return &uri.TypesDirConfig{
		FlowsDir:     flowsDirFromConfig(),
		AssigneesDir: assigneesDirFromConfig(),
	}
}

// inboxDirName returns the configured inbox directory name or "inbox".
func inboxDirName() string {
	if d := viper.GetString("storage.inbox.dir"); d != "" {
		return d
	}
	return "inbox"
}

// dbFileName returns the configured database filename or "db.sqlite".
func dbFileName() string {
	if d := viper.GetString("storage.db_path"); d != "" {
		return filepath.Base(d)
	}
	return "db.sqlite"
}
