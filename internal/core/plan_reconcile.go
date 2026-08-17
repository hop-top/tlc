package core

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ReconcileResult describes the outcome of a plan reconciliation.
type ReconcileResult struct {
	Created   []string // newly created task IDs
	Updated   []string // updated task IDs
	Unchanged []string // unchanged task IDs
	Deleted   []string // deleted task IDs (were TODO)
	Kept      []string // kept task IDs (non-TODO, removed from plan)
	// DeferredCrossProject lists cross-project blocked-by refs that
	// parsed but could not be resolved during this pass. They are
	// persisted under meta["blocked_by_cross_project"]; reporting them
	// lets the caller warn instead of implying nothing happened.
	DeferredCrossProject []UnresolvedEntry
}

// reconcileCtx bundles shared state for a single reconciliation pass.
type reconcileCtx struct {
	svc             *TrackService
	ctx             context.Context
	trackID         string
	projectID       string
	idGen           IDGenerator
	now             time.Time
	existing        map[string]*Task
	titleIdx        map[string]string
	duplicateTitles map[string]bool
	// specTitles is the set of titles present in the incoming spec
	// list. Used by the index fallback to avoid stealing a task whose
	// title is explicitly claimed by another spec elsewhere (the
	// "inserted in the middle" case).
	specTitles map[string]bool
	oldMap     map[int]string
	matched    map[string]bool
	newMap     map[int]string
	result     *ReconcileResult
}

// ReconcileTasksFromPlan reconciles an existing plan mapping with
// updated specs. Creates new, updates changed, skips unchanged,
// deletes removed TODO tasks, keeps non-TODO removed tasks.
func (s *TrackService) ReconcileTasksFromPlan(
	ctx context.Context,
	trackID string,
	specs []PlanTaskSpec,
	projectID string,
	idGen IDGenerator,
	existingMapping map[int]string,
) (*ReconcileResult, error) {
	rc := &reconcileCtx{
		svc:        s,
		ctx:        ctx,
		trackID:    trackID,
		projectID:  projectID,
		idGen:      idGen,
		now:        time.Now().UTC(),
		oldMap:     existingMapping,
		matched:    make(map[string]bool),
		newMap:     make(map[int]string, len(specs)),
		result:     &ReconcileResult{},
		specTitles: make(map[string]bool, len(specs)),
	}
	for _, sp := range specs {
		rc.specTitles[sp.Title] = true
	}

	if err := rc.loadExistingTasks(); err != nil {
		return nil, err
	}

	for i, spec := range specs {
		if err := rc.processSpec(i, spec); err != nil {
			return nil, err
		}
	}

	if err := rc.handleRemovedTasks(); err != nil {
		return nil, err
	}

	deferred, err := s.resolveBlockedByFromMapping(ctx, specs, rc.newMap)
	if err != nil {
		return nil, fmt.Errorf("reconcile: resolve blocked-by: %w", err)
	}
	rc.result.DeferredCrossProject = deferred

	if err := s.UpdateTrack(ctx, trackID, func(t *Track) error {
		t.PlanMapping = rc.newMap
		return nil
	}); err != nil {
		return nil, fmt.Errorf("reconcile: persist mapping: %w", err)
	}

	return rc.result, nil
}

func (rc *reconcileCtx) loadExistingTasks() error {
	rc.existing = make(map[string]*Task, len(rc.oldMap))
	rc.titleIdx = make(map[string]string, len(rc.oldMap))
	rc.duplicateTitles = make(map[string]bool)

	for _, taskID := range rc.oldMap {
		t, err := rc.svc.taskRepo.GetTask(rc.ctx, taskID)
		if err != nil {
			return fmt.Errorf("reconcile: get task %s: %w", taskID, err)
		}
		if t == nil {
			continue
		}
		rc.existing[taskID] = t
		if _, dup := rc.titleIdx[t.Title]; dup {
			// Multiple mapped tasks share this title; remove from
			// titleIdx so those tasks fall back to index matching.
			rc.duplicateTitles[t.Title] = true
			delete(rc.titleIdx, t.Title)
		} else if !rc.duplicateTitles[t.Title] {
			rc.titleIdx[t.Title] = taskID
		}
	}
	return nil
}

// matchSpec finds an existing task for a plan spec. Returns ""
// when no match is found. An existing task can only be claimed once
// per reconciliation pass; subsequent specs that resolve to the same
// task fall through to creating a new task. This prevents two plan
// entries with overlapping match keys from collapsing onto a single
// task row, and prevents an insertion-in-the-middle from stealing
// the displaced task via the index fallback.
func (rc *reconcileCtx) matchSpec(idx int, spec PlanTaskSpec) string {
	// Title match first (handles reorders). Skip if already claimed
	// by an earlier spec — subsequent specs with the same title fall
	// through and end up as their own new task row.
	if id, ok := rc.titleIdx[spec.Title]; ok && !rc.matched[id] {
		return id
	}
	// Index fallback (handles renames + pre-existing duplicate titles).
	// Block this fallback only when the candidate task's title is being
	// claimed by another spec via title match — i.e., the title is in
	// the new plan AND the existing task is not a pre-existing
	// duplicate (since duplicates are deliberately removed from
	// titleIdx and must match by position).
	if id, ok := rc.oldMap[idx]; ok && !rc.matched[id] {
		if existingTask, exists := rc.existing[id]; exists {
			titleClaimedByTitleMatch := rc.specTitles[existingTask.Title] &&
				!rc.duplicateTitles[existingTask.Title]
			if !titleClaimedByTitleMatch {
				return id
			}
		}
	}
	return ""
}

