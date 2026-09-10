package core

import (
	"fmt"
	"os"
	"sync"

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

// ValidateTransition checks if a status transition is allowed.
func (wm *WorkflowManager) ValidateTransition(current, next TaskStatus, force bool) error {
	currentStr := string(current)
	nextStr := string(next)

	// Both statuses must be defined
	if _, ok := wm.statuses[currentStr]; !ok {
		return ErrInvalidTransition{From: current, To: next, Msg: fmt.Sprintf("unknown status: %s", currentStr)}
	}
	if _, ok := wm.statuses[nextStr]; !ok {
		return ErrInvalidTransition{From: current, To: next, Msg: fmt.Sprintf("unknown status: %s", nextStr)}
	}

	// Same status is always valid
	if current == next {
		return nil
	}

	// Terminal statuses cannot transition unless forced.
	if wm.IsTerminal(current) {
		if force {
			return nil
		}
		return ErrInvalidTransition{
			From: current,
			To:   next,
			Msg:  "terminal states are immutable; run 'tlc task reopen <id> --note \"<reason>\"' first",
		}
	}

	if force {
		return nil
	}

	// Check rules
	allowed, hasRule := wm.rules[currentStr]
	if !hasRule {
		return ErrInvalidTransition{
			From: current,
			To:   next,
			Msg:  "no transition rules defined for current status",
		}
	}

	for _, a := range allowed {
		if a == nextStr {
			return nil
		}
	}

	return ErrInvalidTransition{
		From:    current,
		To:      next,
		Msg:     "transition not allowed by state machine",
		Allowed: allowed,
	}
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

// SkippedResolution reports HOW SkippedStatus found its target, so the
// caller can tell the user when the answer came from a fallback rather
// than from their config.
type SkippedResolution int

const (
	// SkippedByRole: a status declares role "skipped". The only
	// unambiguous answer, and the one the built-in vocabulary gives.
	SkippedByRole SkippedResolution = iota
	// SkippedByName: no status declares the role, but a status is
	// literally named SKIPPED. Covers configs written against the
	// pre-role default set, which declared SKIPPED as role "completed".
	SkippedByName
	// SkippedUnresolved: neither, so there is no skip target.
	SkippedUnresolved
)

// SkippedStatus resolves the status that `tlc task skip` targets, and
// reports which rule produced it.
//
// Resolution is deliberately a short, ordered, DECLARED-INTENT-FIRST
// chain rather than a guess:
//
//  1. role "skipped" — the config-level concept. Preferred always.
//  2. a status literally named SKIPPED — a named fallback, and an
//     explicit one: it exists solely so configs predating the role keep
//     working, and callers surface it rather than applying it silently.
//
// Note what is deliberately NOT in the chain: "any terminal status that
// is not the completed one". That would silently elect WONTFIX, or
// CANCELLED, or whichever terminal status happened to be declared
// second, and a command that skips a task into a status the user never
// nominated is worse than a command that refuses. When both rules miss,
// this returns SkippedUnresolved and the caller errors with instructions
// to declare the role.
func (wm *WorkflowManager) SkippedStatus() (TaskStatus, SkippedResolution) {
	if name, ok := wm.roleIndex[config.RoleSkipped]; ok {
		return TaskStatus(name), SkippedByRole
	}
	if _, ok := wm.statuses[string(StatusSkipped)]; ok {
		return StatusSkipped, SkippedByName
	}
	return "", SkippedUnresolved
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
	defaultWorkflowErr  error
	defaultWorkflowOnce sync.Once

	// taskConfigProvider supplies the user's loaded `task` config section
	// to DefaultWorkflow. internal/core must not depend on viper or the
	// CLI, so the CLI registers the provider at init time and core calls
	// back into it. A nil provider (library consumers, most unit tests)
	// falls back to the built-in statuses and state machine.
	taskConfigProvider func() *config.TaskConfig
)

// SetTaskConfigProvider registers the source of the workflow's TaskConfig.
// It must be called before the first DefaultWorkflow call; the singleton
// caches whatever the provider returned on first use.
func SetTaskConfigProvider(fn func() *config.TaskConfig) {
	taskConfigProvider = fn
}

// builtinTaskConfig returns the built-in statuses and transition rules.
func builtinTaskConfig() *config.TaskConfig {
	return &config.TaskConfig{
		Statuses:     config.GetDefaultStatuses(),
		StateMachine: config.GetDefaultStateMachine(),
	}
}

// resolveTaskConfig returns the provider's TaskConfig, or the built-ins
// when no provider is registered or the provider yields nothing.
func resolveTaskConfig() *config.TaskConfig {
	if taskConfigProvider == nil {
		return builtinTaskConfig()
	}
	cfg := taskConfigProvider()
	if cfg == nil {
		return builtinTaskConfig()
	}
	return cfg
}

// DefaultWorkflow returns the process-wide WorkflowManager built from the
// user's configured statuses and state machine.
//
// Construction failure is fatal rather than a silent fall back to the
// built-in TODO/IN_PROGRESS/DONE/SKIPPED set: a user who declared custom
// statuses would otherwise get a workflow that quietly disagrees with
// their config. Callers that want to handle the error use DefaultWorkflowE.
func DefaultWorkflow() *WorkflowManager {
	wm, err := DefaultWorkflowE()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid task workflow configuration: %v\n", err)
		exit(1)
		return nil
	}
	return wm
}

// DefaultWorkflowE is DefaultWorkflow with the construction error returned
// instead of terminating the process.
func DefaultWorkflowE() (*WorkflowManager, error) {
	defaultWorkflowOnce.Do(func() {
		cfg := resolveTaskConfig()
		// Validate applies config defaults (statuses, state machine) and
		// rejects rules referencing undeclared statuses — the failure mode
		// NewWorkflowManager alone does not catch.
		if err := cfg.Validate(); err != nil {
			defaultWorkflowErr = err
			return
		}
		defaultWorkflow, defaultWorkflowErr = NewWorkflowManager(cfg)
	})
	return defaultWorkflow, defaultWorkflowErr
}

// exit is os.Exit, indirected so tests can observe the fatal path.
var exit = os.Exit

// ResetDefaultWorkflow clears the cached workflow singleton so the next
// DefaultWorkflow call rebuilds it from the current provider. Tests only.
func ResetDefaultWorkflow() {
	defaultWorkflowOnce = sync.Once{}
	defaultWorkflow = nil
	defaultWorkflowErr = nil
}
