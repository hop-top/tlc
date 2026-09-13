package cli

import (
	"fmt"
	"io"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// recipeTaskGate is the pre-create hook: the same policy `task create`
// applies to a hand-made task, plus assignment when an engine is given.
func recipeTaskGate(
	w io.Writer, projectID, trackType string, engine *core.AssignmentEngine, warnings *[]string,
) func(*core.Task) error {
	valCfg := getValidationConfig()
	return func(task *core.Task) error {
		if err := core.ValidateTags(task.Tags); err != nil {
			return err //nolint:wrapcheck // the batch prefixes the step; the policy message names the tag
		}
		if err := valCfg.ValidateTaskOp(config.ValidationOpCreate, taskFieldsOf(task)); err != nil {
			return err //nolint:wrapcheck // the batch prefixes the step; the rule message names the field
		}
		if projectID != "" {
			if err := core.GateTaskCreate(projectID, trackType); err != nil {
				return err //nolint:wrapcheck // the gate names the stage and the fix
			}
		}
		if task.Priority != "" {
			core.MarkPriorityManual(task)
		} else if err := deriveTaskPriorityOnCreate(w, task); err != nil {
			return err
		}
		applySchedulingConfig(task)
		if engine != nil && task.AssignedTo == nil {
			assignRecipeTask(task, engine, warnings)
		}
		return nil
	}
}

func taskFieldsOf(t *core.Task) config.TaskFields {
	assigned := ""
	if t.AssignedTo != nil {
		assigned = *t.AssignedTo
	}
	return config.TaskFields{
		Title: t.Title, Description: t.Description, Status: string(t.Status), AssignedTo: assigned,
		Effort: string(t.Effort), Priority: string(t.Priority), Tags: t.Tags, Reference: t.Reference,
	}
}

// loadAssignmentEngine loads the configured assignees for --assign.
func loadAssignmentEngine() (*core.AssignmentEngine, error) {
	dir := assigneesDirFromConfig()
	assignees, err := core.NewAssigneeLoader(dir).LoadAll()
	if err != nil {
		return nil, fmt.Errorf("--assign: %w", err)
	}
	if len(assignees) == 0 {
		return nil, fmt.Errorf("--assign: no assignees found in %s; add one or drop --assign", dir)
	}
	return core.NewAssignmentEngine(assignees), nil
}

// assignRecipeTask picks an assignee for a step; no match is a warning,
// not a failed batch.
func assignRecipeTask(task *core.Task, engine *core.AssignmentEngine, warnings *[]string) {
	assignee, _, err := engine.FindBestAssignee(task)
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("step %s: not assigned: %v", task.StepID, err))
		return
	}
	id := assignee.ID
	task.AssignedTo = &id
}
