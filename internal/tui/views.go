package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	glamour "charm.land/glamour/v2"
	glamourstyles "charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/tui/styles"
)

const (
	headerSpacing      = 2
	searchInputPadding = 10
	kanbanMinColWidth  = 20
	kanbanColCount     = 3

	viewDashboard   = "dashboard"
	viewSearch      = "search"
	viewKanban      = "kanban"
	viewFlows       = "flows"
	viewDetail      = "detail"
	viewThemePicker = "theme_picker"
	viewForm        = "form"
	sortAsc         = "asc"
)

// renderMarkdown renders content with the model's cached glamour renderer.
// If the renderer is nil (not yet built or invalidated), it falls back to
// building one on the fly — but that path is only hit before the first
// WindowSizeMsg arrives.
func (m Model) renderMarkdown(content string) string {
	r := m.mdRenderer
	if r == nil {
		w := m.mdRenderWidth
		if w <= 0 {
			w = 80
		}
		var err error
		r, err = glamour.NewTermRenderer(
			glamour.WithStyles(glamourstyles.DarkStyleConfig),
			glamour.WithWordWrap(w),
		)
		if err != nil {
			return content
		}
		// Note: cannot persist r back into value-receiver model here;
		// renderer is rebuilt on first render after startup or resize.
		// After the first WindowSizeMsg, m.mdRenderer is populated by Update().
	}
	out, err := r.Render(content)
	if err != nil {
		return content
	}
	return out
}

var statusOrder = []core.TaskStatus{
	core.StatusTodo,
	core.StatusInProgress,
	core.StatusDone,
	core.StatusSkipped,
}

