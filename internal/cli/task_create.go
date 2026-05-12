package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"charm.land/huh/v2"
	"charm.land/log/v2"
	"github.com/spf13/cobra"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

var TaskCreateCmd = &cobra.Command{
	Use:   "create [title]",
	Short: "Create new task",
	Long: `Create a new task with the given title.

Without a title argument or with --interactive, prompts for fields
through an interactive form. Mints a durable TypeID and allocates a
per-project monotonic sequence number for the human-facing alias.
Each invocation creates a fresh task, so the operation is not idempotent.`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "no",
	},
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var title string
		if len(args) > 0 {
			title = trimMatchingQuotes(args[0])
		}

		if taskInteractive || title == "" {
			return createTaskInteractive(title)
		}

		meta := make(map[string]interface{})
		if blockedBy := core.NormalizeBlockedBy(taskBlockedBy); len(blockedBy) > 0 {
			meta["blocked_by"] = blockedBy
		}
		if eva := core.NormalizeStringSliceMeta(taskEva); len(eva) > 0 {
			meta["eva"] = eva
		}

		var staleTimeout *time.Duration
		if taskCreateTimeout != "" {
			d, err := time.ParseDuration(taskCreateTimeout)
			if err != nil {
				return fmt.Errorf("invalid --timeout %q: %w", taskCreateTimeout, err)
			}
			staleTimeout = &d
		}

		var sched taskScheduling
		if err := sched.parse(taskDue, taskRemindAt, taskRRule, taskNoAutoRemind); err != nil {
			return err
		}

		err := saveTask(cmd.OutOrStdout(), taskID, title, taskDescription, taskStatus, taskAssignedTo, taskEffort, taskPriority, taskTags, taskReference, meta, staleTimeout, &sched)
		if err != nil {
			return err
		}
		return writeProjection()
	},
}

func createTaskInteractive(initialTitle string) error {
	var (
		title       = initialTitle
		description string
		status      = "TODO"
		assignee    string
		tags        []string
		prio        string
		domain      string
	)

	wm := core.DefaultWorkflow()
	allStatuses := wm.GetAllStatuses()
	statusOptions := make([]huh.Option[string], 0, len(allStatuses))
	for _, s := range allStatuses {
		def, _ := wm.GetStatusDef(core.TaskStatus(s)) //nolint:errcheck // best-effort label lookup
		label := s
		if def != nil && def.Label != "" {
			label = def.Label
		}
		statusOptions = append(statusOptions, huh.NewOption(label, s))
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Task Title").
				Value(&title).
				Validate(func(s string) error {
					if len(s) == 0 {
						return fmt.Errorf("title required")
					}
					return nil
				}),

			huh.NewText().
				Title("Description").
				Value(&description).
				Lines(5),
		),

		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Status").
				Options(statusOptions...).
				Value(&status),

			huh.NewInput().
				Title("Assigned To").
				Placeholder("@username").
				Value(&assignee),
		),

		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Tags").
				Options(
					huh.NewOption("feat", "feat"),
					huh.NewOption("fix", "fix"),
					huh.NewOption("chore", "chore"),
					huh.NewOption("docs", "docs"),
					huh.NewOption("urgent", "urgent"),
				).
				Value(&tags),

			huh.NewSelect[string]().
				Title("Priority").
				Options(
					huh.NewOption("P0 (Critical)", "P0"),
					huh.NewOption("P1 (High)", "P1"),
					huh.NewOption("P2 (Medium)", "P2"),
					huh.NewOption("P3 (Low)", "P3"),
				).
				Value(&prio),

			huh.NewInput().
				Title("Domain").
				Placeholder("cli, core, storage...").
				Value(&domain),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("failed to run form: %w", err)
	}

	meta := make(map[string]interface{})
	if domain != "" {
		meta["domain"] = domain
	}

	err := saveTask(os.Stdout, "", title, description, status, assignee, "", prio, tags, "", meta, nil, nil)
	if err != nil {
		return err
	}
	return writeProjectionGlobal()
}

