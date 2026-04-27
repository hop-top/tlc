package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	glamour "charm.land/glamour/v2"
	glamourstyles "charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	kittui "hop.top/kit/go/console/tui"
	"hop.top/tlc/internal/core"
)

const (
	headerSpacing      = 2
	searchInputPadding = 10
	kanbanMinColWidth  = 20
	kanbanColCount     = 3

	viewDashboard = "dashboard"
	viewSearch    = "search"
	viewKanban    = "kanban"
	viewFlows     = "flows"
	viewDetail    = "detail"
	viewForm      = "form"
	sortAsc       = "asc"
)

// renderMarkdown renders content with the model's cached glamour renderer.
// If the renderer is nil (not yet built or invalidated), it falls back to
// building one on the fly -- but that path is only hit before the first
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

func (m Model) formatStatus(status core.TaskStatus) string {
	return formatStatusWithStyles(status, m.styles)
}

func formatAssignee(assignee *string) string {
	if assignee == nil {
		return "-"
	}
	return "@" + *assignee
}


func (m Model) View() tea.View {
	view := tea.NewView(m.viewString())
	view.AltScreen = true
	return view
}

// viewString returns the rendered string content.
func (m Model) viewString() string {
	if m.err != nil {
		return m.styles.Error.Render(fmt.Sprintf("Error: %v", m.err))
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
// subtracting rendered header + footer + spacing.
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

	titleRendered := m.styles.Title.MaxWidth(m.width).Render(title)

	if m.view == viewSearch {
		m.searchInput.SetWidth(m.width - searchInputPadding)
		searchRendered := m.styles.Muted.Render("Search: ") + m.searchInput.View()
		return lipgloss.JoinVertical(lipgloss.Left, titleRendered, searchRendered)
	}

	if m.searchInput.Value() != "" || len(m.activeFilters) > 0 {
		var pills []kittui.Pill
		if m.searchInput.Value() != "" {
			pills = append(pills, kittui.NewPill("search", m.searchInput.Value()))
		}
		for _, f := range m.activeFilters {
			pills = append(pills, kittui.NewPill(f.Field, fmt.Sprintf("%v", f.Value)))
		}
		bar := kittui.NewPillBar(pills...)
		filterRendered := bar.ViewWithTheme(m.theme, m.width)
		return lipgloss.JoinVertical(lipgloss.Left, titleRendered, filterRendered)
	}

	return titleRendered
}

func (m Model) helpView() string {
	var items []string
	items = append(items, "[j/k] navigate")
	switch m.view {
	case viewDashboard:
		items = append(items,
			"[n]ew", "[c/u] claim/unclaim", "[s] status",
			"[/ ] search", "[f/a] filter tag/assignee",
		)
		if m.searchInput.Value() != "" || len(m.activeFilters) > 0 {
			items = append(items, "[esc] clear filter")
		}
		items = append(items, "[p] sync", "[v] cycle view", "[enter] details")
	case viewKanban:
		items = append(items, "[h/l] move", "[v] cycle view", "[enter] details")
	case viewFlows:
		items = append(items, "[v] cycle view")
	case viewDetail:
		items = append(items,
			"[c/u] claim/unclaim", "[s] status",
			"[o] sort dir", "[esc] back",
		)
	}
	items = append(items, "[r]efresh", "[q]uit")
	return m.styles.Muted.Render(strings.Join(items, "  "))
}

func (m Model) dashboardContent() string {
	if len(m.tasks) == 0 {
		return "No tasks found."
	}
	return m.taskList.View(m.width)
}

func (m Model) detailView() string {
	if len(m.tasks) == 0 || m.selected >= len(m.tasks) {
		return "No task selected."
	}

	task := m.tasks[m.selected]

	var s strings.Builder
	s.WriteString(m.styles.Title.Render(
		fmt.Sprintf("Task %s: %s", task.ID, task.Title),
	))
	s.WriteString("\n\n")

	fmt.Fprintf(&s, "Status:    %s\n", m.formatStatus(task.Status))
	fmt.Fprintf(&s, "Assigned:  %s\n", formatAssignee(task.AssignedTo))
	fmt.Fprintf(&s, "Reference: %s\n", task.Reference)
	if len(task.Tags) > 0 {
		var tagPills []kittui.Pill
		for _, tag := range task.Tags {
			tagPills = append(tagPills, kittui.NewPill("#", tag))
		}
		tagBar := kittui.NewPillBar(tagPills...)
		fmt.Fprintf(&s, "Tags:      %s\n", tagBar.ViewWithTheme(m.theme, m.width))
	} else {
		s.WriteString("Tags:      -\n")
	}

	if task.OriginSystem != nil && *task.OriginSystem != "" {
		syncStatus := "In Sync"
		if task.NeedsPush() {
			syncStatus = m.styles.Warning.Render("Pending Push")
		}
		fmt.Fprintf(&s, "Sync:      %s (%s)\n", syncStatus, *task.OriginSystem)
	}

	s.WriteString("\n")

	if task.Description != "" {
		s.WriteString("Description:\n")
		s.WriteString(m.styles.Box.Render(m.renderMarkdown(task.Description)))
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
	s.WriteString(m.styles.Title.Render("Kanban Board"))
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

	colWidth := (m.width - 4) / kanbanColCount
	if colWidth < kanbanMinColWidth {
		colWidth = kanbanMinColWidth
	}

	cols := make([]string, 0, len(kanbanStatusOrder))
	for _, status := range kanbanStatusOrder {
		tasks := groups[status]

		// Column header.
		header := fmt.Sprintf("%s (%d)",
			strings.ToUpper(string(status)), len(tasks),
		)
		headerStr := lipgloss.NewStyle().
			Width(colWidth).
			Align(lipgloss.Center).
			Bold(true).
			Foreground(lipgloss.Color("245")).
			Render(header)

		// Build kit/tui.List items for this column.
		var items []kittui.Item
		for _, task := range tasks {
			isSelected := len(m.tasks) > 0 && m.tasks[m.selected].ID == task.ID
			items = append(items, &kanbanCardItem{
				task:      task,
				selected:  isSelected,
				colWidth:  colWidth,
				styles:    m.styles,
				tagColors: m.tagColors,
			})
		}

		colList := kittui.NewList(m.height).SetItems(items)
		colContent := colList.View(colWidth)

		cols = append(cols, headerStr+"\n\n"+colContent)
	}

	s.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cols...))
	return s.String()
}

func (m Model) flowsContent() string {
	if len(m.flowRuns) == 0 {
		return "No flow runs found."
	}
	return m.flowList.View(m.width)
}
