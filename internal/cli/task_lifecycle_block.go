package cli

// skip / block / unblock: the two lifecycle gaps that were previously
// reachable only through `task update`'s manual flags.
//
// The house rule is that status is COMMAND-managed, not set by hand:
// claim -> active, unclaim -> initial, complete -> completed,
// reopen -> initial. `skip` closes that set. Blocking is a different
// axis entirely and is treated as one — see the note on TaskBlockCmd.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// errNoSkippedStatus explains the one config shape `skip` cannot serve:
// a vocabulary with no status declaring role "skipped" and none named
// SKIPPED either. The message names the fix rather than guessing a
// target, because electing an arbitrary terminal status would move the
// user's task somewhere they never nominated.
func errNoSkippedStatus(statuses []string) error {
	return fmt.Errorf(
		"no skip target in this workflow: no status declares role %q and none is named %s "+
			"(declared statuses: %s); add `role: %s` to the status that means "+
			"\"finished, but not done\" in task.statuses, or use "+
			"'tlc task update <id> --status <status>' to set it explicitly",
		config.RoleSkipped, core.StatusCancelled, strings.Join(statuses, ", "), config.RoleSkipped,
	)
}

// resolveSkipTarget returns the skip status for wm, plus a warning line
// when the answer came from the name fallback instead of a declared
// role. The warning is deliberately loud: a config that reaches `skip`
// by name is one role declaration away from being unambiguous, and
// silence there is how a fallback becomes permanent.
func resolveSkipTarget(wm *core.WorkflowManager) (core.TaskStatus, string, error) {
	status, how := wm.SkippedStatus()
	switch how {
	case core.SkippedByRole:
		return status, "", nil
	case core.SkippedByName:
		return status, fmt.Sprintf(
			"Warning: no status declares role %q; falling back to the status named %s. "+
				"Add `role: %s` to it in task.statuses to make this explicit.\n",
			config.RoleSkipped, status, config.RoleSkipped,
		), nil
	default:
		return "", "", errNoSkippedStatus(wm.GetAllStatuses())
	}
}

// TaskCancelCmd is the canonical form of the old `skip`; `skip` stays
// as an alias so scripts keep working.
var TaskCancelCmd = &cobra.Command{
	Use:     "cancel <task-id|pattern>...",
	Aliases: []string{"skip"},
	Short:   "Cancel a task (alias: skip)",
	Long: `Transition one or more tasks to the workflow's cancelled status.
('skip' is an alias of 'cancel'.)

The target status is resolved by ROLE, not by name: whichever status
declares ` + "`role: skipped`" + ` in task.statuses is the one used, so a
custom vocabulary works without naming SKIPPED anywhere. When no status
declares that role, a status literally named SKIPPED is used as a
documented fallback and a warning is printed; when neither exists the
command refuses rather than electing an arbitrary terminal status.

Skipping is a state-machine transition like any other, so a config that
forbids it rejects the command. Use --no-verify for the same escape
hatch 'task complete' offers, or 'tlc task update --status --force'.
Re-running on an already-cancelled task converges.

Examples:
  tlc task cancel T-0042 --note "superseded by T-0050"
  tlc task skip T-0042 --note "not needed"        # same command
  tlc task cancel T-0042 --note "x" --dry-run     # preview only`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "yes",
	},
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()
		ctx := context.Background()

		wm := core.DefaultWorkflow()
		skippedStatus, warning, err := resolveSkipTarget(wm)
		if err != nil {
			return err
		}
		if warning != "" {
			_, _ = fmt.Fprint(cmd.OutOrStderr(), warning)
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

		var errs []string
		for _, res := range resolved {
			task := res.Task
			if res.Storage != s {
				defer func() { _ = res.Storage.Close() }()
			}

			user := core.GetCurrentUser()
			prevStatus := task.Status
			logEntry, transErr := transitionTask(ctx, res.Storage, task, skippedStatus, wm,
				transitionOpts{By: user, Note: taskSkipNote, Force: taskSkipNoVerify})
			if transErr != nil {
				errs = append(errs, fmt.Sprintf("%s: failed to transition task: %v", formatTaskAlias(task), transErr))
				continue
			}
			details := fmt.Sprintf("(%s → %s)", prevStatus, skippedStatus)
			appendAuditLog(task, user, "SKIPPED", details, taskSkipNote, task.UpdatedAt)

			if err := saveTaskWithLog(ctx, cmd, task, logEntry, res.Storage); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", formatTaskAlias(task), err))
				continue
			}
			if dryRunSkipsWrite(cmd) {
				printTaskDryRun(cmd, task, "skip",
					fmt.Sprintf("%s → %s", prevStatus, skippedStatus))
				continue
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Cancelled task %s\n", taskDisplayID(task))
		}
		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}
		if dryRunSkipsWrite(cmd) {
			// Nothing was written, so the projection already describes
			// the store; rewriting it would be a side effect of the
			// flag that suppresses side effects.
			return nil
		}
		return writeProjection()
	},
}

