package cli

import (
	"path/filepath"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/uri"
)

// assigneesDirFromConfig returns the assignees directory from config
// or the default "examples/assignees".
func assigneesDirFromConfig() string {
	fc := config.FlowConfig{AssigneesDir: viper.GetString("flow.assignees_dir")}
	return fc.AssigneesDirectory()
}

// flowsDirFromConfig returns the flows directory from config
// or the default "examples/flows".
func flowsDirFromConfig() string {
	fc := config.FlowConfig{Dir: viper.GetString("flow.dir")}
	return fc.FlowsDir()
}

// projectionDirFromConfig returns the projection directory name from
// config or the default "tasks".
func projectionDirFromConfig() string {
	tc := config.TaskConfig{ProjectionDir: viper.GetString("task.projection_dir")}
	return tc.ProjectionDirectory()
}

// tracksDir returns the configured tracks directory name or the default.
func tracksDir() string {
	tc := config.TrackConfig{Dir: viper.GetString("tracks.dir")}
	return tc.TracksDir()
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
	ic := config.InboxConfig{Dir: viper.GetString("storage.inbox.dir")}
	return ic.InboxDir()
}

// dbFileName returns the configured database filename or "db.sqlite".
func dbFileName() string {
	sc := config.StorageConfig{DBPath: viper.GetString("storage.db_path")}
	return filepath.Base(sc.DBFilePath())
}
