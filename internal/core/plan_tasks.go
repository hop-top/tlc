package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// IDGenerator provides sequential task ID generation.
type IDGenerator interface {
	GetNextSequenceID(ctx context.Context, projectID string) (int, error)
}

// metaKeyUnresolved is the task.Meta key used to persist raw
// cross-track blocked-by strings that could not be resolved
// during phase 1 of plan ingestion. Phase 2 promotes entries out
// of this list into meta["blocked_by"] as they become
// resolvable.
const metaKeyUnresolved = "blocked_by_unresolved"

// metaKeyCrossProject is the task.Meta key for cross-project
// blocked-by refs (e.g. "hop-top/tlc#T-0001"). These are stored
// separately from cross-track unresolved refs so that
// ResolvePendingCrossTrackRefs does not treat them as corrupted.
// A dedicated cross-project resolver can process them later.
const metaKeyCrossProject = "blocked_by_cross_project"

// metaKeyPlanOwned is the task.Meta key recording which
// meta["blocked_by"] entries were authored by plan frontmatter.
//
// Plan ingestion owns exactly the edges it declared: a later run may
// retract those, but must leave edges added out-of-band (e.g. via
// `task update --add-blocked-by`) untouched. Without this provenance
// record the two sources are indistinguishable and a re-ingest silently
// destroys manual dependencies.
const metaKeyPlanOwned = "blocked_by_plan"

// PlanIngestResult describes the outcome of a single
// CreateTasksFromPlan call (phase 1).
type PlanIngestResult struct {
	// CreatedIDs is the task ID for each input spec, in order.
	CreatedIDs []string
	// ResolvedRefs maps raw cross-track ref strings ("track#N") to
	// the resolved T-NNNN task IDs. Only entries resolved during
	// this call appear here; phase 2 reports its own map.
	ResolvedRefs map[string]string
	// UnresolvedRefs lists raw cross-track ref strings that were
	// deferred (target track, its plan, or its specific task did
	// not exist yet). Ingestion still succeeds; the caller should
	// run ResolvePendingCrossTrackRefs afterwards to retry.
	UnresolvedRefs []string
	// MappingErr is non-nil when tasks were created successfully
	// but the plan mapping could not be persisted on the track.
	// The caller receives both the result and the error so it can
	// decide whether to warn or abort.
	MappingErr error `json:"-"`
}

