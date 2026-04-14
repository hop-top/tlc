package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	kitcli "hop.top/kit/cli"
	kittui "hop.top/kit/tui"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/tui/styles"
)

// Verify interface compliance.
var (
	_ kittui.Renderer = (*taskItem)(nil)
	_ kittui.Renderer = (*flowRunItem)(nil)
	_ kittui.Renderer = (*headerItem)(nil)
	_ kittui.Renderer = (*spacerItem)(nil)
	_ kittui.Renderer = (*kanbanCardItem)(nil)
)

// taskItem renders a single task row in the dashboard list.
type taskItem struct {
	task      *core.Task
	selected  bool
	styles    *styles.Styles
	tagColors map[string]string
}

func (ti *taskItem) Render(width int) string {
	cursor := " "
	if ti.selected {
		cursor = ti.styles.InProgress.Render("►")
	}

	status := formatStatusWithStyles(ti.task.Status, ti.styles)
	title := ti.task.Title
	if ti.selected {
		title = lipgloss.NewStyle().Bold(true).Render(title)
	}

	assignee := ""
	if ti.task.AssignedTo != nil {
		assignee = fmt.Sprintf(" @%s", *ti.task.AssignedTo)
	}

	tags := ""
	for _, tag := range ti.task.Tags {
		tags += " " + getTagStyleFrom(tag, ti.tagColors, ti.styles).
			Render("#"+tag)
	}

	syncIcon := ""
	if ti.task.NeedsPush() {
		syncIcon = ti.styles.Warning.Render(" ↑")
	}

	return fmt.Sprintf("%s %s %s %s%s%s%s",
		cursor, ti.task.ID, status, title,
		syncIcon, ti.styles.Muted.Render(assignee), tags,
	)
}

// flowRunItem renders a single flow run row with status indicator
// and kit/tui.Progress bar for running flows.
type flowRunItem struct {
	run      *core.FlowRun
	selected bool
	styles   *styles.Styles
	theme    kitcli.Theme
}

func (fi *flowRunItem) Render(_ int) string {
	cursor := " "
	if fi.selected {
		cursor = fi.styles.InProgress.Render("►")
	}

	// Status indicator with semantic colors.
	var statusStr string
	switch fi.run.Status {
	case core.FlowStatusRunning:
		statusStr = fi.styles.InProgress.Render("● " + string(fi.run.Status))
	case core.FlowStatusSucceeded:
		statusStr = fi.styles.Done.Render("✓ " + string(fi.run.Status))
	case core.FlowStatusFailed:
		statusStr = fi.styles.Error.Render("✗ " + string(fi.run.Status))
	case core.FlowStatusQueued:
		statusStr = fi.styles.Muted.Render("◌ " + string(fi.run.Status))
	case core.FlowStatusPaused:
		statusStr = fi.styles.Warning.Render("⏸ " + string(fi.run.Status))
	default:
		statusStr = string(fi.run.Status)
	}

	startedAt := fi.run.StartedAt.Format("2006-01-02 15:04:05")

	line := fmt.Sprintf("%s %s %s %s (%s)",
		cursor, fi.run.ID, fi.run.FlowID, statusStr, startedAt,
	)

	// Show progress bar for running or paused flows.
	if fi.run.Status == core.FlowStatusRunning ||
		fi.run.Status == core.FlowStatusPaused {
		prog := kittui.NewProgress(fi.theme).
			SetPercent(fi.run.Progress).
			SetWidth(20)
		pct := fmt.Sprintf(" %3.0f%%", fi.run.Progress*100)
		line += " " + prog.View() + fi.styles.Muted.Render(pct)
	}

	return line
}

// headerItem renders a status group header.
type headerItem struct {
	text string
}

func (hi *headerItem) Render(_ int) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Bold(true).
		Render(strings.ToUpper(hi.text))
}

// spacerItem renders an empty line.
type spacerItem struct{}

func (si *spacerItem) Render(_ int) string { return "" }

// kanbanCardItem renders a task card in the kanban board.
type kanbanCardItem struct {
	task      *core.Task
	selected  bool
	colWidth  int
	styles    *styles.Styles
	tagColors map[string]string
}

func (ki *kanbanCardItem) Render(_ int) string {
	style := lipgloss.NewStyle().
		Width(ki.colWidth - 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))

	if ki.selected {
		style = style.
			BorderForeground(ki.styles.PrimaryColor).
			Bold(true)
	}

	// Build card content: ID + title + tags.
	var b strings.Builder
	b.WriteString(ki.task.ID)
	b.WriteByte('\n')
	b.WriteString(ki.task.Title)

	if len(ki.task.Tags) > 0 {
		b.WriteByte('\n')
		for i, tag := range ki.task.Tags {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(
				getTagStyleFrom(tag, ki.tagColors, ki.styles).
					Render("#" + tag),
			)
		}
	}

	return style.Render(b.String())
}

// formatStatusWithStyles renders a task status icon using the given styles.
func formatStatusWithStyles(status core.TaskStatus, s *styles.Styles) string {
	switch status {
	case core.StatusTodo:
		return s.Todo.Render("[ ]")
	case core.StatusInProgress:
		return s.InProgress.Render("[~]")
	case core.StatusDone:
		return s.Done.Render("[x]")
	case core.StatusSkipped:
		return s.Skipped.Render("[-]")
	default:
		return string(status)
	}
}

// getTagStyleFrom returns the lipgloss style for a tag from the tagColors map.
func getTagStyleFrom(
	tag string,
	tagColors map[string]string,
	s *styles.Styles,
) lipgloss.Style {
	color, ok := tagColors[tag]
	if !ok {
		h := 0
		for _, c := range tag {
			h += int(c)
		}
		color = s.TagColors[h%len(s.TagColors)]
		tagColors[tag] = color
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
}
