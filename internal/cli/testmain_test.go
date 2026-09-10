package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain sets the COLORTERM env to truecolor before running the suite,
// so that lipgloss v2 detects TrueColor and tests exercise the same ANSI
// rendering path as a real terminal. This catches regressions where ANSI
// escape sequences embedded in table cell data corrupt runewidth
// calculations and truncate visible content.
//
// After the suite, it fails the run if a test dropped a config file into
// the package directory. Several commands persist configuration with
// viper.WriteConfig, which falls back to ".tlc.yaml" in the process
// working directory when no config file is set. A test that reaches such
// a command without chdir'ing away writes a real config file into the
// source tree: it shows up untracked in git status, and any later run in
// that directory discovers it as a project config and inherits its state.
// Tests that exercise those paths must chdir into a temp directory and
// point viper at a config path inside it.
func TestMain(m *testing.M) {
	os.Setenv("COLORTERM", "truecolor")

	// Resolve before running: a test may leave the process in another
	// directory, and the check must target the package directory itself.
	pkgDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: getwd: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	if code == 0 {
		if err := checkNoStrayConfig(pkgDir); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}

	os.Exit(code)
}

// strayConfigNames are the config filenames viper writes when no explicit
// config file is set, relative to the working directory.
var strayConfigNames = []string{".tlc.yaml", ".tlc.yml", ".tlc.json"}

// checkNoStrayConfig reports any config file a test left behind in dir.
// It removes the file so a single offending run does not poison every
// later run in the same checkout, and returns an error naming the file.
func checkNoStrayConfig(dir string) error {
	for _, name := range strayConfigNames {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("test wrote a config file into the package directory: %s (removing it failed: %v)", path, err)
		}
		return fmt.Errorf("test wrote a config file into the package directory: %s\n"+
			"A test reached viper.WriteConfig without a config file set, so viper fell back to the\n"+
			"working directory. Chdir into t.TempDir() and call viper.SetConfigFile with a path\n"+
			"inside it — see setupSyncConfigTest in sync_test.go. The stray file has been removed.", path)
	}
	return nil
}
