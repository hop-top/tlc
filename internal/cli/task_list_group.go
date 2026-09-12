package cli

import (
	"fmt"
	"io"
	"sort"

	"hop.top/tlc/internal/core"
)

// groupByKeys is the closed set of dimensions `task list --group-by`
// accepts, in the order help text and rejection messages name them.
//
// ONE home for the vocabulary, read by the flag-enum registration, the
// validator, the grouper's switch and the rejection message alike. The
// alternative — a literal per site — is how a key gets accepted by the
// flag and then falls through the grouper's switch into a silent
// ungrouped listing, which is the same class of drift the --status and
// --priority canon in internal/core exists to prevent.
//
// A var, not a const slice (Go has none), and never mutated: GroupByKeys
// hands out a copy.
var groupByKeys = []string{"track", "assignee", "tag", "status", "priority", "project"}

// noneGroupName titles the section holding tasks that carry no value for
// the grouping dimension: no track, no assignee, no tags, no status, no
// priority.
//
// Parenthesised on purpose. A bare "none" is indistinguishable from a
// track, assignee or tag literally named "none", and those exist — the
// parens say "this is the renderer talking, not your data". The same
// name serves every dimension so "unset" reads identically across the
// whole command rather than being a nameless trailing bucket for status
// and priority and a blank heading for the rest.
const noneGroupName = "(none)"

// GroupByKeys returns the accepted --group-by dimensions in declaration
// order. The returned slice is a copy.
func GroupByKeys() []string {
	return append([]string(nil), groupByKeys...)
}

// ValidGroupByKey reports whether key names a supported grouping
// dimension. Empty is NOT valid: "" means the flag was never set, and
// callers check that before validating rather than folding the two facts
// into one answer.
func ValidGroupByKey(key string) bool {
	for _, k := range groupByKeys {
		if k == key {
			return true
		}
	}
	return false
}

// unknownGroupByError renders the rejection for an unsupported
// --group-by key, naming the legal set. Rendered FROM groupByKeys rather
// than a literal, so the message cannot outlive a key being added or
// removed.
func unknownGroupByError(key string) error {
	return fmt.Errorf("unknown --group-by %q; valid values: %s",
		key, core.VocabularyList(GroupByKeys()))
}

// TaskGroup is one rendered section of a grouped listing: the group's
// name and the tasks that belong to it.
type TaskGroup struct {
	Name  string
	Tasks []*core.Task
}

// groupTasks partitions an already-fetched, already-sorted result set
// into named groups along key.
//
// Returns an ORDERED SLICE, never a map. Go randomizes map iteration
// order per range, so a map return would reorder the sections of the
// same listing between two runs of the same command — output a human
// diffs and a script parses.
//
// Ordering:
//   - track / assignee / project — group name ascending.
//   - tag — group name ascending.
//   - status / priority — DECLARED LIFECYCLE order, read from the
//     effective vocabulary (core.ConfiguredTaskStatusStrings /
//     ConfiguredPriorityStrings), because those vocabularies are
//     config-driven and declaration order IS rank order. Alphabetical
//     would put DONE ahead of TODO and, for a renamed vocabulary like
//     URGENT/NORMAL/LATER, invert urgency outright.
//
// Within a group, tasks keep the incoming order. The caller already
// applied --sort-by; re-sorting here would silently override it.
//
// TAG GROUPING IS MANY-TO-MANY: a task tagged [api, urgent] appears in
// BOTH groups, so the total row count exceeds the distinct task count.
// That is intended, not a bug to dedupe away — a tag listing that showed
// each task once would have to pick a tag to hide it under.
//
// UNSET VALUES collect into a single trailing noneGroupName group —
// see appendNoneLast. That includes a task with no tags at all, which
// has no tag membership to generate and would otherwise disappear from
// a listing it matched the filters for.
//
// An unknown key yields no groups; callers validate with
// ValidGroupByKey before reaching here.
func groupTasks(tasks []*core.Task, key string) []TaskGroup {
	if len(tasks) == 0 {
		return nil
	}

	// Bucket first, order second. Membership is a map for the O(1)
	// append; the ORDER is decided afterwards, from the sorted names or
	// the declared vocabulary, so the map's iteration order never
	// reaches the output.
	buckets := make(map[string][]*core.Task)
	add := func(name string, t *core.Task) {
		// ONE notion of "unset", applied at the single point where a
		// value becomes a group name. Every dimension's empty value —
		// nil pointer, empty string, absent status — lands in the same
		// named section rather than each key inventing its own.
		if name == "" {
			name = noneGroupName
		}
		buckets[name] = append(buckets[name], t)
	}

	for _, t := range tasks {
		if t == nil {
			continue
		}
		switch key {
		case "tag":
			// Many-to-many: one membership per tag, no dedupe. An
			// untagged task has no membership to generate, so it is
			// placed explicitly — otherwise it would vanish from a
			// listing it matched the filters for.
			if len(t.Tags) == 0 {
				add("", t)
				break
			}
			for _, tag := range t.Tags {
				add(tag, t)
			}
		case "assignee":
			add(derefString(t.AssignedTo), t)
		case "track":
			add(derefString(t.TrackID), t)
		case "project":
			add(derefString(t.ProjectID), t)
		case "status":
			add(string(t.Status), t)
		case "priority":
			add(string(t.Priority), t)
		default:
			return nil
		}
	}

	var order []string
	switch key {
	case "status":
		order = vocabularyOrder(core.ConfiguredTaskStatusStrings(), buckets)
	case "priority":
		order = vocabularyOrder(core.ConfiguredPriorityStrings(), buckets)
	default:
		order = make([]string, 0, len(buckets))
		for name := range buckets {
			if name == noneGroupName {
				continue
			}
			order = append(order, name)
		}
		sort.Strings(order)
		order = appendNoneLast(order, buckets)
	}

	groups := make([]TaskGroup, 0, len(order))
	for _, name := range order {
		groups = append(groups, TaskGroup{Name: name, Tasks: buckets[name]})
	}
	return groups
}

