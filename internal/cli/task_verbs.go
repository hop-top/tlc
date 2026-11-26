package cli

// Status verbs layered on the workflow's roles: submit, and the
// non-human halves of approve/reject. Block/unblock live in
// task_lifecycle_block.go.
//
// Every verb resolves its target by ROLE and moves the task through
// transitionTask, so no verb writes a status directly and the
// workflow's validation is the single gate.

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// Roles the verbs resolve. Roles are advisory strings in config; these
// are the ones the default workflow declares.
const (
	roleReview  = "review"
	roleBlocked = "blocked"
)

var (
	taskSubmitNote     string
	taskSubmitNoVerify bool
)

// metaBlockedFrom is the task-meta key holding the status a task held
// when `block` moved it to the blocked status. `unblock` returns there.
// It lives in meta, next to the block reason's lifetime: set on block,
// removed on unblock, no schema change.
const metaBlockedFrom = "blocked_from"

// transitionOpts carries what a transition's validation may need.
type transitionOpts struct {
	By     string
	Note   string
	Reason string
	Force  bool
}

// transitionTask is the one place verbs change status. It builds the
// full transition context (note, reason, assignee after the move, and
// the resolved status of every blocker), then validates rules and
// guards through the task's workflow and applies the move.
func transitionTask(ctx context.Context, repo core.Repository, task *core.Task, to core.TaskStatus, wm *core.WorkflowManager, o transitionOpts) (*core.LogEntry, error) {
	tc := transitionContext(ctx, repo, task, to, o)
	entry, err := task.TransitionWithContext(tc, o.By, wm)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	return entry, nil
}

// transitionContext fills a core.TransitionContext for moving task to `to`.
func transitionContext(ctx context.Context, repo core.Repository, task *core.Task, to core.TaskStatus, o transitionOpts) core.TransitionContext {
	tc := core.NewTransitionContext(task, to)
	tc.Note = o.Note
	tc.Reason = o.Reason
	tc.Force = o.Force
	tc.Blockers, tc.BlockerLabels = resolveBlockers(ctx, repo, task)
	return tc
}

// resolveBlockers maps each blocked_by ref of task to its current
// status. A ref that cannot be resolved (deleted, other project) is left
// out, which the guard reads as still open. The second result maps each
// resolved ref to its display ID, so a refusal names the blocker the way
// the human view does.
func resolveBlockers(ctx context.Context, repo core.Repository, task *core.Task) (map[string]core.TaskStatus, map[string]string) {
	refs := task.BlockedBy()
	if len(refs) == 0 || repo == nil {
		return nil, nil
	}
	sq, ok := repo.(*storage.SQLiteStorage)
	if !ok {
		sq = nil
	}
	out := make(map[string]core.TaskStatus, len(refs))
	labels := make(map[string]string, len(refs))
	for _, ref := range refs {
		lookup := ref
		if sq != nil {
			if translated, err := parseTaskRefForCLI(ctx, sq, ref); err == nil && translated != "" {
				lookup = translated
			}
		}
		if b, err := repo.GetTask(ctx, lookup); err == nil && b != nil {
			out[ref] = b.Status
			if alias := formatTaskAlias(b); alias != "" {
				labels[ref] = alias
			}
		}
	}
	return out, labels
}

// canTransition reports whether task may move to `to` without force,
// rules and guards included.
func canTransition(ctx context.Context, repo core.Repository, task *core.Task, to core.TaskStatus, wm *core.WorkflowManager) bool {
	twm, err := wm.WorkflowForTask(task)
	if err != nil {
		return false
	}
	return twm.ValidateTransitionContext(transitionContext(ctx, repo, task, to, transitionOpts{})) == nil
}

// move describes one role-targeted verb run over a set of tasks.
type move struct {
	verb      string // dry-run label and error prefix
	past      string // success line prefix ("Submitted")
	audit     string // audit-log action
	logAction string // log-entry action override; empty keeps the status
	toRole    string
	fromRole  string // when set, the task must currently hold this role
	note      string
	reason    string
	by        string
	force     bool
	needNote  bool // refuse an empty note up front
	hint      string
}

