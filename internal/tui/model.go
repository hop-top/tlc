package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/tui/styles"
	"hop.top/tlc/pkg/themepicker"
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


func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 1. Handle common messages first
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - viewportOffset

		// Resize theme picker
		var cmd tea.Cmd
		var tm tea.Model
		tm, cmd = m.themePicker.Update(msg)
		m.themePicker = tm.(themepicker.Model)
		return m, cmd
	case tea.MouseMsg:
		if msg.Type == tea.MouseLeft && m.view == "dashboard" {
			// Basic selection on click (not fully implemented)
		}
	case error:
		m.err = msg
		return m, nil
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
	}

	// 2. Delegate to view-specific handlers
	switch m.view {
	case "dashboard":
		return handleDashboardUpdate(m, msg)
	case "detail":
		return handleDetailUpdate(m, msg)
	case "kanban":
		return handleKanbanUpdate(m, msg)
	case "flows":
		return handleFlowsUpdate(m, msg)
	case "search":
		return handleSearchUpdate(m, msg)
	case "form":
		return handleFormUpdate(m, msg)
	case "theme_picker":
		return handleThemePickerUpdate(m, msg)
	default:
		return handleDashboardUpdate(m, msg)
	}
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


func (m Model) syncViewport() Model {
	line := m.getLineOfSelected()
	if line < m.viewport.YOffset {
		m.viewport.YOffset = line
	} else if line >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.YOffset = line - m.viewport.Height + 1
	}
	return m
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