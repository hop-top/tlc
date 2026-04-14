package core

import (
	"fmt"
	"sync"

	"hop.top/kit/domain"
	"hop.top/tlc/internal/config"
)

// WorkflowManager wraps config into a runtime workflow engine.
type WorkflowManager struct {
	statuses    map[string]*config.StatusDefinition
	statusOrder []string
	rules       map[string][]string
	workflows   map[string]*WorkflowManager
	roleIndex   map[string]string // role -> first status name with that role
	markerIndex map[string]string // TLS marker -> status name
}

// NewWorkflowManager creates a WorkflowManager from a TaskConfig.
func NewWorkflowManager(cfg *config.TaskConfig) (*WorkflowManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("task config must not be nil")
	}

	statuses := cfg.Statuses
	if len(statuses) == 0 {
		statuses = config.GetDefaultStatuses()
	}

	sm := cfg.StateMachine
	if sm == nil {
		sm = config.GetDefaultStateMachine()
	}

	wm, err := newWorkflowManagerFromRules(statuses, sm.Rules)
	if err != nil {
		return nil, err
	}

	// Build per-tag workflow overrides
	if len(cfg.Workflows) > 0 {
		wm.workflows = make(map[string]*WorkflowManager, len(cfg.Workflows))
		for tag, override := range cfg.Workflows {
			if override.StateMachine == nil {
				continue
			}
			tagWM, err := newWorkflowManagerFromRules(statuses, override.StateMachine.Rules)
			if err != nil {
				return nil, fmt.Errorf("workflow override for tag %q: %w", tag, err)
			}
			wm.workflows[tag] = tagWM
		}
	}

	return wm, nil
}

// newWorkflowManagerFromRules builds a WorkflowManager from status definitions and rules.
func newWorkflowManagerFromRules(statuses []config.StatusDefinition, rules map[string][]string) (*WorkflowManager, error) {
	wm := &WorkflowManager{
		statuses:    make(map[string]*config.StatusDefinition, len(statuses)),
		statusOrder: make([]string, 0, len(statuses)),
		rules:       rules,
		roleIndex:   make(map[string]string),
		markerIndex: make(map[string]string),
	}

	for i := range statuses {
		s := &statuses[i]
		wm.statuses[s.Name] = s
		wm.statusOrder = append(wm.statusOrder, s.Name)

		if s.Role != "" {
			if _, exists := wm.roleIndex[s.Role]; !exists {
				wm.roleIndex[s.Role] = s.Name
			}
		}
		if s.TLSMarker != "" {
			wm.markerIndex[s.TLSMarker] = s.Name
		}
	}

	if wm.rules == nil {
		wm.rules = defaultRules()
	}

	return wm, nil
}

// defaultRules returns the built-in transition rules.
func defaultRules() map[string][]string {
	return map[string][]string{
		"TODO":        {"IN_PROGRESS", "SKIPPED"},
		"IN_PROGRESS": {"DONE", "TODO", "SKIPPED"},
	}
}

// StateMachine returns a kit/domain StateMachine built from this
// WorkflowManager's rules. The result is cached after first call.
func (wm *WorkflowManager) StateMachine() *domain.StateMachine {
	return NewTaskStateMachineFromWorkflow(wm, nil)
}

// GetStatusDef returns the StatusDefinition for a given status.
func (wm *WorkflowManager) GetStatusDef(status TaskStatus) (*config.StatusDefinition, error) {
	def, ok := wm.statuses[string(status)]
	if !ok {
		return nil, fmt.Errorf("unknown status: %s", status)
	}
	return def, nil
}

// GetAllStatuses returns all status names in config-defined order.
func (wm *WorkflowManager) GetAllStatuses() []string {
	result := make([]string, len(wm.statusOrder))
	copy(result, wm.statusOrder)
	return result
}

// IsTerminal returns true if the status is terminal.
func (wm *WorkflowManager) IsTerminal(status TaskStatus) bool {
	def, ok := wm.statuses[string(status)]
	if !ok {
		return false
	}
	return def.IsTerminal
}

// StatusForRole returns the first status with the given role.
func (wm *WorkflowManager) StatusForRole(role string) (TaskStatus, error) {
	name, ok := wm.roleIndex[role]
	if !ok {
		return "", fmt.Errorf("no status found for role: %s", role)
	}
	return TaskStatus(name), nil
}

// StatusForTLSMarker returns the status name for a TLS marker character.
func (wm *WorkflowManager) StatusForTLSMarker(marker string) (TaskStatus, bool) {
	name, ok := wm.markerIndex[marker]
	if !ok {
		return "", false
	}
	return TaskStatus(name), true
}

// GetWorkflowForTags returns a tag-specific WorkflowManager if one matches,
// otherwise returns the receiver.
func (wm *WorkflowManager) GetWorkflowForTags(tags []string) *WorkflowManager {
	if wm.workflows == nil {
		return wm
	}
	for _, tag := range tags {
		if override, ok := wm.workflows[tag]; ok {
			return override
		}
	}
	return wm
}

var (
	defaultWorkflow     *WorkflowManager
	defaultWorkflowOnce sync.Once
)

// DefaultWorkflow returns a singleton WorkflowManager with default settings.
func DefaultWorkflow() *WorkflowManager {
	defaultWorkflowOnce.Do(func() {
		cfg := &config.TaskConfig{
			Statuses:     config.GetDefaultStatuses(),
			StateMachine: config.GetDefaultStateMachine(),
		}
		var err error
		defaultWorkflow, err = NewWorkflowManager(cfg)
		if err != nil {
			panic(fmt.Sprintf("failed to create default workflow: %v", err))
		}
	})
	return defaultWorkflow
}
