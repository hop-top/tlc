package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// FlowExecutor handles the execution of flow definitions.
type FlowExecutor struct {
	repo    Repository
	logRepo LogRepository
}

func NewFlowExecutor(repo Repository, logRepo LogRepository) *FlowExecutor {
	return &FlowExecutor{
		repo:    repo,
		logRepo: logRepo,
	}
}

// Execute initiates a flow run.
func (e *FlowExecutor) Execute(ctx context.Context, flow *Flow, by string) (*FlowRun, error) {
	runID := "run:" + uuid.New().String()
	
	run := &FlowRun{
		ID:        runID,
		FlowID:    flow.ID,
		Status:    FlowStatusRunning,
		StartedAt: time.Now(),
		Results:   make(map[string]any),
	}

	if err := e.repo.CreateFlowRun(ctx, run); err != nil {
		return nil, fmt.Errorf("failed to create flow run: %w", err)
	}

	e.emitFlowLog(ctx, flow.ID, runID, by, "FLOW_START", fmt.Sprintf("Starting flow: %s", flow.Name), nil)

	// Step status map for tracking progress in this run
	stepStatuses := make(map[string]StepStatus)
	for id := range flow.Steps {
		stepStatuses[id] = StepStatusPending
	}

	// Identify steps that are children of other steps (parallel, retry)
	// These should not be picked up by the main loop.
	isChild := make(map[string]bool)
	for _, step := range flow.Steps {
		switch step.Type {
		case StepTypeParallel:
			for _, cid := range step.Children {
				isChild[cid] = true
			}
		case StepTypeRetry:
			if step.Child != "" {
				isChild[step.Child] = true
			}
		}
	}

	var mu sync.Mutex
	err := e.runSequential(ctx, flow, run, stepStatuses, &mu, isChild, by)
	
	endedAt := time.Now()
	run.EndedAt = &endedAt
	
	if err != nil {
		run.Status = FlowStatusFailed
		e.repo.UpdateFlowRun(ctx, run)
		e.emitFlowLog(ctx, flow.ID, runID, by, "FLOW_END", fmt.Sprintf("Flow failed: %v", err), map[string]any{"status": "failed", "error": err.Error()})
		return run, err
	}

	run.Status = FlowStatusSucceeded
	e.repo.UpdateFlowRun(ctx, run)
	e.emitFlowLog(ctx, flow.ID, runID, by, "FLOW_END", "Flow completed successfully", map[string]any{"status": "succeeded"})
	
	return run, nil
}

func (e *FlowExecutor) runSequential(ctx context.Context, flow *Flow, run *FlowRun, statuses map[string]StepStatus, mu *sync.Mutex, isChild map[string]bool, by string) error {
	// Simple strategy: repeatedly find ready steps until none left or all succeeded
	for {
		// Check for external control signals (pause/cancel)
		if err := e.checkStatus(ctx, run, by); err != nil {
			return err
		}

		readyStepID := ""
		allSucceeded := true
		
		for id, step := range flow.Steps {
			// Skip steps that are managed by a container
			if isChild[id] {
				continue
			}

			if statuses[id] == StepStatusSucceeded || statuses[id] == StepStatusSkipped {
				continue
			}
			allSucceeded = false
			if statuses[id] == StepStatusFailed || statuses[id] == StepStatusCanceled {
				return fmt.Errorf("step %s reached terminal failure state: %s", id, statuses[id])
			}

			// Check dependencies
			ready := true
			for _, dep := range step.DependsOn {
				if statuses[dep] != StepStatusSucceeded {
					ready = false
					break
				}
			}
			
			if ready {
				readyStepID = id
				break
			}
		}

		if allSucceeded {
			return nil
		}

		if readyStepID == "" {
			return fmt.Errorf("deadlock detected or unreachable steps in flow")
		}

		// Execute the ready step
		if err := e.executeStep(ctx, flow, run, readyStepID, statuses, mu, by); err != nil {
			return err
		}
	}
}

