package workspace

import (
	"fmt"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// ListProjects returns all projects across all spaces in a workspace.
// Each space is resolved to its adapter, which discovers projects.
// Spaces that fail to resolve or discover are skipped with warnings
// collected in the returned error (nil when all succeed).
func ListProjects(ws config.WorkspaceConfig, reg *Registry) ([]core.RegisteredProject, error) {
	var all []core.RegisteredProject
	var errs []error

	for _, space := range ws.Spaces {
		adapter, err := reg.Resolve(space)
		if err != nil {
			errs = append(errs, fmt.Errorf("space %q: %w", space.URI, err))
			continue
		}

		projects, err := adapter.Discover(space)
		if err != nil {
			errs = append(errs, fmt.Errorf("space %q discover: %w", space.URI, err))
			continue
		}

		all = append(all, projects...)
	}

	if len(errs) > 0 {
		// Return partial results alongside a combined error.
		combined := errs[0]
		for _, e := range errs[1:] {
			combined = fmt.Errorf("%w; %w", combined, e)
		}
		return all, combined
	}

	return all, nil
}
