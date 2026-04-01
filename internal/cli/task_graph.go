package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

var taskGraphFormat string

// TaskGraphCmd renders a dependency graph of tasks filtered by blocker relationships.
var TaskGraphCmd = &cobra.Command{
	Use:   "graph [query]",
	Short: "Render a dependency graph of tasks based on blocker relationships",
	Long: `Render a dependency graph showing tasks as nodes and blocked-by relationships as edges.

Supports the same filters as 'task list': --status, --assigned-to, --tag, --priority, --blocked-by.

Output formats:
  ascii   Text-based ASCII tree (default)
  dot     GraphViz DOT language (pipe to 'dot -Tsvg' or 'dot -Tpng')`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		query := core.Query{
			Limit:           taskListLimit,
			Offset:          taskListOffset,
			SortBy:          taskListSortBy,
			SortDirection:   taskListSortDirection,
			IncludeArchived: taskListArchived,
			AllProjects:     taskListAllProjects,
		}

		if len(args) > 0 {
			query.Search = args[0]
		}

		if taskListMine || taskListAssignedTo == "me" {
			taskListAssignedTo = core.GetCurrentUser()
		}

		statusFlags := taskListStatus
		if !cmd.Flags().Changed("status") && !cmd.Flags().Changed("archived") {
			statusFlags = []string{"IN_PROGRESS", "TODO"}
		}
		for _, st := range statusFlags {
			query.Filters = append(query.Filters, core.FieldFilter{Field: "status", Value: st})
		}
		if taskListAssignedTo != "" {
			query.Filters = append(query.Filters, core.FieldFilter{Field: "assigned_to", Value: taskListAssignedTo})
		}
		for _, tag := range taskListTag {
			query.Filters = append(query.Filters, core.FieldFilter{Field: "tags", Operator: core.OpContains, Value: tag})
		}
		for _, p := range taskListPriority {
			query.Filters = append(query.Filters, core.FieldFilter{Field: "priority", Value: strings.ToUpper(p)})
		}

		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		tasks, err := s.ListTasks(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to list tasks: %w", err)
		}

		// Post-query filter for --stale, --blocked, --blocked-by.
		if taskListStale || taskListBlocked || len(taskListBlockedBy) > 0 {
			filtered := tasks[:0]
			for _, t := range tasks {
				if taskListStale && !t.IsStale() {
					continue
				}
				if taskListBlocked && !t.IsBlocked() {
					continue
				}
				if len(taskListBlockedBy) > 0 && !taskBlockedByAny(t, taskListBlockedBy) {
					continue
				}
				filtered = append(filtered, t)
			}
			tasks = filtered
		}

		if len(tasks) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No tasks found.")
			return nil
		}

		format := taskGraphFormat
		if format == "" {
			format = viper.GetString("graph.format")
		}
		if format == "" {
			format = "ascii"
		}

		out := cmd.OutOrStdout()
		switch format {
		case "dot":
			renderDOT(out, tasks)
		default:
			renderASCIIGraph(out, tasks)
		}
		return nil
	},
}

// taskGraph holds the adjacency data for rendering.
type taskGraph struct {
	// nodes: id -> task
	nodes map[string]*core.Task
	// edges: id -> list of ids it is blocked by (that are in-scope)
	blockedBy map[string][]string
	// reverseEdges: id -> list of ids it blocks (that are in-scope)
	blocks map[string][]string
	// roots: tasks not blocked by anything in the graph
	roots []string
}

// buildGraph constructs a graph from the given task slice.
// Only edges whose blocker target is present in the task set are kept.
func buildGraph(tasks []*core.Task) *taskGraph {
	g := &taskGraph{
		nodes:     make(map[string]*core.Task, len(tasks)),
		blockedBy: make(map[string][]string, len(tasks)),
		blocks:    make(map[string][]string, len(tasks)),
	}

	for _, t := range tasks {
		g.nodes[t.ID] = t
	}

	for _, t := range tasks {
		for _, dep := range t.BlockedBy() {
			// Normalise: strip project prefix for local-only lookup.
			depID := dep
			if idx := strings.LastIndex(dep, "/"); idx >= 0 {
				depID = dep[idx+1:]
			}
			if _, ok := g.nodes[depID]; ok {
				g.blockedBy[t.ID] = append(g.blockedBy[t.ID], depID)
				g.blocks[depID] = append(g.blocks[depID], t.ID)
			}
		}
	}

	// Roots: tasks that have no in-scope blockers.
	for id := range g.nodes {
		if len(g.blockedBy[id]) == 0 {
			g.roots = append(g.roots, id)
		}
	}
	sort.Strings(g.roots)
	return g
}

// nodeLabel returns a concise single-line label for a task node.
func nodeLabel(t *core.Task) string {
	title := t.Title
	if len(title) > 45 {
		title = title[:42] + "..."
	}
	return fmt.Sprintf("%s: %s [%s]", t.ID, title, t.Status)
}

