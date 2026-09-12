package cli

// Group heading LABELS: turning the key a listing was grouped on into
// the name a reader recognizes.
//
// groupTasks keys on the stored column, which is the right key to group
// by and the wrong string to print. `--group-by track` buckets on
// task.track_id, and that column holds a 26-char typeid — a heading
// reading "track_01m29x2dnqfba9zqqsrpp8daxj" names nothing the user
// typed and nothing they can look up. `--group-by assignee` has the
// same problem one layer up: an aps profile alias and its canonical
// profile ID are the same person, and `--profile` already resolves
// between them when it FILTERS, so a heading that does not resolve
// leaves grouping and filtering disagreeing about who is who.
//
// Relabeling is a SEPARATE PASS over the finished groups, never a change
// to groupTasks' keying. Grouping on the display name instead would fold
// two distinct tracks that happen to share a title into one section and
// make the section ordering depend on text the store can change under
// it. Keys group; labels print.
//
// The pass is also where the fallback lives, and the fallback is the
// load-bearing part. A deleted track, a profile that no longer resolves,
// a store that will not answer — each must still yield a heading. An
// EMPTY heading is strictly worse than an ugly one: the rows beneath it
// lose their attribution entirely, and the reader cannot tell an unnamed
// section from the "(none)" bucket that legitimately holds unset rows.

import (
	"context"

	"hop.top/tlc/internal/core"
)

// groupLabeler maps a group KEY to the name a heading should show.
//
// Returning the key unchanged is the correct answer for anything
// unresolvable, so implementations never need to signal failure — the
// caller cannot do anything with the error except print the key it
// already has.
type groupLabeler func(key string) string

// trackLister is the slice of the store this file needs: one batched
// read of the track rows.
//
// Narrowed to one method on purpose. The concrete storage handle would
// drag the whole repository interface into the labeling tests, and the
// batching property — the one thing a test must be able to observe — is
// only observable through a fake that counts its calls.
type trackLister interface {
	ListTracks(ctx context.Context, query core.TrackQuery) ([]*core.Track, error)
}

// newTrackLabeler reads every track ONCE and returns a labeler backed by
// the resulting map.
//
// ONE PASS, eagerly, before any group is rendered — not a lazy per-key
// lookup. A lazy labeler is an N+1 that no output can reveal: a query
// per group and a single batched read render byte-identical headings,
// so the cost only ever shows up as latency on a listing with many
// tracks. Memoizing a per-key lookup would not fix it either, since the
// distinct-key count IS the group count.
//
// A store failure is swallowed deliberately. The listing the user asked
// for is still renderable without titles; failing the command outright
// would turn a cosmetic lookup into an outage for output that is
// otherwise complete.
func newTrackLabeler(ctx context.Context, s trackLister) groupLabeler {
	titles := make(map[string]string)

	// AllProjects: the tasks being grouped were already filtered to the
	// project the user asked for, and their track_ids are whatever those
	// rows carry. Re-applying a project filter here would leave a
	// cross-project task's track unresolvable and print a typeid next to
	// resolved neighbors.
	tracks, err := s.ListTracks(ctx, core.TrackQuery{AllProjects: true})
	if err == nil {
		for _, tr := range tracks {
			if tr == nil || tr.Title == "" {
				continue
			}
			titles[tr.ID] = tr.Title
		}
	}

	return func(key string) string {
		if title, ok := titles[key]; ok {
			return title
		}
		// Deleted track, a title the store left empty, or a failed read:
		// the raw key still identifies the section. A short L-NNNN style
		// alias could prefix the title here later without changing this
		// contract.
		return key
	}
}

// newAssigneeLabeler resolves assignee keys through the same profile
// resolution `--profile` applies when filtering, so the two agree on
// which string names a given person.
//
// resolve is injected rather than reaching for core.GetGlobalResolver()
// inside: the global resolver shells out to `aps` on first use, which a
// unit test can neither predict nor isolate.
//
// No batching concern here — resolution is a map lookup against a
// resolver the process already built and cached for its lifetime, so
// per-key calls cost nothing.
func newAssigneeLabeler(resolve func(string) string) groupLabeler {
	if resolve == nil {
		return nil
	}
	return func(key string) string {
		if resolved := resolve(key); resolved != "" {
			return resolved
		}
		return key
	}
}

// applyGroupLabels renames each group's heading through labeler,
// returning a new slice.
//
// ORDER AND MEMBERSHIP ARE UNTOUCHED. groupTasks settled both — names
// ascending, vocabulary rank for status and priority, "(none)" forced
// last — and re-sorting on the labels would quietly relocate that
// contract into this file, where the "(none)" rule does not exist. Two
// tracks titled "Zebra" and "Apple" keep the order their keys gave them.
//
// A nil labeler is the identity pass, so the dimensions with nothing to
// resolve — tag, status, priority, project — share this code path rather
// than branching around it.
func applyGroupLabels(groups []TaskGroup, labeler groupLabeler) []TaskGroup {
	if labeler == nil || len(groups) == 0 {
		return groups
	}

	out := make([]TaskGroup, 0, len(groups))
	for _, g := range groups {
		name := g.Name
		// "(none)" is the RENDERER's word for unset, not a value read
		// from any row, so it is never a lookup key. Resolving it would
		// usually miss harmlessly — but against a track or profile
		// literally named "(none)" it would silently retitle the unset
		// bucket to something that looks like real data.
		if name != noneGroupName {
			if labeled := labeler(name); labeled != "" {
				name = labeled
			}
		}
		out = append(out, TaskGroup{Name: name, Tasks: g.Tasks})
	}
	return out
}

// groupLabelerFor returns the labeler for a grouping dimension, or nil
// when the dimension's keys are already their own display names.
//
// The switch is exhaustive over the dimensions that NEED resolution
// rather than over every key, so a new dimension defaults to rendering
// its raw keys — which is correct for any value that is already a name,
// and visibly wrong (rather than silently blank) for one that is not.
//
//   - tag / status / priority — the stored value IS the name.
//   - project — the project ID is what the user types and filters on;
//     showing anything else would name a thing they cannot use.
func groupLabelerFor(ctx context.Context, key string, s trackLister) groupLabeler {
	switch key {
	case "track":
		if s == nil {
			return nil
		}
		return newTrackLabeler(ctx, s)
	case "assignee":
		return newAssigneeLabeler(core.GetGlobalResolver().Resolve)
	default:
		return nil
	}
}
