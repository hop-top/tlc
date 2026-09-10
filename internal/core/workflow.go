package core

import (
	"fmt"
	"os"
	"slices"
	"strings"
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

	// defaultStatus is the user's `task.default_status`, empty when
	// unset. Read by InitialStatus; carried here so the CLI resolves the
	// create default through the same workflow it validates against,
	// rather than opening a second config channel into internal/core.
	defaultStatus string

	// overrideTag is the tag this manager was built for, empty on the
	// base workflow. Carried so error messages can name the override the
	// user is actually being judged against.
	overrideTag string

	// base is the workflow an override replaces, nil on the base
	// workflow itself. Only the stranded-status fallback reads it.
	base *WorkflowManager
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
	wm.defaultStatus = cfg.DefaultStatus

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
			tagWM.defaultStatus = cfg.DefaultStatus
			// overrideTag keeps the spelling the config author wrote,
			// because it is what error messages quote back at them; the
			// MAP key is folded, because it is what tags are matched
			// against.
			tagWM.overrideTag = tag
			// base is the workflow this override replaces. Kept so a
			// transition out of a status the override left unruled can
			// fall back rather than strand the task; see
			// ValidateTransition.
			tagWM.base = wm
			wm.workflows[workflowTagKey(tag)] = tagWM
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
		// An override REPLACES the base state machine rather than
		// merging with it: WorkflowOverride carries only `state_machine`,
		// so a rule set that lists TODO and omits IN_PROGRESS reads as
		// "these are the transitions", not "these plus the defaults".
		//
		// Replace has one failure mode worth catching, though. A task
		// already sitting in a status the override left unruled would be
		// stranded there — every transition out of it rejected, with the
		// only escape --force — even though the user's base workflow
		// says exactly where it may go. That is not a rule the user
		// wrote; it is a gap they left. So the base workflow answers for
		// statuses the override does not mention, and only for those:
		// any status the override DOES rule is governed wholly by the
		// override, including the transitions it deliberately omits.
		if wm.base != nil {
			if _, baseHasRule := wm.base.rules[currentStr]; baseHasRule {
				return wm.base.ValidateTransition(current, next, force)
			}
		}
		return ErrInvalidTransition{
			From: current,
			To:   next,
			Msg:  wm.ruleContext("no transition rules defined for current status"),
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
		Msg:     wm.ruleContext("transition not allowed by state machine"),
		Allowed: allowed,
	}
}

