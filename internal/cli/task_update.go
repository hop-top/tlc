package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
	"hop.top/kit/go/console/output"
	"hop.top/kit/go/core/util"
	"hop.top/kit/go/runtime/bus"
	"hop.top/kit/go/runtime/domain"
	"hop.top/kit/go/runtime/policy"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
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

		now := time.Now().UTC()
		var errs []string

		for _, res := range resolved {
			task := res.Task
			if res.Storage != s {
				defer func() { _ = res.Storage.Close() }()
			}

			changed := false

			if cmd.Flags().Changed("title") {
				task.Title = taskUpdateTitle
				changed = true
			}
			if cmd.Flags().Changed("description") {
				task.Description = unescapeMarkdown(taskUpdateDescription)
				changed = true
			}
			if cmd.Flags().Changed("assigned-to") {
				if taskUpdateAssignedTo == "null" || taskUpdateAssignedTo == "-" {
					task.AssignedTo = nil
				} else {
					task.AssignedTo = &taskUpdateAssignedTo
				}
				changed = true
			}

			if cmd.Flags().Changed("status") {
				normalized, ok := NormalizeStatus(taskUpdateStatus)
				if !ok {
					errs = append(errs, fmt.Sprintf("%s: unknown status %q; valid values: TODO, IN_PROGRESS, DONE, SKIPPED", formatTaskAlias(task), taskUpdateStatus))
					continue
				}
				nextStatus := core.TaskStatus(normalized)
				wm := core.DefaultWorkflow()
				transitionNote := taskUpdateNote
				if transitionNote == "" {
					transitionNote = "Manual update"
				}
				log, err := task.TransitionWithWorkflow(
					nextStatus, core.GetCurrentUser(), transitionNote, wm, taskUpdateForce,
				)
				if err != nil {
					errs = append(errs, fmt.Sprintf("%s: failed to transition: %v", formatTaskAlias(task), err))
					continue
				}
				if err := res.Storage.AddLog(ctx, log); err != nil {
					fmt.Printf("Warning: failed to write log for %s: %v\n", formatTaskAlias(task), err)
				}
				changed = true
			}

			if cmd.Flags().Changed("effort") {
				// "" and "-" clear the field, matching the existing
				// pattern for --due, --remind-at, --rrule, --assigned-to.
				if taskUpdateEffort == "" || taskUpdateEffort == "-" {
					task.Effort = ""
				} else {
					normalized, ok := NormalizeEffort(taskUpdateEffort)
					if !ok {
						return fmt.Errorf("invalid effort %q: must be one of XS, S, M, L, XL", taskUpdateEffort)
					}
					task.Effort = core.Effort(normalized)
				}
				changed = true
			}

			if cmd.Flags().Changed("priority") {
				// "" and "-" clear the field, matching the existing
				// pattern for --due, --remind-at, --rrule, --assigned-to.
				if taskUpdatePriority == "" || taskUpdatePriority == "-" {
					task.Priority = ""
				} else {
					normalized, ok := NormalizePriority(taskUpdatePriority)
					if !ok {
						return fmt.Errorf("invalid priority %q: must be one of P0, P1, P2, P3", taskUpdatePriority)
					}
					task.Priority = core.Priority(normalized)
				}
				changed = true
			}

			if taskUpdateClearBlockedBy {
				task.SetBlockedBy(nil)
				changed = true
			}

			if taskUpdateClearEva {
				task.SetEva(nil)
				changed = true
			}

			if len(taskUpdateAddEva) > 0 {
				task.AddEva(taskUpdateAddEva)
				changed = true
			}

			if len(taskUpdateRemoveEva) > 0 {
				task.RemoveEva(taskUpdateRemoveEva)
				changed = true
			}

			if len(taskUpdateAddBlockedBy) > 0 {
				validated, err := validateBlockedByRefs(ctx, s, res.Storage, taskUpdateAddBlockedBy)
				if err != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", formatTaskAlias(task), err))
					continue
				}
				task.AddBlockedBy(validated)
				changed = true
			}

			if len(taskUpdateRemoveBlockedBy) > 0 {
				// Translate display aliases and bare seq into typeid
				// form before set-diff so users can remove blockers by
				// the same ID they see in `task show`. parseTaskRefForCLI
				// returns the input unchanged for cross-project refs
				// and on resolution failure, so the translation is
				// safe; empty/whitespace inputs are skipped to keep
				// intent explicit.
				translated := make([]string, 0, len(taskUpdateRemoveBlockedBy))
				for _, raw := range taskUpdateRemoveBlockedBy {
					if strings.TrimSpace(raw) == "" {
						continue
					}
					ref, _ := parseTaskRefForCLI(ctx, res.Storage, raw)
					translated = append(translated, ref)
				}
				task.RemoveBlockedBy(translated)
				changed = true
			}

			if len(taskUpdateAddTags) > 0 || len(taskUpdateRemoveTags) > 0 {
				tagMap := make(map[string]bool)
				for _, t := range task.Tags {
					tagMap[t] = true
				}
				for _, t := range taskUpdateAddTags {
					tagMap[t] = true
				}
				for _, t := range taskUpdateRemoveTags {
					delete(tagMap, t)
				}
				newTags := []string{}
				for t := range tagMap {
					newTags = append(newTags, t)
				}
				task.Tags = newTags
				changed = true
			}

			if cmd.Flags().Changed("blocked") {
				task.BlockedReason = &taskUpdateBlocked
				changed = true
			}
			if cmd.Flags().Changed("unblock") && taskUpdateUnblock {
				task.BlockedReason = nil
				changed = true
			}
			if cmd.Flags().Changed("timeout") {
				d, err := time.ParseDuration(taskUpdateTimeout)
				if err != nil {
					return fmt.Errorf("invalid --timeout %q: %w", taskUpdateTimeout, err)
				}
				task.StaleTimeout = &d
				changed = true
			}

			if cmd.Flags().Changed("track") {
				if taskUpdateTrack == "-" || taskUpdateTrack == "" {
					task.TrackID = nil
				} else {
					resolved, trackErr := resolveTrackID(ctx, res.Storage, taskUpdateTrack)
					if trackErr != nil && errors.Is(trackErr, ErrTrackNotFound) {
						created, createErr := maybeAutoCreateTrack(
							ctx, cmd, res.Storage, taskUpdateTrack,
						)
						if createErr != nil {
							errs = append(errs, fmt.Sprintf(
								"%s: %v", formatTaskAlias(task), createErr,
							))
							continue
						}
						resolved = created
					} else if trackErr != nil {
						errs = append(errs, fmt.Sprintf(
							"%s: %v", formatTaskAlias(task), trackErr,
						))
						continue
					}
					task.TrackID = &resolved
				}
				changed = true
			}

			if cmd.Flags().Changed("due") {
				if taskUpdateDue == "-" || taskUpdateDue == "" {
					task.DueAt = nil
				} else {
					t, err := util.ParseUntil(taskUpdateDue)
					if err != nil {
						errs = append(errs, fmt.Sprintf("%s: invalid --due: %v", formatTaskAlias(task), err))
						continue
					}
					task.DueAt = &t
				}
				changed = true
			}
			if cmd.Flags().Changed("remind-at") {
				if taskUpdateRemindAt == "-" || taskUpdateRemindAt == "" {
					task.RemindAt = nil
				} else {
					t, err := util.ParseUntil(taskUpdateRemindAt)
					if err != nil {
						errs = append(errs, fmt.Sprintf("%s: invalid --remind-at: %v", formatTaskAlias(task), err))
						continue
					}
					task.RemindAt = &t
				}
				changed = true
			}
			if cmd.Flags().Changed("rrule") {
				if taskUpdateRRule == "-" || taskUpdateRRule == "" {
					task.RRule = ""
				} else {
					if err := core.ValidateRRule(taskUpdateRRule); err != nil {
						errs = append(errs, fmt.Sprintf("%s: invalid --rrule: %v", formatTaskAlias(task), err))
						continue
					}
					task.RRule = taskUpdateRRule
				}
				changed = true
			}
			if cmd.Flags().Changed("no-auto-remind") {
				task.NoAutoRemind = taskUpdateNoAutoRemind
				changed = true
			}

			if !changed {
				continue
			}

			task.StaleFiredAt = nil // reset stale crossing state on any change
			var assignedTo string
			if task.AssignedTo != nil {
				assignedTo = *task.AssignedTo
			}
			valCfg := getValidationConfig()
			if err := valCfg.ValidateTaskOp(config.ValidationOpUpdate, config.TaskFields{
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

			task.UpdatedAt = now

			// Local DB is the source of truth; commit it first.
			if err := res.Storage.UpdateTask(ctx, task); err != nil {
				errs = append(errs, fmt.Sprintf("%s: failed to update: %v", formatTaskAlias(task), err))
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
	TaskUpdateCmd.Flags().StringVarP(&taskUpdateEffort, "effort", "e", "", "Effort estimate (XS, S, M, L, XL)")
	TaskUpdateCmd.Flags().StringVarP(&taskUpdatePriority, "priority", "p", "", "Priority (P0, P1, P2, P3)")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateAddBlockedBy, "add-blocked-by", []string{}, "Add blocking task IDs")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateRemoveBlockedBy, "remove-blocked-by", []string{}, "Remove blocking task IDs")
	TaskUpdateCmd.Flags().BoolVar(&taskUpdateClearBlockedBy, "clear-blocked-by", false, "Clear all blocking task IDs")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateAddEva, "add-eva", []string{}, "Add eva annotations")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateRemoveEva, "remove-eva", []string{}, "Remove eva annotations")
	TaskUpdateCmd.Flags().BoolVar(&taskUpdateClearEva, "clear-eva", false, "Clear all eva annotations")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateAddTags, "add-tag", []string{}, "Add tags")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateRemoveTags, "remove-tag", []string{}, "Remove tags")
	TaskUpdateCmd.Flags().BoolVar(&taskUpdateForce, "force", false, "Force status transition (bypass workflow rules)")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateBlocked, "blocked", "", "Set blocked reason")
	TaskUpdateCmd.Flags().BoolVar(&taskUpdateUnblock, "unblock", false, "Clear blocked reason")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateTimeout, "timeout", "", "Stale timeout (e.g. 2h, 30m)")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateTrack, "track", "", "Link to track ID (use '-' to unlink)")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateDue, "due", "", "Due date (tomorrow, in 3d, 2025-05-01)")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateRemindAt, "remind-at", "", "One-shot reminder time")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateRRule, "rrule", "", "Recurring reminder RRULE (use '-' to clear)")
	TaskUpdateCmd.Flags().BoolVar(&taskUpdateNoAutoRemind, "no-auto-remind", false, "Suppress 12h-before-due reminder")
	TaskUpdateCmd.Flags().StringVarP(&taskUpdateNote, "note", "n", "", "Update note (recorded on status transition; required with --amend)")
	TaskUpdateCmd.Flags().BoolVar(&taskUpdateAmend, "amend", false, "Rewrite the most recent log entry's note in place instead of appending; pair with --note. Terminal tasks (DONE/SKIPPED) require --force.")

	TaskDeleteCmd.Flags().BoolVarP(&taskDeleteYes, "yes", "y", false, "Skip confirmation")
	TaskDeleteCmd.Flags().StringVarP(&taskDeleteNote, "note", "n", "", "Delete note (recorded against the transition log)")
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
	stdin, ok := cmd.InOrStdin().(*os.File)
	if !ok {
		return false
	}
	stdout, ok := cmd.OutOrStdout().(*os.File)
	if !ok {
		return false
	}

	stdinInfo, err := stdin.Stat()
	if err != nil || (stdinInfo.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	stdoutInfo, err := stdout.Stat()
	if err != nil || (stdoutInfo.Mode()&os.ModeCharDevice) == 0 {
		return false
	}

	return true
}
