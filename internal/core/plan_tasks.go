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

// PlanIngestResult describes the outcome of CreateTasksFromPlan.
// It reports the newly created task IDs (in input order) and any
// cross-track references that were resolved during ingestion so
// callers can rewrite the source plan file on disk.
type PlanIngestResult struct {
	// CreatedIDs is the task ID for each input spec, in order.
	CreatedIDs []string
	// ResolvedRefs maps raw cross-track ref strings ("track#N") to
	// the resolved T-NNNN task IDs.
	ResolvedRefs map[string]string
}

// CreateTasksFromPlan creates tasks from plan specs, resolving all
// blocked-by references.
//
// Supported entry forms (per BlockedByRef):
//   - int index N: intra-track, 0-based index into the current
//     plan's tasks list.
//   - "T-NNNN": a concrete task ID that must already exist.
//   - "<track-id>#N": a cross-track reference, 1-based task number
//     into the target track's linked plan.
//
// Cross-track refs require the target track to exist AND to have a
// linked plan whose frontmatter has at least N tasks. The Nth task
// title is looked up in the database by (track_id, title). If any
// ref cannot be resolved, nothing is created and an error is
// returned.
func (s *TrackService) CreateTasksFromPlan(
	ctx context.Context,
	trackID string,
	specs []PlanTaskSpec,
	projectID string,
	idGen IDGenerator,
) (*PlanIngestResult, error) {
	if len(specs) == 0 {
		return &PlanIngestResult{}, nil
	}

	// --- Pre-flight validation of every BlockedByRef -------------
	// We resolve everything we can BEFORE touching the database so
	// that a failed ingest leaves no partial state behind.
	resolved := make(map[string]string) // raw -> T-NNNN
	for i, spec := range specs {
		for _, ref := range spec.BlockedBy {
			switch {
			case ref.IsIndex():
				if ref.Index < 0 || ref.Index >= i {
					return nil, fmt.Errorf(
						"plan task %d (%q): blocked-by index %d out "+
							"of range (must be 0..%d); fix the plan "+
							"frontmatter",
						i, spec.Title, ref.Index, i-1,
					)
				}
			case ref.TaskID != "":
				t, err := s.taskRepo.GetTask(ctx, ref.TaskID)
				if err != nil || t == nil {
					return nil, fmt.Errorf(
						"plan task %d (%q): blocked-by references "+
							"task %q which does not exist; create it "+
							"first or use an intra-track index",
						i, spec.Title, ref.TaskID,
					)
				}
			case ref.CrossTrack != nil:
				id, err := s.resolveCrossTrackRef(ctx, ref.CrossTrack)
				if err != nil {
					return nil, fmt.Errorf(
						"plan task %d (%q): %w",
						i, spec.Title, err,
					)
				}
				resolved[ref.Raw()] = id
			}
		}
	}

	// --- Create tasks ------------------------------------------------
	createdIDs := make([]string, 0, len(specs))
	now := time.Now().UTC()

	for i, spec := range specs {
		seq, err := idGen.GetNextSequenceID(ctx, projectID)
		if err != nil {
			return nil, fmt.Errorf(
				"plan task %d (%q): failed to generate ID; %w",
				i, spec.Title, err,
			)
		}
		taskID := fmt.Sprintf("T-%04d", seq)

		// Resolve blocked-by entries to task IDs now that indices
		// can be mapped to createdIDs.
		var blockedBy []string
		for _, ref := range spec.BlockedBy {
			switch {
			case ref.IsIndex():
				blockedBy = append(blockedBy, createdIDs[ref.Index])
			case ref.TaskID != "":
				blockedBy = append(blockedBy, ref.TaskID)
			case ref.CrossTrack != nil:
				blockedBy = append(blockedBy, resolved[ref.Raw()])
			}
		}

		meta := make(map[string]any)
		if len(blockedBy) > 0 {
			meta["blocked_by"] = blockedBy
		}

		var assignee *string
		if spec.AssignedTo != "" {
			a := spec.AssignedTo
			if len(a) > 1 && a[0] == '@' {
				a = a[1:]
			}
			assignee = &a
		}

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
			return nil, fmt.Errorf(
				"plan task %d (%q): failed to create; %w",
				i, spec.Title, err,
			)
		}

		createdIDs = append(createdIDs, taskID)
	}

	return &PlanIngestResult{
		CreatedIDs:   createdIDs,
		ResolvedRefs: resolved,
	}, nil
}

