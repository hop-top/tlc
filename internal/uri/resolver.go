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
	storage *storage.SQLiteStorage
}

// NewResolver creates a new URI resolver.
func NewResolver(s *storage.SQLiteStorage) *Resolver {
	return &Resolver{storage: s}
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
			return nil, fmt.Errorf("task not found: %s", u.ID)
		}
		return &ResolvedTask{Task: task, Storage: r.storage}, nil
	}

	// Case 2: Shorthand project/task or URI
	projectID := u.Space
	taskID := u.ID

	// If ID still contains slashes, it might be project/task
	if strings.Contains(taskID, "/") {
		parts := strings.SplitN(taskID, "/", 2)
		if projectID == "" {
			projectID = parts[0]
		}
		taskID = parts[1]
	}

	if projectID == "" {
		task, err := r.storage.GetTask(ctx, taskID)
		if err != nil {
			return nil, err
		}
		if task == nil {
			return nil, fmt.Errorf("task not found: %s", taskID)
		}
		return &ResolvedTask{Task: task, Storage: r.storage}, nil
	}

	// Look up project in global registry
	regProj, err := r.storage.LookupProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup project %q: %w", projectID, err)
	}
	if regProj == nil {
		return nil, fmt.Errorf("project %q not found in registry", projectID)
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
		return nil, fmt.Errorf("task %q not found in project %q", taskID, projectID)
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

	projectID := u.Space
	flowID := u.ID

	if strings.Contains(flowID, "/") {
		parts := strings.SplitN(flowID, "/", 2)
		if projectID == "" {
			projectID = parts[0]
		}
		flowID = parts[1]
	}

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
	flowsDir := filepath.Join("examples", "flows")
	if projectID != "" {
		regProj, err := r.storage.LookupProject(ctx, projectID)
		if err == nil && regProj != nil {
			// If we have a project, the flows should be in its directory
			projectRoot := filepath.Dir(filepath.Dir(regProj.DBPath))
			flowsDir = filepath.Join(projectRoot, "examples", "flows")
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