// runMove applies mv to every task args resolves to.
func runMove(cmd *cobra.Command, args []string, s *storage.SQLiteStorage, mv move) error {
	ctx := cmdContext(cmd)
	if mv.needNote && mv.note == "" {
		return errNoteRequired(mv.hint)
	}
	wm := core.DefaultWorkflow()
	to, err := wm.StatusForRole(mv.toRole)
	if err != nil {
		return fmt.Errorf("workflow has no %s status; %s needs one: %w", mv.toRole, mv.verb, err)
	}
	var from core.TaskStatus
	if mv.fromRole != "" {
		if from, err = wm.StatusForRole(mv.fromRole); err != nil {
			return fmt.Errorf("workflow has no %s status; %s needs one: %w", mv.fromRole, mv.verb, err)
		}
	}

	resolved, needsConfirm, err := resolveTaskIDs(ctx, args, s, core.Query{})
	if err != nil {
		return err
	}
	if needsConfirm {
		if err := confirmBatch(cmd, resolved, args[0]); err != nil {
			return err
		}
	}

	by := mv.by
	if by == "" {
		by = core.GetCurrentUser()
	}
	var errs []string
	for _, res := range resolved {
		task := res.Task
		if res.Storage != s {
			defer func() { _ = res.Storage.Close() }()
		}
		alias := formatTaskAlias(task)
		prev := task.Status
		if mv.fromRole != "" && !mv.force && task.Status != from {
			errs = append(errs, fmt.Sprintf("%s: cannot %s from %s: only a task in %s can be %s",
				alias, mv.verb, task.Status, from, strings.ToLower(mv.past)))
			continue
		}
		logNote := mv.note
		if logNote == "" {
			logNote = mv.reason
		}
		entry, terr := transitionTask(ctx, res.Storage, task, to, wm, transitionOpts{By: by, Note: logNote, Reason: mv.reason, Force: mv.force})
		if terr != nil {
			errs = append(errs, fmt.Sprintf("%s: failed to transition task: %v", alias, terr))
			continue
		}
		if mv.logAction != "" {
			entry.Action = mv.logAction
		}
		note := mv.note
		if note == "" {
			note = mv.reason
		}
		appendAuditLog(task, by, mv.audit, fmt.Sprintf("(%s → %s)", prev, to), note, task.UpdatedAt)

		if err := saveTaskWithLog(ctx, cmd, task, entry, res.Storage); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", alias, err))
			continue
		}
		if dryRunSkipsWrite(cmd) {
			printTaskDryRun(cmd, task, mv.verb, fmt.Sprintf("%s → %s", prev, to))
			continue
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s task %s\n", mv.past, taskDisplayID(task))
	}
	if len(errs) > 0 {
		return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
	}
	if dryRunSkipsWrite(cmd) {
		return nil
	}
	return writeProjection()
}

// TaskSubmitCmd moves active work to review.
var TaskSubmitCmd = &cobra.Command{
	Use:   "submit <task-id|pattern>...",
	Short: "Submit a task for review",
	Long: `Move one or more tasks from the active status to the review status.

The target is resolved by role (review). The workflow's rules and
guards decide whether the move is allowed; the default workflow
requires the task to have an assignee. Use --no-verify to bypass the
rules, as 'task complete' offers. Reviewers then use 'tlc task approve'
or 'tlc task reject --reason'.

Examples:
  tlc task submit T-0042
  tlc task submit T-0042 --note "ready for review"
  tlc task submit T-0042 --dry-run`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "no",
	},
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()
		return runMove(cmd, args, s, move{
			verb: "submit", past: "Submitted", audit: "SUBMITTED",
			toRole: roleReview, note: taskSubmitNote, force: taskSubmitNoVerify,
		})
	},
}

func init() {
	TaskSubmitCmd.Flags().StringVarP(&taskSubmitNote, "note", "n", "", "Submission note")
	TaskSubmitCmd.Flags().BoolVar(&taskSubmitNoVerify, "no-verify", false, "Skip state machine validation")
}