// resolveCrossTrackRef resolves a "<track-id>#N" reference to a
// concrete task ID by reading the target track's linked plan and
// looking up tasks[N-1].title in the database.
func (s *TrackService) resolveCrossTrackRef(
	ctx context.Context,
	ref *CrossTrackRef,
) (string, error) {
	// 1. Target track must exist.
	target, err := s.repo.GetTrack(ctx, ref.TrackID)
	if err != nil || target == nil {
		return "", fmt.Errorf(
			"cross-track blocked-by %q: prerequisite track %q not "+
				"created; create it and ingest its plan first",
			fmt.Sprintf("%s#%d", ref.TrackID, ref.TaskNum),
			ref.TrackID,
		)
	}

	// 2. Target track must have at least one linked plan.
	plans := extractPlansFromMeta(target.Meta)
	if len(plans) == 0 {
		return "", fmt.Errorf(
			"cross-track blocked-by %q: target track %q has no "+
				"linked plan; ingest its plan first",
			fmt.Sprintf("%s#%d", ref.TrackID, ref.TaskNum),
			ref.TrackID,
		)
	}

	// 3. Parse the first linked plan frontmatter and index into
	//    tasks[N-1].
	fm, err := ParsePlanFrontmatter(plans[0])
	if err != nil {
		return "", fmt.Errorf(
			"cross-track blocked-by %q: cannot read target plan %q; %w",
			fmt.Sprintf("%s#%d", ref.TrackID, ref.TaskNum),
			plans[0], err,
		)
	}
	if fm == nil || len(fm.Tasks) < ref.TaskNum {
		have := 0
		if fm != nil {
			have = len(fm.Tasks)
		}
		return "", fmt.Errorf(
			"cross-track blocked-by %q: out of range; target track "+
				"%q has %d tasks in plan %q",
			fmt.Sprintf("%s#%d", ref.TrackID, ref.TaskNum),
			ref.TrackID, have, plans[0],
		)
	}
	title := fm.Tasks[ref.TaskNum-1].Title

	// 4. Look up the task in the DB by (track_id, title, project_id).
	//    Track IDs are unique only within (project_id, id), so we
	//    must scope by the target track's owning project to avoid
	//    cross-project ambiguity. See TrackService.linkedTasks for
	//    the same pattern.
	var targetProjectID string
	if target.ProjectID != nil {
		targetProjectID = *target.ProjectID
	}
	tasks, err := s.taskRepo.ListTasks(ctx, Query{
		Filters: []FieldFilter{
			{Field: "track_id", Operator: OpEq, Value: ref.TrackID},
			{Field: "title", Operator: OpEq, Value: title},
			{Field: "project_id", Operator: OpEq, Value: targetProjectID},
		},
		AllProjects: true,
	})
	if err != nil {
		return "", fmt.Errorf(
			"cross-track blocked-by %q: query failed; %w",
			fmt.Sprintf("%s#%d", ref.TrackID, ref.TaskNum), err,
		)
	}
	if len(tasks) == 0 {
		return "", fmt.Errorf(
			"cross-track blocked-by %q: no task titled %q found in "+
				"track %q; ingest its plan first",
			fmt.Sprintf("%s#%d", ref.TrackID, ref.TaskNum),
			title, ref.TrackID,
		)
	}
	if len(tasks) > 1 {
		return "", fmt.Errorf(
			"cross-track blocked-by %q: %d tasks titled %q in track "+
				"%q; titles must be unique within a track",
			fmt.Sprintf("%s#%d", ref.TrackID, ref.TaskNum),
			len(tasks), title, ref.TrackID,
		)
	}
	return tasks[0].ID, nil
}

// extractPlansFromMeta pulls the "plans" slice out of a track's
// Meta map, accepting both []string and []any storage shapes.
func extractPlansFromMeta(meta map[string]any) []string {
	if meta == nil {
		return nil
	}
	raw, ok := meta["plans"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
