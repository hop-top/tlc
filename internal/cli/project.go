package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"hop.top/tlc/internal/core"
)

var (
	projectExportFormat string
	projectImportForce  bool
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Project management",
}

var projectExportCmd = &cobra.Command{
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

var projectImportCmd = &cobra.Command{
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

func init() {
	projectExportCmd.Flags().StringVarP(&projectExportFormat, "format", "f", "yaml", "Output format (yaml, json)")
	projectImportCmd.Flags().BoolVar(&projectImportForce, "force", false, "Overwrite existing tasks on conflict")

	projectCmd.AddCommand(projectExportCmd)
	projectCmd.AddCommand(projectImportCmd)
	rootCmd.AddCommand(projectCmd)
}