func formatStatus(status core.TaskStatus) string {
	switch status {
	case core.StatusTodo:
		return styles.Current.Todo.Render("[ ]")
	case core.StatusInProgress:
		return styles.Current.InProgress.Render("[~]")
	case core.StatusDone:
		return styles.Current.Done.Render("[x]")
	case core.StatusSkipped:
		return styles.Current.Skipped.Render("[-]")
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

// getTagStyle returns the lipgloss style for a tag, using the model's in-memory
// tagColors map. No disk I/O here — tagColors is populated in NewModel from viper
// and new assignments are written back to viper lazily (not on every render).
// Maps are reference types in Go, so mutations here propagate to the caller's copy.
func (m Model) getTagStyle(tag string) lipgloss.Style {
	color, ok := m.tagColors[tag]
	if !ok {
		// Stable hash → palette index.
		h := 0
		for _, c := range tag {
			h += int(c)
		}
		color = styles.Current.TagColors[h%len(styles.Current.TagColors)]
		m.tagColors[tag] = color // map mutation propagates; no copy-on-write issue
	}

	return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
}

func (m Model) View() tea.View {
	view := tea.NewView(m.viewString())
	view.AltScreen = true
	return view
}

// viewString returns the rendered string content, wrapped by View() into a
// tea.View. Helper view methods (headerView, helpView, dashboardContent,
// detailView, kanbanView, flowsContent) keep returning string and are
// composed here.
func (m Model) viewString() string {
	if m.err != nil {
		return styles.Current.Error.Render(fmt.Sprintf("Error: %v", m.err))
	}

	if m.view == viewForm {
		if m.form != nil {
			return m.form.View()
		}
		return "Loading form..."
	}

	header := m.headerView()
	footer := m.helpView()

	var content string
	switch m.view {
	case viewThemePicker:
		// themePicker.View() returns tea.View; extract its string content
		// since themepicker is embedded in this Model and inherits our View
		// options (AltScreen, etc.) from the parent.
		return m.themePicker.View().Content
	case viewDetail:
		content = m.detailView()
	case viewKanban:
		content = m.kanbanView()
	case viewFlows:
		content = m.flowsContent()
	default: // dashboard or search
		content = m.dashboardContent()
	}

	m.viewport.SetContent(content)

	// Note: viewport height is computed centrally in Update() via
	// effectiveViewportHeight(). We don't set it here because View() has a
	// value receiver — any mutation would be discarded, causing syncViewport()
	// to read a stale stored height. See Model.effectiveViewportHeight().

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"", // Spacer
		m.viewport.View(),
		"", // Spacer
		footer,
	)
}

// effectiveViewportHeight computes the viewport height available after
// subtracting rendered header + footer + spacing. This is the single source
// of truth used by Update() on WindowSizeMsg to keep the stored viewport
// height in sync with what View() will actually render. Requires m.width
// and m.height to be set before calling (headerView() depends on m.width).
func (m Model) effectiveViewportHeight() int {
	headerHeight := lipgloss.Height(m.headerView())
	footerHeight := lipgloss.Height(m.helpView())
	occupied := headerHeight + footerHeight + headerSpacing
	vh := m.height - occupied
	if vh < 1 {
		vh = 1
	}
	return vh
}

func (m Model) headerView() string {
	title := "TLC Dashboard"
	switch m.view {
	case viewKanban:
		title = "Kanban Board"
	case viewFlows:
		title = "Flow Executions"
	case viewDetail:
		title = "Task Details"
	}

	titleRendered := styles.Current.Title.MaxWidth(m.width).Render(title)

	if m.view == viewSearch {
		m.searchInput.SetWidth(m.width - searchInputPadding)
		searchRendered := styles.Current.Muted.Render("Search: ") + m.searchInput.View()
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
		filterRendered := styles.Current.Muted.Render(fmt.Sprintf("Filtered by: %s", strings.Join(filterParts, ", ")))
		return titleRendered + " | " + filterRendered
	}

	return titleRendered
}

func (m Model) helpView() string {
	var items []string
	items = append(items, "[j/k] navigate")
	switch m.view {
	case viewDashboard:
		items = append(items, "[n]ew", "[c/u] claim/unclaim", "[s] status", "[/ ] search", "[f/a] filter tag/assignee")
		if m.searchInput.Value() != "" || len(m.activeFilters) > 0 {
			items = append(items, "[esc] clear filter")
		}
		items = append(items, "[p] sync", "[t] theme", "[v] cycle view", "[enter] details")
	case viewThemePicker:
		items = append(items, "[enter] select", "[R] fetch remote", "[esc] cancel")
	case viewKanban:
		items = append(items, "[h/l] move", "[v] cycle view", "[enter] details")
	case viewFlows:
		items = append(items, "[v] cycle view")
	case viewDetail:
		items = append(items, "[c/u] claim/unclaim", "[s] status", "[o] sort dir", "[esc] back")
	}
	items = append(items, "[r]efresh", "[q]uit")
	return styles.Current.Muted.Render(strings.Join(items, "  "))
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
				cursor = styles.Current.InProgress.Render("►")
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
				syncIcon = styles.Current.Warning.Render(" ↑")
			}

			fmt.Fprintf(&s, "%s %s %s %s%s%s%s\n", cursor, task.ID, statusIcon, title, syncIcon, styles.Current.Muted.Render(assignee), tags)
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
	s.WriteString(styles.Current.Title.Render(fmt.Sprintf("Task %s: %s", task.ID, task.Title)))
	s.WriteString("\n\n")

	fmt.Fprintf(&s, "Status:    %s\n", formatStatus(task.Status))
	fmt.Fprintf(&s, "Assigned:  %s\n", formatAssignee(task.AssignedTo))
	fmt.Fprintf(&s, "Reference: %s\n", task.Reference)
	fmt.Fprintf(&s, "Tags:      %s\n", strings.Join(task.Tags, ", "))

	if task.OriginSystem != nil && *task.OriginSystem != "" {
		syncStatus := "In Sync"
		if task.NeedsPush() {
			syncStatus = styles.Current.Warning.Render("Pending Push")
		}
		fmt.Fprintf(&s, "Sync:      %s (%s)\n", syncStatus, *task.OriginSystem)
	}

	s.WriteString("\n")

	if task.Description != "" {
		s.WriteString("Description:\n")
		s.WriteString(styles.Current.Box.Render(m.renderMarkdown(task.Description)))
		s.WriteString("\n")
	}

	if len(m.taskLogs) > 0 {
		fmt.Fprintf(&s, "\nLogs (%s):\n", strings.ToUpper(m.logSortDirection))
		for _, l := range m.taskLogs {
			fmt.Fprintf(&s, "  %s  %-15s (%s) %s\n",
				l.Timestamp.Format("2006-01-02 15:04:05"),
				l.Action,
				l.By,
				l.Note,
			)
		}
	}

	return s.String()
}

func (m Model) kanbanView() string {
	var s strings.Builder
	s.WriteString(styles.Current.Title.Render("Kanban Board"))
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

	cols := make([]string, 0, len(kanbanStatusOrder))
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
				style = style.BorderForeground(styles.PrimaryColor).Bold(true) //nolint:staticcheck // PrimaryColor needed until Colors added to Styles struct
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
			cursor = styles.Current.InProgress.Render("►")
		}

		status := string(run.Status)
		startedAt := run.StartedAt.Format("2006-01-02 15:04:05")

		fmt.Fprintf(&s, "%s %s %s %s (%s)\n", cursor, run.ID, run.FlowID, status, startedAt)
	}

	return s.String()
}
