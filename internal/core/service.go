package core

import (
	"context"
	"fmt"
	"time"

	"hop.top/kit/go/runtime/domain"
)

// TaskService provides business logic for task lifecycle management.
// When a domain.Service is configured (via WithDomainRepo), write
// operations (Create, Update) delegate to it for validation, auditing,
// and event publishing. Reads (Get, List) use the legacy repo for
// rich filtering. Custom orchestration methods (Claim, Unclaim, etc.)
// remain as wrappers using the underlying repo directly.
type TaskService struct {
	repo          Repository
	logRepo       LogRepository
	domainSvc     *domain.Service[Task]
	flowDomainSvc *domain.Service[FlowRun]
}

// TaskServiceOption configures a TaskService.
type TaskServiceOption func(*TaskService)

// WithDomainRepo wires a domain.Repository[Task] to enable
// kit/domain CRUD delegation with optional audit/validation/events.
func WithDomainRepo(
	dr domain.Repository[Task],
	opts ...domain.Option[Task],
) TaskServiceOption {
	return func(s *TaskService) {
		s.domainSvc = domain.NewService[Task](dr, opts...)
	}
}

// WithFlowDomainRepo wires a domain.Repository[FlowRun] to enable
// kit/domain CRUD delegation for flow run lifecycle.
func WithFlowDomainRepo(
	dr domain.Repository[FlowRun],
	opts ...domain.Option[FlowRun],
) TaskServiceOption {
	return func(s *TaskService) {
		s.flowDomainSvc = domain.NewService[FlowRun](dr, opts...)
	}
}

func NewTaskService(repo Repository, logRepo LogRepository, opts ...TaskServiceOption) *TaskService {
	s := &TaskService{
		repo:    repo,
		logRepo: logRepo,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// NextTaskID returns the next formatted task ID (e.g. "T-0001") for the given
// project, using the repository's sequence counter. Empty projectID uses the
// global sequence.
func (s *TaskService) NextTaskID(ctx context.Context, projectID string) (string, error) {
	seq, err := s.repo.GetNextSequenceID(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("failed to get next sequence ID: %w", err)
	}
	return FormatTaskSeq(int64(seq)), nil
}

func (s *TaskService) CreateTask(ctx context.Context, task *Task, by string, note string) error {
	if s.domainSvc != nil {
		if err := s.domainSvc.Create(ctx, task); err != nil {
			return fmt.Errorf("failed to create task: %w", err)
		}
	} else {
		if err := s.repo.CreateTask(ctx, task); err != nil {
			return fmt.Errorf("failed to create task: %w", err)
		}
	}

	logEntry := &LogEntry{
		TaskID:    task.ID,
		Timestamp: task.CreatedAt,
		By:        by,
		Action:    ActionCreated,
		Note:      note,
	}

	if err := s.logRepo.AddLog(ctx, logEntry); err != nil {
		return fmt.Errorf("failed to add log: %w", err)
	}
	return nil
}

func (s *TaskService) ListTasks(ctx context.Context, query Query) ([]*Task, error) {
	tasks, err := s.repo.ListTasks(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}
	return tasks, nil
}

func (s *TaskService) ListFlowRuns(ctx context.Context, query Query) ([]*FlowRun, error) {
	runs, err := s.repo.ListFlowRuns(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list flow runs: %w", err)
	}
	return runs, nil
}

func (s *TaskService) ArchiveTasks(ctx context.Context, threshold time.Duration) (int64, error) {
	count, err := s.repo.ArchiveTasks(ctx, threshold)
	if err != nil {
		return 0, fmt.Errorf("failed to archive tasks: %w", err)
	}
	return count, nil
}

func (s *TaskService) UpdateTask(ctx context.Context, task *Task, by string, _ string) error {
	hasOriginSystem := task.OriginSystem != nil && *task.OriginSystem != ""
	var originSystem string
	if hasOriginSystem {
		originSystem = *task.OriginSystem
	} else if task.Meta != nil {
		if os, ok := task.Meta["origin_system"].(string); ok && os != "" {
			hasOriginSystem = true
			originSystem = os
		}
	}

	if hasOriginSystem {
		if task.Meta == nil {
			task.Meta = make(map[string]interface{})
		}
		task.Meta["needs_push"] = true

		logEntry := &LogEntry{
			TaskID:    task.ID,
			Timestamp: time.Now().UTC(),
			By:        by,
			Action:    ActionComment,
			Note:      fmt.Sprintf("Task marked for push to %s", originSystem),
		}
		if err := s.logRepo.AddLog(ctx, logEntry); err != nil {
			return fmt.Errorf("failed to add log: %w", err)
		}
	}

	if s.domainSvc != nil {
		if err := s.domainSvc.Update(ctx, task); err != nil {
			return fmt.Errorf("failed to update task: %w", err)
		}
	} else {
		if err := s.repo.UpdateTask(ctx, task); err != nil {
			return fmt.Errorf("failed to update task: %w", err)
		}
	}
	return nil
}

func (s *TaskService) GetLogs(ctx context.Context, taskID string, sortDirection string) ([]*LogEntry, error) {
	logs, err := s.logRepo.GetLogs(ctx, taskID, sortDirection)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}
	return logs, nil
}

func (s *TaskService) TransitionStatus(ctx context.Context, taskID string, next TaskStatus, by string, note string) error {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}

	wm := DefaultWorkflow()
	logEntry, err := task.TransitionWithWorkflow(next, by, note, wm, false)
	if err != nil {
		return fmt.Errorf("failed to transition task: %w", err)
	}

	if err := s.repo.UpdateTaskWithLog(ctx, task, logEntry); err != nil {
		return fmt.Errorf("failed to update task with log: %w", err)
	}
	return nil
}