func saveTask(w io.Writer, id, title, description, status, assignedTo, effort, priority string, tags []string, reference string, meta map[string]interface{}, staleTimeout *time.Duration, sched *taskScheduling) error {
	description = unescapeMarkdown(description)
	log.Debug("Saving task", "id", id, "title", title, "status", status)

	// Normalize enum fields (empty stays empty — those fields are optional).
	// Done before config validation so downstream rules see canonical values
	// and so the storage row only ever contains canonical forms.
	if status != "" {
		normalized, ok := NormalizeStatus(status)
		if !ok {
			return fmt.Errorf("unknown status %q; valid values: TODO, IN_PROGRESS, DONE, SKIPPED", status)
		}
		status = normalized
	}
	if effort != "" {
		normalized, ok := NormalizeEffort(effort)
		if !ok {
			return fmt.Errorf("invalid effort %q: must be one of XS, S, M, L, XL", effort)
		}
		effort = normalized
	}
	if priority != "" {
		normalized, ok := NormalizePriority(priority)
		if !ok {
			return fmt.Errorf("invalid priority %q: must be one of P0, P1, P2, P3", priority)
		}
		priority = normalized
	}

	// Config-driven validation for create.
	valCfg := getValidationConfig()
	if err := valCfg.ValidateTaskOp(config.ValidationOpCreate, config.TaskFields{
		Title:       title,
		Description: description,
		Status:      status,
		AssignedTo:  assignedTo,
		Effort:      effort,
		Priority:    priority,
		Tags:        tags,
		Reference:   reference,
	}); err != nil {
		return err
	}

	s, err := getStorage()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	if blockedBy := core.NormalizeBlockedBy(meta["blocked_by"]); len(blockedBy) > 0 {
		validated, err := validateBlockedByRefs(ctx, s, s, blockedBy)
		if err != nil {
			return err
		}
		meta["blocked_by"] = validated
	}

	// Mint a durable TypeID when the user didn't pass --id. Seq is allocated
	// atomically by storage on insert (per-project monotonic counter) so we
	// don't pre-allocate here.
	finalID := id
	if finalID == "" {
		finalID = core.NewTaskID()
	}

	now := time.Now().UTC()

	var assigneePtr *string
	if assignedTo != "" {
		assigneePtr = &assignedTo
	}

	task := &core.Task{
		ID:           finalID,
		Title:        title,
		Description:  description,
		Status:       core.TaskStatus(status),
		AssignedTo:   assigneePtr,
		Effort:       core.Effort(effort),
		Priority:     core.Priority(priority),
		Tags:         tags,
		Reference:    reference,
		CreatedAt:    now,
		UpdatedAt:    now,
		Meta:         meta,
		StaleTimeout: staleTimeout,
	}
	if sched != nil {
		task.DueAt = sched.dueAt
		task.RemindAt = sched.remindAt
		task.RRule = sched.rrule
		task.NoAutoRemind = sched.noAutoRemind
	}

	// Apply priority-based scheduling defaults from config.
	applySchedulingConfig(task)

	// Auto-assign project_id if in a project context.
	if proj := core.DetectProject(); proj != nil && proj.ProjectID != "" {
		task.ProjectID = &proj.ProjectID
	}

	// Link to track if specified.
	var parentTrackType string
	if taskTrack != "" {
		resolved, trackErr := resolveTrackID(ctx, s, taskTrack)
		if trackErr != nil && errors.Is(trackErr, ErrTrackNotFound) {
			created, createErr := maybeAutoCreateTrackFromWriter(
				ctx, w, s, taskTrack,
			)
			if createErr != nil {
				return createErr
			}
			resolved = created
		} else if trackErr != nil {
			return trackErr
		}
		task.TrackID = &resolved
		// Read parent track type for the stage gate so feature_freeze
		// can permit fix/chore tasks under fix/chore tracks. Errors are
		// non-fatal here — the gate falls back to treating an unknown
		// type as a feature task (the most-restrictive default).
		if tr, _ := s.GetTrack(ctx, resolved); tr != nil {
			parentTrackType = tr.Type
		}
	}

	// Stage gate: refuse the create when the active scope's stage
	// forbids it. Empty scope (no project context) skips the gate.
	if task.ProjectID != nil && *task.ProjectID != "" {
		if err := core.GateTaskCreate(*task.ProjectID, parentTrackType); err != nil {
			return err
		}
	}

	if task.Reference == "" {
		task.Reference = buildTaskReference(task.ID, core.DetectProject())
	}

	// Retry with a fresh TypeID if the generated ID collides. TypeIDs are
	// uuidv7-backed so collisions are vanishingly rare, but the retry is
	// cheap and keeps create-task robust under any concurrent insert that
	// happened to mint the same suffix.
	for {
		err := s.CreateTask(ctx, task)
		if err == nil {
			break
		}
		if id != "" || !strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("failed to create task: %w", err)
		}
		task.ID = core.NewTaskID()
		task.Seq = 0 // re-allocate seq for the new id
		task.Reference = buildTaskReference(task.ID, core.DetectProject())
	}

	logEntry := &core.LogEntry{
		TaskID:    task.ID,
		Timestamp: now,
		By:        core.GetCurrentUser(),
		Action:    "CREATED",
		Note:      "Task created via CLI",
	}
	if err := s.AddLog(ctx, logEntry); err != nil {
		_, _ = fmt.Fprintf(w, "Warning: failed to write log: %v\n", err)
	}

	// Display alias by default; verbose output adds the durable TypeID.
	alias := core.FormatTaskAlias(task)
	if alias == "" {
		alias = task.ID
	}
	_, _ = fmt.Fprintf(w, "Created task %s: %s\n", alias, task.Title)
	if isShowTypeIDOutput() && alias != task.ID {
		_, _ = fmt.Fprintf(w, "  ID: %s\n", task.ID)
	}
	return nil
}

