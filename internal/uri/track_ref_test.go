package uri

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// seedTrackStore opens a temp store and creates tracks with the given
// slugs, returning the store and the tracks in creation order so tests
// can assert against real seq values rather than assumed ones.
func seedTrackStore(t *testing.T, slugs ...string) (*storage.SQLiteStorage, []*core.Track) {
	t.Helper()

	dir := t.TempDir()
	s, err := storage.NewSQLiteStorage(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	// Seed into whatever project the test process detects for its cwd.
	// Resolution is project-scoped, so a fixture written to the empty
	// bucket while the resolver looks in the ambient project would test
	// nothing.
	projectID := ""
	if proj := core.DetectProject(); proj != nil && proj.InProject {
		projectID = proj.ProjectID
	}

	ctx := context.Background()
	tracks := make([]*core.Track, 0, len(slugs))
	for _, slug := range slugs {
		track := &core.Track{
			ID:     core.NewTrackID(),
			Slug:   slug,
			Title:  "Track " + slug,
			Type:   "fix",
			Status: core.TrackStatusPending,
		}
		if projectID != "" {
			pid := projectID
			track.ProjectID = &pid
		}
		require.NoError(t, s.CreateTrack(ctx, track))
		// Read back through the slug so the fixture carries the seq the
		// store actually assigned rather than one the test assumed.
		stored, err := s.GetTrackBySlug(ctx, projectID, slug)
		require.NoError(t, err)
		require.NotNil(t, stored)
		tracks = append(tracks, stored)
	}
	return s, tracks
}

// TestGetRegistry_RegistersTrackType pins the tlc://tracks/ type into the
// registry alongside the types that already exist.
func TestGetRegistry_RegistersTrackType(t *testing.T) {
	s, _ := seedTrackStore(t)

	reg, err := GetRegistry(s)
	require.NoError(t, err)

	assert.Contains(t, reg.Types(), "track")
}

// TestTrackCompleter_OffersAliasesAndSlugs is the completion contract:
// a track is reachable by the short L-NNNN alias and by its slug. The
// 26-char TypeID is deliberately not offered — the whole point of the
// alias is that raw IDs are not typeable.
func TestTrackCompleter_OffersAliasesAndSlugs(t *testing.T) {
	s, tracks := seedTrackStore(t, "alpha-track-one", "beta-track-two")

	reg, err := GetRegistry(s)
	require.NoError(t, err)

	got, err := reg.Complete(context.Background(), "track", "")
	require.NoError(t, err)

	for _, track := range tracks {
		alias := core.FormatTrackSeq(track.Seq)
		require.NotEmpty(t, alias, "fixture track has no seq")
		assert.Contains(t, got, alias)
		assert.Contains(t, got, track.Slug)
		assert.NotContains(t, got, track.ID,
			"raw TypeIDs must not be offered as completions")
	}
}

// TestTrackCompleter_PrefixFilters keeps the completer honest about the
// prefix argument, which is what the shell actually passes.
func TestTrackCompleter_PrefixFilters(t *testing.T) {
	s, tracks := seedTrackStore(t, "alpha-track-one", "beta-track-two")

	reg, err := GetRegistry(s)
	require.NoError(t, err)

	got, err := reg.Complete(context.Background(), "track", "beta")
	require.NoError(t, err)
	assert.Equal(t, []string{"beta-track-two"}, got)

	// Alias prefixes filter too, case-insensitively, so typing "l-" in a
	// shell narrows to aliases.
	aliases, err := reg.Complete(context.Background(), "track", "l-")
	require.NoError(t, err)
	for _, track := range tracks {
		assert.Contains(t, aliases, core.FormatTrackSeq(track.Seq))
	}
	assert.NotContains(t, aliases, "alpha-track-one")
}

// TestTrackCompleter_EmptyStore mirrors the other completer tests: an
// empty DB yields zero suggestions and no error.
func TestTrackCompleter_EmptyStore(t *testing.T) {
	s, _ := seedTrackStore(t)

	reg, err := GetRegistry(s)
	require.NoError(t, err)

	got, err := reg.Complete(context.Background(), "track", "")
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestResolveTrack_AcceptsURIAndAliasForms is the resolver half of the
// feature. Completion alone makes a URI typeable, not dereferenceable;
// every form the completer can emit must resolve back to the track.
func TestResolveTrack_AcceptsURIAndAliasForms(t *testing.T) {
	s, tracks := seedTrackStore(t, "alpha-track-one", "beta-track-two")
	target := tracks[1]
	alias := core.FormatTrackSeq(target.Seq)

	r := NewResolver(s)
	for _, input := range []string{
		"tlc://tracks/" + alias,
		"tlc://tracks/" + target.Slug,
		"tracks/" + alias,
		alias,
		target.Slug,
		target.ID,
	} {
		t.Run(input, func(t *testing.T) {
			got, err := r.ResolveTrack(context.Background(), input)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, target.ID, got.Track.ID)
		})
	}
}

// TestResolveTrack_NotFound keeps a miss a miss rather than a wrong hit.
func TestResolveTrack_NotFound(t *testing.T) {
	s, _ := seedTrackStore(t, "alpha-track-one")

	r := NewResolver(s)
	_, err := r.ResolveTrack(context.Background(), "tlc://tracks/L-0042")
	require.Error(t, err)
}
