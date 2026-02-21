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
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"hop.top/tlc/internal/core"
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
	case formatJSON:
		data, _ := json.MarshalIndent(tasks, "", "  ")
		_, _ = fmt.Fprintln(out, string(data))
	case formatYAML:
		data, _ := yaml.Marshal(tasks)
		_, _ = fmt.Fprintln(out, string(data))
	case "tls":
		for _, t := range tasks {
			_, _ = fmt.Fprintln(out, formatTLS(t))
		}
	default: // table
		renderTable(out, tasks)
	}
}

func printTask(cmd *cobra.Command, task *core.Task, logs []*core.LogEntry, format string) {
	out := cmd.OutOrStdout()
	switch format {
	case formatJSON:
		result := map[string]interface{}{
			"task": task,
			"logs": logs,
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		_, _ = fmt.Fprintln(out, string(data))
	case formatYAML:
		result := map[string]interface{}{
			"task": task,
			"logs": logs,
		}
		data, _ := yaml.Marshal(result)
		_, _ = fmt.Fprintln(out, string(data))
	default:
		renderTaskDetail(out, task, logs)
	}
}

func formatTLS(t *core.Task) string {
	wm := core.DefaultWorkflow()
	marker := " "
	def, err := wm.GetStatusDef(t.Status)
	if err == nil && def.TLSMarker != "" {
		marker = def.TLSMarker
	}
	status := fmt.Sprintf("[%s]", marker)

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

	rows := make([]table.Row, 0, len(tasks))
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
		table.WithHeight(len(rows)+1),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	tbl.SetStyles(s)

	_, _ = fmt.Fprintln(w, tbl.View())
}

func formatStatus(status core.TaskStatus) string {
	wm := core.DefaultWorkflow()
	def, err := wm.GetStatusDef(status)
	if err != nil {
		return string(status)
	}
	label := def.Name
	if def.Label != "" {
		label = def.Label
	}
	if def.Color != "" {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(def.Color)).Render(label)
	}
	// Fallback to legacy styles for default statuses without explicit color
	switch status {
	case core.StatusTodo:
		return todoStyle.Render(label)
	case core.StatusInProgress:
		return inProgressStyle.Render(label)
	case core.StatusDone:
		return doneStyle.Render(label)
	case core.StatusSkipped:
		return skippedStyle.Render(label)
	default:
		return label
	}
}

func renderTaskDetail(w io.Writer, t *core.Task, logs []*core.LogEntry) {
	_, _ = fmt.Fprintln(w, titleStyle.Render(fmt.Sprintf("Task: %s", t.ID)))
	_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Title:"), t.Title)
	_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Status:"), formatStatus(t.Status))

	assignee := "-"
	if t.AssignedTo != nil {
		assignee = *t.AssignedTo
	}
	_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Assigned:"), assignee)
	_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Tags:"), strings.Join(t.Tags, ", "))
	_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Reference:"), t.Reference)
	_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Created:"), t.CreatedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Updated:"), t.UpdatedAt.Format(time.RFC3339))

	if t.Description != "" {
		_, _ = fmt.Fprintln(w, "\nDescription:")
		r, _ := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(80),
		)
		out, err := r.Render(t.Description)
		if err != nil {
			_, _ = fmt.Fprintln(w, t.Description)
		} else {
			_, _ = fmt.Fprint(w, out)
		}
	}

	if len(logs) > 0 {
		_, _ = fmt.Fprintln(w, "\nLogs:")
		for _, l := range logs {
			_, _ = fmt.Fprintf(w, "  %s  %-15s (%s) %s\n",
				l.Timestamp.Format("2006-01-02 15:04:05"),
				l.Action,
				l.By,
				l.Note,
			)
		}
	}
}
