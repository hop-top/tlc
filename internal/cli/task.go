package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
	"github.com/google/oss-tlc-cli/internal/core"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	taskID          string
	taskTitle       string
	taskDescription string
	taskStatus      string
	taskAssignedTo  string
	taskTags        []string
	taskReference   string
	taskInteractive bool

	taskListStatus     []string
	taskListAssignedTo string
	taskListTag        []string
	taskListMine       bool
	taskListSortBy     string
	taskListSortDirection      string
	taskListLimit      int
	taskListOffset     int

	taskShowLogs     bool
	taskShowLogSortDirection string

	taskUpdateTitle       string
	taskUpdateDescription string
	taskUpdateStatus      string
	taskUpdateAssignedTo  string
	taskUpdateAddTags     []string
	taskUpdateRemoveTags  []string

	taskDeleteYes bool

	taskClaimNote   string
	taskUnclaimNote string
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Task operations",
}

var taskClaimCmd = &cobra.Command{
	Use:   "claim <task-id>",
	Short: "Claim a task for work",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer s.Close()

		ctx := context.Background()
		task, err := s.GetTask(ctx, id)
		if err != nil {
			return err
		}
		if task == nil {
			return fmt.Errorf("task not found: %s", id)
		}

		user := core.GetCurrentUser()
		task.AssignedTo = &user

		log, err := task.Transition(core.StatusInProgress, user, taskClaimNote)
		if err != nil {
			return err
		}

		if err := s.UpdateTask(ctx, task); err != nil {
			return err
		}
		if err := s.AddLog(ctx, log); err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to write log: %v\n", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Claimed task %s\n", id)
		if task.OriginSystem != nil && *task.OriginSystem != "" {
			fmt.Printf("Note: Task %s has local changes. Run 'tlc sync push %s' to sync.\n", task.ID, *task.OriginSystem)
		}
		return syncToTODO()
	},
}

var taskUnclaimCmd = &cobra.Command{
	Use:   "unclaim <task-id>",
	Short: "Release a claimed task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer s.Close()

		ctx := context.Background()
		task, err := s.GetTask(ctx, id)
		if err != nil {
			return err
		}
		if task == nil {
			return fmt.Errorf("task not found: %s", id)
		}

		task.AssignedTo = nil

		log, err := task.Transition(core.StatusTodo, core.GetCurrentUser(), taskUnclaimNote)
		if err != nil {
			return err
		}

		if err := s.UpdateTask(ctx, task); err != nil {
			return err
		}
		if err := s.AddLog(ctx, log); err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to write log: %v\n", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Unclaimed task %s\n", id)
		if task.OriginSystem != nil && *task.OriginSystem != "" {
			fmt.Printf("Note: Task %s has local changes. Run 'tlc sync push %s' to sync.\n", task.ID, *task.OriginSystem)
		}
		return syncToTODO()
	},
}

var taskCreateCmd = &cobra.Command{
	Use:   "create [title]",
	Short: "Create new task",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		title := taskTitle
		if len(args) > 0 {
			title = args[0]
		}

		if taskInteractive || (title == "" && !cmd.Flags().Changed("title")) {
			return createTaskInteractive(title)
		}

		if title == "" {
			return fmt.Errorf("title is required")
		}

		_, err := saveTask(cmd.OutOrStdout(), taskID, title, taskDescription, taskStatus, taskAssignedTo, taskTags, taskReference, make(map[string]interface{}))
		if err != nil {
			return err
		}
		return syncToTODO()
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
				Options(
					huh.NewOption("TODO", "TODO"),
					huh.NewOption("IN_PROGRESS", "IN_PROGRESS"),
					huh.NewOption("DONE", "DONE"),
				).
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
		return err
	}

	meta := make(map[string]interface{})
	if prio != "" {
		meta["prio"] = prio
	}
	if domain != "" {
		meta["domain"] = domain
	}

	_, err := saveTask(os.Stdout, "", title, description, status, assignee, tags, "", meta)
	if err != nil {
		return err
	}
	return syncToTODO()
}

