package core

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// IDGenerator provides sequential task ID generation.
type IDGenerator interface {
	GetNextSequenceID(ctx context.Context, projectID string) (int, error)
}

// CreateTasksFromPlan creates tasks from plan specs, resolving blocked-by
// index references to real task IDs. Returns created task IDs.
//
// For each PlanTaskSpec a Task is created with:
//   - ID from idGen.GetNextSequenceID
//   - TrackID set to trackID
//   - blocked-by resolved: index 0 -> first created task's ID, etc.
//   - tags, effort, priority, assigned-to mapped from spec
func (s *TrackService) CreateTasksFromPlan(
	ctx context.Context,
	trackID string,
	specs []PlanTaskSpec,
	projectID string,
	idGen IDGenerator,
) ([]string, error) {
	if len(specs) == 0 {
		return nil, nil
	}

	createdIDs := make([]string, 0, len(specs))
	now := time.Now().UTC()

	for i, spec := range specs {
		// Validate blocked-by indices.
		for _, idx := range spec.BlockedBy {
			if idx < 0 || idx >= i {
				return createdIDs, fmt.Errorf(
					"plan task %d (%q): blocked-by index %d out of range "+
						"(must be 0..%d); fix the plan frontmatter",
					i, spec.Title, idx, i-1,
				)
			}
		}

		// Generate task ID.
		seq, err := idGen.GetNextSequenceID(ctx, projectID)
		if err != nil {
			return createdIDs, fmt.Errorf(
				"plan task %d (%q): failed to generate ID; %w",
				i, spec.Title, err,
			)
		}
		taskID := fmt.Sprintf("T-%04d", seq)

		// Resolve blocked-by indices to task IDs.
		var blockedBy []string
		for _, idx := range spec.BlockedBy {
			blockedBy = append(blockedBy, createdIDs[idx])
		}

		// Build metadata.
		meta := make(map[string]any)
		if len(blockedBy) > 0 {
			meta["blocked_by"] = blockedBy
		}

		// Normalize assignee.
		var assignee *string
		if spec.AssignedTo != "" {
			a := spec.AssignedTo
			if len(a) > 1 && a[0] == '@' {
				a = a[1:]
			}
			assignee = &a
		}

		// Set project ID.
		var projPtr *string
		if projectID != "" {
			projPtr = &projectID
		}

		task := &Task{
			ID:          taskID,
			Title:       spec.Title,
			Description: strings.TrimSpace(spec.Description),
			Status:      StatusTodo,
			AssignedTo:  assignee,
			Tags:        spec.Tags,
			Effort:      Effort(spec.Effort),
			Priority:    Priority(spec.Priority),
			CreatedAt:   now,
			UpdatedAt:   now,
			TrackID:     &trackID,
			ProjectID:   projPtr,
			Meta:        meta,
		}

		if err := s.taskRepo.CreateTask(ctx, task); err != nil {
			return createdIDs, fmt.Errorf(
				"plan task %d (%q): failed to create; %w",
				i, spec.Title, err,
			)
		}

		createdIDs = append(createdIDs, taskID)
	}

	return createdIDs, nil
}
