package core

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// PlanFrontmatter represents the YAML frontmatter of a plan document.
type PlanFrontmatter struct {
	Title  string         `yaml:"title"`
	Tracks []string       `yaml:"tracks,omitempty"`
	Tasks  []PlanTaskSpec `yaml:"tasks,omitempty"`
}

// PlanTaskSpec describes a task to be created from a plan.
type PlanTaskSpec struct {
	Title       string   `yaml:"title"`
	Description string   `yaml:"description,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
	Effort      string   `yaml:"effort,omitempty"`
	Priority    string   `yaml:"priority,omitempty"`
	AssignedTo  string   `yaml:"assigned-to,omitempty"`
	BlockedBy   []int    `yaml:"blocked-by,omitempty"`
}

// ParsePlanFrontmatter reads a markdown file and extracts YAML frontmatter.
// Frontmatter must be delimited by "---" lines at the start of the file.
// Returns an error if the file cannot be read or the YAML is malformed.
// Returns nil with no error if the file has no frontmatter.
func ParsePlanFrontmatter(path string) (*PlanFrontmatter, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf(
			"plan file %q: cannot open; verify path exists", path,
		)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	// First line must be "---".
	if !scanner.Scan() {
		return nil, nil // empty file
	}
	if strings.TrimSpace(scanner.Text()) != "---" {
		return nil, nil // no frontmatter
	}

	// Collect YAML lines until closing "---".
	var yamlLines []string
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			found = true
			break
		}
		yamlLines = append(yamlLines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf(
			"plan file %q: read error; %v", path, err,
		)
	}
	if !found {
		return nil, fmt.Errorf(
			"plan file %q: unclosed frontmatter; add closing '---' line",
			path,
		)
	}

	raw := strings.Join(yamlLines, "\n")
	var fm PlanFrontmatter
	if err := yaml.Unmarshal([]byte(raw), &fm); err != nil {
		return nil, fmt.Errorf(
			"plan file %q: malformed YAML frontmatter; %v", path, err,
		)
	}

	return &fm, nil
}
