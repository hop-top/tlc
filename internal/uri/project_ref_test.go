package uri

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// fakeProjectLister implements projectLister against an in-memory slice so
// the resolver tests don't need to touch SQLite.
type fakeProjectLister struct {
	projects []core.RegisteredProject
}

func (f *fakeProjectLister) LookupProject(_ context.Context, projectID string) (*core.RegisteredProject, error) {
	for i := range f.projects {
		if f.projects[i].ProjectID == projectID {
			p := f.projects[i]
			return &p, nil
		}
	}
	return nil, nil
}

func (f *fakeProjectLister) ListAllProjects(_ context.Context) ([]core.RegisteredProject, error) {
	out := make([]core.RegisteredProject, len(f.projects))
	copy(out, f.projects)
	return out, nil
}

func mkProj(id, label string) core.RegisteredProject {
	return core.RegisteredProject{
		ProjectID: id,
		Label:     label,
		DBPath:    "/data/" + id + "/db.sqlite",
		Status:    "active",
	}
}

func TestResolveProjectRef_ExactID(t *testing.T) {
	reg := &fakeProjectLister{projects: []core.RegisteredProject{
		mkProj("hop-top/kit", "kit"),
		mkProj("hop-top/tlc", "tlc"),
	}}

	got, err := ResolveProjectRef(context.Background(), reg, "hop-top/kit")
	if err != nil {
		t.Fatalf("ResolveProjectRef returned error: %v", err)
	}
	if got == nil || got.ProjectID != "hop-top/kit" {
		t.Fatalf("want hop-top/kit, got %+v", got)
	}
}

func TestResolveProjectRef_LabelMatch(t *testing.T) {
	reg := &fakeProjectLister{projects: []core.RegisteredProject{
		mkProj("hop-top/kit", "kit"),
		mkProj("hop-top/tlc", "tlc"),
	}}

	got, err := ResolveProjectRef(context.Background(), reg, "kit")
	if err != nil {
		t.Fatalf("ResolveProjectRef returned error: %v", err)
	}
	if got == nil || got.ProjectID != "hop-top/kit" {
		t.Fatalf("want hop-top/kit, got %+v", got)
	}
}

func TestResolveProjectRef_IDPrefix(t *testing.T) {
	reg := &fakeProjectLister{projects: []core.RegisteredProject{
		mkProj("hop-top/kit", "kit"),
		mkProj("acme/tlc", "tlc"),
	}}

	// "hop-top/k" is a unique ID prefix; label-prefix doesn't match
	// because no label starts with "hop-top/k".
	got, err := ResolveProjectRef(context.Background(), reg, "hop-top/k")
	if err != nil {
		t.Fatalf("ResolveProjectRef returned error: %v", err)
	}
	if got == nil || got.ProjectID != "hop-top/kit" {
		t.Fatalf("want hop-top/kit, got %+v", got)
	}
}

func TestResolveProjectRef_LabelPrefix(t *testing.T) {
	reg := &fakeProjectLister{projects: []core.RegisteredProject{
		mkProj("hop-top/kit", "kit-go"),
		mkProj("hop-top/tlc", "tlc"),
	}}

	// "ki" — no exact id, no exact label, no id-prefix; matches label
	// "kit-go".
	got, err := ResolveProjectRef(context.Background(), reg, "ki")
	if err != nil {
		t.Fatalf("ResolveProjectRef returned error: %v", err)
	}
	if got == nil || got.ProjectID != "hop-top/kit" {
		t.Fatalf("want hop-top/kit, got %+v", got)
	}
}

func TestResolveProjectRef_AmbiguousLabel(t *testing.T) {
	reg := &fakeProjectLister{projects: []core.RegisteredProject{
		mkProj("hop-top/kit", "kit"),
		mkProj("acme/kit", "kit"),
	}}

	_, err := ResolveProjectRef(context.Background(), reg, "kit")
	if err == nil {
		t.Fatal("expected ambiguous error, got nil")
	}
	var ambig *ErrAmbiguousProjectRef
	if !errors.As(err, &ambig) {
		t.Fatalf("expected *ErrAmbiguousProjectRef, got %T: %v", err, err)
	}
	if len(ambig.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %v", ambig.Candidates)
	}
	// Candidates are sorted alphabetically.
	if ambig.Candidates[0] != "acme/kit" || ambig.Candidates[1] != "hop-top/kit" {
		t.Fatalf("unexpected candidate order: %v", ambig.Candidates)
	}
}

