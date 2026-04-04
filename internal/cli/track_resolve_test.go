package cli

import (
	"context"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

func TestResolveTrackID_Exact(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now().UTC()
	s.CreateTrack(ctx, &core.Track{
		ID: "inbox-protocol", Title: "Inbox Protocol",
		Type: "feature", Status: "active",
		CreatedAt: now, UpdatedAt: now,
	})

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
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now().UTC()
	s.CreateTrack(ctx, &core.Track{
		ID: "inbox-protocol", Title: "Inbox Protocol",
		Type: "feature", Status: "active",
		CreatedAt: now, UpdatedAt: now,
	})
	s.CreateTrack(ctx, &core.Track{
		ID: "filesystem-projection", Title: "Filesystem",
		Type: "feature", Status: "completed",
		CreatedAt: now, UpdatedAt: now,
	})

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
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now().UTC()
	s.CreateTrack(ctx, &core.Track{
		ID: "task-prompt-nl-interface", Title: "NL",
		Type: "feature", Status: "pending",
		CreatedAt: now, UpdatedAt: now,
	})

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
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now().UTC()
	s.CreateTrack(ctx, &core.Track{
		ID: "feat-alpha", Title: "Alpha",
		Type: "feature", Status: "active",
		CreatedAt: now, UpdatedAt: now,
	})
	s.CreateTrack(ctx, &core.Track{
		ID: "feat-beta", Title: "Beta",
		Type: "feature", Status: "active",
		CreatedAt: now, UpdatedAt: now,
	})

	_, err := resolveTrackID(ctx, s, "feat")
	if err == nil {
		t.Fatal("expected ambiguous error")
	}
}

func TestResolveTrackID_NotFound(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now().UTC()
	s.CreateTrack(ctx, &core.Track{
		ID: "inbox-protocol", Title: "Inbox",
		Type: "feature", Status: "active",
		CreatedAt: now, UpdatedAt: now,
	})

	_, err := resolveTrackID(ctx, s, "zzz-nonexistent")
	if err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestResolveTrackID_Empty(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	_, err := resolveTrackID(context.Background(), s, "")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}
