package cli

// Group headings must read as NAMES, not as internal keys.
//
// groupTasks keys on the stored column, and for `--group-by track` that
// column holds a 26-char typeid: a heading reading
// "track_01m29x2dnqfba9zqqsrpp8daxj" names nothing a human recognizes
// and nothing they typed. Assignee has the same shape of problem one
// layer up — an aps profile alias and its canonical profile ID are the
// same person, and grouping on the raw column splits them into two
// sections.
//
// Three properties are load-bearing here and each is pinned separately,
// because each fails differently:
//
//   - RESOLUTION: the title replaces the key.
//   - FALLBACK: a key with no resolution still renders its raw self. A
//     deleted track must not produce an empty heading — an unnamed
//     section is worse than an ugly one, because the rows under it have
//     no attribution at all.
//   - BATCHING: lookups are ONE pass, not one query per group. The N+1
//     is invisible in output, so only a counting fake catches it; a test
//     that asserts on rendered text alone would pass against either
//     implementation.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

// countingTrackLister records how many times the store was asked for
// tracks, which is the only way the N+1 shows up: a per-group query and
// a single batched pass render identical text.
type countingTrackLister struct {
	tracks []*core.Track
	calls  int
}

func (c *countingTrackLister) ListTracks(
	_ context.Context, _ core.TrackQuery,
) ([]*core.Track, error) {
	c.calls++
	return c.tracks, nil
}

// trackFixture builds a track row with the stored typeid shape, so the
// tests exercise the same key groupTasks actually produces rather than a
// convenient short string.
func trackFixture(id, title string) *core.Track {
	return &core.Track{ID: id, Title: title}
}

// TestTrackGroupHeadingsUseTitles is the headline: the raw typeid must
// not survive into a heading when the track is resolvable.
func TestTrackGroupHeadingsUseTitles(t *testing.T) {
	const (
		adoptionID = "track_01m29x2dnqfba9zqqsrpp8daxj"
		billingID  = "track_01m29x2j5vfsqs5g6q5dwseabx"
	)
	lister := &countingTrackLister{tracks: []*core.Track{
		trackFixture(adoptionID, "Adoption rollout"),
		trackFixture(billingID, "Billing migration"),
	}}

	labeler := newTrackLabeler(context.Background(), lister)
	groups := []TaskGroup{
		{Name: adoptionID, Tasks: []*core.Task{{ID: "T-0001", Title: "a"}}},
		{Name: billingID, Tasks: []*core.Task{{ID: "T-0002", Title: "b"}}},
	}
	labeled := applyGroupLabels(groups, labeler)

	want := []string{"Adoption rollout", "Billing migration"}
	for i, g := range labeled {
		if g.Name != want[i] {
			t.Errorf("group %d heading = %q, want %q", i, g.Name, want[i])
		}
		if strings.HasPrefix(g.Name, "track_") {
			t.Errorf("group %d still renders a raw typeid: %q", i, g.Name)
		}
	}
}

// TestTrackGroupLabelsBatchLookups is the N+1 guard. Ten groups must
// cost ONE pass over the store, not ten.
//
// Asserting "exactly 1" rather than "fewer than 10": a per-group query
// with a memo cache would still scale with the number of DISTINCT
// tracks, which is precisely the number of groups. Only a single pass
// is actually batched.
func TestTrackGroupLabelsBatchLookups(t *testing.T) {
	const groupCount = 10

	tracks := make([]*core.Track, 0, groupCount)
	groups := make([]TaskGroup, 0, groupCount)
	for i := range groupCount {
		id := fmt.Sprintf("track_%026d", i)
		tracks = append(tracks, trackFixture(id, fmt.Sprintf("Track %d", i)))
		groups = append(groups, TaskGroup{
			Name:  id,
			Tasks: []*core.Task{{ID: fmt.Sprintf("T-%04d", i)}},
		})
	}

	lister := &countingTrackLister{tracks: tracks}
	labeler := newTrackLabeler(context.Background(), lister)
	labeled := applyGroupLabels(groups, labeler)

	if lister.calls != 1 {
		t.Errorf("track lookups must batch into one pass; got %d store calls for %d groups",
			lister.calls, groupCount)
	}

	// The batching must not have come at the cost of correctness.
	for i, g := range labeled {
		want := fmt.Sprintf("Track %d", i)
		if g.Name != want {
			t.Errorf("group %d heading = %q, want %q", i, g.Name, want)
		}
	}
}

