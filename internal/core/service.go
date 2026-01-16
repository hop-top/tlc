package core

import (
	"context"
	"fmt"
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
