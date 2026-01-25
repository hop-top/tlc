package tui

import (
	"context"
	"fmt"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/IdeaCraftersLabs/oss-tlc-cli/internal/core"
)

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

func (m Model) syncPull() tea.Msg {
	// Not implemented yet - needs plugin integration
	return nil
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

		// Auto-assign project_id if in a project context
		if proj := core.DetectProject(); proj != nil && proj.ProjectID != "" {
			task.ProjectID = &proj.ProjectID
		}

		err := m.service.CreateTask(ctx, task, core.GetCurrentUser(), "Created via TUI")
		if err != nil {
			return err
		}
		return m.fetchTasks()
	}
}
