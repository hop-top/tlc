//go:build !shimbin

package flowtest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/flowtest"
)

// flowsDir is the canonical location of all example flow definitions.
const flowsDir = "../../examples/flows"

// allFlows returns all YAML flow files under flowsDir.
func allFlows(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(flowsDir)
	require.NoError(t, err, "read flows dir")

	var flows []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		flows = append(flows, filepath.Join(flowsDir, e.Name()))
	}
	require.NotEmpty(t, flows, "no flow files found in %s", flowsDir)
	return flows
}

// TestAllFlowsParse verifies every flow YAML parses without error.
func TestAllFlowsParse(t *testing.T) {
	for _, path := range allFlows(t) {
		path := path
		name := strings.TrimSuffix(filepath.Base(path), ".yaml")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, err := os.Open(path)
			require.NoError(t, err)
			defer f.Close() //nolint:errcheck

			flow, err := core.ParseFlow(f, path)
			require.NoError(t, err, "ParseFlow failed")

			assert.NotEmpty(t, flow.ID, "flow_id must be set")
			assert.NotEmpty(t, flow.EntryStep, "entry_step must be set")
			assert.NotEmpty(t, flow.Steps, "steps must not be empty")

			// entry_step must exist in steps map
			_, ok := flow.Steps[flow.EntryStep]
			assert.True(t, ok, "entry_step %q not found in steps", flow.EntryStep)
		})
	}
}

// TestAllFlowsHaveHappyPathFixture verifies each flow has a happy-path run scaffold.
// This fails until fixtures are created — driving fixture creation via TDD.
func TestAllFlowsHaveHappyPathFixture(t *testing.T) {
	for _, path := range allFlows(t) {
		path := path
		name := strings.TrimSuffix(filepath.Base(path), ".yaml")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			flowBaseDir := filepath.Dir(path)

			runs, err := flowtest.DiscoverRuns(name, flowBaseDir, flowtest.WithRunName("happy-path"))
			require.NoError(t, err, "DiscoverRuns should find happy-path for %s", name)
			require.Len(t, runs, 1)
			assert.Equal(t, "happy-path", runs[0].Name)
			assert.Equal(t, 0, runs[0].ExpectedExit)
		})
	}
}
