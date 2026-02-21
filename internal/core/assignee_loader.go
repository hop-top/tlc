package core

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// AssigneeLoader loads assignee definitions from YAML files.
type AssigneeLoader struct {
	assigneesDir string
}

// NewAssigneeLoader creates a new assignee loader for the given directory.
func NewAssigneeLoader(assigneesDir string) *AssigneeLoader {
	return &AssigneeLoader{
		assigneesDir: assigneesDir,
	}
}

// LoadAll loads all assignees from the assignees directory.
func (l *AssigneeLoader) LoadAll() ([]*Assignee, error) {
	// Check if directory exists
	if _, err := os.Stat(l.assigneesDir); os.IsNotExist(err) {
		return []*Assignee{}, nil // Return empty slice if directory doesn't exist
	}

	entries, err := os.ReadDir(l.assigneesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read assignees directory: %w", err)
	}

	assignees := []*Assignee{}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		assignee, err := l.LoadAssignee(filepath.Join(l.assigneesDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to load assignee %s: %w", entry.Name(), err)
		}

		assignees = append(assignees, assignee)
	}

	return assignees, nil
}

// LoadAssignee loads a single assignee from a YAML file.
func (l *AssigneeLoader) LoadAssignee(filePath string) (*Assignee, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read assignee file: %w", err)
	}

	var assignee Assignee
	if err := yaml.Unmarshal(data, &assignee); err != nil {
		return nil, fmt.Errorf("failed to unmarshal assignee: %w", err)
	}

	return &assignee, nil
}

// GetAssignee retrieves a specific assignee by ID.
func (l *AssigneeLoader) GetAssignee(assigneeID string) (*Assignee, error) {
	assignees, err := l.LoadAll()
	if err != nil {
		return nil, err
	}

	for _, a := range assignees {
		if a.ID == assigneeID {
			return a, nil
		}
	}

	return nil, fmt.Errorf("assignee not found: %s", assigneeID)
}
