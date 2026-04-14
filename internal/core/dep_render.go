package core

import (
	"fmt"
	"sort"
	"strings"
)

// RenderDepTree renders a dependency tree as indented ASCII with
// box-drawing characters. Tasks show ID + title; multi-parent nodes
// annotate their additional dependencies.
func RenderDepTree(strategy *ExecutionStrategy, tasks []*Task) string {
	if strategy == nil || len(strategy.Batches) == 0 {
		return ""
	}

	taskMap := make(map[string]*Task, len(tasks))
	for _, t := range tasks {
		taskMap[t.ID] = t
	}

	// Build adjacency from strategy batches + task deps.
	graph, err := NewDepGraph(tasks)
	if err != nil {
		return ""
	}

	var b strings.Builder
	visited := make(map[string]bool)

	roots := graph.Roots()
	sort.Strings(roots)

	for _, root := range roots {
		renderNode(&b, graph, taskMap, root, "", true, visited)
	}

	return b.String()
}

func renderNode(
	b *strings.Builder,
	g *DepGraph,
	tasks map[string]*Task,
	id string,
	prefix string,
	isLast bool,
	visited map[string]bool,
) {
	t := tasks[id]
	if t == nil {
		return
	}

	// Connector character.
	connector := "\u251c\u2192 " // ├→
	if isLast {
		connector = "\u2514\u2192 " // └→
	}
	// Root nodes get no connector.
	if prefix == "" {
		connector = ""
	}

	// Multi-parent annotation.
	deps := g.Dependencies(id)
	annotation := ""
	if len(deps) > 1 {
		depIDs := make([]string, len(deps))
		copy(depIDs, deps)
		sort.Strings(depIDs)
		annotation = fmt.Sprintf(" [depends: %s]", strings.Join(depIDs, ", "))
	}

	fmt.Fprintf(b, "%s%s%s (%s)%s\n", prefix, connector, t.ID, t.Title, annotation)

	if visited[id] {
		return
	}
	visited[id] = true

	// Child prefix for subsequent lines.
	childPrefix := prefix
	if prefix != "" {
		if isLast {
			childPrefix += "    "
		} else {
			childPrefix += "\u2502   " // │
		}
	} else {
		childPrefix = "  "
	}

	children := g.Dependents(id)
	sort.Strings(children)
	for i, child := range children {
		last := i == len(children)-1
		renderNode(b, g, tasks, child, childPrefix, last, visited)
	}
}

// RenderBatchSummary renders a compact execution strategy summary.
//
// Format:
//
//	Execution Strategy (N batches, max parallelism: M):
//	  Batch 1 (sequential, 1 agent): T-0074 → T-0075 → ...
//	  Batch 2 (parallel, 4 agents): T-0079, T-0080, T-0081, T-0084
func RenderBatchSummary(strategy *ExecutionStrategy) string {
	if strategy == nil || len(strategy.Batches) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Execution Strategy (%d batches, max parallelism: %d):\n",
		len(strategy.Batches), strategy.MaxParallelism)

	for _, batch := range strategy.Batches {
		ids := make([]string, len(batch.Tasks))
		for i, t := range batch.Tasks {
			ids[i] = t.ID
		}

		mode := "sequential"
		agents := "1 agent"
		sep := " \u2192 " // →
		if batch.Parallel {
			mode = "parallel"
			agents = fmt.Sprintf("%d agents", len(batch.Tasks))
			sep = ", "
		}

		fmt.Fprintf(&b, "  Batch %d (%s, %s): %s\n",
			batch.Index+1, mode, agents, strings.Join(ids, sep))
	}

	return b.String()
}

// RenderMermaid generates a Mermaid flowchart TB diagram from the
// execution strategy. Edges represent blocked-by relationships.
// Tasks are grouped into subgraphs per batch.
func RenderMermaid(strategy *ExecutionStrategy, tasks []*Task) string {
	if strategy == nil || len(strategy.Batches) == 0 {
		return ""
	}

	graph, err := NewDepGraph(tasks)
	if err != nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("flowchart TB\n")

	// Emit edges (dependency arrows).
	sorted, sErr := graph.TopologicalSort()
	if sErr != nil {
		return ""
	}
	for _, id := range sorted {
		deps := graph.Dependents(id)
		sort.Strings(deps)
		for _, dep := range deps {
			b.WriteString(fmt.Sprintf("  %s --> %s\n",
				mermaidNodeID(id), mermaidNodeID(dep)))
		}
	}

	// Emit batch subgraphs with node definitions.
	for _, batch := range strategy.Batches {
		if len(batch.Tasks) == 0 {
			continue
		}

		mode := "sequential"
		if batch.Parallel {
			mode = "parallel"
		}
		label := fmt.Sprintf("Batch %d (%s)", batch.Index+1, mode)
		b.WriteString(fmt.Sprintf("  subgraph Batch%d[\"%s\"]\n",
			batch.Index+1, label))

		for _, t := range batch.Tasks {
			nid := mermaidNodeID(t.ID)
			b.WriteString(fmt.Sprintf("    %s[\"%s %s\"]\n",
				nid, t.ID, escapeMermaid(t.Title)))
		}

		b.WriteString("  end\n")
	}

	return b.String()
}

// mermaidNodeID converts a task ID like "T-0074" to a valid Mermaid
// node identifier "T0074" (no hyphens).
func mermaidNodeID(id string) string {
	return strings.ReplaceAll(id, "-", "")
}

// escapeMermaid escapes characters that are special in Mermaid labels.
func escapeMermaid(s string) string {
	return strings.ReplaceAll(s, "\"", "#quot;")
}
