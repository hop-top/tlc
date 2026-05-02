package core

import (
	"context"
	"fmt"
	"time"

	"hop.top/kit/go/runtime/bus"
)

// AgentRunRecord is an audit trail entry for a single agent execution,
// independent of task state.
type AgentRunRecord struct {
	ID          string
	Agent       string
	TargetType  string // "task", "flow", "track"
	TargetID    string
	JobID       string // kit/job ID if async
	ContainerID string
	Status      string
	ExitCode    int
	Error       string
	ResultPath  string
	StartedAt   time.Time
	EndedAt     *time.Time
	CreatedBy   string
}

// AgentEvent is the bus event payload for agent lifecycle events.
type AgentEvent struct {
	RunID      string `json:"run_id"`
	Agent      string `json:"agent"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Status     string `json:"status,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

// UpdateOpts configures state update behaviour.
type UpdateOpts struct {
	NoStateUpdate bool // --no-state-update: skip tlc state transitions
}

// AgentRunStore persists agent run audit records.
type AgentRunStore interface {
	CreateAgentRun(ctx context.Context, r *AgentRunRecord) error
	UpdateAgentRun(ctx context.Context, r *AgentRunRecord) error
}

// StateUpdater maps agent results to tlc state transitions and
// publishes bus events for observability.
type StateUpdater struct {
	taskSvc  *TaskService
	bus      bus.Bus // optional; nil = no events published
	runStore AgentRunStore
}

// NewStateUpdater creates a state updater.
func NewStateUpdater(taskSvc *TaskService, b bus.Bus) *StateUpdater {
	return &StateUpdater{
		taskSvc: taskSvc,
		bus:     b,
	}
}

// WithRunStore attaches a persistent store for agent run records.
func (u *StateUpdater) WithRunStore(s AgentRunStore) *StateUpdater {
	u.runStore = s
	return u
}

// Update applies the agent result to the target entity.
//
// Result status mapping:
//   - succeeded → complete task (DONE)
//   - failed    → leave IN_PROGRESS, append failure summary
//   - partial   → leave IN_PROGRESS, append partial summary
//   - timeout   → leave IN_PROGRESS, note timeout
func (u *StateUpdater) Update(
	ctx context.Context,
	result *AgentResult,
	targetType, targetID string,
	opts UpdateOpts,
) error {
	if opts.NoStateUpdate {
		return nil
	}
	if targetType != "task" {
		// Flow/track state handled by their own executors.
		return nil
	}

	switch result.Status {
	case AgentStatusSucceeded:
		return u.completeTask(ctx, targetID, result.Summary)
	case AgentStatusFailed:
		return u.appendDescription(ctx, targetID,
			fmt.Sprintf("[agent:failed] %s", result.Summary))
	case AgentStatusPartial:
		return u.appendDescription(ctx, targetID,
			fmt.Sprintf("[agent:partial] %s", result.Summary))
	case AgentStatusTimeout:
		return u.appendDescription(ctx, targetID,
			"[agent:timeout] execution timed out")
	default:
		return fmt.Errorf(
			"state updater: unknown result status %q", result.Status,
		)
	}
}

func (u *StateUpdater) completeTask(ctx context.Context, taskID, summary string) error {
	task, err := u.taskSvc.repo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("state updater: get task %s: %w", taskID, err)
	}
	if task == nil {
		return fmt.Errorf("state updater: task %s not found", taskID)
	}

	wm := DefaultWorkflow()
	logEntry, err := task.TransitionWithWorkflow(
		StatusDone, "agent", summary, wm, false,
	)
	if err != nil {
		return fmt.Errorf("state updater: transition task %s: %w", taskID, err)
	}
	logEntry.Action = ActionDone

	if err := u.taskSvc.repo.UpdateTaskWithLog(ctx, task, logEntry); err != nil {
		return fmt.Errorf("state updater: update task %s: %w", taskID, err)
	}
	return nil
}

func (u *StateUpdater) appendDescription(ctx context.Context, taskID, note string) error {
	task, err := u.taskSvc.repo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("state updater: get task %s: %w", taskID, err)
	}
	if task == nil {
		return fmt.Errorf("state updater: task %s not found", taskID)
	}

	if task.Description != "" {
		task.Description += "\n\n"
	}
	task.Description += note
	task.UpdatedAt = time.Now().UTC()

	if err := u.taskSvc.repo.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("state updater: update task %s: %w", taskID, err)
	}
	return nil
}

// CreateRun persists an agent run audit record and publishes a started
// event on the bus.
func (u *StateUpdater) CreateRun(ctx context.Context, record *AgentRunRecord) error {
	if u.runStore != nil {
		if err := u.runStore.CreateAgentRun(ctx, record); err != nil {
			return fmt.Errorf("state updater: create run: %w", err)
		}
	}
	// Topic mirrors events.TopicAgentStarted; kept as string literal here
	// because internal/events imports internal/core (would create cycle).
	u.publishEvent(ctx, "tlc.agent.started", &AgentEvent{
		RunID:      record.ID,
		Agent:      record.Agent,
		TargetType: record.TargetType,
		TargetID:   record.TargetID,
	})
	return nil
}

// UpdateRun updates the audit record and publishes a completed or
// failed event.
func (u *StateUpdater) UpdateRun(
	ctx context.Context,
	runID string,
	result *AgentResult,
	record *AgentRunRecord,
) error {
	now := time.Now().UTC()
	record.Status = string(result.Status)
	record.ExitCode = result.ExitCode
	record.EndedAt = &now

	if result.Status == AgentStatusFailed || result.Status == AgentStatusTimeout {
		record.Error = result.Summary
	}

	if u.runStore != nil {
		if err := u.runStore.UpdateAgentRun(ctx, record); err != nil {
			return fmt.Errorf("state updater: update run: %w", err)
		}
	}

	var durationMs int64
	if !record.StartedAt.IsZero() {
		durationMs = now.Sub(record.StartedAt).Milliseconds()
	}

	switch result.Status {
	case AgentStatusSucceeded, AgentStatusPartial:
		// Mirrors events.TopicAgentCompleted (string literal: avoid import cycle).
		u.publishEvent(ctx, "tlc.agent.completed", &AgentEvent{
			RunID:      runID,
			Agent:      record.Agent,
			TargetType: record.TargetType,
			TargetID:   record.TargetID,
			Status:     string(result.Status),
			Summary:    result.Summary,
			DurationMs: durationMs,
		})
	case AgentStatusFailed, AgentStatusTimeout:
		// Mirrors events.TopicAgentFailed (string literal: avoid import cycle).
		u.publishEvent(ctx, "tlc.agent.failed", &AgentEvent{
			RunID:      runID,
			Agent:      record.Agent,
			TargetType: record.TargetType,
			TargetID:   record.TargetID,
			Error:      result.Summary,
			DurationMs: durationMs,
		})
	}
	return nil
}

func (u *StateUpdater) publishEvent(ctx context.Context, topic string, evt *AgentEvent) {
	if u.bus == nil {
		return
	}
	// Best-effort: don't fail the operation if event publishing fails.
	_ = u.bus.Publish(ctx, bus.NewEvent(bus.Topic(topic), "tlc", evt))
}
