package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"hop.top/kit/go/console/output"
	"hop.top/kit/go/runtime/bus"
	"hop.top/kit/go/runtime/domain"
	"hop.top/kit/go/runtime/policy"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

var TaskUpdateCmd = &cobra.Command{
	Use:   "update <task-id|pattern>...",
	Short: "Update task fields",
	Long: `Update one or more task fields (title, description, status,
assignee, effort, priority, tags, blocked-by, scheduling, eva, etc.).

Use --amend together with --note to rewrite the most recent log
entry instead of appending a new one. Status transitions are validated
against the workflow state machine unless --force is set.`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
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

		if cmd.Flags().Changed("title") && len(resolved) > 1 {
			return fmt.Errorf("--title can only be set when updating a single task")
		}

		// --amend rewrites the most recent log entry in place rather
		// than appending a new one. It is intentionally scoped to
		// --note edits to avoid side effects from re-firing
		// state-machine transitions; status amends would need partial
		// rollback of the previous transition, which is too risky for
		// CLI sugar. See T-0865.
		if cmd.Flags().Changed("amend") && taskUpdateAmend {
			if cmd.Flags().Changed("status") {
				return fmt.Errorf("--amend with --status is not supported; status transitions cannot be safely amended (would skip workflow side effects); re-run without --amend to record a new transition")
			}
			if !cmd.Flags().Changed("note") {
				return fmt.Errorf("--amend requires --note; re-run with: tlc task update <id> --amend --note \"<new note>\"")
			}
			var amendErrs []string
			for _, res := range resolved {
				task := res.Task
				if res.Storage != s {
					defer func() { _ = res.Storage.Close() }()
				}
				if err := amendLatestLogNote(ctx, res.Storage, task, taskUpdateNote, taskUpdateForce); err != nil {
					amendErrs = append(amendErrs, fmt.Sprintf("%s: %v", formatTaskAlias(task), err))
					continue
				}
				fmt.Printf("Amended latest log for %s\n", formatTaskAlias(task))
			}
			if len(amendErrs) > 0 {
				return fmt.Errorf("some tasks failed:\n%s", strings.Join(amendErrs, "\n"))
			}
			return nil
		}

		var errs []string

		for _, res := range resolved {
			task := res.Task
			if res.Storage != s {
				defer func() { _ = res.Storage.Close() }()
			}

			changes := TaskFieldChanges{
				ClearBlockedBy: taskUpdateClearBlockedBy,
				ClearEva:       taskUpdateClearEva,
				AddEva:         taskUpdateAddEva,
				RemoveEva:      taskUpdateRemoveEva,
				AddBlockedBy:   taskUpdateAddBlockedBy,
				AddTags:        taskUpdateAddTags,
				RemoveTags:     taskUpdateRemoveTags,
				Unblock:        cmd.Flags().Changed("unblock") && taskUpdateUnblock,
				AutoCreateTrack: func(ctx context.Context, s *storage.SQLiteStorage, input string) (string, error) {
					return maybeAutoCreateTrack(ctx, cmd, s, input)
				},
			}
			if cmd.Flags().Changed("title") {
				changes.Title = &taskUpdateTitle
			}
			if cmd.Flags().Changed("description") {
				changes.Description = &taskUpdateDescription
			}
			if cmd.Flags().Changed("assigned-to") {
				changes.AssignedTo = &taskUpdateAssignedTo
			}
			if cmd.Flags().Changed("status") {
				changes.Status = &taskUpdateStatus
				changes.StatusNote = taskUpdateNote
				changes.StatusForce = taskUpdateForce
			}
			if cmd.Flags().Changed("effort") {
				changes.Effort = &taskUpdateEffort
			}
			if cmd.Flags().Changed("priority") {
				changes.Priority = &taskUpdatePriority
			}
			if len(taskUpdateRemoveBlockedBy) > 0 {
				changes.RemoveBlockedBy = taskUpdateRemoveBlockedBy
			}
			if cmd.Flags().Changed("blocked") {
				changes.BlockedReason = &taskUpdateBlocked
			}
			if cmd.Flags().Changed("timeout") {
				changes.Timeout = &taskUpdateTimeout
			}
			if cmd.Flags().Changed("track") {
				changes.Track = &taskUpdateTrack
			}
			if cmd.Flags().Changed("due") {
				changes.Due = &taskUpdateDue
			}
			if cmd.Flags().Changed("remind-at") {
				changes.RemindAt = &taskUpdateRemindAt
			}
			if cmd.Flags().Changed("rrule") {
				changes.RRule = &taskUpdateRRule
			}
			if cmd.Flags().Changed("no-auto-remind") {
				changes.NoAutoRemind = &taskUpdateNoAutoRemind
			}
			if cmd.Flags().Changed("note") {
				changes.Note = taskUpdateNote
				changes.NoteFields = editedFieldNames(cmd)
			}

			changed, err := applyTaskFieldChanges(ctx, s, res.Storage, task, changes)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", formatTaskAlias(task), err))
				continue
			}
			if !changed {
				continue
			}

			if task.OriginSystem != nil && *task.OriginSystem != "" {
				// Mirror to the origin system as a best-effort side
				// effect. Sync failure surfaces as a warning but does
				// not roll back local truth (T-0750).
				if err := pushSyncedTask(ctx, task, res.Storage); err != nil {
					_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Warning: %v (local state saved; sync needs retry)\n", err)
				}
			} else {
				fmt.Printf("Updated task %s\n", formatTaskAlias(task))
			}
		}

		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}

		if len(resolved) > 0 {
			return writeProjection()
		}
		fmt.Println("No changes specified; use --title, --description, --status, --assigned-to, or other flags to update")
		return nil
	},
}

