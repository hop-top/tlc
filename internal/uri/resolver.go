package uri

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
	"hop.top/tlc/internal/uriutil"
	"hop.top/uri"
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
	u, err := uri.Parse(input)
	if err != nil {
		return nil, err
	}

	// Case 1: Simple task ID (e.g. "T-0001")
	if u.Scheme == "" && u.Space == "" {
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
	// The hop.top/uri library places the first path segment in Space and the
	// remainder in ID.  For multi-segment project IDs the task ID is always
	// the *last* slash-delimited segment; everything before it (including
	// Space) forms the project ID.
	//
	// Examples:
	//   task://hop-top/tlc/T-0001  → Space=hop-top  ID=tlc/T-0001
	//     → projectID=hop-top/tlc  taskID=T-0001
	//   hop-top/tlc/T-0001         → Space=hop-top  ID=tlc/T-0001  (same)
	//   tlc/T-0001                 → Space=tlc       ID=T-0001
	//     → projectID=tlc          taskID=T-0001
	projectID, taskID := uriutil.SplitProjectTask(u.Space, u.ID)

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
	u, err := uri.Parse(input)
	if err != nil {
		return nil, err
	}

	projectID, flowID := uriutil.SplitProjectTask(u.Space, u.ID)

	// If it's a file path, just parse it
	if _, err := os.Stat(input); err == nil {
		f, err := os.Open(input)
		if err != nil {
			return nil, err
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