func saveTask(w io.Writer, id, title, description, status, assignedTo string, tags []string, reference string, meta map[string]interface{}) (*core.Task, error) {
	log.Debug("Saving task", "id", id, "title", title, "status", status)
	s, err := getStorage()
	if err != nil {
		return nil, err
	}
	defer s.Close()

	ctx := context.Background()

	finalID := id
	if finalID == "" {
		tasks, err := s.ListTasks(ctx, core.Query{})
		if err != nil {
			return nil, err
		}
		finalID = fmt.Sprintf("T-%04d", len(tasks)+1)
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
		Tags:        tags,
		Reference:   reference,
		CreatedAt:   now,
		UpdatedAt:   now,
		Meta:        meta,
	}

	if task.Reference == "" {
		task.Reference = fmt.Sprintf("task://%s", task.ID)
	}

	if err := s.CreateTask(ctx, task); err != nil {
		return nil, err
	}

	logEntry := &core.LogEntry{
		TaskID:    task.ID,
		Timestamp: now,
		By:        core.GetCurrentUser(),
		Action:    "CREATED",
		Note:      "Task created via CLI",
	}
	if err := s.AddLog(ctx, logEntry); err != nil {
		fmt.Fprintf(w, "Warning: failed to write log: %v\n", err)
	}

	fmt.Fprintf(w, "Created task %s: %s\n", task.ID, task.Title)
	return task, nil
}

var taskListCmd = &cobra.Command{
	Use:   "list [query]",
	Short: "List tasks with filters",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer s.Close()

		ctx := context.Background()
		query := core.Query{
			Limit:  taskListLimit,
			Offset: taskListOffset,
			SortBy: taskListSortBy,
			SortDirection:  taskListSortDirection,
		}

		if len(args) > 0 {
			query.Search = args[0]
		}

		if taskListMine {
			taskListAssignedTo = core.GetCurrentUser()
		}

		if taskListStatus != nil {
			for _, st := range taskListStatus {
				query.Filters = append(query.Filters, core.FieldFilter{Field: "status", Value: st})
			}
		}
		if taskListAssignedTo != "" {
			query.Filters = append(query.Filters, core.FieldFilter{Field: "assigned_to", Value: taskListAssignedTo})
		}
		for _, tag := range taskListTag {
			query.Filters = append(query.Filters, core.FieldFilter{Field: "tags", Value: tag})
		}

		tasks, err := s.ListTasks(ctx, query)
		if err != nil {
			return err
		}

		format := viper.GetString("output.format")
		formatTasks(cmd, tasks, format)
		return nil
	},
}

var taskShowCmd = &cobra.Command{
	Use:   "show <task-id>",
	Short: "Show task details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer s.Close()

		ctx := context.Background()
		task, err := s.GetTask(ctx, id)
		if err != nil {
			return err
		}
		if task == nil {
			return fmt.Errorf("task not found: %s", id)
		}

		var logs []*core.LogEntry
		if taskShowLogs || viper.GetString("output.format") != "table" {
			direction := taskShowLogSortDirection
			if direction == "" {
				direction = viper.GetString("ui.log_sort_direction")
			}
			if direction == "" {
				direction = "desc"
			}
			logs, err = s.GetLogs(ctx, id, direction)
			if err != nil {
				return err
			}
		}

		format := viper.GetString("output.format")
		printTask(cmd, task, logs, format)
		return nil
	},
}

var taskUpdateCmd = &cobra.Command{
	Use:   "update <task-id>",
	Short: "Update task fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer s.Close()

		ctx := context.Background()
		task, err := s.GetTask(ctx, id)
		if err != nil {
			return err
		}
		if task == nil {
			return fmt.Errorf("task not found: %s", id)
		}

		changed := false
		now := time.Now().UTC()

		if cmd.Flags().Changed("title") {
			task.Title = taskUpdateTitle
			changed = true
		}
		if cmd.Flags().Changed("description") {
			task.Description = taskUpdateDescription
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
			nextStatus := core.TaskStatus(taskUpdateStatus)
			log, err := task.Transition(nextStatus, core.GetCurrentUser(), "Manual update")
			if err != nil {
				return err
			}
			if err := s.AddLog(ctx, log); err != nil {
				fmt.Printf("Warning: failed to write log: %v\n", err)
			}
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

		if changed {
			task.UpdatedAt = now
			if err := s.UpdateTask(ctx, task); err != nil {
				return err
			}
			fmt.Printf("Updated task %s\n", task.ID)
			
			if task.OriginSystem != nil && *task.OriginSystem != "" {
				fmt.Printf("Note: Task %s has local changes. Run 'tlc sync push %s' to sync.\n", task.ID, *task.OriginSystem)
			}
			
			return syncToTODO()
		} else {
			fmt.Println("No changes specified")
		}

		return nil
	},
}

