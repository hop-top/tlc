package uri

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hop.top/cite/scheme"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
	"hop.top/tlc/internal/uriutil"
)

// Resolver handles resolution of TLC resource URIs.
type Resolver struct {
	storage  *storage.SQLiteStorage
	FlowsDir string // overrides default flows directory; empty = "examples/flows"

	// DBCache caches cross-project DB handles. When nil, ResolveTask
	// opens a fresh handle on every cross-DB lookup and the caller (or
	// a deferred close in ResolvedTask) must close it. When non-nil,
	// handles are owned by the cache; callers MUST invoke
	// ProjectDBCache.Close before the command exits.
	DBCache *ProjectDBCache
}

// NewResolver creates a new URI resolver.
func NewResolver(s *storage.SQLiteStorage) *Resolver {
	return &Resolver{storage: s}
}

// WithDBCache returns a copy of the resolver bound to cache so cross-DB
// handles are reused for the lifetime of the cache.
func (r *Resolver) WithDBCache(cache *ProjectDBCache) *Resolver {
	cp := *r
	cp.DBCache = cache
	return &cp
}

// defaultFlowsDir returns the configured or default flows directory.
func (r *Resolver) defaultFlowsDir() string {
	if r.FlowsDir != "" {
		return r.FlowsDir
	}
	return filepath.Join("examples", "flows")
}

// ResolvedTask represents a task and the storage it was found in.
type ResolvedTask struct {
	Task    *core.Task
	Storage *storage.SQLiteStorage
}

