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
