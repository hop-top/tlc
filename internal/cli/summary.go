package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"hop.top/tlc/internal/core"
)

const noProject = "(no project)"

// renderSummary writes a concise status-grouped summary of tasks to w.
// Tasks are grouped by project, then counted by status.
func renderSummary(w io.Writer, tasks []*core.Task) {
	if len(tasks) == 0 {
		_, _ = fmt.Fprintln(w, "No tasks found")
		return
	}

	groups := groupByProject(tasks)
	projectNames := sortedKeys(groups)

	for i, name := range projectNames {
		if i > 0 {
			_, _ = fmt.Fprintln(w)
		}
		_, _ = fmt.Fprintf(w, "Project: %s\n", name)

		counts := statusCounts(groups[name])
		statusNames := sortedKeys(counts)

		for _, status := range statusNames {
			_, _ = fmt.Fprintf(w, "  %-15s %d\n", status, counts[status])
		}
		_, _ = fmt.Fprintf(w, "  %-15s %d\n", "Total", len(groups[name]))
	}
}

// groupByProject partitions tasks by their ProjectID.
// Tasks without a project are grouped under noProject.
func groupByProject(tasks []*core.Task) map[string][]*core.Task {
	groups := make(map[string][]*core.Task)
	for _, t := range tasks {
		key := noProject
		if t.ProjectID != nil && *t.ProjectID != "" {
			key = *t.ProjectID
		}
		groups[key] = append(groups[key], t)
	}
	return groups
}

// statusCounts returns a map of status name to count for a slice of tasks.
func statusCounts(tasks []*core.Task) map[string]int {
	counts := make(map[string]int)
	for _, t := range tasks {
		counts[string(t.Status)]++
	}
	return counts
}

// sortedKeys returns the keys of a map sorted alphabetically.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// renderCounters writes a flat status-count table to w.
// Unlike renderSummary, tasks are not grouped by project.
func renderCounters(w io.Writer, tasks []*core.Task) {
	if len(tasks) == 0 {
		_, _ = fmt.Fprintln(w, "No tasks found")
		return
	}

	counts := statusCounts(tasks)
	statusNames := sortedKeys(counts)

	_, _ = fmt.Fprintln(w, "Status counts:")
	for _, status := range statusNames {
		_, _ = fmt.Fprintf(w, "  %-15s %d\n", status, counts[status])
	}
}

// summaryLine returns a one-line summary string like "3 TODO, 1 DONE (4 total)".
func summaryLine(tasks []*core.Task) string {
	counts := statusCounts(tasks)
	statusNames := sortedKeys(counts)

	parts := make([]string, 0, len(statusNames))
	for _, s := range statusNames {
		parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
	}
	return fmt.Sprintf("%s (%d total)", strings.Join(parts, ", "), len(tasks))
}
