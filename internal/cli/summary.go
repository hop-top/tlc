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
//
// The slice handed in must be the full match set, not a page of it: the
// printed Total is stated as fact and a truncated one is indistinguishable
// from a real one. Callers behind a --limit must clear it first.
func renderSummary(w io.Writer, tasks []*core.Task) {
	if len(tasks) == 0 {
		_, _ = fmt.Fprintln(w, "No tasks found")
		return
	}

	groups := groupByProject(tasks)
	byProject := make(map[string]map[string]int, len(groups))
	for name, group := range groups {
		byProject[name] = statusCounts(group)
	}
	renderSummaryFromCounts(w, byProject)
}

// renderSummaryFromCounts writes the grouped summary from per-project
// status-count maps. Splitting this from renderSummary lets aggregate
// callers pass counts computed in the store, where the count cannot
// depend on page size.
func renderSummaryFromCounts(w io.Writer, byProject map[string]map[string]int) {
	total := 0
	for _, counts := range byProject {
		for _, n := range counts {
			total += n
		}
	}
	if total == 0 {
		_, _ = fmt.Fprintln(w, "No tasks found")
		return
	}

	for i, name := range sortedKeys(byProject) {
		if i > 0 {
			_, _ = fmt.Fprintln(w)
		}
		_, _ = fmt.Fprintf(w, "Project: %s\n", name)

		counts := byProject[name]
		projectTotal := 0
		for _, status := range sortedKeys(counts) {
			_, _ = fmt.Fprintf(w, "  %-15s %d\n", status, counts[status])
			projectTotal += counts[status]
		}
		_, _ = fmt.Fprintf(w, "  %-15s %d\n", "Total", projectTotal)
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
//
// Same contract as renderSummary: the slice must be the full match set.
func renderCounters(w io.Writer, tasks []*core.Task) {
	if len(tasks) == 0 {
		_, _ = fmt.Fprintln(w, "No tasks found")
		return
	}
	renderCountersFromCounts(w, statusCounts(tasks))
}

// renderCountersFromCounts writes the flat counter table from a
// status-count map, letting aggregate callers pass store-computed counts
// that are independent of page size.
func renderCountersFromCounts(w io.Writer, counts map[string]int) {
	total := 0
	for _, n := range counts {
		total += n
	}
	if total == 0 {
		_, _ = fmt.Fprintln(w, "No tasks found")
		return
	}

	_, _ = fmt.Fprintln(w, "Status counts:")
	for _, status := range sortedKeys(counts) {
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