// vocabularyOrder orders the populated bucket names by the declared
// vocabulary, then appends any name the vocabulary does not know,
// sorted, so a task carrying a value outside the effective vocabulary
// (a stale row, a config narrowed after the fact) still renders rather
// than vanishing from the listing.
//
// The unset status or priority does NOT land in that trailing group: it
// is named noneGroupName during bucketing and forced past everything
// else by appendNoneLast, so "unset" reads the same under --group-by
// status as it does under --group-by assignee.
func vocabularyOrder(canon []string, buckets map[string][]*core.Task) []string {
	known := make(map[string]bool, len(canon))
	order := make([]string, 0, len(buckets))
	for _, name := range canon {
		known[name] = true
		if _, ok := buckets[name]; ok {
			order = append(order, name)
		}
	}
	rest := make([]string, 0, len(buckets))
	for name := range buckets {
		if !known[name] && name != noneGroupName {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return appendNoneLast(append(order, rest...), buckets)
}

// appendNoneLast puts the "(none)" section at the very end of order when
// one is populated.
//
// FORCED, never sorted into place. "(none)" begins with "(", which sorts
// ahead of every letter in ASCII, so a plain sort would open every
// grouped listing with the pile of unset rows — the least informative
// section leading. Populated, named groups lead; the leftovers trail.
// It also has to outrank the values the configured vocabulary does not
// know, because "no status at all" says less than "a status this config
// retired".
func appendNoneLast(order []string, buckets map[string][]*core.Task) []string {
	if _, ok := buckets[noneGroupName]; !ok {
		return order
	}
	return append(order, noneGroupName)
}

// derefString reads a *string field as its value, with the nil pointer
// and the empty string collapsing to "". Grouping treats "unassigned"
// and "assigned to the empty string" as one group; nothing downstream
// can tell them apart, and a schema that allows both is already ambiguous.
func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// groupedPayload shapes grouped results for JSON/YAML as
// {groups: [{name, tasks: [...]}]}.
//
// Built from an ordered SLICE of maps, never a map of name to tasks: a
// Go map would randomize section order per marshal, and the ordering
// groupTasks works to establish — named groups ascending or in
// vocabulary rank, "(none)" forced last — is part of the contract a
// script reads, not a rendering nicety.
//
// Each group's task slice runs through normalizeEmptySlices so an empty
// group serializes `tasks: []` rather than `null`, per the repo-wide
// empty-list contract. Reusing that helper rather than a local nil check
// keeps ONE definition of "empty list" — a second one would be free to
// drift back to null for exactly the payloads it did not cover.
func groupedPayload(groups []TaskGroup) map[string]any {
	out := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		out = append(out, map[string]any{
			"name":  g.Name,
			"tasks": normalizeEmptySlices(g.Tasks),
		})
	}
	return map[string]any{"groups": out}
}

// structuredFormat reports whether format serializes data for a machine
// rather than rendering it for a screen. --group-limit is a display cap,
// so these formats ignore it — and say so.
func structuredFormat(format string) bool {
	switch format {
	case formatJSON, formatYAML, formatVtodo, "tls":
		return true
	default:
		return false
	}
}

// noteGroupLimitIgnored tells the user on stderr that --group-limit did
// not apply, and why.
//
// Silently dropping a flag the user typed is its own defect class: the
// command exits 0 with a payload that looks like the one they asked for.
// The note goes to STDERR so the stdout a script parses stays pure
// payload — the same split noteIgnoredPagination uses for the aggregate
// formats.
func noteGroupLimitIgnored(w io.Writer, format string) {
	_, _ = fmt.Fprintf(w,
		"note: --group-limit ignored with --format %s; structured output carries every task, "+
			"so a consumer never reads a silently truncated list\n", format)
}
