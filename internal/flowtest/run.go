// Package flowtest provides deterministic e2e testing for tlc flow definitions.
package flowtest

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// RunManifest is the optional test.yaml manifest inside a run directory.
type RunManifest struct {
	Name         string   `yaml:"name,omitempty"`
	Description  string   `yaml:"description,omitempty"`
	ExpectedExit int      `yaml:"expected_exit"`
	Passthrough  []string `yaml:"passthrough,omitempty"`
}

// Run describes one named test run for a flow.
type Run struct {
	Name         string
	Dir          string   // path to run dir
	RecordDir    string   // path to record/ subdir → xrr.NewFileCassette
	ContractsDir string   // path to contracts/ subdir
	ExpectedExit int
	Passthrough  []string
}

// options holds optional parameters for DiscoverRuns.
type options struct {
	runName string
}

// Option configures DiscoverRuns.
type Option func(*options)

// WithRunName filters to a single named run.
func WithRunName(name string) Option {
	return func(o *options) { o.runName = name }
}

// DiscoverRuns finds all named runs for flowName in baseDir.
//
// baseDir is the parent of the fixtures/<flowName>/ directory (typically the
// flow file's directory or the project root). The fixtures directory is
// resolved as baseDir/fixtures/<flowName>/. Each subdirectory of that
// fixtures dir is a named run. A test.yaml manifest inside the run dir can
// override defaults. If WithRunName is set, only that run is returned.
func DiscoverRuns(flowName, baseDir string, opts ...Option) ([]Run, error) {
	o := &options{}
	for _, fn := range opts {
		fn(o)
	}

	fixturesDir := filepath.Join(baseDir, "fixtures", flowName)
	entries, err := os.ReadDir(fixturesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf(
				"flowtest: fixtures dir not found for %q: %s", flowName, fixturesDir)
		}
		return nil, fmt.Errorf("flowtest: DiscoverRuns %q: %w", fixturesDir, err)
	}

	var runs []Run
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		if o.runName != "" && dirName != o.runName {
			continue
		}

		runDir := filepath.Join(fixturesDir, dirName)
		run, err := loadRun(flowName, runDir, dirName)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}

	if o.runName != "" && len(runs) == 0 {
		return nil, fmt.Errorf("flowtest: run %q not found in %s", o.runName, fixturesDir)
	}

	return runs, nil
}

// loadRun constructs a Run from a run directory, optionally reading test.yaml.
func loadRun(flowName, runDir, dirName string) (Run, error) {
	run := Run{
		Name:         dirName,
		Dir:          runDir,
		RecordDir:    filepath.Join(runDir, "record"),
		ContractsDir: filepath.Join(runDir, "contracts"),
	}

	manifestPath := filepath.Join(runDir, "test.yaml")
	data, err := os.ReadFile(manifestPath)
	if err == nil {
		var m RunManifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			return Run{}, fmt.Errorf("flowtest: parse manifest %s: %w", manifestPath, err)
		}
		if m.Name != "" {
			run.Name = m.Name
		}
		run.ExpectedExit = m.ExpectedExit
		run.Passthrough = m.Passthrough
	} else if !os.IsNotExist(err) {
		return Run{}, fmt.Errorf("flowtest: read manifest %s: %w", manifestPath, err)
	}

	_ = flowName // reserved for future manifest validation
	return run, nil
}

// MergePassthrough merges CLI passthrough list with manifest passthrough list.
// CLI flags take precedence: if cliPassthrough is non-empty, it wins entirely.
func MergePassthrough(cliPassthrough, manifestPassthrough []string) []string {
	if len(cliPassthrough) > 0 {
		return cliPassthrough
	}
	return manifestPassthrough
}
