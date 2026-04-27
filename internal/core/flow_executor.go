package core

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// AgentRunner executes a flow step by dispatching an agent process and returns
// the parsed step output. Implementations are responsible for subprocess
// lifecycle, env injection, and cassette record/replay.
type AgentRunner interface {
	// CanHandle reports whether this runner should handle the given step.
	// If false, executeTemplateStep falls through to the DB-only ephemeral path.
	CanHandle(step Step) bool
	Run(ctx context.Context, step Step, prompt string) (map[string]any, error)
}

// CompositeAgentRunner dispatches to the first registered runner whose
// CanHandle returns true for the step. Use this to combine runners that
// handle different step types (e.g. SandboxAgentRunner for `task` and
// ExecAgentRunner for `exec`) behind FlowExecutor.WithAgentRunner.
type CompositeAgentRunner struct {
	runners []AgentRunner
}

// NewCompositeAgentRunner constructs a composite from the given runners.
// Order matters: earlier runners win on overlapping CanHandle.
func NewCompositeAgentRunner(runners ...AgentRunner) *CompositeAgentRunner {
	return &CompositeAgentRunner{runners: runners}
}

// CanHandle returns true if any inner runner can handle the step.
func (c *CompositeAgentRunner) CanHandle(step Step) bool {
	for _, r := range c.runners {
		if r.CanHandle(step) {
			return true
		}
	}
	return false
}

// Run dispatches to the first inner runner whose CanHandle returns true.
// Returns an error if no inner runner can handle the step.
func (c *CompositeAgentRunner) Run(ctx context.Context, step Step, prompt string) (map[string]any, error) {
	for _, r := range c.runners {
		if r.CanHandle(step) {
			return r.Run(ctx, step, prompt)
		}
	}
	return nil, fmt.Errorf("composite agent runner: no runner handles step %q (type=%s)",
		step.ID, step.Type)
}

// dependencySatisfied reports whether a predecessor's StepStatus is one
// that allows a descendant to proceed past its `depends_on:` constraint.
// SUCCEEDED is the normal completion state. SKIPPED also satisfies: a
// predecessor pruned by branch routing or gated off by a falsy
// `condition:` will never produce output, but its absence should not
// deadlock the rest of the graph (descendants whose inputs reference a
// missing output already render that input as PENDING via the join
// aggregator). FAILED is intentionally NOT satisfying: the scheduler
// surfaces FAILED predecessors as terminal errors before reaching this
// check, which is the safer default until per-step error policies land.
func dependencySatisfied(s StepStatus) bool {
	return s == StepStatusSucceeded || s == StepStatusSkipped
}

// FlowExecutor handles the execution of flow definitions.
type FlowExecutor struct {
	repo        Repository
	logRepo     LogRepository
	evaKey      string      // X-Eva-Key sent to EVA gateway; read from EVA_KEY env var
	testMode    bool        // when true, unresolved task_ref falls back to ephemeral task
	agentRunner AgentRunner // nil → DB-only ephemeral path; set by WithAgentRunner
}

func NewFlowExecutor(repo Repository, logRepo LogRepository) *FlowExecutor {
	return &FlowExecutor{
		repo:    repo,
		logRepo: logRepo,
	}
}

// WithEvaKey sets the EVA API key used for gate authentication.
// Typically called at construction with os.Getenv("EVA_KEY").
func (e *FlowExecutor) WithEvaKey(key string) *FlowExecutor {
	e.evaKey = key
	return e
}

// WithTestMode enables flow-test mode: unresolved task_ref falls back to an
// ephemeral task instead of returning an error. Use only in tlc flow test.
func (e *FlowExecutor) WithTestMode() *FlowExecutor {
	e.testMode = true
	return e
}

// WithAgentRunner sets the agent dispatcher used by executeTemplateStep.
// When non-nil, each task_template step spawns the runner instead of
// completing via DB-only state transitions.
func (e *FlowExecutor) WithAgentRunner(r AgentRunner) *FlowExecutor {
	e.agentRunner = r
	return e
}