// TestTrackGroupHeadingFallsBackToRawID pins the deleted-track case. A
// track row that no longer exists has no title to show, and the heading
// must NOT collapse to empty: rows under a blank heading have lost their
// attribution entirely, which is a worse outcome than an unfriendly one.
func TestTrackGroupHeadingFallsBackToRawID(t *testing.T) {
	const goneID = "track_01m29x2dnqfba9zqqsrpp8daxj"

	// The store knows nothing about goneID — the track was deleted.
	lister := &countingTrackLister{tracks: nil}
	labeler := newTrackLabeler(context.Background(), lister)

	// The LABELER's own answer, checked before applyGroupLabels sees it.
	// applyGroupLabels keeps the previous name when handed an empty
	// string, which is the right belt-and-braces behavior and exactly
	// what hides a missing fallback from a test that only reads the
	// rendered group: drop the `return key` and the assertion below
	// still passes. The contract is that the labeler ITSELF answers with
	// the key, so that is what gets asserted.
	if got := labeler(goneID); got != goneID {
		t.Errorf("labeler must answer with the raw key for a deleted track; got %q, want %q",
			got, goneID)
	}

	groups := []TaskGroup{{Name: goneID, Tasks: []*core.Task{{ID: "T-0001"}}}}
	labeled := applyGroupLabels(groups, labeler)

	if len(labeled) != 1 {
		t.Fatalf("expected one group, got %d", len(labeled))
	}
	if labeled[0].Name != goneID {
		t.Errorf("unresolvable track must fall back to its raw key; got %q, want %q",
			labeled[0].Name, goneID)
	}
	if strings.TrimSpace(labeled[0].Name) == "" {
		t.Error("unresolvable track rendered an EMPTY heading; rows lose all attribution")
	}
}

// TestTrackGroupLabelerToleratesStoreFailure: a store that errors must
// degrade to raw keys, never crash and never blank the headings. The
// listing the user asked for is still renderable without the titles.
func TestTrackGroupLabelerToleratesStoreFailure(t *testing.T) {
	const id = "track_01m29x2dnqfba9zqqsrpp8daxj"

	labeler := newTrackLabeler(context.Background(), failingTrackLister{})

	// Asserted on the labeler directly — see the note in
	// TestTrackGroupHeadingFallsBackToRawID on why the rendered group
	// alone cannot catch a missing fallback.
	if got := labeler(id); got != id {
		t.Errorf("labeler must answer with the raw key when the store fails; got %q, want %q",
			got, id)
	}

	groups := []TaskGroup{{Name: id, Tasks: []*core.Task{{ID: "T-0001"}}}}
	labeled := applyGroupLabels(groups, labeler)

	if labeled[0].Name != id {
		t.Errorf("store failure must degrade to the raw key; got %q", labeled[0].Name)
	}
}

type failingTrackLister struct{}

func (failingTrackLister) ListTracks(
	_ context.Context, _ core.TrackQuery,
) ([]*core.Track, error) {
	return nil, fmt.Errorf("store unavailable")
}

// TestNoneGroupIsNeverRelabeled: "(none)" is the renderer's own word for
// "unset", not a key to look up. Resolving it would either miss (leaving
// it alone, harmlessly) or — with a track literally titled "(none)" —
// silently retitle the unset bucket.
func TestNoneGroupIsNeverRelabeled(t *testing.T) {
	lister := &countingTrackLister{tracks: []*core.Track{
		trackFixture(noneGroupName, "a track named none"),
	}}
	labeler := newTrackLabeler(context.Background(), lister)

	groups := []TaskGroup{{Name: noneGroupName, Tasks: []*core.Task{{ID: "T-0001"}}}}
	labeled := applyGroupLabels(groups, labeler)

	if labeled[0].Name != noneGroupName {
		t.Errorf("the unset bucket must keep its name; got %q, want %q",
			labeled[0].Name, noneGroupName)
	}
}

