package core

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

// RunPlanExtractor executes the configured extractor command with the plan
// path as argument. Returns parsed task specs, or nil if no extractor
// configured (empty command).
//
// The extractor must write JSON or YAML to stdout in the same shape as
// the frontmatter tasks field. JSON is tried first; on failure, YAML.
func RunPlanExtractor(
	command string,
	planPath string,
) ([]PlanTaskSpec, error) {
	if command == "" {
		return nil, nil
	}

	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, nil
	}

	//nolint:gosec // command is from trusted config
	cmd := exec.Command(parts[0], append(parts[1:], planPath)...)
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			stderr = "; stderr: " + string(exitErr.Stderr)
		}
		return nil, fmt.Errorf(
			"plan extractor %q failed: %w%s; "+
				"verify the command exists and accepts a plan path argument",
			command, err, stderr,
		)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "[]" || trimmed == "null" {
		return nil, nil
	}

	// Try JSON first.
	var specs []PlanTaskSpec
	if err := json.Unmarshal([]byte(trimmed), &specs); err == nil {
		return specs, nil
	}

	// Fall back to YAML.
	if err := yaml.Unmarshal([]byte(trimmed), &specs); err != nil {
		return nil, fmt.Errorf(
			"plan extractor output not valid JSON or YAML: %w; "+
				"extractor must return a task list array",
			err,
		)
	}

	return specs, nil
}