func (s *TaskService) ClaimTask(ctx context.Context, taskID string, by string, note string) error {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}

	if task.AssignedTo != nil && *task.AssignedTo != by {
		return fmt.Errorf("task is already claimed by %s", *task.AssignedTo)
	}

	task.AssignedTo = &by

	hasOriginSystem := task.OriginSystem != nil && *task.OriginSystem != ""
	if !hasOriginSystem && task.Meta != nil {
		if originSystem, ok := task.Meta["origin_system"].(string); ok && originSystem != "" {
			hasOriginSystem = true
		}
	}

	if hasOriginSystem {
		if task.Meta == nil {
			task.Meta = make(map[string]interface{})
		}
		task.Meta["needs_push"] = true
	}

	wm := DefaultWorkflow()
	activeStatus, err := wm.StatusForRole("active")
	if err != nil {
		return fmt.Errorf("workflow has no active status: %w", err)
	}
	logEntry, err := task.TransitionWithWorkflow(activeStatus, by, note, wm, false)
	if err != nil {
		return fmt.Errorf("failed to transition task: %w", err)
	}
	logEntry.Action = ActionClaimed

	if err := s.repo.UpdateTaskWithLog(ctx, task, logEntry); err != nil {
		return fmt.Errorf("failed to update task with log: %w", err)
	}
	return nil
}

func (s *TaskService) UnclaimTask(ctx context.Context, taskID string, by string, note string) error {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}

	task.AssignedTo = nil

	hasOriginSystem := task.OriginSystem != nil && *task.OriginSystem != ""
	if !hasOriginSystem && task.Meta != nil {
		if originSystem, ok := task.Meta["origin_system"].(string); ok && originSystem != "" {
			hasOriginSystem = true
		}
	}

	if hasOriginSystem {
		if task.Meta == nil {
			task.Meta = make(map[string]interface{})
		}
		task.Meta["needs_push"] = true
	}

	wm := DefaultWorkflow()
	initialStatus, err := wm.StatusForRole("initial")
	if err != nil {
		return fmt.Errorf("workflow has no initial status: %w", err)
	}
	logEntry, err := task.TransitionWithWorkflow(initialStatus, by, note, wm, false)
	if err != nil {
		return fmt.Errorf("failed to transition task: %w", err)
	}
	logEntry.Action = ActionReleased

	if err := s.repo.UpdateTaskWithLog(ctx, task, logEntry); err != nil {
		return fmt.Errorf("failed to update task with log: %w", err)
	}
	return nil
}

