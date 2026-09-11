package core

import (
	"fmt"
	"strings"
	"time"
)

type ErrInvalidTransition struct {
	From    TaskStatus
	To      TaskStatus
	Msg     string
	Allowed []string // valid target statuses from From (nil = unknown/no rules)
}

func (e ErrInvalidTransition) Error() string {
	base := fmt.Sprintf("cannot transition task from %s to %s: %s", e.From, e.To, e.Msg)
	if len(e.Allowed) > 0 {
		return fmt.Sprintf(
			"%s; valid transitions from %s: %v; use --force to bypass",
			base, e.From, e.Allowed,
		)
	}
	return base + "; use --force to bypass the state machine"
}

// ValidateTransition checks if a status transition is allowed using the
// default workflow. Kept for backward compatibility; new code should use
// WorkflowManager.ValidateTransition directly.
func ValidateTransition(current, next TaskStatus) error {
	return DefaultWorkflow().ValidateTransition(current, next, false)
}

// TransitionWithWorkflow transitions the task using the given WorkflowManager
// and records a log entry. Set force=true to bypass transition rules.
//
// The workflow the task is actually validated against is resolved from
// the task's own tags, so a `task.workflows` override reaches every
// transition site through this one call rather than each caller
// remembering to consult it.
//
// Resolution happens even when force is set. --force bypasses the RULES,
// not the question of which rules apply, and a task whose tags match two
// overrides has no workflow to force past — reporting that is better than
// silently forcing under an arbitrary one.
func (t *Task) TransitionWithWorkflow(next TaskStatus, by string, note string, wm *WorkflowManager, force bool) (*LogEntry, error) {
	wm, err := wm.WorkflowForTask(t)
	if err != nil {
		return nil, err
	}
	if err := wm.ValidateTransition(t.Status, next, force); err != nil {
		return nil, err
	}
	oldStatus := t.Status
	t.Status = next
	t.UpdatedAt = time.Now().UTC()
	return &LogEntry{
		TaskID:    t.ID,
		Timestamp: t.UpdatedAt,
		By:        by,
		Action:    string(next),
		Note:      fmt.Sprintf("Status changed from %s to %s: %s", oldStatus, next, note),
	}, nil
}

// Transition transitions the task using the default workflow.
// Deprecated: use TransitionWithWorkflow for explicit workflow control.
func (t *Task) Transition(next TaskStatus, by string, note string) (*LogEntry, error) {
	return t.TransitionWithWorkflow(next, by, note, DefaultWorkflow(), false)
}

func (t *Task) NeedsPush() bool {
	if t.OriginSystem == nil || *t.OriginSystem == "" {
		return false
	}
	if t.LastSyncAt == nil {
		return true
	}
	// Use a small buffer to avoid jitter issues with time precision
	return t.UpdatedAt.After(t.LastSyncAt.Add(time.Millisecond))
}

// NormalizeBlockedBy converts supported blocked_by representations into a
// canonical ordered, deduplicated string slice.
func NormalizeBlockedBy(value interface{}) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return normalizeBlockedByStrings(parseBlockedByString(v))
	case []string:
		return normalizeBlockedByStrings(v)
	case []interface{}:
		items := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				items = append(items, s)
			}
		}
		return normalizeBlockedByStrings(items)
	default:
		return nil
	}
}

func parseBlockedByString(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		trimmed = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	}
	if trimmed == "" {
		return nil
	}

	parts := strings.Split(trimmed, ",")
	if len(parts) == 1 && strings.Contains(trimmed, " ") {
		parts = strings.Fields(trimmed)
	}
	return parts
}

func normalizeBlockedByStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// BlockedBy returns the task blockers as a canonical string slice.
func (t *Task) BlockedBy() []string {
	if t == nil || t.Meta == nil {
		return nil
	}
	return NormalizeBlockedBy(t.Meta["blocked_by"])
}

// SetBlockedBy updates blocked_by metadata, removing the key when empty.
func (t *Task) SetBlockedBy(ids []string) {
	if t == nil {
		return
	}

	normalized := normalizeBlockedByStrings(ids)
	if len(normalized) == 0 {
		if t.Meta != nil {
			delete(t.Meta, "blocked_by")
		}
		return
	}

	if t.Meta == nil {
		t.Meta = make(map[string]interface{})
	}
	t.Meta["blocked_by"] = normalized
}

// AddBlockedBy appends blocker IDs while preserving order and uniqueness.
func (t *Task) AddBlockedBy(ids []string) {
	combined := append(append([]string{}, t.BlockedBy()...), ids...)
	t.SetBlockedBy(combined)
}

// normalizeStringSliceMeta converts supported meta value representations
// into a canonical ordered, deduplicated string slice. Shared by
// BlockedBy, Eva, and any future string-slice meta fields.
func NormalizeStringSliceMeta(value interface{}) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return normalizeBlockedByStrings(parseBlockedByString(v))
	case []string:
		return normalizeBlockedByStrings(v)
	case []interface{}:
		items := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				items = append(items, s)
			}
		}
		return normalizeBlockedByStrings(items)
	default:
		return nil
	}
}

// Eva returns the task's eva annotations as a canonical string slice.
func (t *Task) Eva() []string {
	if t == nil || t.Meta == nil {
		return nil
	}
	return NormalizeStringSliceMeta(t.Meta["eva"])
}

// SetEva updates eva metadata, removing the key when empty.
func (t *Task) SetEva(values []string) {
	if t == nil {
		return
	}

	normalized := normalizeBlockedByStrings(values)
	if len(normalized) == 0 {
		if t.Meta != nil {
			delete(t.Meta, "eva")
		}
		return
	}

	if t.Meta == nil {
		t.Meta = make(map[string]interface{})
	}
	t.Meta["eva"] = normalized
}

// AddEva appends eva values while preserving order and uniqueness.
func (t *Task) AddEva(values []string) {
	combined := append(append([]string{}, t.Eva()...), values...)
	t.SetEva(combined)
}

// RemoveEva removes eva values from metadata.
func (t *Task) RemoveEva(values []string) {
	if t == nil {
		return
	}

	toRemove := make(map[string]struct{}, len(values))
	for _, v := range normalizeBlockedByStrings(values) {
		toRemove[v] = struct{}{}
	}
	if len(toRemove) == 0 {
		return
	}

	current := t.Eva()
	if len(current) == 0 {
		return
	}

	filtered := make([]string, 0, len(current))
	for _, v := range current {
		if _, ok := toRemove[v]; ok {
			continue
		}
		filtered = append(filtered, v)
	}
	t.SetEva(filtered)
}

// RemoveBlockedBy removes blocker IDs from blocked_by metadata.
func (t *Task) RemoveBlockedBy(ids []string) {
	if t == nil {
		return
	}

	toRemove := make(map[string]struct{}, len(ids))
	for _, id := range normalizeBlockedByStrings(ids) {
		toRemove[id] = struct{}{}
	}
	if len(toRemove) == 0 {
		return
	}

	current := t.BlockedBy()
	if len(current) == 0 {
		return
	}

	filtered := make([]string, 0, len(current))
	for _, id := range current {
		if _, ok := toRemove[id]; ok {
			continue
		}
		filtered = append(filtered, id)
	}
	t.SetBlockedBy(filtered)
}
