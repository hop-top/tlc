package core

import (
	"hop.top/kit/domain"
	"hop.top/tlc/internal/config"
)

// NewTaskStateMachine builds a domain.StateMachine from a TaskConfig.
// Terminal statuses have no outgoing transitions and are excluded from
// the rules map. The optional EventPublisher enables pre/post hooks.
func NewTaskStateMachine(
	cfg *config.TaskConfig,
	pub domain.EventPublisher,
) *domain.StateMachine {
	statuses := cfg.Statuses
	if len(statuses) == 0 {
		statuses = config.GetDefaultStatuses()
	}

	sm := cfg.StateMachine
	if sm == nil {
		sm = config.GetDefaultStateMachine()
	}

	return buildStateMachine(statuses, sm.Rules, pub)
}

// NewTaskStateMachineFromWorkflow builds a domain.StateMachine from
// a WorkflowManager's internal rules. Use when you already have a
// WorkflowManager and want its rules in kit/domain form.
func NewTaskStateMachineFromWorkflow(
	wm *WorkflowManager,
	pub domain.EventPublisher,
) *domain.StateMachine {
	rules := make(map[domain.State][]domain.State, len(wm.rules))
	for from, tos := range wm.rules {
		targets := make([]domain.State, len(tos))
		for i, to := range tos {
			targets[i] = domain.State(to)
		}
		rules[domain.State(from)] = targets
	}
	return domain.NewStateMachine(rules, pub)
}

// NewTrackStateMachine builds a domain.StateMachine for track lifecycle.
// Rules: pending->active/abandoned, active->completed/abandoned,
// completed->archived/abandoned, abandoned->archived.
// Archived is terminal (no outgoing transitions).
func NewTrackStateMachine(pub domain.EventPublisher) *domain.StateMachine {
	rules := map[domain.State][]domain.State{
		domain.State(TrackStatusPending):   {domain.State(TrackStatusActive), domain.State(TrackStatusAbandoned)},
		domain.State(TrackStatusActive):    {domain.State(TrackStatusCompleted), domain.State(TrackStatusAbandoned)},
		domain.State(TrackStatusCompleted): {domain.State(TrackStatusArchived), domain.State(TrackStatusAbandoned)},
		domain.State(TrackStatusAbandoned): {domain.State(TrackStatusArchived)},
		// archived: terminal — no outgoing transitions
	}
	return domain.NewStateMachine(rules, pub)
}

// NewFlowStateMachine builds a domain.StateMachine for flow run lifecycle.
// Rules: queued->running, running->succeeded/failed/canceled/paused,
// paused->running/canceled. Succeeded/failed/canceled are terminal.
func NewFlowStateMachine(pub domain.EventPublisher) *domain.StateMachine {
	rules := map[domain.State][]domain.State{
		domain.State(FlowStatusQueued):  {domain.State(FlowStatusRunning)},
		domain.State(FlowStatusRunning): {
			domain.State(FlowStatusSucceeded),
			domain.State(FlowStatusFailed),
			domain.State(FlowStatusCanceled),
			domain.State(FlowStatusPaused),
		},
		domain.State(FlowStatusPaused): {
			domain.State(FlowStatusRunning),
			domain.State(FlowStatusCanceled),
		},
		// succeeded, failed, canceled: terminal — no outgoing transitions
	}
	return domain.NewStateMachine(rules, pub)
}

// buildStateMachine converts config status definitions and rules
// into a domain.StateMachine.
func buildStateMachine(
	statuses []config.StatusDefinition,
	configRules map[string][]string,
	pub domain.EventPublisher,
) *domain.StateMachine {
	if configRules == nil {
		configRules = map[string][]string{
			"TODO":        {"IN_PROGRESS", "SKIPPED"},
			"IN_PROGRESS": {"DONE", "TODO", "SKIPPED"},
		}
	}

	// Build terminal set for reference.
	terminal := make(map[string]bool, len(statuses))
	for _, s := range statuses {
		if s.IsTerminal {
			terminal[s.Name] = true
		}
	}

	rules := make(map[domain.State][]domain.State, len(configRules))
	for from, tos := range configRules {
		targets := make([]domain.State, len(tos))
		for i, to := range tos {
			targets[i] = domain.State(to)
		}
		rules[domain.State(from)] = targets
	}

	return domain.NewStateMachine(rules, pub)
}
