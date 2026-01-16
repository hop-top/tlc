package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/oss-tlc-cli/internal/core"
	"github.com/google/oss-tlc-cli/internal/tui/styles"
	"github.com/google/oss-tlc-cli/pkg/themepicker"
	"github.com/spf13/viper"
)

type tasksMsg []*core.Task
type flowRunsMsg []*core.FlowRun
type logsMsg []*core.LogEntry

type Model struct {
	service         *core.TaskService
	view            string // "dashboard", "list", "detail", "search", "form", "kanban", "flows", "theme_picker"
	tasks           []*core.Task
	flowRuns        []*core.FlowRun
	selected        int
	width           int
	height          int
	searchInput     textinput.Model
	activeFilters   []core.FieldFilter
	viewport        viewport.Model
	taskLogs        []*core.LogEntry
	logSortDirection string
	form            *huh.Form
	themePicker     themepicker.Model
	taskTitle       string
	taskDescription string
	err             error
}

func NewModel(service *core.TaskService) Model {

	ti := textinput.New()
	ti.Placeholder = "Search tasks..."

	vp := viewport.New(0, 0)

	direction := viper.GetString("ui.log_sort_direction")
	if direction == "" {
		direction = "desc"
	}

	// Initialize theme picker with default theme
	// In the future, we can load more themes here
	defaultTheme := styles.DefaultTheme()
	tp := themepicker.New([]themepicker.Theme{defaultTheme})
	tp.SetFetcher(themepicker.FetchTheme)

	return Model{
		service:     service,
		view:        "dashboard",
		searchInput: ti,
		viewport:    vp,
		logSortDirection:    direction,
		themePicker: tp,
	}
}

func (m Model) Init() tea.Cmd {
	return m.fetchTasks
}

func (m Model) fetchTasks() tea.Msg {
	query := core.Query{
		Search:  m.searchInput.Value(),
		Filters: m.activeFilters,
	}
	tasks, err := m.service.ListTasks(context.Background(), query)
	if err != nil {
		return err
	}

	// Sort tasks by status order
	statusOrder := map[core.TaskStatus]int{
		core.StatusTodo:       0,
		core.StatusInProgress: 1,
		core.StatusDone:       2,
		core.StatusSkipped:    3,
	}

	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].Status != tasks[j].Status {
			return statusOrder[tasks[i].Status] < statusOrder[tasks[j].Status]
		}
		return tasks[i].ID < tasks[j].ID
	})

	return tasksMsg(tasks)
}

func (m Model) fetchLogs() tea.Msg {
	if len(m.tasks) == 0 || m.selected >= len(m.tasks) {
		return nil
	}
	taskID := m.tasks[m.selected].ID
	logs, err := m.service.GetLogs(context.Background(), taskID, m.logSortDirection)
	if err != nil {
		return err
	}
	return logsMsg(logs)
}

func (m Model) claimTask(id string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		user := core.GetCurrentUser()
		err := m.service.ClaimTask(ctx, id, user, "Claimed via TUI")
		if err != nil {
			return err
		}
		return m.fetchTasks()
	}
}

func (m Model) unclaimTask(id string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		user := core.GetCurrentUser()
		err := m.service.UnclaimTask(ctx, id, user, "Unclaimed via TUI")
		if err != nil {
			return err
		}
		return m.fetchTasks()
	}
}

func (m Model) rotateStatus(task *core.Task) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var next core.TaskStatus
		switch task.Status {
		case core.StatusTodo:
			next = core.StatusInProgress
		case core.StatusInProgress:
			next = core.StatusDone
		case core.StatusDone:
			next = core.StatusTodo
		default:
			next = core.StatusTodo
		}

		err := m.service.TransitionStatus(ctx, task.ID, next, core.GetCurrentUser(), "Rotated via TUI")
		if err != nil {
			return err
		}
		return m.fetchTasks()
	}
}

