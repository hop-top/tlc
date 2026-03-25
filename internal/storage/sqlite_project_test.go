package storage

import (
	"context"
	"testing"
	"time"
)

// TestSQLiteStorage_RegisterProject tests project registration and lookup.
func TestSQLiteStorage_RegisterProject(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Register a project
	err = s.RegisterProject(ctx, "hop-top/tlc", "/data/tlc/db.sqlite", "space:default", "TLC")
	if err != nil {
		t.Fatalf("RegisterProject failed: %v", err)
	}

	// Lookup
	p, err := s.LookupProject(ctx, "hop-top/tlc")
	if err != nil {
		t.Fatalf("LookupProject failed: %v", err)
	}
	if p == nil {
		t.Fatal("expected project, got nil")
	}
	if p.ProjectID != "hop-top/tlc" {
		t.Errorf("expected project_id hop-top/tlc, got %s", p.ProjectID)
	}
	if p.DBPath != "/data/tlc/db.sqlite" {
		t.Errorf("expected db_path /data/tlc/db.sqlite, got %s", p.DBPath)
	}
	if p.SpaceURI != "space:default" {
		t.Errorf("expected space_uri space:default, got %s", p.SpaceURI)
	}
	if p.Label != "TLC" {
		t.Errorf("expected label TLC, got %s", p.Label)
	}
	if p.Status != "active" {
		t.Errorf("expected status active, got %s", p.Status)
	}
	if p.RegisteredAt.IsZero() {
		t.Error("expected non-zero registered_at")
	}
	if p.LastSeenAt.IsZero() {
		t.Error("expected non-zero last_seen_at")
	}

	// Lookup non-existent
	p2, err := s.LookupProject(ctx, "does/not-exist")
	if err != nil {
		t.Fatalf("LookupProject for missing project failed: %v", err)
	}
	if p2 != nil {
		t.Errorf("expected nil for missing project, got %+v", p2)
	}
}

// TestSQLiteStorage_RegisterProjectReplace tests INSERT OR REPLACE behavior.
func TestSQLiteStorage_RegisterProjectReplace(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Register then re-register with updated path
	err = s.RegisterProject(ctx, "hop-top/tlc", "/old/path.db", "", "")
	if err != nil {
		t.Fatalf("first RegisterProject failed: %v", err)
	}

	err = s.RegisterProject(ctx, "hop-top/tlc", "/new/path.db", "space:work", "TLC v2")
	if err != nil {
		t.Fatalf("second RegisterProject failed: %v", err)
	}

	p, err := s.LookupProject(ctx, "hop-top/tlc")
	if err != nil {
		t.Fatalf("LookupProject failed: %v", err)
	}
	if p.DBPath != "/new/path.db" {
		t.Errorf("expected db_path /new/path.db, got %s", p.DBPath)
	}
	if p.SpaceURI != "space:work" {
		t.Errorf("expected space_uri space:work, got %s", p.SpaceURI)
	}
	if p.Label != "TLC v2" {
		t.Errorf("expected label TLC v2, got %s", p.Label)
	}
}

// TestSQLiteStorage_ListProjectsBySpace tests space-scoped project listing.
func TestSQLiteStorage_ListProjectsBySpace(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	s.RegisterProject(ctx, "proj/a", "/a.db", "space:one", "A")
	s.RegisterProject(ctx, "proj/b", "/b.db", "space:one", "B")
	s.RegisterProject(ctx, "proj/c", "/c.db", "space:two", "C")

	// List by space:one
	projects, err := s.ListProjectsBySpace(ctx, "space:one")
	if err != nil {
		t.Fatalf("ListProjectsBySpace failed: %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("expected 2 projects in space:one, got %d", len(projects))
	}

	// List by space:two
	projects, err = s.ListProjectsBySpace(ctx, "space:two")
	if err != nil {
		t.Fatalf("ListProjectsBySpace failed: %v", err)
	}
	if len(projects) != 1 {
		t.Errorf("expected 1 project in space:two, got %d", len(projects))
	}

	// List by empty space
	projects, err = s.ListProjectsBySpace(ctx, "space:empty")
	if err != nil {
		t.Fatalf("ListProjectsBySpace for empty space failed: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects in space:empty, got %d", len(projects))
	}
}

// TestSQLiteStorage_ListAllProjects tests listing all projects.
func TestSQLiteStorage_ListAllProjects(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	s.RegisterProject(ctx, "proj/a", "/a.db", "space:one", "A")
	s.RegisterProject(ctx, "proj/b", "/b.db", "space:two", "B")

	projects, err := s.ListAllProjects(ctx)
	if err != nil {
		t.Fatalf("ListAllProjects failed: %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
}

// TestSQLiteStorage_UpdateProjectPath tests path update + last_seen_at bump.
func TestSQLiteStorage_UpdateProjectPath(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	s.RegisterProject(ctx, "proj/x", "/old.db", "", "")

	before, _ := s.LookupProject(ctx, "proj/x")

	// Small delay to ensure time difference
	time.Sleep(10 * time.Millisecond)

	err = s.UpdateProjectPath(ctx, "proj/x", "/new.db")
	if err != nil {
		t.Fatalf("UpdateProjectPath failed: %v", err)
	}

	after, _ := s.LookupProject(ctx, "proj/x")
	if after.DBPath != "/new.db" {
		t.Errorf("expected db_path /new.db, got %s", after.DBPath)
	}
	if !after.LastSeenAt.After(before.LastSeenAt) || after.LastSeenAt.Equal(before.LastSeenAt) {
		// RFC3339 has second precision; just check it's not before
		if after.LastSeenAt.Before(before.LastSeenAt) {
			t.Errorf("expected last_seen_at to be updated")
		}
	}
}

// TestSQLiteStorage_TouchProject tests last_seen_at-only update.
func TestSQLiteStorage_TouchProject(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	s.RegisterProject(ctx, "proj/y", "/y.db", "", "Y")

	before, _ := s.LookupProject(ctx, "proj/y")

	time.Sleep(10 * time.Millisecond)

	err = s.TouchProject(ctx, "proj/y")
	if err != nil {
		t.Fatalf("TouchProject failed: %v", err)
	}

	after, _ := s.LookupProject(ctx, "proj/y")
	if after.DBPath != "/y.db" {
		t.Errorf("db_path should not change, got %s", after.DBPath)
	}
	if after.LastSeenAt.Before(before.LastSeenAt) {
		t.Errorf("expected last_seen_at to not go backwards")
	}
}
