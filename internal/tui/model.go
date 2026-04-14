package tui

import (
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	glamour "charm.land/glamour/v2"
	glamourstyles "charm.land/glamour/v2/styles"
	"charm.land/huh/v2"
	"github.com/spf13/viper"
	kitcli "hop.top/kit/cli"
	kittui "hop.top/kit/tui"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/tui/styles"
)

type (
	tasksMsg    []*core.Task
	flowRunsMsg []*core.FlowRun
	logsMsg     []*core.LogEntry
)

type Model struct {
	service          *core.TaskService
	view             string // "dashboard", "list", "detail", "search", "form", "kanban", "flows"
	tasks            []*core.Task
	flowRuns         []*core.FlowRun
	selected         int
	width            int
	height           int
	searchInput      textinput.Model
	activeFilters    []core.FieldFilter
	viewport         viewport.Model
	taskLogs         []*core.LogEntry
	logSortDirection string
	form             *huh.Form
	taskTitle        string
	taskDescription  string
	err              error

	// taskList is the kit/tui.List used for dashboard and kanban views.
	taskList kittui.List
	// flowList is the kit/tui.List used for flow runs view.
	flowList kittui.List

	// theme is the kit/cli.Theme used for styling.
	theme kitcli.Theme
	// styles holds TUI styles derived from the theme.
	styles *styles.Styles

	// mdRenderer is a cached glamour renderer; recreated only on window resize.
	mdRenderer    *glamour.TermRenderer
	mdRenderWidth int

	// tagColors holds unsaved tag->color assignments accumulated during a session.
	// Written to viper/disk via persistTagColors cmd, not inside View().
	tagColors map[string]string
}

func NewModel(service *core.TaskService, theme kitcli.Theme) Model {
	ti := textinput.New()
	ti.Placeholder = "Search tasks..."

	vp := viewport.New()

	direction := viper.GetString("ui.log_sort_direction")
	if direction == "" {
		direction = "desc"
	}

	// Seed tagColors from viper so existing config is respected.
	tc := viper.GetStringMapString("ui.tag_colors")
	if tc == nil {
		tc = make(map[string]string)
	}

	return Model{
		service:          service,
		view:             "dashboard",
		searchInput:      ti,
		viewport:         vp,
		logSortDirection: direction,
		theme:            theme,
		styles:           styles.NewFromTheme(theme),
		taskList:         kittui.NewList(1),
		flowList:         kittui.NewList(1),
		tagColors:        tc,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.RequestWindowSize, m.fetchTasks)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := m.updateInner(msg)
	// Recompute viewport height after every Update so view/filter/search
	// transitions that change header/footer height stay in sync.
	m.viewport.SetHeight(m.effectiveViewportHeight())
	return m, cmd
}

func (m Model) updateInner(msg tea.Msg) (Model, tea.Cmd) {
	// 1. Handle common messages first
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.SetWidth(msg.Width)
		vh := m.effectiveViewportHeight()
		m.viewport.SetHeight(vh)
		m.taskList = m.taskList.SetHeight(vh)
		m.flowList = m.flowList.SetHeight(vh)

		// Rebuild cached markdown renderer when width changes.
		wrapWidth := msg.Width - 10
		if wrapWidth != m.mdRenderWidth || m.mdRenderer == nil {
			m.mdRenderWidth = wrapWidth
			if r, err := glamour.NewTermRenderer(
				glamour.WithStyles(glamourstyles.DarkStyleConfig),
				glamour.WithWordWrap(wrapWidth),
			); err == nil {
				m.mdRenderer = r
			}
		}

		return m, nil
	case error:
		m.err = msg
		return m, nil
	case tasksMsg:
		m.tasks = msg
		if m.selected >= len(m.tasks) && len(m.tasks) > 0 {
			m.selected = len(m.tasks) - 1
		}
		m = m.rebuildTaskList()
		m = m.syncViewport()
		// Persist any newly-assigned tag colors asynchronously.
		return m, m.persistTagColors
	case flowRunsMsg:
		m.flowRuns = msg
		m = m.rebuildFlowList()
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

// rebuildTaskList rebuilds the kit/tui.List items from the current tasks,
// grouped by status with headers and spacers.
func (m Model) rebuildTaskList() Model {
	groups := make(map[core.TaskStatus][]*core.Task)
	for _, t := range m.tasks {
		groups[t.Status] = append(groups[t.Status], t)
	}

	var items []kittui.Item
	taskIdx := 0
	for _, status := range statusOrder {
		tasks := groups[status]
		if len(tasks) == 0 {
			continue
		}
		items = append(items, &headerItem{text: string(status)})
		for _, task := range tasks {
			items = append(items, &taskItem{
				task:      task,
				selected:  taskIdx == m.selected,
				styles:    m.styles,
				tagColors: m.tagColors,
			})
			taskIdx++
		}
		items = append(items, &spacerItem{})
	}

	m.taskList = m.taskList.SetItems(items)
	return m
}

// rebuildFlowList rebuilds the kit/tui.List items from the current flow runs.
func (m Model) rebuildFlowList() Model {
	items := make([]kittui.Item, len(m.flowRuns))
	for i, run := range m.flowRuns {
		items[i] = &flowRunItem{
			run:      run,
			selected: i == m.selected,
			styles:   m.styles,
			theme:    m.theme,
		}
	}
	m.flowList = m.flowList.SetItems(items)
	return m
}

func (m Model) syncViewport() Model {
	// Rebuild list items to reflect new selection state.
	m = m.rebuildTaskList()
	m = m.rebuildFlowList()

	line := m.getLineOfSelected()
	if line < m.viewport.YOffset() {
		m.viewport.SetYOffset(line)
	} else if line >= m.viewport.YOffset()+m.viewport.Height() {
		m.viewport.SetYOffset(line - m.viewport.Height() + 1)
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
