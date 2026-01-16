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

func (s *TaskService) ArchiveTasks(ctx context.Context, threshold time.Duration) (int64, error) {
	return s.repo.ArchiveTasks(ctx, threshold)
}

func (s *TaskService) UpdateTask(ctx context.Context, task *Task, by string, note string) error {
	if task.OriginSystem != nil && *task.OriginSystem != "" {
		if task.Meta == nil {
			task.Meta = make(map[string]interface{})
		}
		task.Meta["needs_push"] = true
		
		logEntry := &LogEntry{
			TaskID:    task.ID,
			Timestamp: time.Now().UTC(),
			By:        by,
			Action:    ActionComment,
			Note:      fmt.Sprintf("Task marked for push to %s", *task.OriginSystem),
		}
		if err := s.logRepo.AddLog(ctx, logEntry); err != nil {
			return err
		}
	}

	return s.repo.UpdateTask(ctx, task)
}

func (s *TaskService) GetLogs(ctx context.Context, taskID string, sortDirection string) ([]*LogEntry, error) {
	return s.logRepo.GetLogs(ctx, taskID, sortDirection)
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

	return s.repo.UpdateTaskWithLog(ctx, task, logEntry)
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

	return s.repo.UpdateTaskWithLog(ctx, task, logEntry)
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

	return s.repo.UpdateTaskWithLog(ctx, task, logEntry)
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