var taskDeleteCmd = &cobra.Command{
	Use:   "delete <task-id>",
	Short: "Delete a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer s.Close()

		ctx := context.Background()
		task, err := s.GetTask(ctx, id)
		if err != nil {
			return err
		}
		if task == nil {
			return fmt.Errorf("task not found: %s", id)
		}

		if !taskDeleteYes {
			var confirm bool
			err := huh.NewConfirm().
				Title(fmt.Sprintf("Delete task %s (%s)?", task.ID, task.Title)).
				Description("This action cannot be undone.").
				Value(&confirm).
				Run()
			if err != nil {
				return err
			}
			if !confirm {
				fmt.Println("Aborted")
				return nil
			}
		}

		if err := s.DeleteTask(ctx, id); err != nil {
			return err
		}

		fmt.Printf("Deleted task %s\n", id)
		return syncToTODO()
	},
}

func init() {
	taskCreateCmd.Flags().StringVar(&taskID, "id", "", "Task ID (e.g. T-0042)")
	taskCreateCmd.Flags().StringVarP(&taskTitle, "title", "t", "", "Task title")
	taskCreateCmd.Flags().StringVarP(&taskDescription, "description", "d", "", "Task description")
	taskCreateCmd.Flags().StringVarP(&taskStatus, "status", "s", "TODO", "Initial status")
	taskCreateCmd.Flags().StringVarP(&taskAssignedTo, "assigned-to", "a", "", "Assignee username")
	taskCreateCmd.Flags().StringSliceVar(&taskTags, "tag", []string{}, "Tags (repeatable)")
	taskCreateCmd.Flags().StringVarP(&taskReference, "reference", "r", "", "Reference pointer")
	taskCreateCmd.Flags().BoolVarP(&taskInteractive, "interactive", "i", false, "Interactive prompt mode")

	taskListCmd.Flags().StringSliceVarP(&taskListStatus, "status", "s", []string{}, "Filter by status")
	taskListCmd.Flags().StringVarP(&taskListAssignedTo, "assigned-to", "a", "", "Filter by assignee")
	taskListCmd.Flags().StringSliceVar(&taskListTag, "tag", []string{}, "Filter by tag")
	taskListCmd.Flags().BoolVar(&taskListMine, "mine", false, "Filter by current user")
	taskListCmd.Flags().StringVar(&taskListSortBy, "sort-by", "created_at", "Sort field")
	taskListCmd.Flags().StringVar(&taskListSortDirection, "sort-direction", "desc", "Sort direction (asc, desc)")
	taskListCmd.Flags().IntVarP(&taskListLimit, "limit", "n", 100, "Limit results")
	taskListCmd.Flags().IntVar(&taskListOffset, "offset", 0, "Skip results")

	taskShowCmd.Flags().BoolVar(&taskShowLogs, "logs", false, "Include audit logs")
	taskShowCmd.Flags().StringVar(&taskShowLogSortDirection, "log-sort-direction", "", "Log sort direction (asc, desc)")

	taskUpdateCmd.Flags().StringVarP(&taskUpdateTitle, "title", "t", "", "New title")
	taskUpdateCmd.Flags().StringVarP(&taskUpdateDescription, "description", "d", "", "New description")
	taskUpdateCmd.Flags().StringVarP(&taskUpdateStatus, "status", "s", "", "New status")
	taskUpdateCmd.Flags().StringVarP(&taskUpdateAssignedTo, "assigned-to", "a", "", "New assignee")
	taskUpdateCmd.Flags().StringSliceVar(&taskUpdateAddTags, "add-tag", []string{}, "Add tags")
	taskUpdateCmd.Flags().StringSliceVar(&taskUpdateRemoveTags, "remove-tag", []string{}, "Remove tags")

		taskDeleteCmd.Flags().BoolVarP(&taskDeleteYes, "yes", "y", false, "Skip confirmation")

	

		taskClaimCmd.Flags().StringVarP(&taskClaimNote, "note", "n", "", "Claim note")

		taskUnclaimCmd.Flags().StringVarP(&taskUnclaimNote, "note", "n", "", "Unclaim note")

	

		taskCmd.AddCommand(taskCreateCmd)

		taskCmd.AddCommand(taskListCmd)

		taskCmd.AddCommand(taskShowCmd)

		taskCmd.AddCommand(taskUpdateCmd)

		taskCmd.AddCommand(taskDeleteCmd)

		taskCmd.AddCommand(taskClaimCmd)

		taskCmd.AddCommand(taskUnclaimCmd)

		rootCmd.AddCommand(taskCmd)

	}

	