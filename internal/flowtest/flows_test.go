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
			defer f.Close()

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

// TestFlowAgentFieldParsedFromDisk verifies that a flow YAML with agent: field
// on disk round-trips correctly through ParseFlow.
func TestFlowAgentFieldParsedFromDisk(t *testing.T) {
	path := filepath.Join(flowsDir, "writing-plans.yaml")
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	flow, err := core.ParseFlow(f, path)
	require.NoError(t, err)
	assert.Equal(t, "claude", flow.Agent.Name, "writing-plans.yaml should declare agent: claude")
}

// TestAdapterResolverDispatchesCorrectBinary verifies that AdapterResolver
// picks the adapter whose Binary() matches the flow's declared agent.
func TestAdapterResolverDispatchesCorrectBinary(t *testing.T) {
	adapters := map[string]flowtest.AgentAdapter{
		"claude": flowtest.NewClaudeAdapter(),
		"llm":    flowtest.NewLLMAdapter(),
	}
	flow := &core.Flow{Agent: core.AgentRef{Name: "llm"}}
	resolver := flowtest.NewAdapterResolver(adapters, flow, nil)

	step := core.Step{ID: "s", Type: core.StepTypeTask, Title: "s"}
	a, _, err := resolver.Resolve(step)
	require.NoError(t, err)
	assert.Equal(t, "llm", a.Name())
	assert.Equal(t, "llm", a.Binary())
}

// TestAdapterResolverUnknownAgentFatal verifies that an unknown adapter name
// produces a clear error message naming the adapter.
func TestAdapterResolverUnknownAgentFatal(t *testing.T) {
	adapters := map[string]flowtest.AgentAdapter{
		"claude": flowtest.NewClaudeAdapter(),
	}
	flow := &core.Flow{Agent: core.AgentRef{Name: "does-not-exist"}}
	resolver := flowtest.NewAdapterResolver(adapters, flow, nil)

	step := core.Step{ID: "s", Type: core.StepTypeTask, Title: "s"}
	_, _, err := resolver.Resolve(step)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist")
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
