package core

import (
	"context"
	"fmt"
	"time"
)

type TaskService struct {
	repo    Repository
	logRepo LogRepository
}

func NewTaskService(repo Repository, logRepo LogRepository) *TaskService {
	return &TaskService{
		repo:    repo,
		logRepo: logRepo,
	}
}

func (s *TaskService) CreateTask(ctx context.Context, task *Task, by string, note string) error {
	if err := s.repo.CreateTask(ctx, task); err != nil {
		return err
	}

	logEntry := &LogEntry{
		TaskID:    task.ID,
		Timestamp: task.CreatedAt,
		By:        by,
		Action:    "CREATED",
		Note:      note,
	}

	return s.logRepo.AddLog(ctx, logEntry)
}

func (s *TaskService) ListTasks(ctx context.Context, query Query) ([]*Task, error) {
	return s.repo.ListTasks(ctx, query)
}

func (s *TaskService) ListFlowRuns(ctx context.Context, query Query) ([]*FlowRun, error) {
	return s.repo.ListFlowRuns(ctx, query)
}

func (s *TaskService) TransitionStatus(ctx context.Context, taskID string, next TaskStatus, by string, note string) error {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}

	logEntry, err := task.Transition(next, by, note)
	if err != nil {
		return err
	}

	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return err
	}

	return s.logRepo.AddLog(ctx, logEntry)
}

func (s *TaskService) ClaimTask(ctx context.Context, taskID string, by string, note string) error {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}

	task.AssignedTo = &by
	logEntry, err := task.Transition(StatusInProgress, by, note)
	if err != nil {
		return err
	}
	logEntry.Action = ActionClaimed

	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return err
	}

	return s.logRepo.AddLog(ctx, logEntry)
}

func (s *TaskService) UnclaimTask(ctx context.Context, taskID string, by string, note string) error {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}

	task.AssignedTo = nil
	logEntry, err := task.Transition(StatusTodo, by, note)
	if err != nil {
		return err
	}
	logEntry.Action = ActionReleased

	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return err
	}

	return s.logRepo.AddLog(ctx, logEntry)
}

func (s *TaskService) PauseFlowRun(ctx context.Context, runID string, by string, note string) error {
	run, err := s.repo.GetFlowRun(ctx, runID)
	if err != nil {
		return err
	}
	if run == nil {
		return fmt.Errorf("flow run %s not found", runID)
	}
	run.Status = FlowStatusPaused
	if err := s.repo.UpdateFlowRun(ctx, run); err != nil {
		return err
	}
	return s.logRepo.AddLog(ctx, &LogEntry{
		Timestamp: time.Now().UTC(),
		By:        by,
		Action:    "FLOW_PAUSED",
		Note:      note,
		Meta:      map[string]any{"run_id": runID},
	})
}

func (s *TaskService) ResumeFlowRun(ctx context.Context, runID string, by string, note string) error {
	run, err := s.repo.GetFlowRun(ctx, runID)
	if err != nil {
		return err
	}
	if run == nil {
		return fmt.Errorf("flow run %s not found", runID)
	}
	run.Status = FlowStatusRunning
	if err := s.repo.UpdateFlowRun(ctx, run); err != nil {
		return err
	}
	return s.logRepo.AddLog(ctx, &LogEntry{
		Timestamp: time.Now().UTC(),
		By:        by,
		Action:    "FLOW_RESUMED",
		Note:      note,
		Meta:      map[string]any{"run_id": runID},
	})
}

func (s *TaskService) CancelFlowRun(ctx context.Context, runID string, by string, note string) error {
	run, err := s.repo.GetFlowRun(ctx, runID)
	if err != nil {
		return err
	}
	if run == nil {
		return fmt.Errorf("flow run %s not found", runID)
	}
	run.Status = FlowStatusCanceled
	if err := s.repo.UpdateFlowRun(ctx, run); err != nil {
		return err
	}
	return s.logRepo.AddLog(ctx, &LogEntry{
		Timestamp: time.Now().UTC(),
		By:        by,
		Action:    "FLOW_CANCELED",
		Note:      note,
		Meta:      map[string]any{"run_id": runID},
	})
}

const (
	// Task CRUD Actions
	ActionCreated = "CREATED"
	ActionUpdated = "UPDATED"
	ActionDeleted = "DELETED"

	// Task Status Actions
	ActionClaimed    = "CLAIMED"
	ActionReleased   = "RELEASED"
	ActionReassigned = "REASSIGNED"
	ActionDone       = "DONE"
	ActionSkipped    = "SKIPPED"

	// Task Execution Actions
	ActionExecStart   = "EXEC_START"
	ActionExecEnd     = "EXEC_END"
	ActionExecAttempt = "EXEC_ATTEMPT"

	// Collaboration Actions
	ActionComment  = "COMMENT"
	ActionBlocked  = "BLOCKED"
	ActionSplit    = "SPLIT"
	ActionMerged   = "MERGED"
	ActionMigrated = "MIGRATED"

	// Failure and Retry Actions
	ActionFailure = "FAILURE"
	ActionRetry   = "RETRY"

	// Flow Orchestration Actions
	ActionFlowStart  = "FLOW_START"
	ActionFlowEnd    = "FLOW_END"
	ActionStepStart  = "STEP_START"
	ActionStepEnd    = "STEP_END"
	ActionBranchEval = "BRANCH_EVAL"

	// External System Sync Actions
	ActionSyncImported = "SYNC_IMPORTED"
	ActionSyncPulled   = "SYNC_PULLED"
	ActionSyncPushed   = "SYNC_PUSHED"
	ActionSyncConflict = "SYNC_CONFLICT"
	ActionSyncError    = "SYNC_ERROR"
)