var TaskDeleteCmd = &cobra.Command{
	Use:   "delete <task-id|pattern>...",
	Short: "Delete a task",
	Long: `Delete one or more tasks from the local store.

Prompts for confirmation unless --yes or --no-prompt is set. Honours
the synchronous policy gate (kit/runtime/policy pre_persisted topic)
so configured note-required or block-on-status rules veto the delete
before any row is removed.`,
	Annotations: map[string]string{
		"kit/side-effect": "destructive-local",
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

		// For delete: confirm if multiple targets unless --yes or --no-prompt.
		if (needsConfirm || len(resolved) > 1) && !taskDeleteYes && !taskNoPrompt {
			if err := confirmBatch(cmd, resolved, strings.Join(args, " ")); err != nil {
				return err
			}
		} else if len(resolved) == 1 && !taskDeleteYes && !taskNoPrompt {
			task := resolved[0].Task
			if !deletePromptInteractive(cmd) {
				return errDeleteRequiresYes(formatTaskAlias(task))
			}
			var confirm bool
			err := huh.NewConfirm().
				Title(fmt.Sprintf("Delete task %s (%s)?", formatTaskAlias(task), task.Title)).
				Description("This action cannot be undone.").
				Value(&confirm).
				Run()
			if err != nil {
				return fmt.Errorf("failed to run confirm dialog: %w", err)
			}
			if !confirm {
				return fmt.Errorf("delete aborted; task %s was not deleted", formatTaskAlias(task))
			}
		}

		// Attach --note to ctx so policy.Engine sees it as
		// `context.note` when evaluating the pre_persisted veto. Empty
		// string is fine — the default delete-requires-note policy
		// denies the empty case explicitly.
		policyCtx := context.WithValue(ctx, policy.ContextAttrsKey, map[string]any{
			"note": taskDeleteNote,
		})

		var errs []string
		for _, res := range resolved {
			task := res.Task
			if res.Storage != s {
				defer func() { _ = res.Storage.Close() }()
			}

			var assignedTo string
			if task.AssignedTo != nil {
				assignedTo = *task.AssignedTo
			}
			valCfg := getValidationConfig()
			if err := valCfg.ValidateTaskOp(config.ValidationOpDelete, config.TaskFields{
				Title:       task.Title,
				Description: task.Description,
				Status:      string(task.Status),
				AssignedTo:  assignedTo,
				Effort:      string(task.Effort),
				Priority:    string(task.Priority),
				Tags:        task.Tags,
				Reference:   task.Reference,
			}); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", formatTaskAlias(task), err))
				continue
			}

			// Synchronous policy gate (T-1192). Publish the kit
			// pre_persisted topic with Op=delete; any sync subscriber
			// (the kit/runtime/policy engine wired in PersistentPreRunE)
			// vetoes by returning an error. This mirrors what
			// domain.Service[T].Delete would do internally — tlc's
			// CLI delete bypasses domain.Service for now (T-1232 left
			// the direct repo path intact), so the gate has to live
			// here until the CLI is restructured.
			if err := publishDeletePrePersisted(policyCtx, task.ID); err != nil {
				if denied := policyAsCLIError(err); denied != nil {
					return denied
				}
				errs = append(errs, fmt.Sprintf("%s: %v", formatTaskAlias(task), err))
				continue
			}

			if task.OriginSystem != nil && *task.OriginSystem != "" {
				if err := deleteSyncedTask(policyCtx, task, res.Storage); err != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", formatTaskAlias(task), err))
					continue
				}
			}

			// Record the deletion note against the transition log
			// before the task row is removed. T-1232 dropped the
			// task_logs cascade, so the entry survives the delete
			// for audit / policy review.
			if taskDeleteNote != "" {
				logEntry := &core.LogEntry{
					TaskID:    task.ID,
					Timestamp: time.Now().UTC(),
					By:        core.GetCurrentUser(),
					Action:    core.ActionDeleted,
					Note:      taskDeleteNote,
				}
				if err := res.Storage.AddLog(policyCtx, logEntry); err != nil {
					_, _ = fmt.Fprintf(cmd.OutOrStderr(),
						"Warning: failed to write delete log for %s: %v\n", formatTaskAlias(task), err)
				}
			}

			alias := formatTaskAlias(task)
			if err := res.Storage.DeleteTask(policyCtx, task.ID); err != nil {
				errs = append(errs, fmt.Sprintf("%s: failed to delete: %v", alias, err))
				continue
			}
			fmt.Printf("Deleted task %s\n", alias)
		}

		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}
		return writeProjection()
	},
}