// CreateTasksFromPlan creates tasks from plan specs for trackID
// and resolves each task's blocked-by entries.
//
// Entry forms (BlockedByRef):
//   - int index N        — intra-track, 0-based (existing).
//   - "T-NNNN"           — concrete existing task ID.
//   - "<track-id>#N"     — cross-track, 1-based into target plan.
//
// Hard errors (fail the whole ingest, nothing is created):
//   - intra-track index out of range
//   - concrete "T-NNNN" does not exist
//   - cross-track ref where N > len(target plan tasks)
//   - cross-track ref where target has multiple matching tasks
//
// Soft errors (deferred — task is created with the raw ref
// stored in Meta["blocked_by_unresolved"]):
//   - cross-track ref target track does not exist yet
//   - cross-track ref target track has no linked plan
//   - cross-track ref target task not yet created in DB
//
// After this call, the caller is expected to run
// ResolvePendingCrossTrackRefs for the project to promote any
// refs that became resolvable (two-phase ingestion — see story
// 076).
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

	// --- Pre-flight validation ---------------------------------------
	// Hard-check intra-track indices and concrete T-NNNN refs before
	// touching the DB. Cross-track refs are validated for hard errors
	// (OOR, ambiguity) but not for soft errors; soft refs become
	// deferred entries after tasks are created.
	for i, spec := range specs {
		for _, ref := range spec.BlockedBy {
			switch {
			case ref.IsIndex():
				if ref.Index < 0 || ref.Index >= len(specs) {
					return nil, fmt.Errorf(
						"plan task %d (%q): blocked-by index %d out "+
							"of range (must be 0..%d); fix the plan "+
							"frontmatter",
						i, spec.Title, ref.Index, len(specs)-1,
					)
				}
				if ref.Index == i {
					return nil, fmt.Errorf(
						"plan task %d (%q): blocked-by index %d "+
							"is a self-reference; tasks cannot block "+
							"themselves",
						i, spec.Title, ref.Index,
					)
				}
			case ref.TaskID != "":
				if _, err := s.resolveTaskIDRef(ctx, ref.TaskID); err != nil {
					return nil, fmt.Errorf(
						"plan task %d (%q): %w", i, spec.Title, err,
					)
				}
			case ref.CrossTrack != nil:
				// Hard-error check only. Full resolution happens
				// in the next loop; any soft misses are captured
				// as deferred entries.
				if _, err := s.preflightCrossTrackRef(
					ctx, ref.CrossTrack,
				); err != nil {
					return nil, fmt.Errorf(
						"plan task %d (%q): %w",
						i, spec.Title, err,
					)
				}
			case ref.CrossProject != nil:
				// Cross-project refs are always deferred during
				// phase 1 — resolution requires the external
				// project's DB, which may not be available.
			}
		}
	}

	// --- Pre-allocate IDs and sequences -------------------------------
	// Generate all IDs upfront so forward blocked-by refs (index > i)
	// can be resolved during task creation without an extra pass.
	createdIDs := make([]string, len(specs))
	createdSeqs := make([]int64, len(specs))
	for i := range specs {
		seq, err := idGen.GetNextSequenceID(ctx, projectID)
		if err != nil {
			return nil, fmt.Errorf(
				"plan task %d (%q): failed to allocate sequence; %w",
				i, specs[i].Title, err,
			)
		}
		createdIDs[i] = NewTaskID()
		createdSeqs[i] = int64(seq)
	}

	// --- Create tasks + resolve refs ---------------------------------
	resolved := make(map[string]string)
	unresolved := make([]string, 0)
	now := time.Now().UTC()

	for i, spec := range specs {
		taskID := createdIDs[i]

		var blockedBy []string
		var taskUnresolved []string
		var taskCrossProject []string
		for _, ref := range spec.BlockedBy {
			switch {
			case ref.IsIndex():
				blockedBy = append(blockedBy, createdIDs[ref.Index])
			case ref.TaskID != "":
				id, rErr := s.resolveTaskIDRef(ctx, ref.TaskID)
				if rErr != nil {
					return nil, fmt.Errorf(
						"plan task %d (%q): %w", i, spec.Title, rErr,
					)
				}
				blockedBy = append(blockedBy, id)
			case ref.CrossTrack != nil:
				id, deferred, rErr := s.resolveCrossTrackRef(
					ctx, ref.CrossTrack,
				)
				if rErr != nil {
					// Pre-flight should have caught hard errors,
					// but keep the guard for defensive safety.
					return nil, fmt.Errorf(
						"plan task %d (%q): %w",
						i, spec.Title, rErr,
					)
				}
				if deferred {
					raw := ref.Raw()
					taskUnresolved = append(taskUnresolved, raw)
					unresolved = append(unresolved, raw)
				} else {
					blockedBy = append(blockedBy, id)
					resolved[ref.Raw()] = id
				}
			case ref.CrossProject != nil:
				// Cross-project refs are always deferred; the
				// external project's DB is not available during
				// phase 1 ingestion. Store under a separate meta
				// key so ResolvePendingCrossTrackRefs doesn't
				// treat them as corrupted cross-track entries.
				raw := ref.Raw()
				taskCrossProject = append(taskCrossProject, raw)
				unresolved = append(unresolved, raw)
			}
		}

		meta := make(map[string]any)
		if len(blockedBy) > 0 {
			meta["blocked_by"] = blockedBy
			// Every edge here came from plan frontmatter, so a later
			// re-ingest is free to retract it.
			meta[metaKeyPlanOwned] = blockedBy
		}
		if len(taskUnresolved) > 0 {
			meta[metaKeyUnresolved] = taskUnresolved
		}
		if len(taskCrossProject) > 0 {
			meta[metaKeyCrossProject] = taskCrossProject
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
			Seq:         createdSeqs[i],
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
	}

	// Persist plan mapping on the track: index → task ID.
	mapping := make(map[int]string, len(createdIDs))
	for i, id := range createdIDs {
		mapping[i] = id
	}

	result := &PlanIngestResult{
		CreatedIDs:     createdIDs,
		ResolvedRefs:   resolved,
		UnresolvedRefs: unresolved,
	}

	if err := s.UpdateTrack(ctx, trackID, func(t *Track) error {
		t.PlanMapping = mapping
		return nil
	}); err != nil {
		// Tasks were created successfully but mapping persistence
		// failed. Return the result so the caller can see what was
		// created, alongside a non-nil error.
		result.MappingErr = fmt.Errorf("persist plan mapping: %w", err)
		return result, result.MappingErr
	}

	return result, nil
}

