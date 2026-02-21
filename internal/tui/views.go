package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/tui/styles"
	"github.com/spf13/viper"
)

const (
	headerSpacing      = 2
	viewportOffset     = 6
	searchInputPadding = 10
	kanbanMinColWidth  = 20
	kanbanColCount     = 3
)

var statusOrder = []core.TaskStatus{
	core.StatusTodo,
	core.StatusInProgress,
	core.StatusDone,
	core.StatusSkipped,
}

func formatStatus(status core.TaskStatus) string {
	switch status {
	case core.StatusTodo:
		return styles.TodoStyle.Render("[ ]")
	case core.StatusInProgress:
		return styles.InProgressStyle.Render("[~]")
	case core.StatusDone:
		return styles.DoneStyle.Render("[x]")
	case core.StatusSkipped:
		return styles.SkippedStyle.Render("[-]")
	default:
		return string(status)
	}
}

func formatAssignee(assignee *string) string {
	if assignee == nil {
		return "-"
	}
	return "@" + *assignee
}

func (m Model) getTagStyle(tag string) lipgloss.Style {
	colors := viper.GetStringMapString("ui.tag_colors")
	if colors == nil {
		colors = make(map[string]string)
	}

	color, ok := colors[tag]
	if !ok {
		// Use hash for stable but random-looking color assignment
		h := 0
		for _, c := range tag {
			h += int(c)
		}
		color = styles.TagColors[h%len(styles.TagColors)]

		// Save to config for future consistency
		colors[tag] = color
		viper.Set("ui.tag_colors", colors)
		viper.WriteConfig()
	}

	return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
}

func (m Model) View() string {
	if m.err != nil {
		return styles.ErrorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	if m.view == "form" {
		if m.form != nil {
			return m.form.View()
		}
		return "Loading form..."
	}

	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	header := m.headerView()
	footer := m.helpView()

	var content string
	switch m.view {
	case "theme_picker":
		return m.themePicker.View()
	case "detail":
		content = m.detailView()
	case "kanban":
		content = m.kanbanView()
	case "flows":
		content = m.flowsContent()
	default: // dashboard or search
		content = m.dashboardContent()
	}

	m.viewport.SetContent(content)

	// Calculate available height for viewport
	headerHeight := lipgloss.Height(header)
	footerHeight := lipgloss.Height(footer)
	occupiedHeight := headerHeight + footerHeight + headerSpacing

	vh := m.height - occupiedHeight
	if vh < 1 {
		vh = 1
	}
	m.viewport.Height = vh

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"", // Spacer
		m.viewport.View(),
		"", // Spacer
		footer,
	)
}

func (m Model) headerView() string {
	title := "TLC Dashboard"
	switch m.view {
	case "kanban":
		title = "Kanban Board"
	case "flows":
		title = "Flow Executions"
	case "detail":
		title = "Task Details"
	}

	titleRendered := styles.TitleStyle.MaxWidth(m.width).Render(title)

	if m.view == "search" {
		m.searchInput.Width = m.width - searchInputPadding
		searchRendered := styles.MutedStyle.Render("Search: ") + m.searchInput.View()
		return lipgloss.JoinVertical(lipgloss.Left, titleRendered, searchRendered)
	}

	if m.searchInput.Value() != "" || len(m.activeFilters) > 0 {
		var filterParts []string
		if m.searchInput.Value() != "" {
			filterParts = append(filterParts, fmt.Sprintf("search:%s", m.searchInput.Value()))
		}
		for _, f := range m.activeFilters {
			filterParts = append(filterParts, fmt.Sprintf("%s:%v", f.Field, f.Value))
		}
		filterRendered := styles.MutedStyle.Render(fmt.Sprintf("Filtered by: %s", strings.Join(filterParts, ", ")))
		return titleRendered + " | " + filterRendered
	}

	return titleRendered
}

func (m Model) helpView() string {
	var items []string
	items = append(items, "[j/k] navigate")
	switch m.view {
	case "dashboard":
		items = append(items, "[n]ew", "[c/u] claim/unclaim", "[s] status", "[/ ] search", "[f/a] filter tag/assignee")
		if m.searchInput.Value() != "" || len(m.activeFilters) > 0 {
			items = append(items, "[esc] clear filter")
		}
		items = append(items, "[p] sync", "[t] theme", "[v] cycle view", "[enter] details")
	case "theme_picker":
		items = append(items, "[enter] select", "[R] fetch remote", "[esc] cancel")
	case "kanban":
		items = append(items, "[h/l] move", "[v] cycle view", "[enter] details")
	case "flows":
		items = append(items, "[v] cycle view")
	case "detail":
		items = append(items, "[c/u] claim/unclaim", "[s] status", "[o] sort dir", "[esc] back")
	}
	items = append(items, "[r]efresh", "[q]uit")
	return styles.MutedStyle.Render(strings.Join(items, "  "))
}

