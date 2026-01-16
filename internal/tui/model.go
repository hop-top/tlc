package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/oss-tlc-cli/internal/core"
	"github.com/google/oss-tlc-cli/internal/tui/styles"
)

type tasksMsg []*core.Task
type flowRunsMsg []*core.FlowRun

type Model struct {
	service         *core.TaskService
	view            string // "dashboard", "list", "detail", "search", "form", "kanban", "flows"
	tasks           []*core.Task
	flowRuns        []*core.FlowRun
	selected        int
	width           int
	height          int
	searchInput     textinput.Model
	form            *huh.Form
	taskTitle       string
	taskDescription string
	err             error
}

func NewModel(service *core.TaskService) Model {

ti := textinput.New()
ti.Placeholder = "Search tasks..."

return Model{
		service:     service,
		view:        "dashboard",
		searchInput: ti,
	}
}

func (m Model) Init() tea.Cmd {
	return m.fetchTasks
}

func (m Model) fetchTasks() tea.Msg {
	query := core.Query{
		Search: m.searchInput.Value(),
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
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tasksMsg:
		m.tasks = msg
		if m.selected >= len(m.tasks) && len(m.tasks) > 0 {
			m.selected = len(m.tasks) - 1
		}
		return m, nil
	case flowRunsMsg:
		m.flowRuns = msg
		return m, nil
	case error:
		m.err = msg
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "/":
			m.view = "search"
			m.searchInput.Focus()
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
		case "k", "up":
			if m.selected > 0 {
				m.selected--
			}
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
		        case "n":
		            if m.view != "flows" {
		                return m, m.createTask()
		            }
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
		            }
		        case "esc", "backspace":
		            m.view = "dashboard"
		        }
		    }
		    return m, nil
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

	switch m.view {
	case "form":
		if m.form != nil {
			return m.form.View()
		}
		return "Loading form..."
	case "detail":
		return m.detailView()
	case "kanban":
		return m.kanbanView()
	case "flows":
		return m.flowsView()
	case "search":
		return m.dashboardView()
	default:
		return m.dashboardView()
	}
}

func (m Model) flowsView() string {
	var s strings.Builder
	s.WriteString(styles.TitleStyle.Render("Flow Executions"))
	s.WriteString("\n\n")

	if len(m.flowRuns) == 0 {
		s.WriteString("No flow runs found.")
	} else {
		for i, run := range m.flowRuns {
			cursor := " "
			if i == m.selected {
				cursor = styles.InProgressStyle.Render("►")
			}

			status := string(run.Status)
			startedAt := run.StartedAt.Format("2006-01-02 15:04:05")

			s.WriteString(fmt.Sprintf("%s %s %s %s (%s)\n", cursor, run.ID, run.FlowID, status, startedAt))
		}
	}

	s.WriteString("\n")
	s.WriteString(m.helpView())

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
	s.WriteString("\n\n")
	s.WriteString(m.helpView())

	return s.String()
}

func (m Model) dashboardView() string {
	var s strings.Builder
	s.WriteString(styles.TitleStyle.Render("TLC Dashboard"))
	s.WriteString("\n\n")

	if m.view == "search" {
		s.WriteString("Search: ")
		s.WriteString(m.searchInput.View())
		s.WriteString("\n\n")
	} else if m.searchInput.Value() != "" {
		s.WriteString(styles.MutedStyle.Render(fmt.Sprintf("Filtered by: %s", m.searchInput.Value())))
		s.WriteString("\n\n")
	}

	if len(m.tasks) == 0 {
		s.WriteString("No tasks found.")
	} else {
		// ... existing grouping logic ...
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
				if len(task.Tags) > 0 {
					tags = " #" + strings.Join(task.Tags, " #")
				}

				s.WriteString(fmt.Sprintf("%s %s %s %s%s%s\n", cursor, task.ID, statusIcon, title, styles.MutedStyle.Render(assignee), styles.MutedStyle.Render(tags)))
				currentIndex++
			}
			s.WriteString("\n")
		}
	}

	s.WriteString("\n")
	s.WriteString(m.helpView())

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
		items = append(items, "[n]ew", "[c/u] claim/unclaim", "[s] status", "[/ ] search", "[p] sync", "[v] cycle view", "[enter] details")
	case "kanban":
		items = append(items, "[h/l] move", "[v] cycle view", "[enter] details")
	case "flows":
		items = append(items, "[v] cycle view")
	case "detail":
		items = append(items, "[c/u] claim/unclaim", "[s] status", "[esc] back")
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

	s.WriteString(m.helpView())

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

func formatAssignee(assignee *string) string {
	if assignee == nil {
		return "-"
	}
	return "@" + *assignee
}