// ResolveTask resolves a task ID or tlc:// URI into a Task object.
// Flexible ID forms are normalised first; see NormalizeTaskID.
func (r *Resolver) ResolveTask(ctx context.Context, input string) (*ResolvedTask, error) {
	input = NormalizeTaskID(input)

	// Decompose the input into (namespace, id) without going through
	// scheme.Parse, which now requires a scheme and a non-empty namespace.
	// Bare task IDs ("T-0001") and shorthand project/task refs lack one
	// or both; we still need to route those forms.
	u, parseErr := splitTaskInput(input)
	if parseErr != nil {
		return nil, parseErr
	}

	// Case 1: Simple task ID (e.g. "T-0001")
	if u.Scheme == "" && u.Namespace == "" {
		task, err := r.storage.GetTask(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		if task == nil {
			return nil, &ErrTaskNotFound{ID: u.ID}
		}
		return &ResolvedTask{Task: task, Storage: r.storage}, nil
	}

	// Case 2: Shorthand project/task or absolute URI.
	//
	// The first path segment is the namespace and the remainder is the
	// id. For multi-segment project IDs the task ID is always the *last*
	// slash-delimited segment; everything before it (including Namespace)
	// forms the project ID.
	//
	// Examples:
	//   task://hop-top/tlc/T-0001  → Namespace=hop-top  ID=tlc/T-0001
	//     → projectID=hop-top/tlc  taskID=T-0001
	//   hop-top/tlc/T-0001         → Namespace=hop-top  ID=tlc/T-0001  (same)
	//   tlc/T-0001                 → Namespace=tlc       ID=T-0001
	//     → projectID=tlc          taskID=T-0001
	projectID, taskID := uriutil.SplitProjectTask(u.Namespace, u.ID)
	// Re-normalise the extracted task ID so URI forms accept the same
	// case-insensitive aliases as bare inputs (e.g. ".../t-0001" → T-0001).
	taskID = NormalizeTaskID(taskID)

	if projectID == "" {
		task, err := r.storage.GetTask(ctx, taskID)
		if err != nil {
			return nil, err
		}
		if task == nil {
			return nil, &ErrTaskNotFound{ID: taskID}
		}
		return &ResolvedTask{Task: task, Storage: r.storage}, nil
	}

	// Look up project in registry with fuzzy fallbacks (label, prefix).
	regProj, err := ResolveProjectRef(ctx, r.storage, projectID)
	if err != nil {
		return nil, err
	}

	// Open project database via cache when present so multiple lookups
	// in the same command share a single handle.
	var projStorage *storage.SQLiteStorage
	cached := r.DBCache != nil
	if cached {
		projStorage, err = r.DBCache.Open(regProj.DBPath)
	} else {
		projStorage, err = storage.NewSQLiteStorage(regProj.DBPath)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open project database %q: %w", regProj.DBPath, err)
	}

	// Look up the task in the resolved project's DB scoped to the
	// canonical project_id (so a label/prefix-matched project still
	// hits the right rows).
	task, err := projStorage.GetTaskInProject(ctx, taskID, regProj.ProjectID)
	if err != nil {
		if !cached {
			_ = projStorage.Close()
		}
		return nil, err
	}
	if task == nil {
		// Fall back to an unscoped lookup so DBs that pre-date
		// project_id stamping (project_id='' in old rows) still
		// resolve. GetTask without a current-project context returns
		// the global bucket entry first.
		task, err = projStorage.GetTaskInProject(ctx, taskID, "")
		if err != nil {
			if !cached {
				_ = projStorage.Close()
			}
			return nil, err
		}
	}
	if task == nil {
		if !cached {
			_ = projStorage.Close()
		}
		return nil, &ErrTaskNotFound{ID: taskID, ProjectID: regProj.ProjectID}
	}

	return &ResolvedTask{Task: task, Storage: projStorage}, nil
}

// ResolvedFlow represents a flow and the directory it was found in.
type ResolvedFlow struct {
	Flow *core.Flow
	Path string
}

// ResolveFlow resolves a flow ID or URI into a Flow object.
func (r *Resolver) ResolveFlow(ctx context.Context, input string) (*ResolvedFlow, error) {
	u, err := splitTaskInput(input)
	if err != nil {
		return nil, err
	}

	projectID, flowID := uriutil.SplitProjectTask(u.Namespace, u.ID)

	// If it's a file path, just parse it
	if _, err := os.Stat(input); err == nil {
		f, err := os.Open(input)
		if err != nil {
			return nil, fmt.Errorf("open flow file %s: %w", input, err)
		}
		defer f.Close()
		flow, err := core.ParseFlow(f, input)
		if err != nil {
			return nil, err
		}
		return &ResolvedFlow{Flow: flow, Path: input}, nil
	}

	// Determine flows directory
	flowsDir := r.defaultFlowsDir()
	if projectID != "" {
		regProj, err := r.storage.LookupProject(ctx, projectID)
		if err == nil && regProj != nil {
			// If we have a project, the flows should be in its directory
			projectRoot := filepath.Dir(filepath.Dir(regProj.DBPath))
			flowsDir = filepath.Join(projectRoot, r.defaultFlowsDir())
		}
	}

	// Search for flow ID in flows directory
	entries, err := os.ReadDir(flowsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read flows directory %q: %w", flowsDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(flowsDir, entry.Name())
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		flow, err := core.ParseFlow(f, entry.Name())
		_ = f.Close()
		if err != nil {
			continue
		}
		if flow.ID == flowID {
			return &ResolvedFlow{Flow: flow, Path: path}, nil
		}
	}

	return nil, fmt.Errorf("flow %q not found", flowID)
}

// splitTaskInput decomposes a task ref into the same shape the legacy
// hop.top/cite.Parse used to return (Scheme, Namespace, ID). It tolerates
// bare task IDs and shorthand "ns/.../id" forms that scheme.Parse (which
// requires scheme + non-empty namespace) rejects.
func splitTaskInput(input string) (*scheme.URI, error) {
	if input == "" {
		return nil, fmt.Errorf("uri: empty input")
	}

	// Full scheme:// URI — delegate to the upstream parser when it can
	// satisfy its own contract (scheme present + non-empty host). Fall
	// through to manual decomposition only when upstream errors apply
	// to historically valid shorthand forms.
	if i := strings.Index(input, "://"); i > 0 {
		schemeName := input[:i]
		rest := strings.TrimPrefix(input[i+3:], "")
		// Trim query/fragment; resolver does not consume them today.
		if j := strings.IndexAny(rest, "?#"); j >= 0 {
			rest = rest[:j]
		}
		var ns, id string
		if k := strings.Index(rest, "/"); k >= 0 {
			ns = rest[:k]
			id = rest[k+1:]
		} else {
			ns = rest
		}
		return &scheme.URI{Scheme: schemeName, Namespace: ns, ID: id, Original: input}, nil
	}

	// Bare or shorthand input (no "://"). Split on the first "/" so
	// "tlc/T-0001" → Namespace="tlc", ID="T-0001" and "T-0001" alone
	// becomes ID="T-0001" with empty Scheme + Namespace.
	if i := strings.Index(input, "/"); i >= 0 {
		return &scheme.URI{Namespace: input[:i], ID: input[i+1:], Original: input}, nil
	}
	return &scheme.URI{ID: input, Original: input}, nil
}
