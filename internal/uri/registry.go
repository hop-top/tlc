package uri

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
	"hop.top/uri/scheme"
)

// TypesDirConfig holds configurable directory paths for URI type registration.
type TypesDirConfig struct {
	FlowsDir     string // empty = "examples/flows"
	AssigneesDir string // empty = "examples/assignees"
}

func (c *TypesDirConfig) flowsDir() string {
	if c != nil && c.FlowsDir != "" {
		return c.FlowsDir
	}
	return filepath.Join("examples", "flows")
}

func (c *TypesDirConfig) assigneesDir() string {
	if c != nil && c.AssigneesDir != "" {
		return c.AssigneesDir
	}
	return filepath.Join("examples", "assignees")
}

// RegisterTypes registers tlc-specific URI types with the registry.
// dirs is optional; nil uses defaults.
func RegisterTypes(reg *scheme.Registry, s *storage.SQLiteStorage, dirs ...*TypesDirConfig) error {
	var dc *TypesDirConfig
	if len(dirs) > 0 {
		dc = dirs[0]
	}
	registrations := []scheme.TypeRegistration{
		projectCompletion(s),
		taskCompletion(s),
		assigneeCompletion(dc),
		tagCompletion(s),
		flowCompletion(dc),
	}
	for _, r := range registrations {
		if err := reg.Register(r); err != nil {
			return fmt.Errorf("register uri type %q: %w", r.Name, err)
		}
	}
	return nil
}

func projectCompletion(s *storage.SQLiteStorage) scheme.TypeRegistration {
	return scheme.TypeRegistration{
		Name: "project",
		Completer: func(ctx context.Context, prefix string) ([]string, error) {
			projects, err := s.ListAllProjects(ctx)
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(projects))
			for _, p := range projects {
				if strings.HasPrefix(p.ProjectID, prefix) {
					ids = append(ids, p.ProjectID)
				}
			}
			return ids, nil
		},
	}
}

func taskCompletion(s *storage.SQLiteStorage) scheme.TypeRegistration {
	return scheme.TypeRegistration{
		Name: "task",
		Completer: func(ctx context.Context, prefix string) ([]string, error) {
			tasks, err := s.ListTasks(ctx, core.Query{AllProjects: true})
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(tasks))
			for _, t := range tasks {
				if strings.HasPrefix(t.ID, prefix) {
					ids = append(ids, t.ID)
				}
			}
			return ids, nil
		},
	}
}

func assigneeCompletion(dc *TypesDirConfig) scheme.TypeRegistration {
	return scheme.TypeRegistration{
		Name: "assignee",
		Completer: func(_ context.Context, prefix string) ([]string, error) {
			loader := core.NewAssigneeLoader(dc.assigneesDir())
			assignees, err := loader.LoadAll()
			if err != nil {
				return nil, err
			}
			slugs := make([]string, 0, len(assignees))
			for _, a := range assignees {
				if strings.HasPrefix(a.ID, prefix) {
					slugs = append(slugs, a.ID)
				}
			}
			return slugs, nil
		},
	}
}

func tagCompletion(s *storage.SQLiteStorage) scheme.TypeRegistration {
	return scheme.TypeRegistration{
		Name: "tag",
		Completer: func(ctx context.Context, prefix string) ([]string, error) {
			tags, err := s.ListAllTags(ctx)
			if err != nil {
				return nil, err
			}
			filtered := make([]string, 0, len(tags))
			for _, t := range tags {
				if strings.HasPrefix(t, prefix) {
					filtered = append(filtered, t)
				}
			}
			return filtered, nil
		},
	}
}

func flowCompletion(dc *TypesDirConfig) scheme.TypeRegistration {
	return scheme.TypeRegistration{
		Name: "flow",
		Completer: func(_ context.Context, prefix string) ([]string, error) {
			flowsDir := dc.flowsDir()
			entries, err := os.ReadDir(flowsDir)
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(entries))
			for _, entry := range entries {
				if !flowFileCandidate(entry) {
					continue
				}
				id, ok := parseFlowID(filepath.Join(flowsDir, entry.Name()))
				if !ok || !strings.HasPrefix(id, prefix) {
					continue
				}
				ids = append(ids, id)
			}
			return ids, nil
		},
	}
}

func flowFileCandidate(entry os.DirEntry) bool {
	if entry.IsDir() {
		return false
	}
	name := entry.Name()
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}

func parseFlowID(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()
	flow, err := core.ParseFlow(f, filepath.Base(path))
	if err != nil {
		return "", false
	}
	return flow.ID, true
}