// ruleContext names the override a rejection came from, so a user whose
// transition was refused by a tag-specific state machine is not left
// reading it against the base one.
func (wm *WorkflowManager) ruleContext(msg string) string {
	if wm.overrideTag == "" {
		return msg
	}
	return fmt.Sprintf("%s for tag %q", msg, wm.overrideTag)
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

// InitialStatus resolves the status a newly created task lands in when
// the caller nominated none.
//
// Order is `task.default_status` first, the `initial`-role status second.
// The config value wins because it is the only way a user can pick
// between two statuses that both declare role "initial"; the role is the
// fallback because `default_status` is optional and most configs omit it.
//
// A default_status naming an undeclared status is ignored rather than
// honored: config validation already warns about it, and resolving to a
// status the workflow does not know would fail later with a message
// pointing at create rather than at the config.
func (wm *WorkflowManager) InitialStatus() (TaskStatus, error) {
	if wm.defaultStatus != "" {
		if _, ok := wm.statuses[wm.defaultStatus]; ok {
			return TaskStatus(wm.defaultStatus), nil
		}
	}
	return wm.StatusForRole(config.RoleInitial)
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

// ErrAmbiguousWorkflow reports a task whose tags match more than one
// `task.workflows` override, so no single state machine can be said to
// govern it.
type ErrAmbiguousWorkflow struct {
	// Tags are the matching tags, sorted, so the message is stable.
	Tags []string
}

func (e ErrAmbiguousWorkflow) Error() string {
	return fmt.Sprintf(
		"task tags match %d workflow overrides (%s); a task may carry at "+
			"most one tag declared under task.workflows — remove all but "+
			"one from the task, or drop the surplus override from config",
		len(e.Tags), strings.Join(e.Tags, ", "),
	)
}

// workflowTagKey folds one tag to the spelling `task.workflows` is
// keyed and looked up by.
//
// Case-insensitive, because that is what a tag already means everywhere
// else: TagVocabulary.Admits lowercases before matching, so a project
// with a closed policy accepts `Hotfix`, `hotfix` and `HOTFIX` as the
// one tag. An override keyed by exact spelling would have made the two
// tag subsystems disagree — a tag the policy admits, silently governed
// by no workflow.
//
// This cannot be done at decode time instead. viper lowercases map keys
// on its own, so a `Hotfix:` override arrives here as `hotfix` with the
// author's casing already gone and nothing to re-case it against: unlike
// statuses and priorities, tags are free-form user data with no declared
// vocabulary. Folding at the map boundary also covers the callers that
// never touch viper — NewWorkflowManager is exported and takes a
// TaskConfig built by hand.
//
// Folding is the only transformation: `hotfix` and `hot-fix` stay two
// distinct keys, so this collapses spellings of one tag and never two
// different tags.
func workflowTagKey(tag string) string {
	return strings.ToLower(strings.TrimSpace(tag))
}

// GetWorkflowForTags returns the workflow that governs a task carrying
// these tags: the override for its single matching tag, or the receiver
// when no tag matches.
//
// Matching MORE than one override is an error rather than a pick.
//
// The alternative rules all reduce to choosing a winner the user never
// nominated. First-match is what this used to do, and it was not even
// deterministic: a task's tags are rebuilt by ranging a map, so a task
// tagged both `hotfix` and `experimental` got either workflow depending
// on the run. Declaration order is unavailable — the decoder lands
// config in a Go map, which has none. Lexicographic order is stable but
// meaningless: `alpha` beating `hotfix` encodes nothing the user
// intended. Refusing keeps the property that matters — the workflow a
// task is validated against is one the user can point at in config — and
// says exactly which tags to disambiguate.
//
// Tags matching no override are ignored, so the common case (one
// workflow tag plus any number of ordinary labels) is unaffected.
func (wm *WorkflowManager) GetWorkflowForTags(tags []string) (*WorkflowManager, error) {
	if len(wm.workflows) == 0 {
		return wm, nil
	}

	var matched []string
	for _, tag := range tags {
		if _, ok := wm.workflows[workflowTagKey(tag)]; ok {
			matched = append(matched, workflowTagKey(tag))
		}
	}

	// Dedupe: a task carrying the same tag twice is not ambiguous.
	slices.Sort(matched)
	matched = slices.Compact(matched)

	switch len(matched) {
	case 0:
		return wm, nil
	case 1:
		return wm.workflows[matched[0]], nil
	default:
		return nil, ErrAmbiguousWorkflow{Tags: matched}
	}
}

// WorkflowForTask returns the workflow governing one task, resolved from
// its tags. The single entry point transition sites use, so per-tag
// overrides cannot be honored on some paths and skipped on others.
func (wm *WorkflowManager) WorkflowForTask(t *Task) (*WorkflowManager, error) {
	if t == nil {
		return wm, nil
	}
	return wm.GetWorkflowForTags(t.Tags)
}

// OverrideTag reports the tag whose override produced this workflow, and
// whether one did at all. Callers use it to name the state machine a
// rejection came from.
func (wm *WorkflowManager) OverrideTag() (string, bool) {
	return wm.overrideTag, wm.overrideTag != ""
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
		// ValidateWorkflow applies config defaults (statuses, state
		// machine) and rejects rules referencing undeclared statuses —
		// the failure mode NewWorkflowManager alone does not catch.
		//
		// ValidateWorkflow, not Validate: failure here is FATAL, so this
		// path must check only what genuinely makes a workflow
		// unbuildable. The advisory checks Validate adds on top — a
		// stale `task.scheduling.by_priority` key, say — belong on the
		// CLI's warning-only path, not on the one that stops every
		// command in the tool from running.
		if err := cfg.ValidateWorkflow(); err != nil {
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
