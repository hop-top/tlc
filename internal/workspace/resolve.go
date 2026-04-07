package workspace

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// ResolveTargetProject determines which project a task should be created in
// when operating at workspace scope.
//
// Resolution order:
//  1. Check if cwd is inside a registered project's db_path parent directory
//  2. Check if cwd is inside any space (via adapter.Contains()) — narrow to
//     that space's projects
//  3. If exactly one candidate: use it
//  4. If multiple candidates or no match: prompt the user
func ResolveTargetProject(
	ws config.WorkspaceConfig,
	reg *Registry,
	projects []core.RegisteredProject,
	cwd string,
) (*core.RegisteredProject, error) {
	if len(projects) == 0 {
		return nil, fmt.Errorf("no projects found in workspace")
	}

	candidates := findCandidates(ws, reg, projects, cwd)

	if len(candidates) == 1 {
		return &candidates[0], nil
	}

	// Prompt from candidates if we have some, otherwise prompt from all.
	choices := candidates
	if len(choices) == 0 {
		choices = projects
	}

	selected, err := promptProjectSelection(ws, choices)
	if err != nil {
		return nil, fmt.Errorf("project selection: %w", err)
	}
	return selected, nil
}

// findCandidates returns matching projects for a cwd without prompting.
// Exported for testing.
func findCandidates(
	ws config.WorkspaceConfig,
	reg *Registry,
	projects []core.RegisteredProject,
	cwd string,
) []core.RegisteredProject {
	// Phase 1: check if cwd is inside a project's db_path parent.
	if match := matchByDBPath(projects, cwd); match != nil {
		return []core.RegisteredProject{*match}
	}

	// Phase 2: check via adapter Contains() per space.
	return matchByAdapter(ws, reg, projects, cwd)
}

// matchByDBPath checks if cwd is a parent/ancestor of any project's db_path
// directory. Returns the matching project or nil.
func matchByDBPath(projects []core.RegisteredProject, cwd string) *core.RegisteredProject {
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return nil
	}

	var matches []core.RegisteredProject
	for i := range projects {
		p := &projects[i]
		if p.DBPath == "" {
			continue
		}
		// db_path is typically <project>/.tlc/tasks.db; go up to project root.
		projDir := filepath.Dir(filepath.Dir(p.DBPath))
		absProjDir, err := filepath.Abs(projDir)
		if err != nil {
			continue
		}

		if isSubpath(absCwd, absProjDir) {
			matches = append(matches, *p)
		}
	}

	if len(matches) == 1 {
		return &matches[0]
	}
	return nil
}

// matchByAdapter uses space adapters to find projects whose space contains cwd.
func matchByAdapter(
	ws config.WorkspaceConfig,
	reg *Registry,
	projects []core.RegisteredProject,
	cwd string,
) []core.RegisteredProject {
	var candidates []core.RegisteredProject
	seen := make(map[string]bool)

	for _, space := range ws.Spaces {
		adapter, err := reg.Resolve(space)
		if err != nil {
			continue
		}

		hit := adapter.Contains(space, cwd)
		if hit == "" {
			continue
		}

		// Narrow to projects that belong to this space.
		for i := range projects {
			p := &projects[i]
			if seen[p.ProjectID] {
				continue
			}
			if p.SpaceURI == space.URI {
				candidates = append(candidates, *p)
				seen[p.ProjectID] = true
			}
		}
	}

	return candidates
}

// isSubpath reports whether child is equal to or under parent.
func isSubpath(child, parent string) bool {
	if child == parent {
		return true
	}
	prefix := parent
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(child, prefix)
}

// promptProjectSelection presents an interactive selector for projects.
func promptProjectSelection(
	ws config.WorkspaceConfig,
	projects []core.RegisteredProject,
) (*core.RegisteredProject, error) {
	if len(projects) == 0 {
		return nil, fmt.Errorf("no projects to select from")
	}

	// Build label map: space URI -> label.
	spaceLabels := make(map[string]string)
	for _, sp := range ws.Spaces {
		if sp.Label != "" {
			spaceLabels[sp.URI] = sp.Label
		}
	}

	// Check if projects span multiple spaces (for grouping).
	multiSpace := false
	if len(projects) > 1 {
		uri := projects[0].SpaceURI
		for _, p := range projects[1:] {
			if p.SpaceURI != uri {
				multiSpace = true
				break
			}
		}
	}

	opts := make([]huh.Option[int], 0, len(projects))
	for i, p := range projects {
		label := p.ProjectID
		if multiSpace {
			if sl, ok := spaceLabels[p.SpaceURI]; ok {
				label = fmt.Sprintf("[%s] %s", sl, p.ProjectID)
			}
		}
		opts = append(opts, huh.NewOption(label, i))
	}

	var selected int
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title("Which project for this task?").
				Options(opts...).
				Value(&selected),
		),
	)

	if err := form.Run(); err != nil {
		return nil, err
	}

	result := projects[selected]
	return &result, nil
}
