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

	if err := s.resolveBlockedByFromMapping(ctx, specs, rc.newMap); err != nil {
		return nil, fmt.Errorf("reconcile: resolve blocked-by: %w", err)
	}

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
	// Index fallback (handles renames). Only safe when the candidate
	// task's title isn't being explicitly claimed by another spec via
	// title match; otherwise we'd merge an inserted/renamed spec onto
	// a task that another spec will rightfully claim.
	if id, ok := rc.oldMap[idx]; ok && !rc.matched[id] {
		if existingTask, exists := rc.existing[id]; exists {
			if !rc.specTitles[existingTask.Title] {
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
) error {
	for i, spec := range specs {
		taskID, ok := mapping[i]
		if !ok {
			continue
		}
		// Always resolve — when spec.BlockedBy is empty, this clears
		// any stale blocked_by on the task.
		if err := s.resolveOneTaskBlockedBy(ctx, taskID, spec.BlockedBy, mapping); err != nil {
			return err
		}
	}
	return nil
}

func (s *TrackService) resolveOneTaskBlockedBy(
	ctx context.Context,
	taskID string,
	refs []BlockedByRef,
	mapping map[int]string,
) error {
	task, err := s.taskRepo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task %s for blocked-by: %w", taskID, err)
	}
	if task == nil {
		return nil // task may have been deleted; skip
	}

	blockedBy, unresolved, err := s.resolveRefList(ctx, taskID, refs, mapping)
	if err != nil {
		return err
	}

	if task.Meta == nil {
		task.Meta = make(map[string]any)
	}
	if len(blockedBy) > 0 {
		task.Meta["blocked_by"] = blockedBy
	} else {
		delete(task.Meta, "blocked_by")
	}
	if len(unresolved) > 0 {
		task.Meta[metaKeyUnresolved] = unresolved
	} else {
		delete(task.Meta, metaKeyUnresolved)
	}
	task.UpdatedAt = time.Now().UTC()
	if err := s.taskRepo.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("update blocked-by for %s: %w", taskID, err)
	}
	return nil
}

// resolveRefList resolves a slice of BlockedByRef into concrete IDs
// and deferred (unresolved) raw strings.
func (s *TrackService) resolveRefList(
	ctx context.Context,
	taskID string,
	refs []BlockedByRef,
	mapping map[int]string,
) (blockedBy, unresolved []string, err error) {
	for _, ref := range refs {
		switch {
		case ref.IsIndex():
			if depID, found := mapping[ref.Index]; found {
				blockedBy = append(blockedBy, depID)
			}
		case ref.TaskID != "":
			blockedBy = append(blockedBy, ref.TaskID)
		case ref.CrossTrack != nil:
			id, deferred, rErr := s.resolveCrossTrackRef(
				ctx, ref.CrossTrack,
			)
			if rErr != nil {
				return nil, nil, fmt.Errorf(
					"cross-track ref %s for task %s: %w",
					ref.Raw(), taskID, rErr,
				)
			}
			if deferred {
				unresolved = append(unresolved, ref.Raw())
			} else {
				blockedBy = append(blockedBy, id)
			}
		}
	}
	return blockedBy, unresolved, nil
}
