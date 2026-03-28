package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

var TaskCreateCmd = &cobra.Command{
	Use:   "create [title]",
	Short: "Create new task",
	Args:  cobra.MaximumNArgs(1),
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

		err := saveTask(cmd.OutOrStdout(), taskID, title, taskDescription, taskStatus, taskAssignedTo, taskEffort, taskPriority, taskTags, taskReference, meta)
		if err != nil {
			return err
		}
		return syncTODOAll()
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
		def, _ := wm.GetStatusDef(core.TaskStatus(s))
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

	err := saveTask(os.Stdout, "", title, description, status, assignee, "", prio, tags, "", meta)
	if err != nil {
		return err
	}
	return syncToTODO()
}

func saveTask(w io.Writer, id, title, description, status, assignedTo, effort, priority string, tags []string, reference string, meta map[string]interface{}) error {
	log.Debug("Saving task", "id", id, "title", title, "status", status)
	if !core.ValidEffort(core.Effort(effort)) {
		return fmt.Errorf("invalid effort %q: must be one of XS, S, M, L, XL", effort)
	}
	if !core.ValidPriority(core.Priority(priority)) {
		return fmt.Errorf("invalid priority %q: must be one of P0, P1, P2, P3", priority)
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

	finalID := id
	if finalID == "" {
		var projectID string
		if proj := core.DetectProject(); proj != nil && proj.ProjectID != "" {
			projectID = proj.ProjectID
		}
		seq, err := s.GetNextSequenceID(ctx, projectID)
		if err != nil {
			return fmt.Errorf("failed to get next sequence ID: %w", err)
		}
		finalID = fmt.Sprintf("T-%04d", seq)
	}

	now := time.Now().UTC()

	var assigneePtr *string
	if assignedTo != "" {
		assigneePtr = &assignedTo
	}

	task := &core.Task{
		ID:          finalID,
		Title:       title,
		Description: description,
		Status:      core.TaskStatus(status),
		AssignedTo:  assigneePtr,
		Effort:      core.Effort(effort),
		Priority:    core.Priority(priority),
		Tags:        tags,
		Reference:   reference,
		CreatedAt:   now,
		UpdatedAt:   now,
		Meta:        meta,
	}

	// Auto-assign project_id if in a project context.
	if proj := core.DetectProject(); proj != nil && proj.ProjectID != "" {
		task.ProjectID = &proj.ProjectID
	}

	if task.Reference == "" {
		task.Reference = fmt.Sprintf("task://%s", task.ID)
	}

	// Retry with a fresh sequence ID if the generated ID collides with an existing task
	// (sequence can lag behind tasks created via import or explicit --id).
	for {
		err := s.CreateTask(ctx, task)
		if err == nil {
			break
		}
		if id != "" || !strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("failed to create task: %w", err)
		}
		var projectID string
		if proj := core.DetectProject(); proj != nil && proj.ProjectID != "" {
			projectID = proj.ProjectID
		}
		seq, seqErr := s.GetNextSequenceID(ctx, projectID)
		if seqErr != nil {
			return fmt.Errorf("failed to create task: %w", err)
		}
		task.ID = fmt.Sprintf("T-%04d", seq)
		task.Reference = fmt.Sprintf("task://%s", task.ID)
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

	_, _ = fmt.Fprintf(w, "Created task %s: %s\n", task.ID, task.Title)
	return nil
}

func init() {
	TaskCreateCmd.Flags().StringVar(&taskID, "id", "", "Task ID (e.g. T-0042)")
	TaskCreateCmd.Flags().StringVarP(&taskDescription, "description", "d", "", "Task description")
	TaskCreateCmd.Flags().StringVarP(&taskStatus, "status", "s", "TODO", "Initial status")
	TaskCreateCmd.Flags().StringVarP(&taskAssignedTo, "assigned-to", "a", "", "Assignee username")
	TaskCreateCmd.Flags().StringVarP(&taskEffort, "effort", "e", "", "Effort estimate (XS, S, M, L, XL)")
	TaskCreateCmd.Flags().StringVarP(&taskPriority, "priority", "p", "", "Priority (P0, P1, P2, P3)")
	TaskCreateCmd.Flags().StringSliceVar(&taskBlockedBy, "blocked-by", []string{}, "Blocking task IDs (repeatable)")
	TaskCreateCmd.Flags().StringSliceVar(&taskTags, "tag", []string{}, "Tags (repeatable)")
	TaskCreateCmd.Flags().StringVarP(&taskReference, "reference", "r", "", "Reference pointer")
	TaskCreateCmd.Flags().BoolVarP(&taskInteractive, "interactive", "i", false, "Interactive prompt mode")
}
