package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/tui/styles"
	"hop.top/tlc/pkg/themepicker"
)

const (
	keyCtrlC     = "ctrl+c"
	keyDown      = "down"
	keyEnter     = "enter"
	keyEsc       = "esc"
	keyBackspace = "backspace"
	sortDesc     = "desc"
)

func handleDashboardUpdate(m Model, msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", keyCtrlC:
			return m, tea.Quit
		case "/":
			m.view = viewSearch
			m.searchInput.Focus()
			m.searchInput.SetValue("")
			return m, nil
		case "j", keyDown:
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
			m.view = viewKanban
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
			return m.createTask()
		case "t":
			m.view = viewThemePicker
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
		case keyEnter:
			if len(m.tasks) > 0 {
				m.view = viewDetail
				m.viewport.GotoTop()
				return m, m.fetchLogs
			}
		case keyEsc:
			if m.searchInput.Value() != "" || len(m.activeFilters) > 0 {
				m.searchInput.SetValue("")
				m.activeFilters = nil
				return m, m.fetchTasks
			}
			m.view = viewDashboard
			return m, nil
		case keyBackspace:
			m.view = viewDashboard
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
		case "q", keyCtrlC:
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
			if m.logSortDirection == sortDesc {
				m.logSortDirection = sortAsc
			} else {
				m.logSortDirection = sortDesc
			}
			return m, m.fetchLogs
		case keyEsc:
			m.view = viewDashboard
			m.viewport.GotoTop()
			return m, nil
		case keyBackspace:
			m.view = viewDashboard
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
		case "q", keyCtrlC:
			return m, tea.Quit
		case "j", keyDown:
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
			m.view = viewFlows
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
		case keyEnter:
			if len(m.tasks) > 0 {
				m.view = viewDetail
				m.viewport.GotoTop()
				return m, m.fetchLogs
			}
		case keyEsc:
			m.view = viewDashboard
			m.viewport.GotoTop()
			return m, nil
		case keyBackspace:
			m.view = viewDashboard
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
		case "q", keyCtrlC:
			return m, tea.Quit
		case "j", keyDown:
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
			m.view = viewDashboard
			return m, m.fetchTasks
		case keyEsc:
			m.view = viewDashboard
			return m, nil
		case keyBackspace:
			m.view = viewDashboard
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
		case keyEnter, keyEsc:
			m.view = viewDashboard
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
		m.view = viewDashboard
		return m, m.saveTask(m.taskTitle, m.taskDescription)
	}
	if m.form.State == huh.StateAborted {
		m.view = viewDashboard
		return m, nil
	}
	return m, cmd
}

func handleThemePickerUpdate(m Model, msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == keyEsc {
			m.view = viewDashboard
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
		if err := viper.WriteConfig(); err != nil {
			log.Warn("Failed to save theme preference", "error", err)
		}

		m.view = viewDashboard
		return m, nil
	}

	var cmd tea.Cmd
	var tm tea.Model
	tm, cmd = m.themePicker.Update(msg)
	m.themePicker = tm.(themepicker.Model)
	return m, cmd
}
