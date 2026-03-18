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

// RegisterTypes registers tlc-specific URI types with the registry.
func RegisterTypes(reg *uri.Registry, s *storage.SQLiteStorage) error {
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
			assigneesDir := filepath.Join("examples", "assignees")
			loader := core.NewAssigneeLoader(assigneesDir)
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
			flowsDir := filepath.Join("examples", "flows")
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
