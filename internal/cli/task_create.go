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
			if !interactiveAvailable(cmd) {
				return fmt.Errorf(
					"interactive task creation requires a terminal; " +
						"pass a title and flags instead",
				)
			}
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

		status, err := resolveInitialStatus(taskStatus)
		if err != nil {
			return err
		}

		err = saveTask(cmd.OutOrStdout(), taskID, title, taskDescription, status, taskAssignedTo, taskEffort, taskPriority, taskTags, taskReference, meta, staleTimeout, &sched)
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
		status      string
		assignee    string
		tags        []string
		prio        string
		domain      string
	)

	wm := core.DefaultWorkflow()
	// Preselect the same status the non-interactive path would choose,
	// rather than a hardcoded TODO that a renamed vocabulary does not
	// contain (the select would then open on no option at all).
	if initial, err := wm.InitialStatus(); err == nil {
		status = string(initial)
	}
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
			return unknownStatusError(status)
		}
		status = normalized
	}
	if effort != "" {
		normalized, ok := NormalizeEffort(effort)
		if !ok {
			return unknownEffortError(effort)
		}
		effort = normalized
	}
	if priority != "" {
		normalized, ok := NormalizePriority(priority)
		if !ok {
			return invalidPriorityError(priority)
		}
		priority = normalized
	}

	// The tag vocabulary gate. Before the regex rules below, not after:
	// ValidationConfig can only see tags as one comma-joined string, so
	// it can assert a shape but never membership, and its message would
	// name a pattern rather than the offending tag.
	if err := core.ValidateTags(tags); err != nil {
		return err
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

// resolveInitialStatus returns the status a create lands in.
//
// An explicit --status passes through untouched, so scripts naming a
// status keep working and an invalid one is still rejected downstream by
// saveTask's normaliser (which names the configured vocabulary).
//
// Empty means the user nominated nothing, and the answer comes from the
// workflow: task.default_status when set, else the initial-role status.
// Both live in the same TaskConfig the workflow already validates
// against, so create cannot disagree with the config that gates it.
func resolveInitialStatus(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	wm, err := core.DefaultWorkflowE()
	if err != nil {
		return "", fmt.Errorf("invalid task workflow configuration: %w", err)
	}
	status, err := wm.InitialStatus()
	if err != nil {
		return "", fmt.Errorf(
			"no initial status to create into: %w; "+
				"set task.default_status or give a status role \"initial\"", err,
		)
	}
	return string(status), nil
}

// initialStatusFlagUsageDefault is the usage string the flag carries at
// registration time.
//
// It names no status on purpose. Registration runs from init(), long
// before any config file is read, and DefaultWorkflowE caches its answer
// in a sync.Once — so resolving a concrete status here would permanently
// pin the process to the BUILT-IN vocabulary and defeat the whole fix.
// refreshCreateStatusUsage fills in the real one after initConfig.
const initialStatusFlagUsageDefault = "Initial status (default: configured task.default_status)"

// initialStatusFlagUsage renders --status help that stays true under a
// renamed vocabulary. The flag default is empty, so cobra prints no
// "(default ...)" of its own; naming the resolved status here keeps help
// from implying that omitting the flag leaves the status unset.
//
// Reads the config provider directly rather than going through
// DefaultWorkflowE. This runs from the help path, which fires BEFORE
// argv's `-c key=value` overrides are merged; DefaultWorkflowE memoises
// its answer in a sync.Once, so resolving through it here would pin the
// process to the pre-override config and silently drop those overrides
// for every later caller.
func initialStatusFlagUsage() string {
	status := core.ConfiguredInitialTaskStatus()
	if status == "" {
		return initialStatusFlagUsageDefault
	}
	return fmt.Sprintf("Initial status (default %s)", status)
}

// refreshCreateStatusUsage re-renders the --status usage string against
// the now-loaded config. Called from the same post-initConfig points as
// restampConfiguredStatusEnum, for the same reason: the flag is
// registered before the config file is read.
//
// Only the leading prose is replaced. kit appends its own "(one of:
// ...)" enum suffix to the same Usage string, and restampConfiguredStatusEnum
// rewrites that half; overwriting the whole string here would drop the
// vocabulary list from help.
func refreshCreateStatusUsage() {
	f := TaskCreateCmd.Flags().Lookup("status")
	if f == nil {
		return
	}
	suffix := ""
	if i := strings.Index(f.Usage, "(one of:"); i >= 0 {
		suffix = " " + f.Usage[i:]
	}
	f.Usage = initialStatusFlagUsage() + suffix
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
	// Empty, not "TODO": a hardcoded default is supplied on every run and
	// so outranks task.default_status, which under a renamed vocabulary
	// means create is rejected by the user's own config. Empty keeps "not
	// specified" distinguishable from an explicit choice; resolveInitialStatus
	// fills it in.
	cmd.Flags().StringVarP(&taskStatus, "status", "s", "", initialStatusFlagUsageDefault)
	cmd.Flags().StringVarP(&taskAssignedTo, "assigned-to", "a", "", "Assignee username")
	cmd.Flags().StringVarP(&taskEffort, "effort", "e", "", "Effort estimate")
	cmd.Flags().StringVarP(&taskPriority, "priority", "p", "", "Priority")
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
