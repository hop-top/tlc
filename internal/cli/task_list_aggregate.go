package cli

// Aggregate output for `task list` (`--summary` / `--counters`, and the same
// two as `--format` values). Aggregates count the match set, never a page of
// it, so everything that decides *whether* a run is an aggregate, and how
// its counts are obtained, lives here rather than inline in the RunE.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// aggregateFormat returns the aggregate output format the invocation
// selects, or "" for ordinary list output.
//
// An aggregate has more than one spelling and they must agree. `--counters`
// and `--summary` are the flag form; `--format counters` / `-f summary` and
// a config `output.format: summary` reach the same renderers through
// output.format. Keying on the bools alone left the format spellings on the
// paginated path, silently reporting the page size as the count — so the
// effective output.format is consulted too.
//
// --counters wins over --summary, matching the flag precedence the tail-end
// format resolution has always applied.
func aggregateFormat() string {
	if taskListCounters {
		return formatCounters
	}
	if taskListSummary {
		return formatSummary
	}
	return aggregateFormatValue(viper.GetString("output.format"))
}

// aggregateFormatValue maps an output.format value to its aggregate format,
// or "" when the value names a non-aggregate renderer.
func aggregateFormatValue(format string) string {
	switch format {
	case formatCounters, formatSummary:
		return format
	default:
		return ""
	}
}

// taskPostFilter is a predicate applied to tasks after the store query,
// expressing a condition the WHERE fragment cannot.
type taskPostFilter func(*core.Task) bool

// taskListPostFilters returns the filters this invocation applies in Go
// after the store query returns.
//
// The aggregate fast path is gated on this slice being empty, rather than on
// an allowlist of flag names kept in sync by hand. An allowlist has to
// mirror the filter block below it, and the next filter added would be
// missing from the copy — over-counting silently, which is the defect class
// the counting path exists to fix. Here a filter cannot be added without
// also disqualifying the fast path, because they are the same list.
func taskListPostFilters(trackQIDs []core.QualifiedTrackID) []taskPostFilter {
	var filters []taskPostFilter

	if taskListStale {
		filters = append(filters, func(t *core.Task) bool { return t.IsStale() })
	}
	if len(taskListBlockedBy) > 0 {
		ids := taskListBlockedBy
		filters = append(filters, func(t *core.Task) bool { return taskBlockedByAny(t, ids) })
	}
	if len(trackQIDs) > 0 {
		qids := trackQIDs
		filters = append(filters, func(t *core.Task) bool { return taskMatchesQualifiedTracks(t, qids) })
	}

	return filters
}

// applyPostFilters keeps only the tasks every filter accepts.
func applyPostFilters(tasks []*core.Task, filters []taskPostFilter) []*core.Task {
	if len(filters) == 0 {
		return tasks
	}
	kept := tasks[:0]
	for _, t := range tasks {
		match := true
		for _, keep := range filters {
			if !keep(t) {
				match = false
				break
			}
		}
		if match {
			kept = append(kept, t)
		}
	}
	return kept
}

// renderAggregateCounts writes store-computed counts in the selected
// aggregate format. Counts arrive grouped by project because that is the
// dimension --summary renders; --counters flattens them by summing across
// projects, which is exactly what a flat counter table means.
func renderAggregateCounts(
	cmd *cobra.Command, format string, byProject map[string]map[string]int,
) error {
	out := cmd.OutOrStdout()
	switch format {
	case formatCounters:
		renderCountersFromCounts(out, flattenProjectCounts(byProject))
	case formatSummary:
		renderSummaryFromCounts(out, labelProjectCounts(byProject))
	default:
		return fmt.Errorf("unsupported aggregate format %q; valid values: summary, counters", format)
	}
	return nil
}

// flattenProjectCounts sums per-project status counts into one status map.
func flattenProjectCounts(byProject map[string]map[string]int) map[string]int {
	flat := make(map[string]int)
	for _, counts := range byProject {
		for status, n := range counts {
			flat[status] += n
		}
	}
	return flat
}