// Execute initiates a flow run and returns the run record, per-step output
// maps (stepID → parsed JSON output), and any error.
func (e *FlowExecutor) Execute(ctx context.Context, flow *Flow, by string) (*FlowRun, map[string]map[string]any, error) {
	runID := "run:" + uuid.New().String()

	run := &FlowRun{
		ID:        runID,
		FlowID:    flow.ID,
		Status:    FlowStatusRunning,
		StartedAt: time.Now(),
		Results:   make(map[string]any),
	}

	if err := e.repo.CreateFlowRun(ctx, run); err != nil {
		return nil, nil, fmt.Errorf("failed to create flow run: %w", err)
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

	stepOutputs := make(map[string]map[string]any)
	var mu sync.Mutex
	err := e.runSequential(ctx, flow, run, stepStatuses, stepOutputs, &mu, isChild, by)

	endedAt := time.Now()
	run.EndedAt = &endedAt

	if err != nil {
		run.Status = FlowStatusFailed
		if updateErr := e.repo.UpdateFlowRun(ctx, run); updateErr != nil {
			return nil, stepOutputs, fmt.Errorf("flow failed and update failed: %v (original error: %w)", updateErr, err)
		}
		e.emitFlowLog(ctx, flow.ID, runID, by, "FLOW_END", fmt.Sprintf("Flow failed: %v", err), map[string]any{"status": "failed", "error": err.Error()})
		return run, stepOutputs, err
	}

	run.Status = FlowStatusSucceeded
	if updateErr := e.repo.UpdateFlowRun(ctx, run); updateErr != nil {
		return nil, stepOutputs, fmt.Errorf("flow succeeded but update failed: %w", updateErr)
	}
	e.emitFlowLog(ctx, flow.ID, runID, by, "FLOW_END", "Flow completed successfully", map[string]any{"status": "succeeded"})

	return run, stepOutputs, nil
}

func (e *FlowExecutor) runSequential(ctx context.Context, flow *Flow, run *FlowRun, statuses map[string]StepStatus, stepOutputs map[string]map[string]any, mu *sync.Mutex, isChild map[string]bool, by string) error {
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

			// Check dependencies. A predecessor satisfies depends_on when it
			// has reached a non-blocking terminal state — either SUCCEEDED
			// (normal completion) or SKIPPED (e.g. pruned branch arm or
			// gated by a falsy `condition:`). FAILED predecessors are
			// caught earlier in this loop and abort the run, so they never
			// reach this check.
			ready := true
			for _, dep := range step.DependsOn {
				if !dependencySatisfied(statuses[dep]) {
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
		output, err := e.executeStep(ctx, flow, run, readyStepID, statuses, stepOutputs, mu, by)
		if output != nil {
			stepOutputs[readyStepID] = output
		}
		if err != nil {
			return err
		}
	}
}

func (e *FlowExecutor) checkStatus(ctx context.Context, run *FlowRun, _ string) error {
	for {
		current, err := e.repo.GetFlowRun(ctx, run.ID)
		if err != nil {
			return fmt.Errorf("failed to get flow run: %w", err)
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

func (e *FlowExecutor) executeStep(ctx context.Context, flow *Flow, run *FlowRun, stepID string, statuses map[string]StepStatus, stepOutputs map[string]map[string]any, mu *sync.Mutex, by string) (map[string]any, error) {
	step := flow.Steps[stepID]

	mu.Lock()
	statuses[stepID] = StepStatusRunning
	mu.Unlock()

	e.emitStepLog(ctx, flow.ID, run.ID, stepID, by, "STEP_START", fmt.Sprintf("Starting step: %s", step.Title), nil)

	var (
		output map[string]any
		err    error
	)
	switch step.Type {
	case StepTypeTask:
		output, err = e.executeTaskStep(ctx, step, by)
	case StepTypeParallel:
		err = e.executeParallelStep(ctx, flow, run, step, statuses, stepOutputs, mu, by)
	case StepTypeRetry:
		err = e.executeRetryStep(ctx, flow, run, step, statuses, stepOutputs, mu, by)
	case StepTypeBranch:
		err = e.executeBranchStep(ctx, flow, step, statuses, mu, by)
	case StepTypeJoin:
		// Aggregate predecessor outputs + states into a structured map.
		// Predecessors are step.WaitFor (explicit) or step.DependsOn (fallback).
		// See docs/task-flow-spec-0.1-dev.md "Step Type: join" for schema.
		output = aggregateJoinOutput(step, statuses, stepOutputs, mu)
	case StepTypeExec:
		output, err = e.executeExecStep(ctx, step)
	default:
		return nil, fmt.Errorf("unsupported step type for execution: %s", step.Type)
	}

	mu.Lock()
	if err != nil {
		statuses[stepID] = StepStatusFailed
		mu.Unlock()
		e.emitStepLog(ctx, flow.ID, run.ID, stepID, by, "STEP_END", fmt.Sprintf("Step failed: %v", err), map[string]any{"status": "failed"})
		return nil, err
	}

	// EVA gate: validate step output before marking succeeded.
	if step.Gate != nil {
		if gateErr := RunEvaGate(ctx, step.Gate, output, e.evaKey); gateErr != nil {
			statuses[stepID] = StepStatusFailed
			mu.Unlock()
			e.emitStepLog(ctx, flow.ID, run.ID, stepID, by, "STEP_END",
				fmt.Sprintf("EVA gate rejected step: %v", gateErr),
				map[string]any{"status": "failed", "gate_error": gateErr.Error()})
			return nil, gateErr
		}
	}

	statuses[stepID] = StepStatusSucceeded
	mu.Unlock()

	e.updateProgress(ctx, flow, run, statuses, mu)
	e.emitStepLog(ctx, flow.ID, run.ID, stepID, by, "STEP_END", "Step completed", map[string]any{"status": "succeeded"})
	return output, nil
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
	if err := e.repo.UpdateFlowRun(ctx, run); err != nil {
		return
	}
}

func (e *FlowExecutor) executeParallelStep(ctx context.Context, flow *Flow, run *FlowRun, step Step, statuses map[string]StepStatus, stepOutputs map[string]map[string]any, mu *sync.Mutex, by string) error {
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

			out, err := e.executeStep(ctx, flow, run, cid, statuses, stepOutputs, mu, by)
			if out != nil {
				mu.Lock()
				stepOutputs[cid] = out
				mu.Unlock()
			}
			if err != nil {
				errOnce.Do(func() {
					firstErr = err
				})
			}
		}(childID)
	}

	wg.Wait()
	return firstErr
}

func (e *FlowExecutor) executeRetryStep(ctx context.Context, flow *Flow, run *FlowRun, step Step, statuses map[string]StepStatus, stepOutputs map[string]map[string]any, mu *sync.Mutex, by string) error {
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

		out, err := e.executeStep(ctx, flow, run, step.Child, statuses, stepOutputs, mu, by)
		if err == nil {
			if out != nil {
				mu.Lock()
				stepOutputs[step.Child] = out
				mu.Unlock()
			}
			return nil
		}
		lastErr = err
	}

	return fmt.Errorf("retry exhausted after %d attempts: %w", maxAttempts, lastErr)
}

// executeExecStep dispatches a `type: exec` step to the registered agent
// runner. Unlike `task` steps, exec steps require a runner — there is no
// DB-only fallback because the step's contract is "produce stdout/stderr/
// exit_code from a literal argv". If no runner is registered (or the runner
// declines), the step fails fast with a clear error.
func (e *FlowExecutor) executeExecStep(ctx context.Context, step Step) (map[string]any, error) {
	if step.Exec == nil || len(step.Exec.Argv) == 0 {
		return nil, fmt.Errorf("flow exec step %q: missing or empty exec.argv", step.ID)
	}
	if e.agentRunner == nil {
		return nil, fmt.Errorf(
			"flow exec step %q: no agent runner registered; exec steps require a runner",
			step.ID)
	}
	if !e.agentRunner.CanHandle(step) {
		return nil, fmt.Errorf(
			"flow exec step %q: registered runner cannot handle exec steps", step.ID)
	}
	return e.agentRunner.Run(ctx, step, "")
}

func (e *FlowExecutor) executeTaskStep(ctx context.Context, step Step, by string) (map[string]any, error) {
	// task_template steps: dispatch via agentRunner when set, else ephemeral DB path.
	if step.TaskRef == "" && step.TaskTemplate != nil {
		return e.executeTemplateStep(ctx, step, by)
	}

	taskID := step.TaskRef
	task, err := e.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		if e.testMode {
			// Unresolved task_ref in test mode: fall back to ephemeral task.
			synthetic := step
			if synthetic.TaskTemplate == nil {
				synthetic.TaskTemplate = &TaskTemplate{Title: step.Title}
			}
			return e.executeTemplateStep(ctx, synthetic, by)
		}
		return nil, fmt.Errorf("task %s not found", taskID)
	}

	wm := DefaultWorkflow()
	activeStatus, _ := wm.StatusForRole("active")       //nolint:errcheck // well-known role
	completedStatus, _ := wm.StatusForRole("completed") //nolint:errcheck // well-known role
	initialStatus, _ := wm.StatusForRole("initial")     //nolint:errcheck // well-known role

	if task.Status == initialStatus {
		task.Status = activeStatus
		task.UpdatedAt = time.Now()
		if err := e.repo.UpdateTask(ctx, task); err != nil {
			return nil, fmt.Errorf("failed to update task: %w", err)
		}
		if err := e.logRepo.AddLog(ctx, &LogEntry{
			TaskID:    taskID,
			Timestamp: time.Now(),
			By:        by,
			Action:    "CLAIMED",
			Note:      "Claimed by flow executor",
		}); err != nil {
			return nil, fmt.Errorf("failed to add log: %w", err)
		}
	}

	task.Status = completedStatus
	task.UpdatedAt = time.Now()
	if err := e.repo.UpdateTask(ctx, task); err != nil {
		return nil, fmt.Errorf("failed to update task: %w", err)
	}
	if err := e.logRepo.AddLog(ctx, &LogEntry{
		TaskID:    taskID,
		Timestamp: time.Now(),
		By:        by,
		Action:    "DONE",
		Note:      "Completed by flow executor",
	}); err != nil {
		return nil, fmt.Errorf("failed to add log: %w", err)
	}

	return nil, nil
}

func (e *FlowExecutor) emitFlowLog(ctx context.Context, flowID, runID, by, action, note string, meta map[string]any) {
	if e.logRepo == nil {
		return
	}
	_ = e.logRepo.AddLog(ctx, &LogEntry{ //nolint:errcheck // best-effort audit log
		Timestamp: time.Now(),
		By:        by,
		Action:    action,
		Note:      note,
		Meta: map[string]any{
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
	_ = e.logRepo.AddLog(ctx, &LogEntry{ //nolint:errcheck // best-effort audit log
		Timestamp: time.Now(),
		By:        by,
		Action:    action,
		Note:      note,
		Meta: map[string]any{
			"flow_id": flowID,
			"run_id":  runID,
			"step_id": stepID,
			"extra":   meta,
		},
	})
}

// executeTemplateStep creates an ephemeral task from step.TaskTemplate, runs it
// through the standard task lifecycle, then completes it. No persistent task ID
// is required — the task is identified by a generated UUID.
func (e *FlowExecutor) executeTemplateStep(ctx context.Context, step Step, by string) (map[string]any, error) {
	// Agent dispatch path: runner handles subprocess, cassette, and output.
	// Falls through to DB-only path if runner is nil or declines the step.
	if e.agentRunner != nil && e.agentRunner.CanHandle(step) {
		prompt := buildAgentPrompt(step)
		output, err := e.agentRunner.Run(ctx, step, prompt)
		if err != nil {
			return nil, fmt.Errorf("flow template step %q: agent run: %w", step.ID, err)
		}
		return output, nil
	}

	// DB-only ephemeral path (test mode w/o runner, or normal non-test execution).
	wm := DefaultWorkflow()
	initialStatus, _ := wm.StatusForRole("initial")     //nolint:errcheck // well-known role
	activeStatus, _ := wm.StatusForRole("active")       //nolint:errcheck // well-known role
	completedStatus, _ := wm.StatusForRole("completed") //nolint:errcheck // well-known role

	task := &Task{
		ID:          generateTaskID(),
		Title:       step.TaskTemplate.Title,
		Description: step.TaskTemplate.Description,
		Status:      initialStatus,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if proj := DetectProject(); proj != nil && proj.InProject && proj.ProjectID != "" {
		task.ProjectID = &proj.ProjectID
	}
	if err := e.repo.CreateTask(ctx, task); err != nil {
		return nil, fmt.Errorf("flow template step %q: create task: %w", step.ID, err)
	}

	task.Status = activeStatus
	task.UpdatedAt = time.Now()
	if err := e.repo.UpdateTask(ctx, task); err != nil {
		return nil, fmt.Errorf("flow template step %q: claim task: %w", step.ID, err)
	}

	task.Status = completedStatus
	task.UpdatedAt = time.Now()
	if err := e.repo.UpdateTask(ctx, task); err != nil {
		return nil, fmt.Errorf("flow template step %q: complete task: %w", step.ID, err)
	}
	_ = e.logRepo.AddLog(ctx, &LogEntry{ //nolint:errcheck // best-effort audit log
		TaskID:    task.ID,
		Timestamp: time.Now(),
		By:        by,
		Action:    "DONE",
		Note:      "Completed by flow executor (template step)",
	})
	return nil, nil
}

// buildAgentPrompt constructs the prompt sent to the agent for a template step.
func buildAgentPrompt(step Step) string {
	desc := ""
	if step.TaskTemplate != nil {
		desc = step.TaskTemplate.Description
	}
	return fmt.Sprintf("Step: %s\n\n%s", step.ID, desc)
}

// executeBranchStep evaluates branch cases and marks unchosen downstream paths
// as skipped so the scheduler does not deadlock waiting for them.
// In flow test mode (no runtime condition data), the default_next branch is taken.
func (e *FlowExecutor) executeBranchStep(_ context.Context, flow *Flow, step Step, statuses map[string]StepStatus, mu *sync.Mutex, _ string) error {
	// Determine chosen next step: first case whose condition evaluates true, else default.
	// At flow-test time we have no runtime values, so we always take default_next.
	chosen := step.DefaultNext

	// Mark all non-chosen case targets as skipped so the scheduler can proceed.
	mu.Lock()
	for _, c := range step.Cases {
		if c.Next != chosen {
			if statuses[c.Next] == StepStatusPending {
				statuses[c.Next] = StepStatusSkipped
			}
		}
	}
	mu.Unlock()

	// Validate chosen target exists.
	if chosen != "" {
		if _, ok := flow.Steps[chosen]; !ok {
			return fmt.Errorf("branch step %q: default_next %q not found in flow", step.ID, chosen)
		}
	}
	return nil
}

// ExtractTasksFromFlow generates tasks from flow steps with task templates.
// This enables flows to coordinate task creation for assignee execution.
//
// inputs holds the resolved flow input variables (see ResolveFlowInputs);
// {{name}} placeholders inside task template fields are substituted with
// their values before the tasks are returned.
func (e *FlowExecutor) ExtractTasksFromFlow(_ context.Context, flow *Flow, runID string, inputs map[string]any) ([]*Task, error) {
	tasks := []*Task{}

	for stepID, step := range flow.Steps {
		if step.Type == StepTypeTask && step.TaskTemplate != nil {
			task := e.generateTaskFromTemplate(flow, runID, stepID, step, inputs)
			tasks = append(tasks, task)
		}
	}

	return tasks, nil
}

// generateTaskFromTemplate creates a task from a step's task template,
// substituting any {{var}} placeholders using the resolved inputs.
func (e *FlowExecutor) generateTaskFromTemplate(flow *Flow, runID, stepID string, step Step, inputs map[string]any) *Task {
	taskID := generateTaskID()

	wm := DefaultWorkflow()
	initialStatus, _ := wm.StatusForRole("initial") //nolint:errcheck // well-known role

	task := &Task{
		ID:          taskID,
		Title:       SubstituteFlowInputs(step.TaskTemplate.Title, inputs),
		Description: SubstituteFlowInputs(step.TaskTemplate.Description, inputs),
		Status:      initialStatus,
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

	if proj := DetectProject(); proj != nil && proj.InProject && proj.ProjectID != "" {
		task.ProjectID = &proj.ProjectID
	}

	return task
}

// ExecuteForTest exposes single-step execution for unit testing.
// Executes only the named entry step and its downstream (via recursive executeStep).
// Do not use outside tests.
func (e *FlowExecutor) ExecuteForTest(
	ctx context.Context,
	flow *Flow,
	run *FlowRun,
	statuses map[string]StepStatus,
	by string,
) error {
	mu := &sync.Mutex{}
	stepOutputs := make(map[string]map[string]any)
	_, err := e.executeStep(ctx, flow, run, flow.EntryStep, statuses, stepOutputs, mu, by)
	return err
}

// generateTaskID creates a unique task ID (e.g., T-0042).
func generateTaskID() string {
	// Simple UUID-based ID for now
	// In production, this should use a counter from the database
	return fmt.Sprintf("T-%s", uuid.New().String()[:8])
}

// aggregateJoinOutput builds the join step's structured output from its
// predecessor steps. Predecessor IDs come from step.WaitFor (canonical) or
// fall back to step.DependsOn when WaitFor is empty.
//
// Output schema (consumed by EVA contracts on the impl branch):
//
//	{
//	  "steps": {
//	    "<id>": {"state": "<UPPER>", "output": {...}|null},
//	    ...
//	  },
//	  "summary": "<id>: <UPPER>\n..."
//	}
//
// State names are uppercased (SUCCEEDED, FAILED, SKIPPED, …) so `contains`
// evaluators can assert on `<id>: SUCCEEDED` without case juggling.
// Predecessor order in `summary` follows the declared list (stable for
// deterministic test assertions). Missing predecessors render as PENDING.
func aggregateJoinOutput(step Step, statuses map[string]StepStatus, stepOutputs map[string]map[string]any, mu *sync.Mutex) map[string]any {
	preds := step.WaitFor
	if len(preds) == 0 {
		preds = step.DependsOn
	}

	steps := make(map[string]any, len(preds))
	lines := make([]string, 0, len(preds))

	mu.Lock()
	defer mu.Unlock()
	for _, id := range preds {
		state := strings.ToUpper(string(statuses[id]))
		if state == "" {
			state = "PENDING"
		}
		entry := map[string]any{"state": state}
		if out, ok := stepOutputs[id]; ok && out != nil {
			entry["output"] = out
		} else {
			entry["output"] = nil
		}
		steps[id] = entry
		lines = append(lines, fmt.Sprintf("%s: %s", id, state))
	}

	return map[string]any{
		"steps":   steps,
		"summary": strings.Join(lines, "\n"),
	}
}
