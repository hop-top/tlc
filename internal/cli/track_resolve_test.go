package cli

import (
	"context"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

func createTrack(t *testing.T, ctx context.Context, s interface{ CreateTrack(context.Context, *core.Track) error }, id, title, typ string, status core.TrackStatus) {
	t.Helper()
	now := time.Now().UTC()
	if err := s.CreateTrack(ctx, &core.Track{
		ID: id, Title: title, Type: typ, Status: status,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create track %s: %v", id, err)
	}
}

func testStorage(t *testing.T) (*context.Context, func()) {
	t.Helper()
	ctx, cleanup := setupTestDir(t)
	return &ctx, cleanup
}

func TestResolveTrackID_Exact(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	createTrack(t, ctx, s, "inbox-protocol", "Inbox", "feature", core.TrackStatusActive)

	got, err := resolveTrackID(ctx, s, "inbox-protocol")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "inbox-protocol" {
		t.Errorf("got %q, want inbox-protocol", got)
	}
}

func TestResolveTrackID_Prefix(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	createTrack(t, ctx, s, "inbox-protocol", "Inbox", "feature", core.TrackStatusActive)
	createTrack(t, ctx, s, "filesystem-projection", "FS", "feature", core.TrackStatusCompleted)

	got, err := resolveTrackID(ctx, s, "inbox")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "inbox-protocol" {
		t.Errorf("got %q, want inbox-protocol", got)
	}
}

func TestResolveTrackID_Fuzzy(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	createTrack(t, ctx, s, "task-prompt-nl-interface", "NL", "feature", core.TrackStatusPending)

	got, err := resolveTrackID(ctx, s, "task-prompt-nl-inter")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "task-prompt-nl-interface" {
		t.Errorf("got %q, want task-prompt-nl-interface", got)
	}
}

func TestResolveTrackID_AmbiguousPrefix(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	createTrack(t, ctx, s, "feat-alpha", "Alpha", "feature", core.TrackStatusActive)
	createTrack(t, ctx, s, "feat-beta", "Beta", "feature", core.TrackStatusActive)

	_, err = resolveTrackID(ctx, s, "feat")
	if err == nil {
		t.Fatal("expected ambiguous error")
	}
}

func TestResolveTrackID_NotFound(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	createTrack(t, ctx, s, "inbox-protocol", "Inbox", "feature", core.TrackStatusActive)

	_, err = resolveTrackID(ctx, s, "zzz-nonexistent")
	if err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestResolveTrackID_Empty(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	_, err = resolveTrackID(context.Background(), s, "")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}