func (e *FlowExecutor) checkStatus(ctx context.Context, run *FlowRun, by string) error {
	for {
		current, err := e.repo.GetFlowRun(ctx, run.ID)
		if err != nil {
			return err
		}
		if current == nil {
			return fmt.Errorf("flow run %s disappeared", run.ID)
		}

		run.Status = current.Status

		switch run.Status {
		case FlowStatusCanceled:
			return fmt.Errorf("flow execution canceled")
		case FlowStatusPaused:
			// Busy wait or sleep? Sleep is better.
			time.Sleep(1 * time.Second)
			continue
		default:
			return nil
		}
	}
}

func (e *FlowExecutor) executeStep(ctx context.Context, flow *Flow, run *FlowRun, stepID string, statuses map[string]StepStatus, mu *sync.Mutex, by string) error {
	step := flow.Steps[stepID]

	mu.Lock()
	statuses[stepID] = StepStatusRunning
	mu.Unlock()

	e.emitStepLog(ctx, flow.ID, run.ID, stepID, by, "STEP_START", fmt.Sprintf("Starting step: %s", step.Title), nil)

	var err error
	switch step.Type {
	case StepTypeTask:
		err = e.executeTaskStep(ctx, step, by)
	case StepTypeParallel:
		err = e.executeParallelStep(ctx, flow, run, step, statuses, mu, by)
	case StepTypeRetry:
		err = e.executeRetryStep(ctx, flow, run, step, statuses, mu, by)
	default:
		// Other types (branch, etc.) will be implemented in subsequent tasks
		return fmt.Errorf("unsupported step type for execution: %s", step.Type)
	}

	mu.Lock()
	if err != nil {
		statuses[stepID] = StepStatusFailed
		mu.Unlock()
		e.emitStepLog(ctx, flow.ID, run.ID, stepID, by, "STEP_END", fmt.Sprintf("Step failed: %v", err), map[string]any{"status": "failed"})
		return err
	}

	statuses[stepID] = StepStatusSucceeded
	mu.Unlock()

	e.updateProgress(ctx, flow, run, statuses, mu)
	e.emitStepLog(ctx, flow.ID, run.ID, stepID, by, "STEP_END", "Step completed", map[string]any{"status": "succeeded"})
	return nil
}

func (e *FlowExecutor) updateProgress(ctx context.Context, flow *Flow, run *FlowRun, statuses map[string]StepStatus, mu *sync.Mutex) {
	if len(flow.Steps) == 0 {
		return
	}
	mu.Lock()
	completed := 0
	for _, s := range statuses {
		if s == StepStatusSucceeded || s == StepStatusSkipped || s == StepStatusFailed {
			completed++
		}
	}
	run.Progress = float64(completed) / float64(len(flow.Steps))
	mu.Unlock()
	e.repo.UpdateFlowRun(ctx, run)
}

func (e *FlowExecutor) executeParallelStep(ctx context.Context, flow *Flow, run *FlowRun, step Step, statuses map[string]StepStatus, mu *sync.Mutex, by string) error {
	var wg sync.WaitGroup
	var errOnce sync.Once
	var firstErr error

	// MaxConcurrency handling (semaphore)
	var sem chan struct{}
	if step.MaxConcurrency > 0 {
		sem = make(chan struct{}, step.MaxConcurrency)
	}

	for _, childID := range step.Children {
		wg.Add(1)
		go func(cid string) {
			defer wg.Done()
			
			if sem != nil {
				sem <- struct{}{}
				defer func() { <-sem }()
			}

			if err := e.executeStep(ctx, flow, run, cid, statuses, mu, by); err != nil {
				errOnce.Do(func() {
					firstErr = err
				})
			}
		}(childID)
	}

	wg.Wait()
	return firstErr
}

func (e *FlowExecutor) executeRetryStep(ctx context.Context, flow *Flow, run *FlowRun, step Step, statuses map[string]StepStatus, mu *sync.Mutex, by string) error {
	maxAttempts := 1
	backoffMS := 0
	if step.Policy != nil {
		if step.Policy.MaxAttempts > 1 {
			maxAttempts = step.Policy.MaxAttempts
		}
		backoffMS = step.Policy.BackoffMS
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			e.emitStepLog(ctx, flow.ID, run.ID, step.ID, by, "RETRY", fmt.Sprintf("Retrying step, attempt %d", attempt), map[string]any{"attempt": attempt})
			if backoffMS > 0 {
				time.Sleep(time.Duration(backoffMS) * time.Millisecond)
			}
			// Reset child status to allow re-execution
			mu.Lock()
			statuses[step.Child] = StepStatusPending
			mu.Unlock()
		}

		err := e.executeStep(ctx, flow, run, step.Child, statuses, mu, by)
		if err == nil {
			return nil
		}
		lastErr = err
	}

	return fmt.Errorf("retry exhausted after %d attempts: %w", maxAttempts, lastErr)
}

