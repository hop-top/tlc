package uri

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
	"hop.top/uri"
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
func RegisterTypes(reg *uri.Registry, s *storage.SQLiteStorage, dirs ...*TypesDirConfig) error {
	var dc *TypesDirConfig
	if len(dirs) > 0 {
		dc = dirs[0]
	}
	// Project completion
	err := reg.Register(uri.TypeRegistration{
		Name: "project",
		Completer: func(ctx context.Context, prefix string) ([]string, error) {
			projects, err := s.ListAllProjects(ctx)
			if err != nil {
				return nil, err
			}
			var ids []string
			for _, p := range projects {
				if strings.HasPrefix(p.ProjectID, prefix) {
					ids = append(ids, p.ProjectID)
				}
			}
			return ids, nil
		},
	})
	if err != nil {
		return err
	}

	// Task completion
	err = reg.Register(uri.TypeRegistration{
		Name: "task",
		Completer: func(ctx context.Context, prefix string) ([]string, error) {
			tasks, err := s.ListTasks(ctx, core.Query{AllProjects: true})
			if err != nil {
				return nil, err
			}
			var ids []string
			for _, t := range tasks {
				if strings.HasPrefix(t.ID, prefix) {
					ids = append(ids, t.ID)
				}
			}
			return ids, nil
		},
	})
	if err != nil {
		return err
	}

	// Assignee completion
	err = reg.Register(uri.TypeRegistration{
		Name: "assignee",
		Completer: func(ctx context.Context, prefix string) ([]string, error) {
			loader := core.NewAssigneeLoader(dc.assigneesDir())
			assignees, err := loader.LoadAll()
			if err != nil {
				return nil, err
			}
			var slugs []string
			for _, a := range assignees {
				if strings.HasPrefix(a.ID, prefix) {
					slugs = append(slugs, a.ID)
				}
			}
			return slugs, nil
		},
	})
	if err != nil {
		return err
	}

	// Tag completion
	err = reg.Register(uri.TypeRegistration{
		Name: "tag",
		Completer: func(ctx context.Context, prefix string) ([]string, error) {
			tags, err := s.ListAllTags(ctx)
			if err != nil {
				return nil, err
			}
			var filtered []string
			for _, t := range tags {
				if strings.HasPrefix(t, prefix) {
					filtered = append(filtered, t)
				}
			}
			return filtered, nil
		},
	})
	if err != nil {
		return err
	}

	// Flow completion
	err = reg.Register(uri.TypeRegistration{
		Name: "flow",
		Completer: func(ctx context.Context, prefix string) ([]string, error) {
			flowsDir := dc.flowsDir()
			entries, err := os.ReadDir(flowsDir)
			if err != nil {
				return nil, err
			}
			var ids []string
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
					continue
				}
				f, err := os.Open(filepath.Join(flowsDir, entry.Name()))
				if err != nil {
					continue
				}
				flow, err := core.ParseFlow(f, entry.Name())
				_ = f.Close()
				if err != nil {
					continue
				}
				if strings.HasPrefix(flow.ID, prefix) {
					ids = append(ids, flow.ID)
				}
			}
			return ids, nil
		},
	})
	if err != nil {
		return err
	}

	return nil
}