func (s *TaskService) updateFlowRunStatus(ctx context.Context, runID, action, by, note string, status FlowStatus) error {
	var run *FlowRun
	var err error
	if s.flowDomainSvc != nil {
		run, err = s.flowDomainSvc.Get(ctx, runID)
	} else {
		run, err = s.repo.GetFlowRun(ctx, runID)
	}
	if err != nil {
		return fmt.Errorf("failed to get flow run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("flow run %s not found", runID)
	}
	run.Status = status
	if s.flowDomainSvc != nil {
		if err := s.flowDomainSvc.Update(ctx, run); err != nil {
			return fmt.Errorf("failed to update flow run: %w", err)
		}
	} else {
		if err := s.repo.UpdateFlowRun(ctx, run); err != nil {
			return fmt.Errorf("failed to update flow run: %w", err)
		}
	}
	if err := s.logRepo.AddLog(ctx, &LogEntry{
		Timestamp: time.Now().UTC(),
		By:        by,
		Action:    action,
		Note:      note,
		Meta:      map[string]any{"run_id": runID},
	}); err != nil {
		return fmt.Errorf("failed to add log: %w", err)
	}
	return nil
}

func (s *TaskService) PauseFlowRun(ctx context.Context, runID string, by string, note string) error {
	return s.updateFlowRunStatus(ctx, runID, "FLOW_PAUSED", by, note, FlowStatusPaused)
}

func (s *TaskService) ResumeFlowRun(ctx context.Context, runID string, by string, note string) error {
	return s.updateFlowRunStatus(ctx, runID, "FLOW_RESUMED", by, note, FlowStatusRunning)
}

func (s *TaskService) CancelFlowRun(ctx context.Context, runID string, by string, note string) error {
	return s.updateFlowRunStatus(ctx, runID, "FLOW_CANCELED", by, note, FlowStatusCanceled)
}

// CreateTaskWithAssignment creates a task and auto-assigns it to the best-matching assignee.
func (s *TaskService) CreateTaskWithAssignment(ctx context.Context, task *Task, engine *AssignmentEngine, by string, note string) error {
	// Create task first
	if err := s.CreateTask(ctx, task, by, note); err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}

	// Find best assignee
	assignee, score, err := engine.FindBestAssignee(task)
	if err != nil {
		// Log warning but don't fail task creation
		logEntry := &LogEntry{
			TaskID:    task.ID,
			Timestamp: time.Now().UTC(),
			By:        "assignment-engine",
			Action:    ActionComment,
			Note:      fmt.Sprintf("Warning: could not auto-assign task: %v", err),
		}
		if logErr := s.logRepo.AddLog(ctx, logEntry); logErr != nil {
			return fmt.Errorf("failed to write assignment warning log: %w", logErr)
		}
		return nil
	}

	// Assign task
	assigneeID := assignee.ID
	task.AssignedTo = &assigneeID

	// Log assignment
	logEntry := &LogEntry{
		TaskID:    task.ID,
		Timestamp: time.Now().UTC(),
		By:        "assignment-engine",
		Action:    ActionAutoAssigned,
		Note:      fmt.Sprintf("Auto-assigned to %s (match score: %.1f)", assignee.Name, score),
		Meta: map[string]any{
			"assignee_id": assignee.ID,
			"score":       score,
		},
	}

	if err := s.repo.UpdateTaskWithLog(ctx, task, logEntry); err != nil {
		return fmt.Errorf("failed to update task with log: %w", err)
	}

	return nil
}

// DelegateTask delegates a task from one assignee to another based on delegation rules.
func (s *TaskService) DelegateTask(ctx context.Context, taskID string, fromAssignee *Assignee, reason, by, _ string) error {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}

	// Check if delegation is allowed
	newAssigneeID, canDelegate := fromAssignee.CanDelegate(reason)
	if !canDelegate {
		return fmt.Errorf("no delegation rule for reason: %s", reason)
	}

	// Update task assignment
	oldAssignee := "unassigned"
	if task.AssignedTo != nil {
		oldAssignee = *task.AssignedTo
	}
	task.AssignedTo = &newAssigneeID
	wm := DefaultWorkflow()
	initialStatus, err := wm.StatusForRole("initial")
	if err != nil {
		return fmt.Errorf("workflow has no initial status: %w", err)
	}
	task.Status = initialStatus
	task.UpdatedAt = time.Now().UTC()

	// Create log entry
	logEntry := &LogEntry{
		TaskID:    task.ID,
		Timestamp: task.UpdatedAt,
		By:        by,
		Action:    ActionDelegated,
		Note:      fmt.Sprintf("Delegated from %s to %s: %s", oldAssignee, newAssigneeID, reason),
		Meta: map[string]any{
			"from_assignee": oldAssignee,
			"to_assignee":   newAssigneeID,
			"reason":        reason,
		},
	}

	if err := s.repo.UpdateTaskWithLog(ctx, task, logEntry); err != nil {
		return fmt.Errorf("failed to update task with log: %w", err)
	}
	return nil
}