// resolveTaskIDRef resolves a same-project "T-NNNN" blocked-by ref to
// the target task's durable ID.
//
// Stored blocked_by entries are durable task IDs, so persisting the raw
// display alias would leave a dangling edge that resolves to nothing.
// The repository accepts either form on lookup; only its answer's ID is
// authoritative.
func (s *TrackService) resolveTaskIDRef(
	ctx context.Context,
	ref string,
) (string, error) {
	t, err := s.taskRepo.GetTask(ctx, ref)
	if err != nil || t == nil {
		return "", fmt.Errorf(
			"blocked-by references task %q which does not exist; "+
				"create it first or use an intra-track index", ref,
		)
	}
	if t.ID == "" {
		return ref, nil
	}
	return t.ID, nil
}

// preflightCrossTrackRef validates a cross-track ref for *hard*
// errors only — index out of range against the target plan's
// frontmatter, DB query failure, or multiple DB matches for the
// target title. Soft conditions (target track missing, target
// plan missing, target task missing) return nil; they become
// deferred refs.
//
// Preflight runs the same DB lookup used by resolveCrossTrackRef
// so the "hard errors fail the whole ingest, nothing is created"
// contract holds: ambiguity and query failures are caught before
// any tasks are created.
func (s *TrackService) preflightCrossTrackRef(
	ctx context.Context,
	ref *CrossTrackRef,
) (softMiss bool, err error) {
	target, err := s.repo.GetTrack(ctx, ref.TrackID)
	if err != nil || target == nil {
		return true, nil // deferred
	}
	plans := extractPlansFromMeta(target.Meta)
	if len(plans) == 0 {
		return true, nil // deferred
	}
	fm, fmErr := ParsePlanFrontmatter(plans[0])
	if fmErr != nil {
		return false, fmt.Errorf(
			"cross-track blocked-by %q: cannot read target plan %q; %w",
			fmt.Sprintf("%s#%d", ref.TrackID, ref.TaskNum),
			plans[0], fmErr,
		)
	}
	if fm == nil || len(fm.Tasks) < ref.TaskNum {
		have := 0
		if fm != nil {
			have = len(fm.Tasks)
		}
		return false, fmt.Errorf(
			"cross-track blocked-by %q: out of range; target track "+
				"%q has %d tasks in plan %q",
			fmt.Sprintf("%s#%d", ref.TrackID, ref.TaskNum),
			ref.TrackID, have, plans[0],
		)
	}
	title := fm.Tasks[ref.TaskNum-1].Title

	// Same lookup as resolveCrossTrackRef: scoped by the target
	// track's project_id to prevent cross-project ambiguity.
	var targetProjectID string
	if target.ProjectID != nil {
		targetProjectID = *target.ProjectID
	}
	tasks, qErr := s.taskRepo.ListTasks(ctx, Query{
		Filters: []FieldFilter{
			{Field: "track_id", Operator: OpEq, Value: ref.TrackID},
			{Field: "title", Operator: OpEq, Value: title},
			{Field: "project_id", Operator: OpEq, Value: targetProjectID},
		},
		AllProjects: true,
	})
	if qErr != nil {
		return false, fmt.Errorf(
			"cross-track blocked-by %q: query failed; %w",
			fmt.Sprintf("%s#%d", ref.TrackID, ref.TaskNum), qErr,
		)
	}
	if len(tasks) > 1 {
		return false, fmt.Errorf(
			"cross-track blocked-by %q: %d tasks titled %q in track "+
				"%q; titles must be unique within a track",
			fmt.Sprintf("%s#%d", ref.TrackID, ref.TaskNum),
			len(tasks), title, ref.TrackID,
		)
	}
	// 0 matches → deferred (the target title exists in the plan
	// but no task has been created for it yet — the common
	// circular case).
	return false, nil
}

