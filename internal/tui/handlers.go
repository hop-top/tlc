package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"hop.top/tlc/internal/tui/styles"
	"hop.top/tlc/pkg/themepicker"
	"github.com/spf13/viper"
)

func handleDashboardUpdate(m Model, msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "/":
			m.view = "search"
			m.searchInput.Focus()
			m.searchInput.SetValue("")
			return m, nil
		case "j", "down":
			if m.selected < len(m.tasks)-1 {
				m.selected++
			}
			m = m.syncViewport()
			return m, nil
		case "k", "up":
			if m.selected > 0 {
				m.selected--
			}
			m = m.syncViewport()
			return m, nil
		case "r":
			return m, m.fetchTasks
		case "p":
			return m, m.syncPull
		case "v":
			m.selected = 0
			m.view = "kanban"
			return m, nil
		case "f":
			if len(m.tasks) > 0 {
				task := m.tasks[m.selected]
				for _, tag := range task.Tags {
					m = m.addFilter("tags", tag)
				}
				return m, m.fetchTasks
			}
		case "a":
			if len(m.tasks) > 0 {
				task := m.tasks[m.selected]
				if task.AssignedTo != nil {
					m = m.addFilter("assigned_to", *task.AssignedTo)
					return m, m.fetchTasks
				}
			}
		case "n":
			return m, m.createTask()
		case "t":
			m.view = "theme_picker"
			tm, _ := m.themePicker.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			m.themePicker = tm.(themepicker.Model)
			return m, nil
		case "c":
			if len(m.tasks) > 0 {
				return m, m.claimTask(m.tasks[m.selected].ID)
			}
		case "u":
			if len(m.tasks) > 0 {
				return m, m.unclaimTask(m.tasks[m.selected].ID)
			}
		case "s":
			if len(m.tasks) > 0 {
				return m, m.rotateStatus(m.tasks[m.selected])
			}
		case "enter":
			if len(m.tasks) > 0 {
				m.view = "detail"
				m.viewport.GotoTop()
				return m, m.fetchLogs
			}
		case "esc":
			if m.searchInput.Value() != "" || len(m.activeFilters) > 0 {
				m.searchInput.SetValue("")
				m.activeFilters = nil
				return m, m.fetchTasks
			}
			m.view = "dashboard"
			return m, nil
		case "backspace":
			m.view = "dashboard"
			return m, nil
		}
	}

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	return m, vpCmd
}

func handleDetailUpdate(m Model, msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			return m, m.fetchTasks
		case "c":
			if len(m.tasks) > 0 {
				return m, m.claimTask(m.tasks[m.selected].ID)
			}
		case "u":
			if len(m.tasks) > 0 {
				return m, m.unclaimTask(m.tasks[m.selected].ID)
			}
		case "s":
			if len(m.tasks) > 0 {
				return m, m.rotateStatus(m.tasks[m.selected])
			}
		case "o":
			if m.logSortDirection == "desc" {
				m.logSortDirection = "asc"
			} else {
				m.logSortDirection = "desc"
			}
			return m, m.fetchLogs
		case "esc":
			m.view = "dashboard"
			m.viewport.GotoTop()
			return m, nil
		case "backspace":
			m.view = "dashboard"
			m.viewport.GotoTop()
			return m, nil
		}
	}

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	return m, vpCmd
}

func handleKanbanUpdate(m Model, msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if m.selected < len(m.tasks)-1 {
				m.selected++
			}
			m = m.syncViewport()
			return m, nil
		case "k", "up":
			if m.selected > 0 {
				m.selected--
			}
			m = m.syncViewport()
			return m, nil
		case "h", "left":
			if len(m.tasks) > 0 {
				return m, m.moveTask(m.tasks[m.selected], -1)
			}
		case "l", "right":
			if len(m.tasks) > 0 {
				return m, m.moveTask(m.tasks[m.selected], 1)
			}
		case "r":
			return m, m.fetchTasks
		case "v":
			m.selected = 0
			m.view = "flows"
			return m, m.fetchFlowRuns
		case "c":
			if len(m.tasks) > 0 {
				return m, m.claimTask(m.tasks[m.selected].ID)
			}
		case "u":
			if len(m.tasks) > 0 {
				return m, m.unclaimTask(m.tasks[m.selected].ID)
			}
		case "s":
			if len(m.tasks) > 0 {
				return m, m.rotateStatus(m.tasks[m.selected])
			}
		case "enter":
			if len(m.tasks) > 0 {
				m.view = "detail"
				m.viewport.GotoTop()
				return m, m.fetchLogs
			}
		case "esc":
			m.view = "dashboard"
			m.viewport.GotoTop()
			return m, nil
		case "backspace":
			m.view = "dashboard"
			m.viewport.GotoTop()
			return m, nil
		}
	}

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	return m, vpCmd
}

func handleFlowsUpdate(m Model, msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if m.selected < len(m.flowRuns)-1 {
				m.selected++
			}
			m = m.syncViewport()
			return m, nil
		case "k", "up":
			if m.selected > 0 {
				m.selected--
			}
			m = m.syncViewport()
			return m, nil
		case "r":
			return m, m.fetchFlowRuns
		case "v":
			m.selected = 0
			m.view = "dashboard"
			return m, m.fetchTasks
		case "esc":
			m.view = "dashboard"
			return m, nil
		case "backspace":
			m.view = "dashboard"
			return m, nil
		}
	}

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	return m, vpCmd
}

func handleSearchUpdate(m Model, msg tea.Msg) (Model, tea.Cmd) {
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

func handleFormUpdate(m Model, msg tea.Msg) (Model, tea.Cmd) {
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

func handleThemePickerUpdate(m Model, msg tea.Msg) (Model, tea.Cmd) {
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
				themes = append(themes, styles.DefaultTheme())

				for _, name := range names {
					themes = append(themes, themepicker.LazyTheme{Name: name})
				}
				return themes
			}
		}
	case []themepicker.Theme:
		m.themePicker = themepicker.New(msg)
		m.themePicker.SetFetcher(themepicker.FetchTheme)
		tm, _ := m.themePicker.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.themePicker = tm.(themepicker.Model)
		return m, nil
	case themepicker.ThemeSelectedMsg:
		var themeToApply styles.Theme

		if t, ok := msg.Theme.(styles.Theme); ok {
			themeToApply = t
		} else if bt, ok := msg.Theme.(themepicker.BasicTheme); ok {
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
				TagColors:  styles.DefaultTheme().TagColors,
			}
		}

		styles.ApplyTheme(themeToApply)
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