func (rc *reconcileCtx) processSpec(idx int, spec PlanTaskSpec) error {
	taskID := rc.matchSpec(idx, spec)
	if taskID != "" {
		return rc.updateExisting(idx, taskID, spec)
	}
	return rc.createNew(idx, spec)
}

func (rc *reconcileCtx) updateExisting(
	idx int, taskID string, spec PlanTaskSpec,
) error {
	rc.matched[taskID] = true
	task := rc.existing[taskID]

	if needsUpdate(task, spec) {
		applySpecToTask(task, spec, rc.now)
		if err := rc.svc.taskRepo.UpdateTask(rc.ctx, task); err != nil {
			return fmt.Errorf("reconcile: update task %s: %w", taskID, err)
		}
		rc.result.Updated = append(rc.result.Updated, taskID)
	} else {
		rc.result.Unchanged = append(rc.result.Unchanged, taskID)
	}
	rc.newMap[idx] = taskID
	return nil
}

func (rc *reconcileCtx) createNew(idx int, spec PlanTaskSpec) error {
	seq, err := rc.idGen.GetNextSequenceID(rc.ctx, rc.projectID)
	if err != nil {
		return fmt.Errorf(
			"reconcile: task %d (%q): seq allocation: %w",
			idx, spec.Title, err,
		)
	}
	taskID := NewTaskID()

	task := taskFromSpec(spec, taskID, rc.trackID, rc.projectID, rc.now)
	task.Seq = int64(seq)
	if err := rc.svc.taskRepo.CreateTask(rc.ctx, task); err != nil {
		return fmt.Errorf(
			"reconcile: task %d (%q): create: %w",
			idx, spec.Title, err,
		)
	}
	rc.result.Created = append(rc.result.Created, taskID)
	rc.newMap[idx] = taskID
	return nil
}

func (rc *reconcileCtx) handleRemovedTasks() error {
	for _, taskID := range rc.oldMap {
		if rc.matched[taskID] {
			continue
		}
		task, ok := rc.existing[taskID]
		if !ok {
			continue
		}
		if task.Status == StatusTodo {
			if err := rc.svc.taskRepo.DeleteTask(rc.ctx, taskID); err != nil {
				return fmt.Errorf("reconcile: delete task %s: %w", taskID, err)
			}
			rc.result.Deleted = append(rc.result.Deleted, taskID)
		} else {
			rc.result.Kept = append(rc.result.Kept, taskID)
		}
	}
	return nil
}

// taskFromSpec builds a new Task from a PlanTaskSpec.
func taskFromSpec(
	spec PlanTaskSpec, taskID, trackID, projectID string, now time.Time,
) *Task {
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

	return &Task{
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
		Meta:        make(map[string]any),
	}
}

// needsUpdate checks if a task's mutable fields differ from the spec.
func needsUpdate(task *Task, spec PlanTaskSpec) bool {
	if task.Title != spec.Title {
		return true
	}
	if strings.TrimSpace(task.Description) != strings.TrimSpace(spec.Description) {
		return true
	}
	if task.Effort != Effort(spec.Effort) {
		return true
	}
	if task.Priority != Priority(spec.Priority) {
		return true
	}
	if !assigneeEqual(task.AssignedTo, spec.AssignedTo) {
		return true
	}
	return !tagsEqual(task.Tags, spec.Tags)
}

// assigneeEqual compares the task's assignee with the spec's.
func assigneeEqual(taskAssignee *string, specAssignee string) bool {
	normalized := specAssignee
	if len(normalized) > 1 && normalized[0] == '@' {
		normalized = normalized[1:]
	}
	current := ""
	if taskAssignee != nil {
		current = *taskAssignee
	}
	return current == normalized
}

// applySpecToTask updates a task's mutable fields from a plan spec.
func applySpecToTask(task *Task, spec PlanTaskSpec, now time.Time) {
	task.Title = spec.Title
	task.Description = strings.TrimSpace(spec.Description)
	task.Effort = Effort(spec.Effort)
	task.Priority = Priority(spec.Priority)
	task.Tags = spec.Tags
	if spec.AssignedTo != "" {
		a := spec.AssignedTo
		if len(a) > 1 && a[0] == '@' {
			a = a[1:]
		}
		task.AssignedTo = &a
	} else {
		task.AssignedTo = nil
	}
	task.UpdatedAt = now
}

