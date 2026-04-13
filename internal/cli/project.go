package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"hop.top/kit/output"
	"hop.top/tlc/internal/core"
)

var projectPruneYes bool

var (
	projectExportFormat string
	projectImportForce  bool
	projectListFormat   string
)

var ProjectCmd = &cobra.Command{
	Use:   "project",
	Short: "Project management",
}

var ProjectExportCmd = &cobra.Command{
	Use:   "export [file]",
	Short: "Export project tasks and logs",
	Long:  "Export all tasks and logs to a YAML or JSON file. Writes to stdout if no file is specified.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		// Get all tasks (no filters, high limit).
		query := core.Query{
			Limit:           10000,
			IncludeArchived: true,
			AllProjects:     false,
		}
		tasks, err := s.ListTasks(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to list tasks: %w", err)
		}

		// Collect logs for each task.
		var allLogs []*core.LogEntry
		for _, t := range tasks {
			logs, err := s.GetTaskLogs(ctx, t.ID)
			if err != nil {
				return fmt.Errorf("failed to get logs for %s: %w", t.ID, err)
			}
			allLogs = append(allLogs, logs...)
		}

		projectID := ""
		det := core.DetectProject()
		if det != nil && det.ProjectID != "" {
			projectID = det.ProjectID
		}

		export := core.ProjectExport{
			Version:   "1",
			ProjectID: projectID,
			Tasks:     tasks,
			Logs:      allLogs,
		}

		var data []byte
		switch projectExportFormat {
		case "json":
			data, err = json.MarshalIndent(export, "", "  ")
		default:
			data, err = yaml.Marshal(export)
		}
		if err != nil {
			return fmt.Errorf("failed to marshal export: %w", err)
		}

		if len(args) > 0 {
			if err := os.WriteFile(args[0], data, 0o644); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Exported %d tasks and %d logs to %s\n",
				len(tasks), len(allLogs), args[0])
		} else {
			_, _ = cmd.OutOrStdout().Write(data)
		}
		return nil
	},
}

var ProjectImportCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import tasks and logs from an export file",
	Long:  "Import tasks and logs from a YAML or JSON export. Skips existing task IDs unless --force is set.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		data, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		var export core.ProjectExport

		// Try YAML first (superset of JSON).
		if err := yaml.Unmarshal(data, &export); err != nil {
			// Fall back to JSON.
			if jsonErr := json.Unmarshal(data, &export); jsonErr != nil {
				return fmt.Errorf("failed to parse export file (tried YAML and JSON): %w", err)
			}
		}

		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		var created, skipped, updated int
		for _, task := range export.Tasks {
			existing, err := s.GetTask(ctx, task.ID)
			if err != nil {
				return fmt.Errorf("failed to check task %s: %w", task.ID, err)
			}

			if existing != nil {
				if projectImportForce {
					if err := s.UpdateTask(ctx, task); err != nil {
						return fmt.Errorf("failed to update task %s: %w", task.ID, err)
					}
					updated++
				} else {
					skipped++
				}
				continue
			}

			if err := s.CreateTask(ctx, task); err != nil {
				return fmt.Errorf("failed to create task %s: %w", task.ID, err)
			}
			created++
		}

		var logsImported int
		for _, entry := range export.Logs {
			if err := s.AddLog(ctx, entry); err != nil {
				return fmt.Errorf("failed to import log for %s: %w", entry.TaskID, err)
			}
			logsImported++
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"Imported: %d created, %d updated, %d skipped, %d logs\n",
			created, updated, skipped, logsImported)

		return syncTODOAll()
	},
}

var ProjectListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all registered projects",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := context.Background()
		s, err := getStorageRaw()
		if err != nil {
			return fmt.Errorf("failed to open storage: %w", err)
		}
		defer func() { _ = s.Close() }()

		projects, err := s.ListAllProjects(ctx)
		if err != nil {
			return fmt.Errorf("failed to list projects: %w", err)
		}

		format := projectListFormat
		if format == "" {
			format = viper.GetString("output.format")
		}
		if format == "" {
			format = formatTable
		}

		switch format {
		case formatJSON, formatYAML:
			if err := output.Render(cmd.OutOrStdout(), format, projects); err != nil {
				return err
			}
		default:
			renderProjectTable(cmd, projects)
		}
		return nil
	},
}

func renderProjectTable(cmd *cobra.Command, projects []core.RegisteredProject) {
	w := cmd.OutOrStdout()

	if len(projects) == 0 {
		dimStyle := lipgloss.NewStyle().Foreground(mutedColor)
		_, _ = fmt.Fprintln(w, dimStyle.Render("No projects registered."))
		return
	}

	headers := []string{"ID", "Label", "DB Path", "Space URI", "Status"}

	rows := make([][]string, 0, len(projects))
	for _, p := range projects {
		rows = append(rows, []string{
			p.ProjectID,
			p.Label,
			p.DBPath,
			p.SpaceURI,
			p.Status,
		})
	}

	renderTTYTable(w, headers, rows, termWidth())
}

var ProjectPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove stale project registrations",
	Long:  "Delete registered projects whose database file no longer exists on disk.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := context.Background()
		s, err := getStorageRaw()
		if err != nil {
			return fmt.Errorf("failed to open storage: %w", err)
		}
		defer func() { _ = s.Close() }()

		projects, err := s.ListAllProjects(ctx)
		if err != nil {
			return fmt.Errorf("failed to list projects: %w", err)
		}

		var stale []core.RegisteredProject
		for _, p := range projects {
			if _, statErr := os.Stat(p.DBPath); os.IsNotExist(statErr) {
				stale = append(stale, p)
			}
		}

		if len(stale) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No stale projects found.")
			return nil
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Found %d stale project(s):\n", len(stale))
		for _, p := range stale {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s  (%s)\n", p.ProjectID, p.DBPath)
		}

		if !projectPruneYes {
			_, _ = fmt.Fprint(cmd.OutOrStdout(), "Remove these projects? [y/N] ")
			var answer string
			_, _ = fmt.Fscan(cmd.InOrStdin(), &answer)
			if answer != "y" && answer != "Y" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
				return nil
			}
		}

		removed := 0
		for _, p := range stale {
			if delErr := s.DeleteProject(ctx, p.ProjectID); delErr != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "failed to remove %s: %v\n", p.ProjectID, delErr)
				continue
			}
			removed++
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Removed %d project(s).\n", removed)
		return nil
	},
}

func init() {
	ProjectExportCmd.Flags().StringVarP(&projectExportFormat, "format", "f", "yaml", "Output format (yaml, json)")
	ProjectImportCmd.Flags().BoolVar(&projectImportForce, "force", false, "Overwrite existing tasks on conflict")
	ProjectListCmd.Flags().StringVarP(&projectListFormat, "format", "f", "", "Output format (table, json, yaml)")
	ProjectPruneCmd.Flags().BoolVarP(&projectPruneYes, "yes", "y", false, "Skip confirmation prompt")

	ProjectCmd.AddCommand(ProjectExportCmd)
	ProjectCmd.AddCommand(ProjectImportCmd)
	ProjectCmd.AddCommand(ProjectInitCmd)
	ProjectCmd.AddCommand(ProjectListCmd)
	ProjectCmd.AddCommand(ProjectPruneCmd)
	RootCmd.AddCommand(ProjectCmd)
}
