package storage

import (
	"context"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

func newSeqTrack(slug, title string) *core.Track {
	now := time.Now().UTC().Truncate(time.Second)
	return &core.Track{
		ID:        core.NewTrackID(),
		Slug:      slug,
		Title:     title,
		Type:      core.TrackTypeFeature,
		Status:    core.TrackStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// TestCreateTrackAssignsSeqBackOntoStruct pins the write-path contract:
// CreateTrack allocates the per-project sequence AND echoes it onto the
// caller's struct, so the caller can render the alias without a re-read.
func TestCreateTrackAssignsSeqBackOntoStruct(t *testing.T) {
	s := newTrackTestStorage(t)
	ctx := context.Background()

	first := newSeqTrack("alpha", "Alpha")
	if err := s.CreateTrack(ctx, first); err != nil {
		t.Fatalf("CreateTrack: %v", err)
	}
	if first.Seq != 1 {
		t.Errorf("first track Seq = %d; want 1 (allocated value must land on the struct)", first.Seq)
	}

	second := newSeqTrack("beta", "Beta")
	if err := s.CreateTrack(ctx, second); err != nil {
		t.Fatalf("CreateTrack: %v", err)
	}
	if second.Seq != 2 {
		t.Errorf("second track Seq = %d; want 2", second.Seq)
	}
}

// TestCreateTrackHonoursPresetSeq mirrors the task path: a caller-supplied
// non-zero Seq is written as-is rather than reallocated.
func TestCreateTrackHonoursPresetSeq(t *testing.T) {
	s := newTrackTestStorage(t)
	ctx := context.Background()

	track := newSeqTrack("preset", "Preset")
	track.Seq = 99
	if err := s.CreateTrack(ctx, track); err != nil {
		t.Fatalf("CreateTrack: %v", err)
	}
	if track.Seq != 99 {
		t.Errorf("preset Seq mutated to %d; want 99", track.Seq)
	}

	got, err := s.GetTrack(ctx, track.ID)
	if err != nil {
		t.Fatalf("GetTrack: %v", err)
	}
	if got == nil {
		t.Fatal("expected track, got nil")
	}
	if got.Seq != 99 {
		t.Errorf("loaded Seq = %d; want 99", got.Seq)
	}
}

// TestTrackReadPathsCarrySeq pins that every track read path hydrates Seq,
// not just the insert.
func TestTrackReadPathsCarrySeq(t *testing.T) {
	s := newTrackTestStorage(t)
	ctx := context.Background()

	track := newSeqTrack("read-paths", "Read Paths")
	if err := s.CreateTrack(ctx, track); err != nil {
		t.Fatalf("CreateTrack: %v", err)
	}
	want := track.Seq
	if want == 0 {
		t.Fatal("fixture has no allocated seq")
	}

	byID, err := s.GetTrack(ctx, track.ID)
	if err != nil {
		t.Fatalf("GetTrack: %v", err)
	}
	if byID == nil || byID.Seq != want {
		t.Errorf("GetTrack Seq = %v; want %d", seqOf(byID), want)
	}

	bySlug, err := s.GetTrackBySlug(ctx, "", track.Slug)
	if err != nil {
		t.Fatalf("GetTrackBySlug: %v", err)
	}
	if bySlug == nil || bySlug.Seq != want {
		t.Errorf("GetTrackBySlug Seq = %v; want %d", seqOf(bySlug), want)
	}

	list, err := s.ListTracks(ctx, core.TrackQuery{AllProjects: true})
	if err != nil {
		t.Fatalf("ListTracks: %v", err)
	}
	var found bool
	for _, tr := range list {
		if tr.ID == track.ID {
			found = true
			if tr.Seq != want {
				t.Errorf("ListTracks Seq = %d; want %d", tr.Seq, want)
			}
		}
	}
	if !found {
		t.Fatal("created track missing from ListTracks")
	}
}

func seqOf(t *core.Track) any {
	if t == nil {
		return "<nil track>"
	}
	return t.Seq
}
