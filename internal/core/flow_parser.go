package core

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseFlow parses a flow definition from a reader based on the file extension.
func ParseFlow(r io.Reader, filename string) (*Flow, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read flow definition: %w", err)
	}

	var flow Flow
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &flow); err != nil {
			return nil, fmt.Errorf("failed to parse YAML flow: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &flow); err != nil {
			return nil, fmt.Errorf("failed to parse JSON flow: %w", err)
		}
	default:
		// Try YAML as fallback if no extension
		if err := yaml.Unmarshal(data, &flow); err == nil {
			return &flow, nil
		}
		// Then try JSON
		if err := json.Unmarshal(data, &flow); err != nil {
			return nil, fmt.Errorf("unsupported flow format and fallback parsing failed: %s", filename)
		}
	}

	// Basic validation
	if err := ValidateFlow(&flow); err != nil {
		return nil, err
	}

	return &flow, nil
}

// ValidateFlow checks a flow definition for structural integrity.
func ValidateFlow(f *Flow) error {
	if f.ID == "" {
		return fmt.Errorf("flow_id is required")
	}
	if f.EntryStep == "" {
		return fmt.Errorf("entry_step is required")
	}
	if len(f.Steps) == 0 {
		return fmt.Errorf("flow must contain at least one step")
	}

	// Verify entry step exists
	if _, ok := f.Steps[f.EntryStep]; !ok {
		return fmt.Errorf("entry_step %s not found in steps", f.EntryStep)
	}

	// Verify all dependencies and children references
	for id, step := range f.Steps {
		for _, dep := range step.DependsOn {
			if _, ok := f.Steps[dep]; !ok {
				return fmt.Errorf("step %s depends on non-existent step %s", id, dep)
			}
		}

		switch step.Type {
		case StepTypeParallel:
			if len(step.Children) == 0 {
				return fmt.Errorf("parallel step %s must contain at least one child", id)
			}
			for _, child := range step.Children {
				if _, ok := f.Steps[child]; !ok {
					return fmt.Errorf("parallel step %s references non-existent child %s", id, child)
				}
			}
		case StepTypeRetry:
			if step.Child == "" {
				return fmt.Errorf("retry step %s must reference a child step", id)
			}
			if _, ok := f.Steps[step.Child]; !ok {
				return fmt.Errorf("retry step %s references non-existent child %s", id, step.Child)
			}
		case StepTypeJoin:
			if len(step.WaitFor) == 0 {
				return fmt.Errorf("join step %s must contain at least one step in wait_for", id)
			}
			for _, w := range step.WaitFor {
				if _, ok := f.Steps[w]; !ok {
					return fmt.Errorf("join step %s references non-existent step %s in wait_for", id, w)
				}
			}
		}
	}

	// Cycle detection (simple DFS)
	if err := detectCycles(f); err != nil {
		return err
	}

	return nil
}

func detectCycles(f *Flow) error {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var check func(string) error
	check = func(id string) error {
		visited[id] = true
		recStack[id] = true

		step := f.Steps[id]
		// In TFS, edges are primarily depends_on (reverse logic for execution, 
		// but for definition validation, we check if dependency graph is DAG)
		for _, dep := range step.DependsOn {
			if !visited[dep] {
				if err := check(dep); err != nil {
					return err
				}
			} else if recStack[dep] {
				return fmt.Errorf("cycle detected involving step %s", dep)
			}
		}

		recStack[id] = false
		return nil
	}

	for id := range f.Steps {
		if !visited[id] {
			if err := check(id); err != nil {
				return err
			}
		}
	}

	return nil
}