// renderASCIIGraph prints a text-based graph.
//
// Layout strategy: topological levels — each task appears at the leftmost level
// consistent with all its blockers appearing above it.  When a task has no
// in-scope blockers it is a root at level 0.  Tasks that appear in multiple
// sub-trees are printed once (at the deepest level they would naturally occupy).
func renderASCIIGraph(w io.Writer, tasks []*core.Task) {
	g := buildGraph(tasks)

	// Topological level assignment (Kahn-style BFS).
	level := make(map[string]int, len(g.nodes))
	inDegree := make(map[string]int, len(g.nodes))
	for id := range g.nodes {
		inDegree[id] = len(g.blockedBy[id])
	}

	queue := make([]string, 0, len(g.roots))
	for _, r := range g.roots {
		queue = append(queue, r)
		level[r] = 0
	}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, child := range sortedStrings(g.blocks[cur]) {
			if l := level[cur] + 1; l > level[child] {
				level[child] = l
			}
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	// Group by level.
	maxLevel := 0
	for _, l := range level {
		if l > maxLevel {
			maxLevel = l
		}
	}
	byLevel := make([][]string, maxLevel+1)
	for id, l := range level {
		byLevel[l] = append(byLevel[l], id)
	}
	for i := range byLevel {
		sort.Strings(byLevel[i])
	}

	// Handle tasks not reached by BFS (cycles / disconnected from roots).
	inGraph := make(map[string]bool, len(level))
	for id := range level {
		inGraph[id] = true
	}

	_, _ = fmt.Fprintln(w, "Task Dependency Graph")
	_, _ = fmt.Fprintln(w, strings.Repeat("─", 60))

	printed := make(map[string]bool, len(g.nodes))

	for l, ids := range byLevel {
		indent := strings.Repeat("  ", l)
		prefix := indent
		if l > 0 {
			prefix = indent + "└─ "
		}
		for _, id := range ids {
			t := g.nodes[id]
			_, _ = fmt.Fprintf(w, "%s%s\n", prefix, nodeLabel(t))
			printed[id] = true
		}
	}

	// Orphaned (cycle members or unreachable).
	var orphans []string
	for _, t := range tasks {
		if !printed[t.ID] {
			orphans = append(orphans, t.ID)
		}
	}
	sort.Strings(orphans)
	if len(orphans) > 0 {
		_, _ = fmt.Fprintln(w, "\n(tasks with circular or unresolved dependencies)")
		for _, id := range orphans {
			_, _ = fmt.Fprintf(w, "  ? %s\n", nodeLabel(g.nodes[id]))
		}
	}

	_, _ = fmt.Fprintln(w, strings.Repeat("─", 60))
	_, _ = fmt.Fprintf(w, "%d task(s)\n", len(tasks))
}

// renderDOT outputs a GraphViz DOT representation.
func renderDOT(w io.Writer, tasks []*core.Task) {
	g := buildGraph(tasks)

	_, _ = fmt.Fprintln(w, "digraph tasks {")
	_, _ = fmt.Fprintln(w, `  rankdir=LR;`)
	_, _ = fmt.Fprintln(w, `  node [shape=box, style=filled, fontname="monospace"];`)

	// Node definitions.
	ids := make([]string, 0, len(g.nodes))
	for id := range g.nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		t := g.nodes[id]
		color := dotColor(t.Status)
		label := dotEscape(fmt.Sprintf("%s\\n%s\\n[%s]", t.ID, truncate(t.Title, 40), t.Status))
		_, _ = fmt.Fprintf(w, "  %q [label=%q, fillcolor=%q];\n", id, label, color)
	}

	// Edges: blocker → blocked (arrow direction: A blocks B means A→B).
	_, _ = fmt.Fprintln(w)
	for _, id := range ids {
		for _, dep := range sortedStrings(g.blockedBy[id]) {
			// dep blocks id
			_, _ = fmt.Fprintf(w, "  %q -> %q;\n", dep, id)
		}
	}

	_, _ = fmt.Fprintln(w, "}")
}

// dotColor maps a task status to a GraphViz fill color.
func dotColor(status core.TaskStatus) string {
	switch status {
	case core.StatusTodo:
		return "#ffffff"
	case core.StatusInProgress:
		return "#aee4f8"
	case core.StatusDone:
		return "#a8e6a3"
	case core.StatusSkipped:
		return "#f5dfa5"
	default:
		return "#eeeeee"
	}
}

// dotEscape handles basic label quoting for DOT output.
func dotEscape(s string) string {
	return s
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func sortedStrings(ss []string) []string {
	out := make([]string, len(ss))
	copy(out, ss)
	sort.Strings(out)
	return out
}

func init() {
	TaskGraphCmd.Flags().StringSliceVarP(&taskListStatus, "status", "s", []string{}, "Filter by status")
	TaskGraphCmd.Flags().StringVarP(&taskListAssignedTo, "assigned-to", "a", "", "Filter by assignee")
	TaskGraphCmd.Flags().StringSliceVar(&taskListTag, "tag", []string{}, "Filter by tag")
	TaskGraphCmd.Flags().BoolVar(&taskListMine, "mine", false, "Filter by current user")
	TaskGraphCmd.Flags().BoolVar(&taskListArchived, "archived", false, "Show archived tasks")
	TaskGraphCmd.Flags().BoolVar(&taskListAllProjects, "all-projects", false, "Show tasks from all projects")
	TaskGraphCmd.Flags().StringVar(&taskListSortBy, "sort-by", "created_at", "Sort field")
	TaskGraphCmd.Flags().StringVar(&taskListSortDirection, "sort-direction", "desc", "Sort direction (asc, desc)")
	TaskGraphCmd.Flags().IntVarP(&taskListLimit, "limit", "n", 100, "Limit results")
	TaskGraphCmd.Flags().IntVar(&taskListOffset, "offset", 0, "Skip results")
	TaskGraphCmd.Flags().BoolVar(&taskListStale, "stale", false, "Show only stale tasks")
	TaskGraphCmd.Flags().BoolVar(&taskListBlocked, "blocked", false, "Show only blocked tasks")
	TaskGraphCmd.Flags().StringSliceVar(&taskListPriority, "priority", []string{}, "Filter by priority (P0, P1, P2, P3)")
	TaskGraphCmd.Flags().StringSliceVar(&taskListBlockedBy, "blocked-by", []string{}, "Show only tasks blocked by the given task IDs")
	TaskGraphCmd.Flags().StringVar(&taskGraphFormat, "format", "", "Output format: ascii (default), dot")
}
