package cli

import (
	"fmt"
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
		buckets[name] = append(buckets[name], t)
	}

	for _, t := range tasks {
		if t == nil {
			continue
		}
		switch key {
		case "tag":
			// Many-to-many: one membership per tag, no dedupe.
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
			order = append(order, name)
		}
		sort.Strings(order)
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
// The empty name — the unset status or priority — is such a name and
// lands in that trailing group. Naming and placement of a "(none)"
// section is a rendering concern and is deliberately not decided here.
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
		if !known[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append(order, rest...)
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