// UnblockTasks finds and unblocks tasks that were blocked by the completed task.
func (s *TaskService) UnblockTasks(ctx context.Context, completedTaskID string, assignee *Assignee, by string) error {
	unblockedTypes := assignee.CheckUnblocks(nil)
	if len(unblockedTypes) == 0 {
		return nil
	}

	// Find tasks that were blocked by this task
	// For now, we'll use metadata to track blockers
	allTasks, err := s.repo.ListTasks(ctx, Query{})
	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	for _, task := range allTasks {
		if task.Meta == nil {
			continue
		}

		// Check if this task is blocked by the completed task
		blockedBy := task.BlockedBy()
		if !contains(blockedBy, completedTaskID) {
			continue
		}

		// Check if the task type is in the unblocks list
		taskType := extractTaskType(task)
		if !contains(unblockedTypes, taskType) {
			continue
		}

		task.UpdatedAt = time.Now().UTC()
		task.RemoveBlockedBy([]string{completedTaskID})
		if len(task.BlockedBy()) > 0 {
			if err := s.repo.UpdateTask(ctx, task); err != nil {
				return fmt.Errorf("failed to update task blockers: %w", err)
			}
			continue
		}

		logEntry := &LogEntry{
			TaskID:    task.ID,
			Timestamp: task.UpdatedAt,
			By:        by,
			Action:    ActionUnblocked,
			Note:      fmt.Sprintf("Unblocked by completion of %s", completedTaskID),
			Meta: map[string]any{
				"unblocked_by": completedTaskID,
			},
		}

		if err := s.repo.UpdateTaskWithLog(ctx, task, logEntry); err != nil {
			return fmt.Errorf("failed to update task with log: %w", err)
		}
	}

	return nil
}

// extractTaskType extracts the task type from task metadata or requirements.
func extractTaskType(task *Task) string {
	if task.Meta == nil {
		return ""
	}

	requirements := extractTaskRequirements(task)
	if requirements == nil || len(requirements.Capabilities) == 0 {
		return ""
	}

	// Return the first capability as the task type
	return requirements.Capabilities[0]
}

const (
	// Task CRUD Actions.
	ActionCreated = "CREATED"
	ActionUpdated = "UPDATED"
	ActionDeleted = "DELETED"

	// Task Status Actions.
	ActionClaimed    = "CLAIMED"
	ActionReleased   = "RELEASED"
	ActionReassigned = "REASSIGNED"
	ActionDone       = "DONE"
	ActionSkipped    = "SKIPPED"

	// Recipe Execution Actions: human-gate decisions and a claim taken
	// over from a stale actor. Retries reuse ActionRetry below.
	ActionApproved  = "APPROVED"
	ActionRejected  = "REJECTED"
	ActionReclaimed = "RECLAIMED"

	// Task Execution Actions.
	ActionExecStart   = "EXEC_START"
	ActionExecEnd     = "EXEC_END"
	ActionExecAttempt = "EXEC_ATTEMPT"

	// Collaboration Actions.
	ActionComment  = "COMMENT"
	ActionBlocked  = "BLOCKED"
	ActionSplit    = "SPLIT"
	ActionMerged   = "MERGED"
	ActionMigrated = "MIGRATED"

	// Failure and Retry Actions.
	ActionFailure = "FAILURE"
	ActionRetry   = "RETRY"

	// Flow Orchestration Actions.
	ActionFlowStart  = "FLOW_START"
	ActionFlowEnd    = "FLOW_END"
	ActionStepStart  = "STEP_START"
	ActionStepEnd    = "STEP_END"
	ActionBranchEval = "BRANCH_EVAL"

	// External System Sync Actions.
	ActionSyncImported = "SYNC_IMPORTED"
	ActionSyncPulled   = "SYNC_PULLED"
	ActionSyncPushed   = "SYNC_PUSHED"
	ActionSyncConflict = "SYNC_CONFLICT"
	ActionSyncError    = "SYNC_ERROR"

	// Assignee Actions (new).
	ActionAutoAssigned = "AUTO_ASSIGNED"
	ActionDelegated    = "DELEGATED"
	ActionUnblocked    = "UNBLOCKED"
)
