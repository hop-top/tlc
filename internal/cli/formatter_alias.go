package cli

import (
	"context"
	"strings"

	"hop.top/tlc/internal/core"
)

// aliasReference returns ref with the trailing typeid replaced by its
// human-readable alias when the project segment matches the
// locally-detected project. Foreign refs (different project, or
// unresolvable) are returned unchanged. Malformed input is also
// returned unchanged — never panic on a bad URI.
//
// Lookup uses the supplied storage handle; pass core.GetTaskByID-shaped
// access to avoid opening fresh DB handles per call. Pass currentProject
// from core.DetectProject() once at the call site.
//
// Behaviour matrix (currentProject = "hop-top/tlc"):
//
//	tlc://hop-top/tlc/task_01k...      → tlc://hop-top/tlc/T-1313
//	tlc://hop-top/tlc/track_01k...     → tlc://hop-top/tlc/<slug>
//	tlc://other/proj/task_01k...       → unchanged (foreign project)
//	tlc:///T-1313                      → unchanged (already alias)
//	tlc://hop-top/tlc/T-1313           → unchanged (already alias)
//	tlc://hop-top/tlc/task_01k... (404)→ unchanged (lookup miss)
//	https://...                        → unchanged (foreign scheme)
//	""                                 → unchanged
func aliasReference(ctx context.Context, s aliasStorage, ref, currentProject string) string {
	if ref == "" || currentProject == "" || s == nil {
		return ref
	}
	prefix := "tlc://" + currentProject + "/"
	if !strings.HasPrefix(ref, prefix) {
		return ref
	}
	tail := strings.TrimPrefix(ref, prefix)
	// Bail on anything beyond a bare identifier (query, fragment, path).
	if strings.ContainsAny(tail, "/?#") {
		return ref
	}
	switch {
	case core.IsTaskID(tail):
		t, err := s.GetTask(ctx, tail)
		if err != nil || t == nil {
			return ref
		}
		alias := core.FormatTaskAlias(t)
		if alias == "" {
			return ref
		}
		return prefix + alias
	case core.IsTrackID(tail):
		tr, err := s.GetTrack(ctx, tail)
		if err != nil || tr == nil {
			return ref
		}
		if tr.Slug == "" {
			return ref
		}
		return prefix + tr.Slug
	default:
		// Already aliased (T-NNNN, slug) or unrecognised — leave alone.
		return ref
	}
}

// aliasStorage is the minimal storage surface aliasReference needs. Lets
// callers pass the existing *storage.SQLiteStorage without widening
// dependencies; tests fake the two methods directly.
type aliasStorage interface {
	GetTask(ctx context.Context, id string) (*core.Task, error)
	GetTrack(ctx context.Context, id string) (*core.Track, error)
}