func (e *FlowExecutor) executeTaskStep(ctx context.Context, step Step, by string) error {
	// Resolve task
	taskID := step.TaskRef
	// This should use the TaskService or Repository to transition status
	// For now, we'll simulate or use e.repo
	task, err := e.repo.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}

	// Simulation of task execution for now
	// In real implementation, this would call a Runner
	if task.Status == StatusTodo {
		task.Status = StatusInProgress
		task.UpdatedAt = time.Now()
		e.repo.UpdateTask(ctx, task)
		e.logRepo.AddLog(ctx, &LogEntry{
			TaskID:    taskID,
			Timestamp: time.Now(),
			By:        by,
			Action:    "CLAIMED",
			Note:      "Claimed by flow executor",
		})
	}

	// Complete task
	task.Status = StatusDone
	task.UpdatedAt = time.Now()
	e.repo.UpdateTask(ctx, task)
	e.logRepo.AddLog(ctx, &LogEntry{
		TaskID:    taskID,
		Timestamp: time.Now(),
		By:        by,
		Action:    "DONE",
		Note:      "Completed by flow executor",
	})

	return nil
}

func (e *FlowExecutor) emitFlowLog(ctx context.Context, flowID, runID, by, action, note string, meta map[string]any) {
	if e.logRepo == nil {
		return
	}
	e.logRepo.AddLog(ctx, &LogEntry{
		Timestamp: time.Now(),
		By:        by,
		Action:    action,
		Note:      note,
		Meta:      map[string]any{
			"flow_id": flowID,
			"run_id":  runID,
			"extra":   meta,
		},
	})
}

func (e *FlowExecutor) emitStepLog(ctx context.Context, flowID, runID, stepID, by, action, note string, meta map[string]any) {
	if e.logRepo == nil {
		return
	}
	e.logRepo.AddLog(ctx, &LogEntry{
		Timestamp: time.Now(),
		By:        by,
		Action:    action,
		Note:      note,
		Meta:      map[string]any{
			"flow_id": flowID,
			"run_id":  runID,
			"step_id": stepID,
			"extra":   meta,
		},
	})
}

// ExtractTasksFromFlow generates tasks from flow steps with task templates.
// This enables flows to coordinate task creation for assignee execution.
func (e *FlowExecutor) ExtractTasksFromFlow(ctx context.Context, flow *Flow, runID string) ([]*Task, error) {
	tasks := []*Task{}

	for stepID, step := range flow.Steps {
		if step.Type == StepTypeTask && step.TaskTemplate != nil {
			task, err := e.generateTaskFromTemplate(flow, runID, stepID, step)
			if err != nil {
				return nil, fmt.Errorf("failed to generate task for step %s: %w", stepID, err)
			}
			tasks = append(tasks, task)
		}
	}

	return tasks, nil
}

// generateTaskFromTemplate creates a task from a step's task template.
func (e *FlowExecutor) generateTaskFromTemplate(flow *Flow, runID, stepID string, step Step) (*Task, error) {
	taskID := generateTaskID()

	task := &Task{
		ID:          taskID,
		Title:       step.TaskTemplate.Title,
		Description: step.TaskTemplate.Description,
		Status:      StatusTodo,
		Reference:   flow.ID,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
		Meta: map[string]interface{}{
			"flow_id":      flow.ID,
			"flow_run_id":  runID,
			"step_id":      stepID,
			"requirements": step.TaskTemplate.Requirements,
			"context":      step.TaskTemplate.Context,
		},
	}

	return task, nil
}

// generateTaskID creates a unique task ID (e.g., T-0042).
func generateTaskID() string {
	// Simple UUID-based ID for now
	// In production, this should use a counter from the database
	return fmt.Sprintf("T-%s", uuid.New().String()[:8])
}
