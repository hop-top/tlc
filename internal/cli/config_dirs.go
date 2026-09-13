package cli

import (
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/uri"
)

// recipesDirName is the recipe directory inside a project's config dir
// and inside the user config dir.
const recipesDirName = "recipes"

// assigneesDirFromConfig returns the assignees directory from config
// or the default "examples/assignees".
func assigneesDirFromConfig() string {
	rc := config.RecipeConfig{AssigneesDir: viper.GetString("recipe.assignees_dir")}
	return rc.AssigneesDirectory()
}

// recipeDirsFromConfig returns the recipe search path in precedence order:
// recipe.dir when set (relative to the project root), the project's
// <config dir>/recipes, then the user's config dir. Missing directories
// are skipped by the locator, so an unset layer costs nothing.
func recipeDirsFromConfig() []string {
	var dirs []string
	cfgDir := projectConfigDir()
	rc := config.RecipeConfig{Dir: viper.GetString("recipe.dir")}
	if d := rc.RecipeDir(); d != "" {
		dirs = append(dirs, projectRelativePath(cfgDir, d))
	}
	if cfgDir != "" {
		dirs = append(dirs, filepath.Join(cfgDir, recipesDirName))
	}
	if user, err := config.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(user, recipesDirName))
	}
	return dirs
}

// recipeLocator returns the locator over the configured search path.
func recipeLocator() *core.DirLocator {
	return &core.DirLocator{Dirs: recipeDirsFromConfig()}
}

// projectConfigDir returns the project's config directory (.tlc or
// .hop/tlc) derived from the loaded config file, or "" when no project
// config is loaded. Unlike resolveConfigDir it never creates anything.
func projectConfigDir() string {
	used := viper.ConfigFileUsed()
	if used == "" {
		return ""
	}
	dir := filepath.Dir(used)
	configDirName := config.LocalConfigDir(config.DetectMode())
	if filepath.Base(dir) == filepath.Base(configDirName) {
		return dir
	}
	return filepath.Join(dir, configDirName)
}

// projectRelativePath anchors a relative config path at the project root
// (the parent of the config dir) when a project config is loaded;
// otherwise it is left as given, relative to the working directory.
func projectRelativePath(cfgDir, p string) string {
	if cfgDir == "" || filepath.IsAbs(p) {
		return p
	}
	root := cfgDir
	depth := len(strings.Split(filepath.ToSlash(config.LocalConfigDir(config.DetectMode())), "/"))
	for range depth {
		root = filepath.Dir(root)
	}
	return filepath.Join(root, p)
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
		AssigneesDir: assigneesDirFromConfig(),
		RecipeDirs:   recipeDirsFromConfig(),
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