func (m Model) dashboardContent() string {
	if len(m.tasks) == 0 {
		return "No tasks found."
	}

	var s strings.Builder
	groups := make(map[core.TaskStatus][]*core.Task)
	for _, t := range m.tasks {
		groups[t.Status] = append(groups[t.Status], t)
	}

	currentIndex := 0
	for _, status := range statusOrder {
		tasks := groups[status]
		if len(tasks) == 0 {
			continue
		}

		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true).Render(strings.ToUpper(string(status))))
		s.WriteString("\n")

		for _, task := range tasks {
			cursor := " "
			if currentIndex == m.selected {
				cursor = styles.InProgressStyle.Render("►")
			}

			statusIcon := formatStatus(task.Status)
			title := task.Title
			if currentIndex == m.selected {
				title = lipgloss.NewStyle().Bold(true).Render(title)
			}

			assignee := ""
			if task.AssignedTo != nil {
				assignee = fmt.Sprintf(" @%s", *task.AssignedTo)
			}

			tags := ""
			for _, tag := range task.Tags {
				tags += " " + m.getTagStyle(tag).Render("#"+tag)
			}

			syncIcon := ""
			if task.NeedsPush() {
				syncIcon = styles.WarningStyle.Render(" ↑")
			}

			s.WriteString(fmt.Sprintf("%s %s %s %s%s%s%s\n", cursor, task.ID, statusIcon, title, syncIcon, styles.MutedStyle.Render(assignee), tags))
			currentIndex++
		}

		s.WriteString("\n")
	}

	return s.String()
}

func (m Model) detailView() string {
	if len(m.tasks) == 0 || m.selected >= len(m.tasks) {
		return "No task selected."
	}

	task := m.tasks[m.selected]

	var s strings.Builder
	s.WriteString(styles.TitleStyle.Render(fmt.Sprintf("Task %s: %s", task.ID, task.Title)))
	s.WriteString("\n\n")

	s.WriteString(fmt.Sprintf("Status:    %s\n", formatStatus(task.Status)))
	s.WriteString(fmt.Sprintf("Assigned:  %s\n", formatAssignee(task.AssignedTo)))
	s.WriteString(fmt.Sprintf("Reference: %s\n", task.Reference))
	s.WriteString(fmt.Sprintf("Tags:      %s\n", strings.Join(task.Tags, ", ")))

	if task.OriginSystem != nil && *task.OriginSystem != "" {
		syncStatus := "In Sync"
		if task.NeedsPush() {
			syncStatus = styles.WarningStyle.Render("Pending Push")
		}
		s.WriteString(fmt.Sprintf("Sync:      %s (%s)\n", syncStatus, *task.OriginSystem))
	}

	s.WriteString("\n")

	if task.Description != "" {
		s.WriteString("Description:\n")
		renderer, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(m.width-10),
		)
		if err == nil {
			out, err := renderer.Render(task.Description)
			if err == nil {
				s.WriteString(styles.BoxStyle.Render(out))
			} else {
				s.WriteString(styles.BoxStyle.Render(task.Description))
			}
		} else {
			s.WriteString(styles.BoxStyle.Render(task.Description))
		}
		s.WriteString("\n")
	}

	if len(m.taskLogs) > 0 {
		s.WriteString(fmt.Sprintf("\nLogs (%s):\n", strings.ToUpper(m.logSortDirection)))
		for _, l := range m.taskLogs {
			s.WriteString(fmt.Sprintf("  %s  %-15s (%s) %s\n",
				l.Timestamp.Format("2006-01-02 15:04:05"),
				l.Action,
				l.By,
				l.Note,
			))
		}
	}

	return s.String()
}

func (m Model) kanbanView() string {
	var s strings.Builder
	s.WriteString(styles.TitleStyle.Render("Kanban Board"))
	s.WriteString("\n\n")

	kanbanStatusOrder := []core.TaskStatus{
		core.StatusTodo,
		core.StatusInProgress,
		core.StatusDone,
	}

	groups := make(map[core.TaskStatus][]*core.Task)
	for _, t := range m.tasks {
		groups[t.Status] = append(groups[t.Status], t)
	}

	var cols []string
	colWidth := (m.width - 4) / kanbanColCount
	if colWidth < kanbanMinColWidth {
		colWidth = kanbanMinColWidth
	}

	for _, status := range kanbanStatusOrder {
		var col strings.Builder
		tasks := groups[status]

		header := fmt.Sprintf("%s (%d)", strings.ToUpper(string(status)), len(tasks))
		col.WriteString(lipgloss.NewStyle().
			Width(colWidth).
			Align(lipgloss.Center).
			Bold(true).
			Foreground(lipgloss.Color("245")).
			Render(header))
		col.WriteString("\n\n")

		for _, task := range tasks {
			isSelected := false
			if len(m.tasks) > 0 && m.tasks[m.selected].ID == task.ID {
				isSelected = true
			}

			style := lipgloss.NewStyle().
				Width(colWidth - 2).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240"))

			if isSelected {
				style = style.BorderForeground(styles.PrimaryColor).Bold(true)
			}

			card := fmt.Sprintf("%s\n%s", task.ID, task.Title)
			col.WriteString(style.Render(card))
			col.WriteString("\n")
		}
		cols = append(cols, col.String())
	}

	s.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cols...))
	return s.String()
}

func (m Model) flowsContent() string {
	if len(m.flowRuns) == 0 {
		return "No flow runs found."
	}

	var s strings.Builder
	for i, run := range m.flowRuns {
		cursor := " "
		if i == m.selected {
			cursor = styles.InProgressStyle.Render("►")
		}

		status := string(run.Status)
		startedAt := run.StartedAt.Format("2006-01-02 15:04:05")

		s.WriteString(fmt.Sprintf("%s %s %s %s (%s)\n", cursor, run.ID, run.FlowID, status, startedAt))
	}

	return s.String()
}