// resolveCrossTrackRef attempts to resolve a "<track-id>#N"
// reference to a concrete task ID by reading the target track's
// linked plan and looking up tasks[N-1].title in the database.
//
// Return semantics:
//   - (id, false, nil) — resolved.
//   - ("",  true,  nil) — deferred (soft miss: target track,
//     its plan, or its specific task does not exist yet).
//   - ("", false, err) — hard error (out of range, ambiguity,
//     query failure). Caller must treat this as fatal.
func (s *TrackService) resolveCrossTrackRef(
	ctx context.Context,
	ref *CrossTrackRef,
) (string, bool, error) {
	target, err := s.repo.GetTrack(ctx, ref.TrackID)
	if err != nil || target == nil {
		return "", true, nil
	}
	plans := extractPlansFromMeta(target.Meta)
	if len(plans) == 0 {
		return "", true, nil
	}
	fm, fmErr := ParsePlanFrontmatter(plans[0])
	if fmErr != nil {
		return "", false, fmt.Errorf(
			"cross-track blocked-by %q: cannot read target plan %q; %w",
			fmt.Sprintf("%s#%d", ref.TrackID, ref.TaskNum),
			plans[0], fmErr,
		)
	}
	if fm == nil || len(fm.Tasks) < ref.TaskNum {
		have := 0
		if fm != nil {
			have = len(fm.Tasks)
		}
		return "", false, fmt.Errorf(
			"cross-track blocked-by %q: out of range; target track "+
				"%q has %d tasks in plan %q",
			fmt.Sprintf("%s#%d", ref.TrackID, ref.TaskNum),
			ref.TrackID, have, plans[0],
		)
	}
	title := fm.Tasks[ref.TaskNum-1].Title

	var targetProjectID string
	if target.ProjectID != nil {
		targetProjectID = *target.ProjectID
	}
	// Tasks reference the track by its durable TypeID (target.ID), not
	// by the user-typed slug (ref.TrackID). Use target.ID to match the
	// foreign key column and let the user supply slug or typeid in
	// plan refs interchangeably.
	tasks, err := s.taskRepo.ListTasks(ctx, Query{
		Filters: []FieldFilter{
			{Field: "track_id", Operator: OpEq, Value: target.ID},
			{Field: "title", Operator: OpEq, Value: title},
			{Field: "project_id", Operator: OpEq, Value: targetProjectID},
		},
		AllProjects: true,
	})
	if err != nil {
		return "", false, fmt.Errorf(
			"cross-track blocked-by %q: query failed; %w",
			fmt.Sprintf("%s#%d", ref.TrackID, ref.TaskNum), err,
		)
	}
	if len(tasks) == 0 {
		// Deferred: the target title exists in the plan but no
		// task has been created for it yet. This is the common
		// case during circular ingestion — the other side will
		// create the task in its own phase 1.
		return "", true, nil
	}
	if len(tasks) > 1 {
		return "", false, fmt.Errorf(
			"cross-track blocked-by %q: %d tasks titled %q in track "+
				"%q; titles must be unique within a track",
			fmt.Sprintf("%s#%d", ref.TrackID, ref.TaskNum),
			len(tasks), title, ref.TrackID,
		)
	}
	return tasks[0].ID, false, nil
}

