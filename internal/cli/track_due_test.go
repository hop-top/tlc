package cli

import (
	"bytes"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// TestTrackCreate_Due verifies that `track create --due tomorrow`
// parses with util.ParseUntil and persists DueAt as UTC RFC3339 per
// docs/temporal-spec-0.1.md §4.
func TestTrackCreate_Due(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		resetTrackFlags()
		defer resetTrackFlags()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "create", "Ship release", "--type", "feature",
			"--due", "tomorrow",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track create --due failed: %v", err)
		}

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		track, err := s.GetTrack(ctx, "ship-release")
		if err != nil {
			t.Fatalf("GetTrack: %v", err)
		}
		if track == nil {
			t.Fatal("track not found after create")
		}
		if track.DueAt == nil {
			t.Fatal("DueAt should be set")
		}
		diff := track.DueAt.Sub(time.Now())
		if diff < 23*time.Hour || diff > 25*time.Hour {
			t.Errorf("DueAt should be ~24h from now, got %v", diff)
		}
	})
}

// TestTrackUpdate_ClearDue verifies the "" / "-" sentinel clears DueAt
// on update, mirroring the task update contract.
func TestTrackUpdate_ClearDue(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		resetTrackFlags()
		defer resetTrackFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		due := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID: "due-track", Slug: "due-track",
			Title: "Has due", Type: "feature",
			DueAt: &due,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("CreateTrack: %v", err)
		}
		s.Close()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "update", "due-track", "--due", "-",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track update --due - failed: %v", err)
		}

		s2, _ := getStorageRaw()
		defer s2.Close()
		got, _ := s2.GetTrack(ctx, "due-track")
		if got == nil {
			t.Fatal("track not found after update")
		}
		if got.DueAt != nil {
			t.Errorf("DueAt should be nil after --due -, got %v", *got.DueAt)
		}
	})
}

// TestTrackUpdate_OverwriteDue verifies a new --due value overwrites
// the existing DueAt and is parsed through util.ParseUntil.
func TestTrackUpdate_OverwriteDue(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		resetTrackFlags()
		defer resetTrackFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID: "shift-track", Slug: "shift-track",
			Title: "Shift me", Type: "feature",
			DueAt: &old,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("CreateTrack: %v", err)
		}
		s.Close()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "update", "shift-track",
			"--due", "2026-12-31",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track update --due failed: %v", err)
		}

		s2, _ := getStorageRaw()
		defer s2.Close()
		got, _ := s2.GetTrack(ctx, "shift-track")
		if got == nil {
			t.Fatal("track not found after update")
		}
		if got.DueAt == nil {
			t.Fatal("DueAt should be set after overwrite")
		}
		want := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
		if !got.DueAt.Equal(want) {
			t.Errorf("expected DueAt %v, got %v", want, *got.DueAt)
		}
	})
}