// TestAssigneeGroupHeadingsResolveProfiles pins the assignee half: an
// alias and its canonical profile ID name the same person, and the
// heading shows the resolved form — the same resolution `--profile`
// applies when it FILTERS, so grouping and filtering agree on identity.
func TestAssigneeGroupHeadingsResolveProfiles(t *testing.T) {
	resolve := func(s string) string {
		if s == "jb" {
			return "jadb"
		}
		return s
	}
	labeler := newAssigneeLabeler(resolve)

	groups := []TaskGroup{
		{Name: "jb", Tasks: []*core.Task{{ID: "T-0001"}}},
		{Name: "alice", Tasks: []*core.Task{{ID: "T-0002"}}},
	}
	labeled := applyGroupLabels(groups, labeler)

	if labeled[0].Name != "jadb" {
		t.Errorf("alias must resolve to the profile ID; got %q, want %q", labeled[0].Name, "jadb")
	}
	// An unresolvable assignee is the common case (a plain username with
	// no aps profile) and must render unchanged rather than vanish.
	if labeled[1].Name != "alice" {
		t.Errorf("unresolvable assignee must fall back to the raw key; got %q", labeled[1].Name)
	}
}

// TestApplyGroupLabelsNilLabelerIsIdentity: every dimension that has no
// labels to resolve — status, priority, tag, project — must pass through
// untouched, and the no-labeler path is what those use.
func TestApplyGroupLabelsNilLabelerIsIdentity(t *testing.T) {
	groups := []TaskGroup{
		{Name: "TODO", Tasks: []*core.Task{{ID: "T-0001"}}},
		{Name: "DONE", Tasks: []*core.Task{{ID: "T-0002"}}},
	}
	labeled := applyGroupLabels(groups, nil)

	if len(labeled) != len(groups) {
		t.Fatalf("group count changed: got %d, want %d", len(labeled), len(groups))
	}
	for i, g := range labeled {
		if g.Name != groups[i].Name {
			t.Errorf("group %d renamed without a labeler: got %q, want %q",
				i, g.Name, groups[i].Name)
		}
	}
}

// TestApplyGroupLabelsPreservesOrderAndTasks: relabeling is a rename of
// the sections, never a reordering or a regrouping of them. Two tracks
// whose titles sort opposite to their IDs must keep the order groupTasks
// established, so the ordering contract lives in exactly one place.
func TestApplyGroupLabelsPreservesOrderAndTasks(t *testing.T) {
	const (
		firstID  = "track_01aaaaaaaaaaaaaaaaaaaaaaaa"
		secondID = "track_01bbbbbbbbbbbbbbbbbbbbbbbb"
	)
	lister := &countingTrackLister{tracks: []*core.Track{
		trackFixture(firstID, "Zebra"),
		trackFixture(secondID, "Apple"),
	}}
	labeler := newTrackLabeler(context.Background(), lister)

	groups := []TaskGroup{
		{Name: firstID, Tasks: []*core.Task{{ID: "T-0001"}, {ID: "T-0002"}}},
		{Name: secondID, Tasks: []*core.Task{{ID: "T-0003"}}},
	}
	labeled := applyGroupLabels(groups, labeler)

	// Titles sort Apple < Zebra, but the incoming order stands.
	if labeled[0].Name != "Zebra" || labeled[1].Name != "Apple" {
		t.Errorf("relabeling must not reorder groups; got %q then %q",
			labeled[0].Name, labeled[1].Name)
	}
	if len(labeled[0].Tasks) != 2 || len(labeled[1].Tasks) != 1 {
		t.Errorf("relabeling must not move tasks; got %d and %d",
			len(labeled[0].Tasks), len(labeled[1].Tasks))
	}
	if labeled[0].Tasks[0].ID != "T-0001" {
		t.Errorf("task order within a group changed; got %q", labeled[0].Tasks[0].ID)
	}
}
