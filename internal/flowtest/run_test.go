package flowtest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hop.top/tlc/internal/flowtest"
)

// makeFixturesRoot creates a run dir at <root>/fixtures/<flowName>/<runName>/
// and optionally writes a test.yaml manifest. Returns the project root (to
// pass as baseDir to DiscoverRuns).
func makeFixturesRoot(t *testing.T, flowName, runName string, manifest []byte) string {
	t.Helper()
	root := t.TempDir()
	runDir := filepath.Join(root, "fixtures", flowName, runName)
	require.NoError(t, os.MkdirAll(filepath.Join(runDir, "record"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(runDir, "contracts"), 0o755))
	if manifest != nil {
		require.NoError(t, os.WriteFile(filepath.Join(runDir, "test.yaml"), manifest, 0o644))
	}
	return root
}

func TestRunResolutionByDirName(t *testing.T) {
	root := makeFixturesRoot(t, "pr-review-loop", "happy-path", nil)
	runs, err := flowtest.DiscoverRuns("pr-review-loop", root)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, "happy-path", runs[0].Name)
	assert.Equal(t, 0, runs[0].ExpectedExit)
	assert.Empty(t, runs[0].Passthrough)
}

func TestRunResolutionWithManifest(t *testing.T) {
	manifest := []byte(`
expected_exit: 1
passthrough:
  - docker
description: hits max_iterations cap
`)
	root := makeFixturesRoot(t, "pr-review-loop", "max-iterations", manifest)
	runs, err := flowtest.DiscoverRuns("pr-review-loop", root)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, 1, runs[0].ExpectedExit)
	assert.Equal(t, []string{"docker"}, runs[0].Passthrough)
}

func TestRunResolutionSingleNamed(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"happy-path", "edge-case"} {
		runDir := filepath.Join(root, "fixtures", "pr-review-loop", name)
		require.NoError(t, os.MkdirAll(filepath.Join(runDir, "record"), 0o755))
	}

	runs, err := flowtest.DiscoverRuns("pr-review-loop", root, flowtest.WithRunName("happy-path"))
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, "happy-path", runs[0].Name)
}

func TestRunResolutionNotFound(t *testing.T) {
	root := t.TempDir()
	fixturesDir := filepath.Join(root, "fixtures", "pr-review-loop", "some-run")
	require.NoError(t, os.MkdirAll(fixturesDir, 0o755))
	_, err := flowtest.DiscoverRuns("pr-review-loop", root, flowtest.WithRunName("missing"))
	require.Error(t, err)
}

func TestMergePassthrough(t *testing.T) {
	// CLI wins when non-empty
	got := flowtest.MergePassthrough([]string{"wrangler"}, []string{"docker"})
	assert.Equal(t, []string{"wrangler"}, got)

	// manifest used when CLI empty
	got = flowtest.MergePassthrough(nil, []string{"docker"})
	assert.Equal(t, []string{"docker"}, got)

	// both empty
	got = flowtest.MergePassthrough(nil, nil)
	assert.Nil(t, got)
}

func TestRunResolutionManifestVars(t *testing.T) {
	manifest := []byte(`
expected_exit: 0
vars:
  pr: "42"
  depth: deep
`)
	root := makeFixturesRoot(t, "code-review", "happy-path", manifest)
	runs, err := flowtest.DiscoverRuns("code-review", root)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, map[string]string{"pr": "42", "depth": "deep"}, runs[0].Vars)
}

func TestRunResolutionNoManifestHasNoVars(t *testing.T) {
	root := makeFixturesRoot(t, "code-review", "happy-path", nil)
	runs, err := flowtest.DiscoverRuns("code-review", root)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Empty(t, runs[0].Vars)
}
