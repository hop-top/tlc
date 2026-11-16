package core

import (
	"fmt"
	"strings"

	"hop.top/tlc/internal/config"
)

// TransitionContext carries everything a transition needs to be judged:
// the move itself plus the facts the named guard checks read. Core stays
// storage-free, so the caller resolves blocker statuses and passes them.
type TransitionContext struct {
	From TaskStatus
	To   TaskStatus
	// Note is the transition note; read by the "note" check.
	Note string
	// Reason is the block reason; read by the "reason" check.
	Reason string
	// Assignee is the task's assignee AFTER the transition; read by the
	// "assignee" check. Empty means unassigned.
	Assignee string
	// BlockedBy lists the task's blocked_by IDs.
	BlockedBy []string
	// Blockers maps blocked_by ID to its current status. An ID missing
	// from the map (cross-project, deleted) counts as still open.
	Blockers map[string]TaskStatus
	// Force bypasses rules and guards; the bypass is reported through
	// WorkflowManager.CheckTransition's forced result.
	Force bool
}

// NewTransitionContext builds the context for moving t to next, filling
// From, Assignee and BlockedBy from the task. The caller sets Note,
// Reason and Blockers (and Assignee when the move assigns one).
func NewTransitionContext(t *Task, next TaskStatus) TransitionContext {
	tc := TransitionContext{To: next}
	if t == nil {
		return tc
	}
	tc.From = t.Status
	if t.AssignedTo != nil {
		tc.Assignee = *t.AssignedTo
	}
	tc.BlockedBy = t.BlockedBy()
	return tc
}

// ErrGuardRefused reports a transition the rules allow but a guard
// refuses. It names the transition, the guard and the failed check.
type ErrGuardRefused struct {
	From   TaskStatus
	To     TaskStatus
	Guard  config.GuardDefinition
	Check  string
	Detail string
}

func (e ErrGuardRefused) Error() string {
	return fmt.Sprintf(
		"%s → %s refused by guard %s: %s: %s; use --force to bypass",
		e.From, e.To, guardLabel(e.Guard), e.Check, e.Detail,
	)
}

// guardLabel renders a guard's selector, e.g. "[to: done]".
func guardLabel(g config.GuardDefinition) string {
	var parts []string
	if g.From != "" {
		parts = append(parts, "from: "+g.From)
	}
	if g.To != "" {
		parts = append(parts, "to: "+g.To)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// CheckTransition judges a transition against the rules and then the
// guards. With Force set it never fails; forced reports whether force
// actually bypassed a refusal (and bypassed is that refusal), so callers
// can record the bypass in the audit entry.
func (wm *WorkflowManager) CheckTransition(tc TransitionContext) (forced bool, bypassed error, err error) {
	refusal := wm.checkRulesAndGuards(tc)
	if refusal == nil {
		return false, nil, nil
	}
	if tc.Force {
		return true, refusal, nil
	}
	return false, nil, refusal
}

// ValidateTransitionContext is CheckTransition without the forced result.
func (wm *WorkflowManager) ValidateTransitionContext(tc TransitionContext) error {
	_, _, err := wm.CheckTransition(tc)
	return err
}

func (wm *WorkflowManager) checkRulesAndGuards(tc TransitionContext) error {
	if err := wm.ValidateTransition(tc.From, tc.To, false); err != nil {
		return err
	}
	if tc.From == tc.To {
		return nil
	}
	for _, g := range wm.guards {
		if !guardMatches(g, tc) {
			continue
		}
		for _, check := range g.Require {
			if detail := runGuardCheck(check, tc, wm); detail != "" {
				return ErrGuardRefused{
					From: tc.From, To: tc.To, Guard: g, Check: check, Detail: detail,
				}
			}
		}
	}
	return nil
}

func guardMatches(g config.GuardDefinition, tc TransitionContext) bool {
	if g.From != "" && g.From != string(tc.From) {
		return false
	}
	if g.To != "" && g.To != string(tc.To) {
		return false
	}
	return true
}

// runGuardCheck returns "" when the check passes, else why it failed.
func runGuardCheck(check string, tc TransitionContext, wm *WorkflowManager) string {
	switch check {
	case config.CheckNote:
		if strings.TrimSpace(tc.Note) == "" {
			return "a note is required"
		}
	case config.CheckReason:
		if strings.TrimSpace(tc.Reason) == "" {
			return "a reason is required"
		}
	case config.CheckAssignee:
		if strings.TrimSpace(tc.Assignee) == "" {
			return "task has no assignee"
		}
	case config.CheckBlockersClear:
		var open []string
		for _, id := range tc.BlockedBy {
			st, ok := tc.Blockers[id]
			if !ok || !wm.IsTerminal(st) {
				open = append(open, id)
			}
		}
		if len(open) == 1 {
			return open[0] + " is still open"
		}
		if len(open) > 1 {
			return strings.Join(open, ", ") + " are still open"
		}
	}
	return ""
}