// ResolveResult is the outcome of a phase-2 pass.
type ResolveResult struct {
	// PromotedTasks is the number of tasks that had at least one
	// unresolved ref promoted into blocked_by during this pass.
	PromotedTasks int
	// PlansRewritten is the set of plan.md paths that were
	// rewritten on disk because every ref in them is now resolved.
	PlansRewritten []string
	// StillUnresolved lists (taskID, raw-ref) pairs that remained
	// unresolved at the end of the pass.
	StillUnresolved []UnresolvedEntry
}

// UnresolvedEntry names a single still-pending cross-track ref.
type UnresolvedEntry struct {
	TaskID string
	Ref    string
}

// ResolvePendingCrossTrackRefs walks every task in the given
// project with a non-empty Meta["blocked_by_unresolved"] list and
// tries to resolve each entry against the current DB state. Newly
// resolvable refs are promoted into Meta["blocked_by"] and
// dropped from the unresolved list. Any plan.md whose tasks all
// have every ref resolved is rewritten on disk (frontmatter-only).
//
// The seedRefs map lets the caller feed in refs that were already
// resolved during the current phase-1 pass so that they get
// written into the plan file alongside refs promoted in phase 2.
// The map is keyed by plan path; each inner map is raw-ref ->
// resolved T-NNNN. seedRefs may be nil.
//
// Safe to call repeatedly; a pass that resolves nothing is a
// no-op. Called from track_update after every --add-plan.
func (s *TrackService) ResolvePendingCrossTrackRefs(
	ctx context.Context,
	projectID string,
	seedRefs map[string]map[string]string,
) (*ResolveResult, error) {
	res := &ResolveResult{}

	// Load every task in the project. Unresolved refs live in
	// Meta so the filter cannot be pushed into the query; a full
	// project scan is fine because ingestion is interactive.
	// Always scope to the requested project bucket, including the
	// empty-string sentinel — otherwise a phase 2 pass with
	// projectID=="" would rewrite plans from other projects.
	q := Query{
		AllProjects: true,
		Filters: []FieldFilter{
			{Field: "project_id", Operator: OpEq, Value: projectID},
		},
	}
	tasks, err := s.taskRepo.ListTasks(ctx, q)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve pending refs: list tasks: %w", err,
		)
	}

	// plansToCheck collects the plan paths whose owning tracks
	// had at least one task touched during this pass, so we can
	// test each one for "all refs now resolved" afterwards.
	plansToCheck := make(map[string]struct{})
	// refMapByPlan accumulates raw->resolved substitutions per
	// plan, for the rewrite step. Seeded with the caller-provided
	// phase-1 refs.
	refMapByPlan := make(map[string]map[string]string)
	for plan, m := range seedRefs {
		if len(m) == 0 {
			continue
		}
		plansToCheck[plan] = struct{}{}
		cp := make(map[string]string, len(m))
		for k, v := range m {
			cp[k] = v
		}
		refMapByPlan[plan] = cp
	}

	for _, task := range tasks {
		raw, ok := task.Meta[metaKeyUnresolved]
		if !ok {
			continue
		}
		pending := toStringSlice(raw)
		if len(pending) == 0 {
			continue
		}

		stillPending := make([]string, 0, len(pending))
		newlyResolved := make(map[string]string)
		for _, refStr := range pending {
			parsed, pErr := parseBlockedByRef(refStr)
			if pErr != nil || parsed.CrossTrack == nil {
				// Corrupted unresolved entry; keep as-is so a
				// human can inspect it, and report it as still
				// unresolved so plan-rewrite gating stays
				// conservative.
				stillPending = append(stillPending, refStr)
				res.StillUnresolved = append(
					res.StillUnresolved,
					UnresolvedEntry{TaskID: task.ID, Ref: refStr},
				)
				continue
			}
			id, deferred, rErr := s.resolveCrossTrackRef(
				ctx, parsed.CrossTrack,
			)
			if rErr != nil {
				return nil, fmt.Errorf(
					"resolve pending refs: task %s: %w",
					task.ID, rErr,
				)
			}
			if deferred {
				stillPending = append(stillPending, refStr)
				res.StillUnresolved = append(
					res.StillUnresolved,
					UnresolvedEntry{TaskID: task.ID, Ref: refStr},
				)
				continue
			}
			newlyResolved[refStr] = id
		}

		if len(newlyResolved) == 0 {
			continue
		}

		// Promote newly resolved entries into blocked_by. They
		// originate from plan frontmatter, so mark them plan-owned
		// and retractable by a later ingest.
		combined := append([]string{}, task.BlockedBy()...)
		owned := NormalizeStringSliceMeta(task.Meta[metaKeyPlanOwned])
		for _, v := range newlyResolved {
			combined = append(combined, v)
			owned = append(owned, v)
		}
		task.SetBlockedBy(combined)
		setStringSliceMeta(task, metaKeyPlanOwned, owned)

		// Update the unresolved meta entry.
		if len(stillPending) == 0 {
			delete(task.Meta, metaKeyUnresolved)
		} else {
			task.Meta[metaKeyUnresolved] = stillPending
		}
		task.UpdatedAt = time.Now().UTC()
		if err := s.taskRepo.UpdateTask(ctx, task); err != nil {
			return nil, fmt.Errorf(
				"resolve pending refs: update task %s: %w",
				task.ID, err,
			)
		}
		res.PromotedTasks++

		// Find the plan(s) owning this task so we can maybe
		// rewrite them later.
		if task.TrackID != nil {
			owner, gErr := s.repo.GetTrack(ctx, *task.TrackID)
			if gErr != nil || owner == nil {
				continue
			}
			for _, p := range extractPlansFromMeta(owner.Meta) {
				plansToCheck[p] = struct{}{}
				if refMapByPlan[p] == nil {
					refMapByPlan[p] = make(map[string]string)
				}
				for k, v := range newlyResolved {
					refMapByPlan[p][k] = v
				}
			}
		}
	}

	// Rewrite plans whose tasks have no remaining unresolved
	// refs. For each candidate plan, check the plan's owning
	// track's tasks — if none still carry unresolved refs, the
	// plan is safe to rewrite.
	stillByTrack := make(map[string]bool)
	for _, u := range res.StillUnresolved {
		// Find the track for this task to mark it dirty.
		t, _ := s.taskRepo.GetTask(ctx, u.TaskID) //nolint:errcheck // best-effort track lookup
		if t != nil && t.TrackID != nil {
			stillByTrack[*t.TrackID] = true
		}
	}
	// Map plan -> owning track ids to filter.
	planOwners := make(map[string][]string)
	// Expensive but bounded: list all tracks in the project and
	// index by their plans.
	tq := TrackQuery{AllProjects: true}
	if projectID != "" {
		tq.ProjectID = &projectID
	}
	tracks, trErr := s.repo.ListTracks(ctx, tq)
	if trErr != nil {
		return nil, fmt.Errorf(
			"resolve pending refs: list tracks: %w", trErr,
		)
	}
	for _, tr := range tracks {
		for _, p := range extractPlansFromMeta(tr.Meta) {
			planOwners[p] = append(planOwners[p], tr.ID)
		}
	}

	var sortedPlans []string
	for p := range plansToCheck {
		sortedPlans = append(sortedPlans, p)
	}
	sort.Strings(sortedPlans)

	for _, p := range sortedPlans {
		dirty := false
		for _, owner := range planOwners[p] {
			if stillByTrack[owner] {
				dirty = true
				break
			}
		}
		if dirty {
			continue
		}
		if err := RewritePlanBlockedByRefs(p, refMapByPlan[p]); err != nil {
			return nil, fmt.Errorf(
				"resolve pending refs: rewrite %q: %w", p, err,
			)
		}
		res.PlansRewritten = append(res.PlansRewritten, p)
	}
	return res, nil
}

// toStringSlice coerces a meta value into a []string, accepting
// both []string and []any (JSON) shapes.
func toStringSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				out = append(out, str)
			}
		}
		return out
	default:
		return nil
	}
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
	return toStringSlice(raw)
}