// labelProjectCounts replaces the store's empty-string project key with the
// display label the slice-path renderer uses, so both paths print alike.
func labelProjectCounts(byProject map[string]map[string]int) map[string]map[string]int {
	labeled := make(map[string]map[string]int, len(byProject))
	for project, counts := range byProject {
		key := project
		if key == "" {
			key = noProject
		}
		labeled[key] = counts
	}
	return labeled
}

// noteIgnoredPagination reports on stderr that pagination was dropped —
// but only when the note carries information.
//
// The note exists because silently resolving two contradictory flags is what
// produced the original defect. Firing it on --limit having been *typed*
// gets that wrong twice over: an explicit -n above the match set could not
// have truncated anything, so the note is noise; and a config
// `defaults.limit` that would have truncated never flips pflag's Changed
// bit, so the case that needed telling stayed silent. Pagination was
// requested (by flag or by config) AND the match set exceeds it: both, or
// no note.
func noteIgnoredPagination(cmd *cobra.Command, aggregate string, requested bool, total int) {
	if !requested {
		return
	}
	limit := taskListLimit
	if limit <= 0 || total <= limit {
		return
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
		"note: --limit/--offset ignored with --%s; counted all %d matches, not the first %d\n",
		aggregate, total, limit)
}

// totalProjectCounts totals every status bucket across every project.
func totalProjectCounts(byProject map[string]map[string]int) int {
	total := 0
	for _, counts := range byProject {
		total += sumCounts(counts)
	}
	return total
}

// applyStaleDefaults seeds the project default stale timeout on tasks with
// none of their own, then auto-fires stale hooks once per crossing
// (StaleFiredAt == nil guards re-fire), persisting the timestamp.
func applyStaleDefaults(ctx context.Context, s taskStaleUpdater, tasks []*core.Task) {
	var taskCfg config.TaskConfig
	_ = viper.UnmarshalKey("task", &taskCfg) //nolint:errcheck // best-effort config load
	_ = taskCfg.Validate()                   //nolint:errcheck // best-effort validation

	for _, t := range tasks {
		if t.StaleTimeout == nil && taskCfg.Stale.DefaultTimeout > 0 {
			d := taskCfg.Stale.DefaultTimeout
			t.StaleTimeout = &d
		}
	}

	if len(taskCfg.Stale.Hooks) == 0 {
		return
	}
	now := time.Now().UTC()
	for _, t := range tasks {
		if t.IsStale() && t.StaleFiredAt == nil {
			_ = core.RunStaleHooks(t, taskCfg.Stale.Hooks) //nolint:errcheck // best-effort stale hook
			t.StaleFiredAt = &now
			_ = s.UpdateTask(ctx, t) //nolint:errcheck // best-effort stale timestamp persist
		}
	}
}

// taskStaleUpdater is the single store method the stale bookkeeping needs.
type taskStaleUpdater interface {
	UpdateTask(ctx context.Context, task *core.Task) error
}

// taskBlockedByAny reports whether t is blocked by any of the given IDs.
func taskBlockedByAny(t *core.Task, ids []string) bool {
	blockers := t.BlockedBy()
	for _, want := range ids {
		for _, b := range blockers {
			if strings.EqualFold(b, want) {
				return true
			}
		}
	}
	return false
}

// taskMatchesQualifiedTracks reports whether t matches any of the qualified
// track IDs. Local QIDs match any task with that track_id regardless of
// project. Qualified QIDs match only tasks whose track_id and project_id
// both match.
func taskMatchesQualifiedTracks(t *core.Task, qids []core.QualifiedTrackID) bool {
	if t.TrackID == nil || *t.TrackID == "" {
		return false
	}
	for _, q := range qids {
		if *t.TrackID != q.TrackID {
			continue
		}
		if q.IsLocal() {
			return true
		}
		// Qualified: must also match project.
		if t.ProjectID != nil && *t.ProjectID == q.ProjectID() {
			return true
		}
	}
	return false
}
