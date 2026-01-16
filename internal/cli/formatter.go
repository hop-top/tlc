package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/oss-tlc-cli/internal/core"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	primaryColor = lipgloss.Color("39")
	successColor = lipgloss.Color("42")
	warningColor = lipgloss.Color("214")
	errorColor   = lipgloss.Color("196")
	mutedColor   = lipgloss.Color("241")

	titleStyle = lipgloss.NewStyle().Foreground(primaryColor).Bold(true)
	labelStyle = lipgloss.NewStyle().Foreground(mutedColor).Width(12)

	todoStyle       = lipgloss.NewStyle()
	inProgressStyle = lipgloss.NewStyle().Foreground(primaryColor)
	doneStyle       = lipgloss.NewStyle().Foreground(successColor).Bold(true)
	skippedStyle    = lipgloss.NewStyle().Foreground(warningColor)
)

func formatTasks(cmd *cobra.Command, tasks []*core.Task, format string) {
	out := cmd.OutOrStdout()
	switch format {
	case "json":
		data, _ := json.MarshalIndent(tasks, "", "  ")
		fmt.Fprintln(out, string(data))
	case "yaml":
		data, _ := yaml.Marshal(tasks)
		fmt.Fprintln(out, string(data))
	case "tls":
		for _, t := range tasks {
			fmt.Fprintln(out, formatTLS(t))
		}
	default: // table
		renderTable(out, tasks)
	}
}

func printTask(cmd *cobra.Command, task *core.Task, logs []*core.LogEntry, format string) {
	out := cmd.OutOrStdout()
	switch format {
	case "json":
		result := map[string]interface{}{
			"task": task,
			"logs": logs,
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Fprintln(out, string(data))
	case "yaml":
		result := map[string]interface{}{
			"task": task,
			"logs": logs,
		}
		data, _ := yaml.Marshal(result)
		fmt.Fprintln(out, string(data))
	default:
		renderTaskDetail(out, task, logs)
	}
}

func formatTLS(t *core.Task) string {
	status := "[ ]"
	switch t.Status {
	case core.StatusInProgress:
		status = "[~]"
	case core.StatusDone:
		status = "[x]"
	case core.StatusSkipped:
		status = "[-]"
	}

	parts := []string{status, t.ID, t.Title}

	if t.AssignedTo != nil && *t.AssignedTo != "" {
		parts = append(parts, "@"+*t.AssignedTo)
	}

	for _, tag := range t.Tags {
		parts = append(parts, "#"+tag)
	}

	if t.Reference != "" && t.Reference != "task://"+t.ID {
		parts = append(parts, "ref:"+t.Reference)
	}

	if !t.CreatedAt.IsZero() {
		parts = append(parts, "created_at="+t.CreatedAt.Format(time.RFC3339))
	}
	if !t.UpdatedAt.IsZero() {
		parts = append(parts, "updated_at="+t.UpdatedAt.Format(time.RFC3339))
	}

	for k, v := range t.Meta {
		switch k {
		case "prio":
			parts = append(parts, "prio:"+fmt.Sprintf("%v", v))
		case "domain":
			parts = append(parts, "domain:"+fmt.Sprintf("%v", v))
		case "due":
			parts = append(parts, "due:"+fmt.Sprintf("%v", v))
		default:
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}

	return strings.Join(parts, " ")
}

func renderTable(w io.Writer, tasks []*core.Task) {
	columns := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Title", Width: 40},
		{Title: "Status", Width: 15},
		{Title: "Assigned", Width: 15},
	}


	rows := []table.Row{}
	for _, t := range tasks {
		assignee := "-"
		if t.AssignedTo != nil {
			assignee = *t.AssignedTo
		}
	
		rows = append(rows, table.Row{
			t.ID,
			t.Title,
			formatStatus(t.Status),
			assignee,
		})
	}
	tbl := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(false),
		table.WithHeight(len(rows) + 1),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	tbl.SetStyles(s)

	fmt.Fprintln(w, tbl.View())
}

func formatStatus(status core.TaskStatus) string {
	switch status {
	case core.StatusTodo:
		return todoStyle.Render("TODO")
	case core.StatusInProgress:
		return inProgressStyle.Render("IN_PROGRESS")
	case core.StatusDone:
		return doneStyle.Render("DONE")
	case core.StatusSkipped:
		return skippedStyle.Render("SKIPPED")
	default:
		return string(status)
	}
}

func renderTaskDetail(w io.Writer, t *core.Task, logs []*core.LogEntry) {
	fmt.Fprintln(w, titleStyle.Render(fmt.Sprintf("Task: %s", t.ID)))
	fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Title:"), t.Title)
	fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Status:"), formatStatus(t.Status))
	
	assignee := "-"
	if t.AssignedTo != nil {
		assignee = *t.AssignedTo
	}
	fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Assigned:"), assignee)
	fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Tags:"), strings.Join(t.Tags, ", "))
	fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Reference:"), t.Reference)
	fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Created:"), t.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Updated:"), t.UpdatedAt.Format(time.RFC3339))

	if t.Description != "" {
		fmt.Fprintln(w, "\nDescription:")
		r, _ := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(80),
		)
		out, err := r.Render(t.Description)
		if err != nil {
			fmt.Fprintln(w, t.Description)
		} else {
			fmt.Fprint(w, out)
		}
	}

	if len(logs) > 0 {
		fmt.Fprintln(w, "\nLogs:")
		for _, l := range logs {
			fmt.Fprintf(w, "  %s  %-15s (%s) %s\n", 
				l.Timestamp.Format("2006-01-02 15:04:05"),
				l.Action,
				l.By,
				l.Note,
			)
		}
	}
}
