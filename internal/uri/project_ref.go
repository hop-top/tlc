package uri

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// ErrAmbiguousProjectRef is returned when a project ref string matches
// multiple registered projects and no single best match can be chosen.
type ErrAmbiguousProjectRef struct {
	Input      string
	Candidates []string
}

func (e *ErrAmbiguousProjectRef) Error() string {
	return fmt.Sprintf(
		"project ref %q is ambiguous; matches %d projects: %s; use the full project ID",
		e.Input, len(e.Candidates), strings.Join(e.Candidates, ", "),
	)
}

// projectLister is the subset of *storage.SQLiteStorage that the project
// ref resolver needs. Defined as an interface so tests can swap in a fake.
type projectLister interface {
	LookupProject(ctx context.Context, projectID string) (*core.RegisteredProject, error)
	ListAllProjects(ctx context.Context) ([]core.RegisteredProject, error)
}

// ResolveProjectRef resolves a fuzzy project reference into a registered
// project entry. The match cascade is:
//
//  1. Exact ID match (e.g. "hop-top/kit")
//  2. Exact label match (e.g. "kit") — single hit only
//  3. ID-prefix match (e.g. "hop-top/k") — single hit only
//  4. Label-prefix match (e.g. "ki") — single hit only
//
// Each fallback only succeeds with a single match. When a stage matches
// more than one project the function returns *ErrAmbiguousProjectRef
// listing the full candidate IDs so the agent can disambiguate.
//
// Returns *ErrProjectNotFound when no stage matches.
func ResolveProjectRef(
	ctx context.Context,
	registry projectLister,
	input string,
) (*core.RegisteredProject, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, &ErrProjectNotFound{ProjectID: input}
	}

	// Stage 1: exact ID match — cheapest, single SELECT.
	if p, err := registry.LookupProject(ctx, input); err != nil {
		return nil, fmt.Errorf("failed to lookup project %q: %w", input, err)
	} else if p != nil {
		return p, nil
	}

	all, err := registry.ListAllProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	// Filter out non-active rows so deleted/archived projects don't
	// cause spurious ambiguity.
	active := make([]core.RegisteredProject, 0, len(all))
	for _, p := range all {
		if p.Status != "" && p.Status != "active" {
			continue
		}
		active = append(active, p)
	}

	// Stage 2: exact label match.
	if p, err := singleMatch(input, active, func(p core.RegisteredProject) bool {
		return p.Label == input
	}); err != nil || p != nil {
		return p, err
	}

	// Stage 3: ID-prefix match.
	if p, err := singleMatch(input, active, func(p core.RegisteredProject) bool {
		return strings.HasPrefix(p.ProjectID, input)
	}); err != nil || p != nil {
		return p, err
	}

	// Stage 4: label-prefix match.
	if p, err := singleMatch(input, active, func(p core.RegisteredProject) bool {
		return p.Label != "" && strings.HasPrefix(p.Label, input)
	}); err != nil || p != nil {
		return p, err
	}

	return nil, &ErrProjectNotFound{ProjectID: input}
}

// singleMatch scans projects for the first stage where exactly one row
// satisfies pred. Returns (match, nil) on a unique hit, (nil, ambiguous)
// on >1 hits, and (nil, nil) when the stage has no hits so the caller
// can fall through to the next stage.
func singleMatch(
	input string,
	projects []core.RegisteredProject,
	pred func(core.RegisteredProject) bool,
) (*core.RegisteredProject, error) {
	var hits []core.RegisteredProject
	for _, p := range projects {
		if pred(p) {
			hits = append(hits, p)
		}
	}
	switch len(hits) {
	case 0:
		return nil, nil
	case 1:
		match := hits[0]
		return &match, nil
	default:
		ids := make([]string, len(hits))
		for i, h := range hits {
			ids[i] = h.ProjectID
		}
		sort.Strings(ids)
		return nil, &ErrAmbiguousProjectRef{Input: input, Candidates: ids}
	}
}

// ProjectDBCache caches opened cross-project SQLite handles by db_path so
// a single command invocation reusing the same cross-project lookup pays
// the open cost only once. The zero value is ready to use; callers MUST
// invoke Close before the command exits to release the underlying
// connections.
type ProjectDBCache struct {
	mu      sync.Mutex
	handles map[string]*storage.SQLiteStorage
}

// NewProjectDBCache returns an empty cache.
func NewProjectDBCache() *ProjectDBCache {
	return &ProjectDBCache{handles: make(map[string]*storage.SQLiteStorage)}
}

// Open returns a handle to the SQLite file at dbPath, opening it if not
// already cached. Subsequent calls with the same path return the cached
// handle. The handle is owned by the cache; do not close it directly —
// call (*ProjectDBCache).Close once when the command exits.
func (c *ProjectDBCache) Open(dbPath string) (*storage.SQLiteStorage, error) {
	if c == nil {
		return storage.NewSQLiteStorage(dbPath)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.handles == nil {
		c.handles = make(map[string]*storage.SQLiteStorage)
	}
	if h, ok := c.handles[dbPath]; ok {
		return h, nil
	}
	h, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open project database %q: %w", dbPath, err)
	}
	c.handles[dbPath] = h
	return h, nil
}

// Close closes every cached handle. Safe to call multiple times; safe on
// a nil receiver.
func (c *ProjectDBCache) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for path, h := range c.handles {
		_ = h.Close()
		delete(c.handles, path)
	}
}