// tagsEqual compares two tag slices for equality.
func tagsEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// resolveBlockedByFromMapping re-resolves intra-track blocked-by
// refs for tasks in the mapping based on the current plan specs.
func (s *TrackService) resolveBlockedByFromMapping(
	ctx context.Context,
	specs []PlanTaskSpec,
	mapping map[int]string,
) ([]UnresolvedEntry, error) {
	var deferred []UnresolvedEntry
	for i, spec := range specs {
		taskID, ok := mapping[i]
		if !ok {
			continue
		}
		// Always resolve — when spec.BlockedBy is empty this retracts
		// the edges a previous ingest of this plan authored, while
		// leaving out-of-band edges in place.
		crossProject, err := s.resolveOneTaskBlockedBy(
			ctx, taskID, spec.BlockedBy, mapping,
		)
		if err != nil {
			return nil, err
		}
		for _, ref := range crossProject {
			deferred = append(deferred, UnresolvedEntry{
				TaskID: taskID, Ref: ref,
			})
		}
	}
	return deferred, nil
}

func (s *TrackService) resolveOneTaskBlockedBy(
	ctx context.Context,
	taskID string,
	refs []BlockedByRef,
	mapping map[int]string,
) ([]string, error) {
	task, err := s.taskRepo.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get task %s for blocked-by: %w", taskID, err)
	}
	if task == nil {
		return nil, nil // task may have been deleted; skip
	}

	blockedBy, unresolved, crossProject, err := s.resolveRefList(
		ctx, taskID, refs, mapping,
	)
	if err != nil {
		return nil, err
	}

	if task.Meta == nil {
		task.Meta = make(map[string]any)
	}
	task.SetBlockedBy(mergePlanBlockedBy(task, blockedBy))
	setStringSliceMeta(task, metaKeyPlanOwned, blockedBy)
	setStringSliceMeta(task, metaKeyUnresolved, unresolved)
	setStringSliceMeta(task, metaKeyCrossProject, crossProject)
	task.UpdatedAt = time.Now().UTC()
	if err := s.taskRepo.UpdateTask(ctx, task); err != nil {
		return nil, fmt.Errorf("update blocked-by for %s: %w", taskID, err)
	}
	return crossProject, nil
}

// mergePlanBlockedBy computes a task's new blocked_by set for a plan
// ingest that declared planEdges.
//
// The plan owns only the edges a previous ingest recorded under
// metaKeyPlanOwned. Those are dropped and replaced by planEdges; every
// other current edge is out-of-band (added via `task update
// --add-blocked-by` or a sync plugin) and is preserved untouched.
//
// Ordering keeps the surviving manual edges first so a plan re-ingest
// does not churn unrelated entries.
func mergePlanBlockedBy(task *Task, planEdges []string) []string {
	priorPlanOwned := make(map[string]struct{})
	for _, id := range NormalizeStringSliceMeta(task.Meta[metaKeyPlanOwned]) {
		priorPlanOwned[id] = struct{}{}
	}

	merged := make([]string, 0, len(planEdges))
	for _, id := range task.BlockedBy() {
		if _, planAuthored := priorPlanOwned[id]; planAuthored {
			continue // retractable — re-added below only if still declared
		}
		merged = append(merged, id)
	}
	return append(merged, planEdges...)
}

// setStringSliceMeta stores values under key, removing the key when the
// slice is empty so absent provenance stays absent rather than becoming
// an empty list.
func setStringSliceMeta(task *Task, key string, values []string) {
	normalized := NormalizeStringSliceMeta(values)
	if len(normalized) == 0 {
		delete(task.Meta, key)
		return
	}
	task.Meta[key] = normalized
}

// resolveRefList resolves a slice of BlockedByRef into concrete IDs,
// deferred cross-track raw strings, and deferred cross-project raw
// strings. Every ref lands in exactly one bucket so none can be lost.
func (s *TrackService) resolveRefList(
	ctx context.Context,
	taskID string,
	refs []BlockedByRef,
	mapping map[int]string,
) (blockedBy, unresolved, crossProject []string, err error) {
	for _, ref := range refs {
		switch {
		case ref.IsIndex():
			if depID, found := mapping[ref.Index]; found {
				blockedBy = append(blockedBy, depID)
			}
		case ref.TaskID != "":
			id, rErr := s.resolveTaskIDRef(ctx, ref.TaskID)
			if rErr != nil {
				return nil, nil, nil, fmt.Errorf(
					"task %s: %w", taskID, rErr,
				)
			}
			blockedBy = append(blockedBy, id)
		case ref.CrossTrack != nil:
			id, deferred, rErr := s.resolveCrossTrackRef(
				ctx, ref.CrossTrack,
			)
			if rErr != nil {
				return nil, nil, nil, fmt.Errorf(
					"cross-track ref %s for task %s: %w",
					ref.Raw(), taskID, rErr,
				)
			}
			if deferred {
				unresolved = append(unresolved, ref.Raw())
			} else {
				blockedBy = append(blockedBy, id)
			}
		case ref.CrossProject != nil:
			// Resolution needs the external project's DB, which is not
			// available here. Record the ref as deferred rather than
			// letting it fall through the switch — a ref that parses
			// must never vanish without a trace.
			crossProject = append(crossProject, ref.Raw())
		}
	}
	return blockedBy, unresolved, crossProject, nil
}
