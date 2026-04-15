package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"hop.top/tlc/internal/core"
)

var flowDryRun bool

// dryRunFlow validates and prints the execution plan for a parsed flow
// without dispatching agents.
func dryRunFlow(out io.Writer, flow *core.Flow) error {
	order, err := flowTopoSort(flow)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(out, "Dry-run: %s (%s)\n", flow.Name, flow.ID)
	_, _ = fmt.Fprintf(out, "Steps: %d\n\n", len(order))

	for i, stepID := range order {
		step := flow.Steps[stepID]
		agent := resolveStepAgent(flow, step)

		_, _ = fmt.Fprintf(out, "%d. [%s] %s\n", i+1, stepID, step.Title)
		_, _ = fmt.Fprintf(out, "   type: %s\n", step.Type)

		if agent != "" {
			_, _ = fmt.Fprintf(out, "   agent: %s\n", agent)
		}

		if len(step.DependsOn) > 0 {
			_, _ = fmt.Fprintf(out, "   depends_on: %s\n",
				strings.Join(step.DependsOn, ", "))
		}

		_, _ = fmt.Fprintln(out)
	}

	_, _ = fmt.Fprintln(out, "No agents dispatched (dry-run)")
	return nil
}

// flowTopoSort returns step IDs in topological (execution) order using
// Kahn's algorithm over depends_on edges.
func flowTopoSort(flow *core.Flow) ([]string, error) {
	// Build in-degree map.
	inDegree := make(map[string]int, len(flow.Steps))
	for id := range flow.Steps {
		inDegree[id] = 0
	}
	for id, step := range flow.Steps {
		for _, dep := range step.DependsOn {
			inDegree[id]++
			_ = dep // edge from dep → id
		}
	}

	// Seed queue with zero-in-degree nodes.
	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)

	var result []string
	for len(queue) > 0 {
		sort.Strings(queue)
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		// Find all steps that depend on this node.
		for id, step := range flow.Steps {
			for _, dep := range step.DependsOn {
				if dep == node {
					inDegree[id]--
					if inDegree[id] == 0 {
						queue = append(queue, id)
					}
				}
			}
		}
	}

	if len(result) != len(flow.Steps) {
		return nil, fmt.Errorf("cycle detected in flow steps")
	}

	return result, nil
}

// resolveStepAgent returns the effective agent name for a step,
// falling back to the flow-level default.
func resolveStepAgent(flow *core.Flow, step core.Step) string {
	if !step.Agent.IsZero() {
		return step.Agent.Name
	}
	if !flow.Agent.IsZero() {
		return flow.Agent.Name
	}
	return ""
}

func init() {
	FlowRunCmd.Flags().BoolVar(&flowDryRun, "dry-run", false,
		"Preview execution plan without running agents")
}