func (m Model) createTask() tea.Cmd {
	m.taskTitle = ""
	m.taskDescription = ""

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Task Title").
				Value(&m.taskTitle).
				Validate(func(s string) error {
					if len(s) == 0 {
						return fmt.Errorf("title required")
					}
					return nil
				}),
			huh.NewText().
				Title("Description").
				Value(&m.taskDescription).
				Lines(5),
		),
	)
	m.view = "form"
	return m.form.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 1. Handle common messages first
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 6 // Initial estimate, refined in View()
		
		// Resize theme picker
		var cmd tea.Cmd
		var tm tea.Model
		tm, cmd = m.themePicker.Update(msg)
		m.themePicker = tm.(themepicker.Model)
		return m, cmd
	case tea.MouseMsg:
		if msg.Type == tea.MouseLeft && m.view == "dashboard" {
			// Basic selection on click
			// In a real TUI we'd map coords, but for now we'll support navigation
		}
	case error:
		m.err = msg
		return m, nil
	}

	// 2. View-specific handlers
	if m.view == "theme_picker" {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if msg.String() == "esc" {
				m.view = "dashboard"
				return m, nil
			}
			if msg.String() == "R" {
				// Trigger fetch remote themes
				return m, func() tea.Msg {
					names, err := themepicker.FetchThemeNames()
					if err != nil {
						return fmt.Errorf("failed to fetch themes: %w", err)
					}
					
					var themes []themepicker.Theme
					themes = append(themes, styles.DefaultTheme()) // Keep default
					
					for _, name := range names {
						themes = append(themes, themepicker.LazyTheme{Name: name})
					}
					return themes // Returns []themepicker.Theme as Msg
				}
			}
		case []themepicker.Theme:
			// Re-init picker with new themes
			m.themePicker = themepicker.New(msg)
			m.themePicker.SetFetcher(themepicker.FetchTheme)
			// Resize it immediately
			tm, _ := m.themePicker.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			m.themePicker = tm.(themepicker.Model)
			return m, nil
		case themepicker.ThemeSelectedMsg:
			var themeToApply styles.Theme
			
			// Apply theme
			if t, ok := msg.Theme.(styles.Theme); ok {
				themeToApply = t
			} else if bt, ok := msg.Theme.(themepicker.BasicTheme); ok {
				// Convert BasicTheme to styles.Theme
				themeToApply = styles.Theme{
					Name:       bt.NameVal,
					Primary:    bt.PrimaryVal,
					Secondary:  bt.SecondaryVal,
					Success:    bt.SuccessVal,
					Warning:    bt.WarningVal,
					Error:      bt.ErrorVal,
					Muted:      bt.MutedVal,
					Background: bt.BackgroundVal,
					Foreground: bt.ForegroundVal,
					TagColors:  styles.DefaultTheme().TagColors, // Keep default tag colors for now or map
				}
			}
			
			styles.ApplyTheme(themeToApply)
			
			// Save to config
			viper.Set("ui.theme", themeToApply.Name)
			viper.WriteConfig()

			m.view = "dashboard"
			return m, nil
		}
		
		var cmd tea.Cmd
		var tm tea.Model
		tm, cmd = m.themePicker.Update(msg)
		m.themePicker = tm.(themepicker.Model)
		return m, cmd
	}

	if m.view == "form" && m.form != nil {
		form, cmd := m.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.form = f
		}
		if m.form.State == huh.StateCompleted {
			m.view = "dashboard"
			return m, m.saveTask(m.taskTitle, m.taskDescription)
		}
		if m.form.State == huh.StateAborted {
			m.view = "dashboard"
			return m, nil
		}
		return m, cmd
	}

	if m.view == "search" {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter", "esc":
				m.view = "dashboard"
				return m, m.fetchTasks
			}
		}
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		// Live search: trigger fetch on every update
		return m, tea.Batch(cmd, m.fetchTasks)
	}

	// 3. Main navigation handlers
	switch msg := msg.(type) {
	case tasksMsg:
		m.tasks = msg
		if m.selected >= len(m.tasks) && len(m.tasks) > 0 {
			m.selected = len(m.tasks) - 1
		}
		m = m.syncViewport()
		return m, nil
	case flowRunsMsg:
		m.flowRuns = msg
		return m, nil
	case logsMsg:
		m.taskLogs = msg
		m = m.syncViewport()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "/":
			m.view = "search"
			m.searchInput.Focus()
			m.searchInput.SetValue("") // Clear on start
			return m, nil
		case "j", "down":
			if m.view == "flows" {
				if m.selected < len(m.flowRuns)-1 {
					m.selected++
				}
			} else {
				if m.selected < len(m.tasks)-1 {
					m.selected++
				}
			}
			m = m.syncViewport()
		case "k", "up":
			if m.selected > 0 {
				m.selected--
			}
			m = m.syncViewport()
		case "h", "left":
			if m.view == "kanban" && len(m.tasks) > 0 {
				return m, m.moveTask(m.tasks[m.selected], -1)
			}
		case "l", "right":
			if m.view == "kanban" && len(m.tasks) > 0 {
				return m, m.moveTask(m.tasks[m.selected], 1)
			}
		case "r":
			if m.view == "flows" {
				return m, m.fetchFlowRuns
			}
			return m, m.fetchTasks
		case "p":
			return m, m.syncPull
		case "v":
			m.selected = 0
			switch m.view {
			case "dashboard":
				m.view = "kanban"
			case "kanban":
				m.view = "flows"
				return m, m.fetchFlowRuns
			default:
				m.view = "dashboard"
				return m, m.fetchTasks
			}
		case "f": // Filter by tags of selected task
			if m.view == "dashboard" && len(m.tasks) > 0 {
				task := m.tasks[m.selected]
				for _, tag := range task.Tags {
					m = m.addFilter("tags", tag)
				}
				return m, m.fetchTasks
			}
		case "a": // Filter by assignee of selected task
			if m.view == "dashboard" && len(m.tasks) > 0 {
				task := m.tasks[m.selected]
				if task.AssignedTo != nil {
					m = m.addFilter("assigned_to", *task.AssignedTo)
					return m, m.fetchTasks
				}
			}
		case "n":
			if m.view != "flows" {
				return m, m.createTask()
			}
		case "t":
			m.view = "theme_picker"
			// Ensure it has correct size
			tm, _ := m.themePicker.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			m.themePicker = tm.(themepicker.Model)
			return m, nil
		case "c":
			if (m.view == "dashboard" || m.view == "detail" || m.view == "kanban") && len(m.tasks) > 0 {
				return m, m.claimTask(m.tasks[m.selected].ID)
			}
		case "u":
			if (m.view == "dashboard" || m.view == "detail" || m.view == "kanban") && len(m.tasks) > 0 {
				return m, m.unclaimTask(m.tasks[m.selected].ID)
			}
		case "s":
			if (m.view == "dashboard" || m.view == "detail" || m.view == "kanban") && len(m.tasks) > 0 {
				return m, m.rotateStatus(m.tasks[m.selected])
			}
		case "enter":
			if m.view == "flows" {
				// Show flow details? Not implemented yet
			} else if len(m.tasks) > 0 {
				m.view = "detail"
				return m, m.fetchLogs
			}
		case "o": // Toggle log order
			if m.view == "detail" {
				if m.logSortDirection == "desc" {
					m.logSortDirection = "asc"
				} else {
					m.logSortDirection = "desc"
				}
				return m, m.fetchLogs
			}
		case "esc":
			if m.searchInput.Value() != "" || len(m.activeFilters) > 0 {
				m.searchInput.SetValue("")
				m.activeFilters = nil
				return m, m.fetchTasks
			}
			m.view = "dashboard"
		case "backspace":
			m.view = "dashboard"
		}
	}

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	return m, vpCmd
}
func (m Model) saveTask(title, description string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		now := time.Now().UTC()
		// Get next ID - simple hack for now
		tasks, _ := m.service.ListTasks(ctx, core.Query{})
		id := fmt.Sprintf("T-%04d", len(tasks)+1)

		task := &core.Task{
			ID:          id,
			Title:       title,
			Description: description,
			Status:      core.StatusTodo,
			Reference:   fmt.Sprintf("task://%s", id),
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		err := m.service.CreateTask(ctx, task, core.GetCurrentUser(), "Created via TUI")
		if err != nil {
			return err
		}
		return m.fetchTasks()
	}
}