var TaskBlockCmd = &cobra.Command{
	Use:   "block <task-id|pattern>...",
	Short: "Record why a task is blocked",
	Long: `Record a blocking reason on one or more tasks.

When the workflow declares a status with role blocked (the default one
does), blocking moves the task there, validated like any transition, and
remembers the status it came from so 'tlc task unblock' can return it.
In a custom vocabulary with no blocked status, blocking only sets the
blocked reason and leaves the status untouched.

It is also distinct from the dependency graph. --add-blocked-by on
'task update' records that a task is blocked BY another task, a fact the
graph derives; this command records a free-text reason a human supplies.
A task can carry both at once and they are never merged.

Requires a reason via --note (or its --reason alias): a blocked task
with no stated reason tells a reader nothing they could not already see.
Re-running with the same reason converges.

Examples:
  tlc task block T-0042 --reason "waiting on vendor SLA"
  tlc task block T-0042 --reason "waiting on vendor SLA" --dry-run`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "yes",
	},
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		reason := taskBlockNote
		if reason == "" {
			return fmt.Errorf(
				"a reason is required; re-run with: tlc task block %s --note \"<reason>\"",
				args[0],
			)
		}

		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()
		ctx := context.Background()

		resolved, needsConfirm, err := resolveTaskIDs(ctx, args, s, core.Query{})
		if err != nil {
			return err
		}
		if needsConfirm {
			if err := confirmBatch(cmd, resolved, args[0]); err != nil {
				return err
			}
		}

		user := core.GetCurrentUser()
		wm := core.DefaultWorkflow()
		var errs []string
		for _, res := range resolved {
			task := res.Task
			if res.Storage != s {
				defer func() { _ = res.Storage.Close() }()
			}

			prevReason := ""
			if task.BlockedReason != nil {
				prevReason = *task.BlockedReason
			}

			prevStatus := task.Status
			blocked := reason
			task.UpdatedAt = time.Now().UTC()

			details := fmt.Sprintf("(blocked: %s)", reason)
			if prevReason != "" && prevReason != reason {
				details = fmt.Sprintf("(blocked reason changed from %q to %q)", prevReason, reason)
			}
			logEntry := &core.LogEntry{
				TaskID:    task.ID,
				Timestamp: task.UpdatedAt,
				By:        user,
				Action:    core.ActionBlocked,
				Note:      reason,
			}

			// With a blocked status in the vocabulary, blocking is a
			// status move: remember where the task came from, validate
			// the move, and let unblock return there. A task already in
			// the blocked status only has its reason replaced.
			if blockedStatus, bErr := wm.StatusForRole(roleBlocked); bErr == nil && task.Status != blockedStatus {
				entry, terr := transitionTask(ctx, res.Storage, task, blockedStatus, wm,
					transitionOpts{By: user, Reason: reason, Force: taskBlockNoVerify})
				if terr != nil {
					errs = append(errs, fmt.Sprintf("%s: failed to transition task: %v", formatTaskAlias(task), terr))
					continue
				}
				if task.Meta == nil {
					task.Meta = map[string]interface{}{}
				}
				task.Meta[metaBlockedFrom] = string(prevStatus)
				entry.Action = core.ActionBlocked
				entry.Note = fmt.Sprintf("Status changed from %s to %s: %s", prevStatus, blockedStatus, reason)
				logEntry = entry
				details = fmt.Sprintf("(%s → %s, blocked: %s)", prevStatus, blockedStatus, reason)
			}
			task.BlockedReason = &blocked
			appendAuditLog(task, user, "BLOCKED", details, reason, task.UpdatedAt)

			if err := saveTaskWithLog(ctx, cmd, task, logEntry, res.Storage); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", formatTaskAlias(task), err))
				continue
			}
			if dryRunSkipsWrite(cmd) {
				printTaskDryRun(cmd, task, "block", blockDryRunDetail(prevReason, reason))
				continue
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Blocked task %s\n", taskDisplayID(task))
		}
		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}
		if dryRunSkipsWrite(cmd) {
			return nil
		}
		return writeProjection()
	},
}

