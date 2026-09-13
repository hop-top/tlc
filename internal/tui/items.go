package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	kittui "hop.top/kit/go/console/tui"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/tui/styles"
)

// Verify interface compliance.
var (
	_ kittui.Renderer = (*taskItem)(nil)
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

	return fmt.Sprintf(
		"%s %s %s %s%s%s%s",
		cursor, displayAlias(ti.task), status, title,
		syncIcon, ti.styles.Muted.Render(assignee), tags,
	)
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

	// Build card content: alias + title + tags.
	var b strings.Builder
	b.WriteString(displayAlias(ki.task))
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

// displayAlias renders a task's human-side reference. Prefers the
// FormatTaskAlias (T-NNNN, derived from Seq) and falls back to the
// durable id when Seq is unset (e.g. legacy or unseeded test data).
func displayAlias(t *core.Task) string {
	if t == nil {
		return ""
	}
	if alias := core.FormatTaskAlias(t); alias != "" {
		return alias
	}
	return t.ID
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