func (m Model) moveTask(task *core.Task, dir int) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		statusOrder := []core.TaskStatus{
			core.StatusTodo,
			core.StatusInProgress,
			core.StatusDone,
		}

		currentIdx := -1
		for i, s := range statusOrder {
			if s == task.Status {
				currentIdx = i
				break
			}
		}

		if currentIdx == -1 {
			return nil
		}

		newIdx := currentIdx + dir
		if newIdx < 0 || newIdx >= len(statusOrder) {
			return nil
		}

		next := statusOrder[newIdx]
		err := m.service.TransitionStatus(ctx, task.ID, next, core.GetCurrentUser(), "Moved via Kanban")
		if err != nil {
			return err
		}
		return m.fetchTasks()
	}
}

func (m Model) fetchFlowRuns() tea.Msg {
	query := core.Query{
		Limit: 50,
	}
	runs, err := m.service.ListFlowRuns(context.Background(), query)
	if err != nil {
		return err
	}
	return flowRunsMsg(runs)
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
	// We use 1 line for spacing after header and 1 line before footer
	headerHeight := lipgloss.Height(header)
	footerHeight := lipgloss.Height(footer)
	occupiedHeight := headerHeight + footerHeight + 2
	
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
		m.searchInput.Width = m.width - 10
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

func (m Model) kanbanView() string {
	var s strings.Builder
	s.WriteString(styles.TitleStyle.Render("Kanban Board"))
	s.WriteString("\n\n")

	statusOrder := []core.TaskStatus{
		core.StatusTodo,
		core.StatusInProgress,
		core.StatusDone,
	}

	groups := make(map[core.TaskStatus][]*core.Task)
	for _, t := range m.tasks {
		groups[t.Status] = append(groups[t.Status], t)
	}

	var cols []string
	colWidth := (m.width - 4) / 3
	if colWidth < 20 {
		colWidth = 20
	}

	for _, status := range statusOrder {
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

func (m Model) addFilter(field, value string) Model {
	// Avoid duplicate filters
	for _, f := range m.activeFilters {
		if f.Field == field && f.Value == value {
			return m
		}
	}
	m.activeFilters = append(m.activeFilters, core.FieldFilter{
		Field:    field,
		Operator: core.OpEq,
		Value:    value,
	})
	return m
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

	statusOrder := []core.TaskStatus{
		core.StatusTodo,
		core.StatusInProgress,
		core.StatusDone,
		core.StatusSkipped,
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

			

							s.WriteString(fmt.Sprintf("%s %s %s %s%s%s\n", cursor, task.ID, statusIcon, title, styles.MutedStyle.Render(assignee), tags))

							currentIndex++

						}

			
		s.WriteString("\n")
	}

	return s.String()
}

func (m Model) syncPull() tea.Msg {
	// Not implemented yet - needs plugin integration
	return nil
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
	s.WriteString("\n")

	if task.Description != "" {
		s.WriteString("Description:\n")
		// Render description as markdown
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

func (m Model) syncViewport() Model {
	line := m.getLineOfSelected()
	if line < m.viewport.YOffset {
		m.viewport.YOffset = line
	} else if line >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.YOffset = line - m.viewport.Height + 1
	}
	return m
}

func (m Model) formatAssignee(assignee *string) string {
	if assignee == nil {
		return "-"
	}
	return "@" + *assignee
}

func (m Model) getLineOfSelected() int {
	if m.view == "flows" {
		return m.selected
	}

	// For dashboard, we need to account for headers and spacing
	groups := make(map[core.TaskStatus][]*core.Task)
	for _, t := range m.tasks {
		groups[t.Status] = append(groups[t.Status], t)
	}

	statusOrder := []core.TaskStatus{
		core.StatusTodo,
		core.StatusInProgress,
		core.StatusDone,
		core.StatusSkipped,
	}

	line := 0
	taskIdx := 0
	for _, status := range statusOrder {
		tasks := groups[status]
		if len(tasks) == 0 {
			continue
		}

		// Header line
		line++

		for range tasks {
			if taskIdx == m.selected {
				return line
			}
			line++
			taskIdx++
		}
		// Empty line after group
		line++
	}

	return 0
}

func formatAssignee(assignee *string) string {
	if assignee == nil {
		return "-"
	}
	return "@" + *assignee
}