func TestResolveProjectRef_AmbiguousIDPrefix(t *testing.T) {
	reg := &fakeProjectLister{projects: []core.RegisteredProject{
		mkProj("hop-top/kit", "kit"),
		mkProj("hop-top/kid", "kid"),
	}}

	// "hop-top/k" prefix matches both projects → ambiguous, surfaced
	// before the label-prefix stage.
	_, err := ResolveProjectRef(context.Background(), reg, "hop-top/k")
	if err == nil {
		t.Fatal("expected ambiguous error, got nil")
	}
	var ambig *ErrAmbiguousProjectRef
	if !errors.As(err, &ambig) {
		t.Fatalf("expected *ErrAmbiguousProjectRef, got %T: %v", err, err)
	}
}

func TestResolveProjectRef_NotFound(t *testing.T) {
	reg := &fakeProjectLister{projects: []core.RegisteredProject{
		mkProj("hop-top/kit", "kit"),
	}}

	_, err := ResolveProjectRef(context.Background(), reg, "nonexistent")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	var notFound *ErrProjectNotFound
	if !errors.As(err, &notFound) {
		t.Fatalf("expected *ErrProjectNotFound, got %T: %v", err, err)
	}
}

func TestResolveProjectRef_EmptyInput(t *testing.T) {
	reg := &fakeProjectLister{}

	_, err := ResolveProjectRef(context.Background(), reg, "")
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
	var notFound *ErrProjectNotFound
	if !errors.As(err, &notFound) {
		t.Fatalf("expected *ErrProjectNotFound, got %T: %v", err, err)
	}
}

func TestResolveProjectRef_IgnoresInactive(t *testing.T) {
	// An archived project with the same label should not contribute to
	// ambiguity at the fallback stages.
	archived := mkProj("acme/kit", "kit")
	archived.Status = "archived"

	reg := &fakeProjectLister{projects: []core.RegisteredProject{
		mkProj("hop-top/kit", "kit"),
		archived,
	}}

	got, err := ResolveProjectRef(context.Background(), reg, "kit")
	if err != nil {
		t.Fatalf("ResolveProjectRef returned error: %v", err)
	}
	if got == nil || got.ProjectID != "hop-top/kit" {
		t.Fatalf("want hop-top/kit, got %+v", got)
	}
}

func TestResolveProjectRef_TrimsWhitespace(t *testing.T) {
	reg := &fakeProjectLister{projects: []core.RegisteredProject{
		mkProj("hop-top/kit", "kit"),
	}}

	got, err := ResolveProjectRef(context.Background(), reg, "  kit  ")
	if err != nil {
		t.Fatalf("ResolveProjectRef returned error: %v", err)
	}
	if got == nil || got.ProjectID != "hop-top/kit" {
		t.Fatalf("want hop-top/kit, got %+v", got)
	}
}

// TestProjectDBCache_ReusesHandle verifies that opening the same path
// twice yields the same SQLiteStorage pointer.
func TestProjectDBCache_ReusesHandle(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "p.db")

	cache := NewProjectDBCache()
	defer cache.Close()

	h1, err := cache.Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	h2, err := cache.Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	if h1 != h2 {
		t.Fatal("cache returned different handles for the same path")
	}
}

// TestProjectDBCache_NilOpen falls through to a fresh storage when the
// cache pointer is nil so unconfigured callers still work.
func TestProjectDBCache_NilOpen(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "p.db")

	var c *ProjectDBCache
	h, err := c.Open(path)
	if err != nil {
		t.Fatalf("nil Open: %v", err)
	}
	if h == nil {
		t.Fatal("expected handle, got nil")
	}
	defer func() { _ = h.Close() }()
}

// TestProjectDBCache_CloseClosesAll verifies Close releases every cached
// handle and resets the cache so a subsequent Open creates a new one.
func TestProjectDBCache_CloseClosesAll(t *testing.T) {
	tmp := t.TempDir()
	path1 := filepath.Join(tmp, "a.db")
	path2 := filepath.Join(tmp, "b.db")

	cache := NewProjectDBCache()

	if _, err := cache.Open(path1); err != nil {
		t.Fatalf("Open(a): %v", err)
	}
	if _, err := cache.Open(path2); err != nil {
		t.Fatalf("Open(b): %v", err)
	}

	cache.Close()

	// After Close, opening the same path returns a fresh handle.
	h, err := cache.Open(path1)
	if err != nil {
		t.Fatalf("Open after Close: %v", err)
	}
	if h == nil {
		t.Fatal("expected new handle after Close")
	}
	cache.Close()
}

// Ensure the existing storage package's *SQLiteStorage satisfies the
// projectLister interface so production callers compile.
var _ projectLister = (*storage.SQLiteStorage)(nil)
