package core

import (
	"fmt"
	"sort"
)

// DepGraph represents a directed acyclic graph of task dependencies
// built from blocked-by relationships.
type DepGraph struct {
	tasks   map[string]*Task   // task ID → task
	adj     map[string][]string // task ID → list of dependents (tasks blocked by this)
	reverse map[string][]string // task ID → list of dependencies (tasks this is blocked by)
	ids     []string            // all task IDs in deterministic order
}

// Batch represents a group of tasks that can execute together.
type Batch struct {
	Index    int     `json:"index" yaml:"index"`
	Tasks    []*Task `json:"tasks" yaml:"tasks"`
	Parallel bool    `json:"parallel" yaml:"parallel"`
}

// ExecutionStrategy describes how tasks should execute respecting deps.
type ExecutionStrategy struct {
	Batches        []Batch  `json:"batches" yaml:"batches"`
	CriticalPath   []string `json:"critical_path" yaml:"critical_path"`
	TotalTasks     int      `json:"total_tasks" yaml:"total_tasks"`
	MaxParallelism int      `json:"max_parallelism" yaml:"max_parallelism"`
}

// NewDepGraph builds a dependency graph from a slice of tasks using
// their blocked-by metadata. Tasks whose blocked-by references point
// to IDs not in the provided set are silently ignored (cross-project deps).
func NewDepGraph(tasks []*Task) (*DepGraph, error) {
	g := &DepGraph{
		tasks:   make(map[string]*Task, len(tasks)),
		adj:     make(map[string][]string),
		reverse: make(map[string][]string),
		ids:     make([]string, 0, len(tasks)),
	}

	for _, t := range tasks {
		if t == nil {
			continue
		}
		g.tasks[t.ID] = t
		g.ids = append(g.ids, t.ID)
	}
	sort.Strings(g.ids)

	for _, t := range tasks {
		if t == nil {
			continue
		}
		for _, dep := range t.BlockedBy() {
			if _, ok := g.tasks[dep]; !ok {
				continue // cross-project or unknown dep; skip
			}
			g.adj[dep] = append(g.adj[dep], t.ID)
			g.reverse[t.ID] = append(g.reverse[t.ID], dep)
		}
	}

	return g, nil
}

// TopologicalSort returns task IDs in topological order using Kahn's
// algorithm. Returns an error if the graph contains a cycle.
func (g *DepGraph) TopologicalSort() ([]string, error) {
	inDegree := make(map[string]int, len(g.ids))
	for _, id := range g.ids {
		inDegree[id] = len(g.reverse[id])
	}

	// Seed queue with nodes having zero in-degree.
	queue := make([]string, 0)
	for _, id := range g.ids {
		if inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}

	var result []string
	for len(queue) > 0 {
		// Sort queue for deterministic output.
		sort.Strings(queue)
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		deps := make([]string, len(g.adj[node]))
		copy(deps, g.adj[node])
		sort.Strings(deps)
		for _, dep := range deps {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(result) != len(g.ids) {
		return nil, fmt.Errorf(
			"dependency cycle detected: processed %d of %d tasks; "+
				"run 'tlc task list --track <id>' to inspect blocked-by refs",
			len(result), len(g.ids),
		)
	}

	return result, nil
}

// ComputeStrategy computes an ExecutionStrategy with BFS-level batching.
// Tasks whose dependencies are all satisfied by prior batches are grouped
// into the current batch.
func (g *DepGraph) ComputeStrategy() (*ExecutionStrategy, error) {
	sorted, err := g.TopologicalSort()
	if err != nil {
		return nil, err
	}

	if len(sorted) == 0 {
		return &ExecutionStrategy{}, nil
	}

	// Assign each task to a batch level = max(dep levels) + 1.
	level := make(map[string]int, len(sorted))
	for _, id := range sorted {
		maxDepLevel := -1
		for _, dep := range g.reverse[id] {
			if l, ok := level[dep]; ok && l > maxDepLevel {
				maxDepLevel = l
			}
		}
		level[id] = maxDepLevel + 1
	}

	// Group tasks by level.
	maxLevel := 0
	for _, l := range level {
		if l > maxLevel {
			maxLevel = l
		}
	}

	batches := make([]Batch, maxLevel+1)
	for i := range batches {
		batches[i] = Batch{Index: i}
	}
	for _, id := range sorted {
		l := level[id]
		batches[l].Tasks = append(batches[l].Tasks, g.tasks[id])
	}

	maxPar := 0
	for i := range batches {
		batches[i].Parallel = len(batches[i].Tasks) > 1
		if len(batches[i].Tasks) > maxPar {
			maxPar = len(batches[i].Tasks)
		}
	}

	return &ExecutionStrategy{
		Batches:        batches,
		CriticalPath:   g.CriticalPath(),
		TotalTasks:     len(sorted),
		MaxParallelism: maxPar,
	}, nil
}

// CriticalPath returns the longest dependency chain (by task count).
func (g *DepGraph) CriticalPath() []string {
	if len(g.ids) == 0 {
		return nil
	}

	// Compute longest path from each node using memoization.
	memo := make(map[string][]string, len(g.ids))
	var longest func(id string) []string
	longest = func(id string) []string {
		if cached, ok := memo[id]; ok {
			return cached
		}

		deps := make([]string, len(g.adj[id]))
		copy(deps, g.adj[id])
		sort.Strings(deps)

		best := []string{id}
		for _, dep := range deps {
			sub := longest(dep)
			candidate := append([]string{id}, sub...)
			if len(candidate) > len(best) {
				best = candidate
			}
		}
		memo[id] = best
		return best
	}

	var critPath []string
	roots := make([]string, 0)
	for _, id := range g.ids {
		if len(g.reverse[id]) == 0 {
			roots = append(roots, id)
		}
	}
	sort.Strings(roots)

	for _, root := range roots {
		path := longest(root)
		if len(path) > len(critPath) {
			critPath = path
		}
	}

	return critPath
}

// Roots returns task IDs with no dependencies (entry points).
func (g *DepGraph) Roots() []string {
	var roots []string
	for _, id := range g.ids {
		if len(g.reverse[id]) == 0 {
			roots = append(roots, id)
		}
	}
	return roots
}

// Dependents returns the IDs of tasks directly blocked by the given task.
func (g *DepGraph) Dependents(id string) []string {
	return g.adj[id]
}

// Dependencies returns the IDs of tasks that block the given task.
func (g *DepGraph) Dependencies(id string) []string {
	return g.reverse[id]
}

// HasDeps returns true if any task in the graph has blocked-by deps.
func (g *DepGraph) HasDeps() bool {
	for _, deps := range g.reverse {
		if len(deps) > 0 {
			return true
		}
	}
	return false
}