// buildTaskReference constructs an absolute task URI.
// Format: tlc://<projectID>/<taskID> when a project is detected;
// falls back to tlc://<taskID> (relative) otherwise.
func buildTaskReference(taskID string, proj *core.ProjectDetection) string {
	if proj != nil && proj.ProjectID != "" {
		return fmt.Sprintf("tlc://%s/%s", proj.ProjectID, taskID)
	}
	return fmt.Sprintf("tlc:///%s", taskID)
}

// ResetCreateFlags clears all flag state on TaskCreateCmd using
// cobra's built-in ResetFlags, then re-registers with fresh defaults.
// Call before Execute() to prevent stale state from a prior
// invocation leaking into the next one (T-0231).
func ResetCreateFlags() {
	TaskCreateCmd.ResetFlags()
	registerCreateFlags(TaskCreateCmd)
}

// registerCreateFlags binds task-create flags to package-level vars.
func registerCreateFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&taskID, "id", "", "Task ID (e.g. T-0042)")
	cmd.Flags().StringVarP(&taskDescription, "description", "d", "", "Task description")
	cmd.Flags().StringVarP(&taskStatus, "status", "s", "TODO", "Initial status")
	cmd.Flags().StringVarP(&taskAssignedTo, "assigned-to", "a", "", "Assignee username")
	cmd.Flags().StringVarP(&taskEffort, "effort", "e", "", "Effort estimate (XS, S, M, L, XL)")
	cmd.Flags().StringVarP(&taskPriority, "priority", "p", "", "Priority (P0, P1, P2, P3)")
	cmd.Flags().StringSliceVar(&taskBlockedBy, "blocked-by", []string{}, "Blocking task IDs (repeatable)")
	cmd.Flags().StringSliceVar(&taskTags, "tag", []string{}, "Tags (repeatable)")
	cmd.Flags().StringVarP(&taskReference, "reference", "r", "", "Reference pointer")
	cmd.Flags().BoolVarP(&taskInteractive, "interactive", "i", false, "Interactive prompt mode")
	cmd.Flags().StringVar(&taskCreateTimeout, "timeout", "", "Stale timeout (e.g. 2h)")
	cmd.Flags().StringVar(&taskTrack, "track", "", "Link task to a track ID")
	cmd.Flags().StringVar(&taskDue, "due", "", "Due date (tomorrow, in 3d, 2025-05-01)")
	cmd.Flags().StringVar(&taskRemindAt, "remind-at", "", "One-shot reminder time")
	cmd.Flags().StringVar(&taskRRule, "rrule", "", "Recurring reminder RRULE (e.g. FREQ=DAILY, FREQ=WEEKLY;BYDAY=MO,WE,FR)")
	cmd.Flags().BoolVar(&taskNoAutoRemind, "no-auto-remind", false, "Suppress 12h-before-due reminder")
	cmd.Flags().StringSliceVar(&taskEva, "eva", []string{}, "Eva annotations (repeatable)")
}

func init() {
	registerCreateFlags(TaskCreateCmd)
}