// publishDeletePrePersisted fires kit.runtime.entity.pre_persisted with
// Op=delete on the process bus.  Any sync subscriber (the policy
// engine) can veto by returning an error.  When the bus is not yet
// initialised (e.g. unit tests that bypass PersistentPreRunE), the
// publish is skipped and nil returned — adopters who want enforcement
// in tests should construct a bus and wire policy themselves.
func publishDeletePrePersisted(ctx context.Context, taskID string) error {
	b := GetEventBus()
	if b == nil {
		return nil
	}
	payload := domain.PreEntityPayload{
		Op:       domain.OpDelete,
		Phase:    domain.PhasePrePersisted,
		EntityID: taskID,
	}
	ev := bus.NewEvent("kit.runtime.entity.pre_persisted", "tlc.task.cli", payload)
	return b.Publish(ctx, ev)
}

// policyAsCLIError returns a *output.Error envelope (CONFLICT, exit 4)
// when err originates from a policy.Engine veto, else nil.  The kit
// cli RunE wrapper round-trips *output.Error unchanged via the
// asCLIError interface, so the resulting envelope reaches Execute()
// with its ExitCode intact.  exitCodeFor in root.go reads that code.
func policyAsCLIError(err error) *output.Error {
	if err == nil {
		return nil
	}
	var pde *policy.PolicyDeniedError
	if errors.As(err, &pde) {
		return &output.Error{
			Code:     output.CodeConflict,
			Message:  pde.Error(),
			ExitCode: 4,
		}
	}
	if errors.Is(err, domain.ErrConflict) {
		return &output.Error{
			Code:     output.CodeConflict,
			Message:  err.Error(),
			ExitCode: 4,
		}
	}
	return nil
}

func init() {
	TaskUpdateCmd.Flags().StringVarP(&taskUpdateTitle, "title", "t", "", "New title")
	TaskUpdateCmd.Flags().StringVarP(&taskUpdateDescription, "description", "d", "", "New description")
	TaskUpdateCmd.Flags().StringVarP(&taskUpdateStatus, "status", "s", "", "New status")
	TaskUpdateCmd.Flags().StringVarP(&taskUpdateAssignedTo, "assigned-to", "a", "", "New assignee")
	TaskUpdateCmd.Flags().StringVarP(&taskUpdateEffort, "effort", "e", "", "Effort estimate")
	TaskUpdateCmd.Flags().StringVarP(&taskUpdatePriority, "priority", "p", "", "Priority")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateAddBlockedBy, "add-blocked-by", []string{}, "Add blocking task IDs")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateRemoveBlockedBy, "remove-blocked-by", []string{}, "Remove blocking task IDs")
	TaskUpdateCmd.Flags().BoolVar(&taskUpdateClearBlockedBy, "clear-blocked-by", false, "Clear all blocking task IDs")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateAddEva, "add-eva", []string{}, "Add eva annotations")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateRemoveEva, "remove-eva", []string{}, "Remove eva annotations")
	TaskUpdateCmd.Flags().BoolVar(&taskUpdateClearEva, "clear-eva", false, "Clear all eva annotations")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateAddTags, "add-tag", []string{}, "Add tags")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateRemoveTags, "remove-tag", []string{}, "Remove tags")
	TaskUpdateCmd.Flags().BoolVar(&taskUpdateForce, "force", false, "Force status transition (bypass workflow rules)")
	// --blocked / --unblock predate 'task block' / 'task unblock' and
	// stay supported verbatim: scripts depend on them, and removing an
	// escape hatch is not part of adding a command. They write the same
	// field the commands do, through the same taskFieldChanges path, so
	// the two can never diverge in behavior — only in ergonomics, which
	// is what the help text below points at.
	TaskUpdateCmd.Flags().StringVar(&taskUpdateBlocked, "blocked", "", "Set blocked reason (see also: tlc task block)")
	TaskUpdateCmd.Flags().BoolVar(&taskUpdateUnblock, "unblock", false, "Clear blocked reason (see also: tlc task unblock)")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateTimeout, "timeout", "", "Stale timeout (e.g. 2h, 30m)")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateTrack, "track", "", "Link to track ID (use '-' to unlink)")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateDue, "due", "", "Due date (tomorrow, in 3d, 2025-05-01)")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateRemindAt, "remind-at", "", "One-shot reminder time")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateRRule, "rrule", "", "Recurring reminder RRULE (use '-' to clear)")
	TaskUpdateCmd.Flags().BoolVar(&taskUpdateNoAutoRemind, "no-auto-remind", false, "Suppress 12h-before-due reminder")
	TaskUpdateCmd.Flags().StringVarP(&taskUpdateNote, "note", "n", "", "Update note (recorded against the update; required with --amend)")
	TaskUpdateCmd.Flags().BoolVar(&taskUpdateAmend, "amend", false, "Rewrite the most recent log entry's note in place instead of appending; pair with --note. Terminal tasks (DONE/SKIPPED) require --force.")

	TaskDeleteCmd.Flags().BoolVarP(&taskDeleteYes, "yes", "y", false, "Skip confirmation")
	TaskDeleteCmd.Flags().StringVarP(&taskDeleteNote, "note", "n", "", "Delete note (recorded against the transition log)")
}

