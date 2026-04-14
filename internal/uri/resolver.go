package uri

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
	"hop.top/uri"
)

// Resolver handles resolution of TLC resource URIs.
type Resolver struct {
	storage  *storage.SQLiteStorage
	FlowsDir string // overrides default flows directory; empty = "examples/flows"
}

// NewResolver creates a new URI resolver.
func NewResolver(s *storage.SQLiteStorage) *Resolver {
	return &Resolver{storage: s}
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
	projectID, taskID := splitProjectTask(u.Space, u.ID)

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

	// Look up project in global registry
	regProj, err := r.storage.LookupProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup project %q: %w", projectID, err)
	}
	if regProj == nil {
		return nil, &ErrProjectNotFound{ProjectID: projectID}
	}

	// Open project database
	projStorage, err := storage.NewSQLiteStorage(regProj.DBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open project database %q: %w", regProj.DBPath, err)
	}

	task, err := projStorage.GetTask(ctx, taskID)
	if err != nil {
		_ = projStorage.Close()
		return nil, err
	}
	if task == nil {
		_ = projStorage.Close()
		return nil, &ErrTaskNotFound{ID: taskID, ProjectID: projectID}
	}

	return &ResolvedTask{Task: task, Storage: projStorage}, nil
}

// splitProjectTask derives (projectID, taskID) from the Space and ID fields
// returned by hop.top/uri.Parse.
//
// The task ID is always the last slash-delimited segment of the combined
// "space/id" path.  Everything before it is the project ID.
//
//	space="hop-top"  id="tlc/T-0001"  → ("hop-top/tlc", "T-0001")
//	space="tlc"      id="T-0001"      → ("tlc",          "T-0001")
//	space=""         id="T-0001"      → ("",              "T-0001")
func splitProjectTask(space, id string) (projectID, taskID string) {
	combined := id
	if space != "" {
		combined = space + "/" + id
	}
	if idx := strings.LastIndex(combined, "/"); idx >= 0 {
		return combined[:idx], combined[idx+1:]
	}
	return "", combined
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

	projectID, flowID := splitProjectTask(u.Space, u.ID)

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