var TaskUnblockCmd = &cobra.Command{
	Use:   "unblock <task-id|pattern>...",
	Short: "Clear a task's blocked reason",
	Long: `Clear the blocked reason on one or more tasks.

Clears the reason set by 'tlc task block'. A task in the blocked status
returns to the status it was blocked from; if that move is no longer
allowed it goes to the initial status. Dependency edges are untouched. To drop a dependency edge use
'tlc task update --remove-blocked-by' instead.

--note is optional: the prior reason is already in the audit trail, so
unblocking carries its own explanation in a way blocking does not.
Re-running on an unblocked task converges.

Examples:
  tlc task unblock T-0042
  tlc task unblock T-0042 --note "vendor replied" --dry-run`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "yes",
	},
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()
		ctx := context.Background()

		resolved, needsConfirm, err := resolveTaskIDs(ctx, args, s, core.Query{})
		if err != nil {
			return err
		}
		if needsConfirm {
			if err := confirmBatch(cmd, resolved, args[0]); err != nil {
				return err
			}
		}

		user := core.GetCurrentUser()
		wm := core.DefaultWorkflow()
		var errs []string
		for _, res := range resolved {
			task := res.Task
			if res.Storage != s {
				defer func() { _ = res.Storage.Close() }()
			}

			prevReason := ""
			if task.BlockedReason != nil {
				prevReason = *task.BlockedReason
			}

			prevStatus := task.Status
			task.BlockedReason = nil
			task.UpdatedAt = time.Now().UTC()

			details := "(unblocked)"
			if prevReason != "" {
				details = fmt.Sprintf("(unblocked, was %q)", prevReason)
			}
			logEntry := &core.LogEntry{
				TaskID:    task.ID,
				Timestamp: task.UpdatedAt,
				By:        user,
				Action:    core.ActionBlocked,
				Note:      unblockLogNote(prevReason, taskUnblockNote),
			}

			// A task in the blocked status returns to the status it
			// came from; when that move is no longer allowed it falls
			// back to the initial status.
			if blockedStatus, bErr := wm.StatusForRole(roleBlocked); bErr == nil && task.Status == blockedStatus {
				back, terr := unblockTarget(ctx, res.Storage, task, wm)
				if terr != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", formatTaskAlias(task), terr))
					continue
				}
				entry, terr := transitionTask(ctx, res.Storage, task, back, wm,
					transitionOpts{By: user, Note: taskUnblockNote, Force: taskUnblockNoVerify})
				if terr != nil {
					errs = append(errs, fmt.Sprintf("%s: failed to transition task: %v", formatTaskAlias(task), terr))
					continue
				}
				delete(task.Meta, metaBlockedFrom)
				entry.Action = core.ActionBlocked
				entry.Note = fmt.Sprintf("Status changed from %s to %s: %s", prevStatus, back, logEntry.Note)
				logEntry = entry
				details = fmt.Sprintf("(%s → %s, unblocked)", prevStatus, back)
			}
			appendAuditLog(task, user, "UNBLOCKED", details, taskUnblockNote, task.UpdatedAt)

			if err := saveTaskWithLog(ctx, cmd, task, logEntry, res.Storage); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", formatTaskAlias(task), err))
				continue
			}
			if dryRunSkipsWrite(cmd) {
				printTaskDryRun(cmd, task, "unblock", unblockDryRunDetail(prevReason))
				continue
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Unblocked task %s\n", taskDisplayID(task))
		}
		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}
		if dryRunSkipsWrite(cmd) {
			return nil
		}
		return writeProjection()
	},
}