// editedFieldNames returns the names of the non-status fields this
// invocation actually changed, for the `fields` key in an UPDATED log
// row's meta. "status" is excluded because the transition row already
// records it, and note/amend/force are inputs rather than edited fields.
func editedFieldNames(cmd *cobra.Command) []string {
	skip := map[string]bool{
		"status": true, "note": true, "amend": true, "force": true,
		"yes": true, "no-prompt": true,
	}
	var fields []string
	// Visit only walks flags the user actually set.
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if skip[f.Name] {
			return
		}
		fields = append(fields, f.Name)
	})
	return fields
}

// amendLatestLogNote rewrites the note of the most recent log entry for
// task in place, preserving the "Status changed from X to Y: " prefix on
// status-transition entries. It refuses to amend tasks in terminal
// statuses (DONE / SKIPPED) unless force is true. The amend is recorded
// in the log row's meta as `amended_at` (ISO timestamp) so the audit
// trail still shows the row was edited; no schema migration is needed
// because task_logs.meta is a JSON column.
func amendLatestLogNote(ctx context.Context, store interface {
	core.LogRepository
}, task *core.Task, newNote string, force bool,
) error {
	wm := core.DefaultWorkflow()
	if wm.IsTerminal(task.Status) && !force {
		return fmt.Errorf(
			"cannot amend log on terminal task (status=%s); re-run with --force to override",
			task.Status,
		)
	}

	// Use ListLogs (not GetLogs) because GetLogs applies core.DetectProject
	// scoping which can target the wrong project_id when amending a task
	// resolved from a different project DB. Limit 1 keeps the read tight.
	logs, err := store.ListLogs(ctx, core.LogQuery{
		TaskID:        task.ID,
		Limit:         1,
		SortDirection: "desc",
	})
	if err != nil {
		return fmt.Errorf("failed to read logs: %w", err)
	}
	if len(logs) == 0 {
		return fmt.Errorf("no log entries to amend; use 'tlc task update <id> --status <s> --note \"...\"' to add one")
	}
	latest := logs[0]

	// Preserve the status-transition prefix so the audit trail still
	// reads naturally. The prefix shape is set by
	// Task.TransitionWithWorkflow:
	//   "Status changed from <FROM> to <TO>: <note>"
	// For non-transition logs (CREATED, DELETED, …) the note is
	// replaced verbatim.
	rewritten := newNote
	const prefix = "Status changed from "
	if strings.HasPrefix(latest.Note, prefix) {
		if idx := strings.Index(latest.Note, ": "); idx > 0 {
			rewritten = latest.Note[:idx+2] + newNote
		}
	}

	meta := latest.Meta
	if meta == nil {
		meta = make(map[string]any)
	}
	meta["amended_at"] = time.Now().UTC().Format(time.RFC3339)
	meta["amended_by"] = core.GetCurrentUser()

	return store.UpdateLogNote(ctx, latest.ID, rewritten, meta)
}

func deletePromptInteractive(cmd *cobra.Command) bool {
	return interactiveAvailable(cmd)
}