// unblockTarget picks where an unblocked task goes: the recorded
// pre-block status when the workflow still allows the move, else the
// initial status.
func unblockTarget(ctx context.Context, repo core.Repository, task *core.Task, wm *core.WorkflowManager) (core.TaskStatus, error) {
	if rec, ok := task.Meta[metaBlockedFrom].(string); ok && rec != "" {
		if canTransition(ctx, repo, task, core.TaskStatus(rec), wm) {
			return core.TaskStatus(rec), nil
		}
	}
	initial, err := wm.StatusForRole("initial")
	if err != nil {
		return "", fmt.Errorf("workflow has no initial status: %w", err)
	}
	return initial, nil
}

// blockDryRunDetail and unblockDryRunDetail phrase the reason change a
// preview would make, mirroring the audit-log details the real path
// records. Both name the PRIOR reason, which is the fact a reader cannot
// recover from the command line they just typed — and the fact that
// decides whether the real run sets a reason or replaces one.
func blockDryRunDetail(prevReason, reason string) string {
	if prevReason != "" && prevReason != reason {
		return fmt.Sprintf("reason changed from %q to %q", prevReason, reason)
	}
	return fmt.Sprintf("blocked: %s", reason)
}

func unblockDryRunDetail(prevReason string) string {
	if prevReason != "" {
		return fmt.Sprintf("clear reason %q", prevReason)
	}
	return "already unblocked"
}

// unblockLogNote composes the log note for an unblock, preserving the
// reason being cleared so the trail says what was resolved, not merely
// that something was.
func unblockLogNote(prevReason, note string) string {
	switch {
	case prevReason == "" && note == "":
		return "Unblocked"
	case note == "":
		return fmt.Sprintf("Unblocked (was: %s)", prevReason)
	case prevReason == "":
		return fmt.Sprintf("Unblocked: %s", note)
	default:
		return fmt.Sprintf("Unblocked (was: %s): %s", prevReason, note)
	}
}

func init() {
	TaskCancelCmd.Flags().StringVarP(&taskSkipNote, "note", "n", "", "Cancellation note (required where the workflow demands one)")
	TaskCancelCmd.Flags().BoolVar(&taskSkipNoVerify, "no-verify", false, "Skip state machine validation")

	TaskBlockCmd.Flags().StringVarP(&taskBlockNote, "note", "n", "", "Reason the task is blocked (required)")
	// --reason is the word a user reaches for here, and the field it
	// writes is literally called blocked_reason; --note is kept as the
	// name every other lifecycle command uses. Same variable, so the
	// two can never disagree.
	TaskBlockCmd.Flags().StringVar(&taskBlockNote, "reason", "", "Alias for --note")

	TaskBlockCmd.Flags().BoolVar(&taskBlockNoVerify, "no-verify", false, "Skip state machine validation")
	TaskUnblockCmd.Flags().StringVarP(&taskUnblockNote, "note", "n", "", "Unblock note (optional)")
	TaskUnblockCmd.Flags().BoolVar(&taskUnblockNoVerify, "no-verify", false, "Skip state machine validation")
}
