package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/log/v2"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

func (m Model) fetchTasks() tea.Msg {
	query := core.Query{
		Search:  m.searchInput.Value(),
		Filters: m.activeFilters,
	}
	tasks, err := m.service.ListTasks(context.Background(), query)
	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	// Sort tasks by the CONFIGURED status order. Declaration order is
	// rank order, so the ranks come from the vocabulary rather than a
	// literal map of the built-in four — which ranked every custom
	// status 0 and collapsed the grouping to ID order.
	ranks := statusRanks()

	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].Status != tasks[j].Status {
			return ranks[tasks[i].Status] < ranks[tasks[j].Status]
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
		return fmt.Errorf("failed to get logs: %w", err)
	}
	return logsMsg(logs)
}

func (m Model) syncPull() tea.Msg {
	// Not implemented yet - needs plugin integration
	return nil
}

// persistTagColors writes the in-memory tagColors map to the viper config file.
// Called asynchronously (as a tea.Cmd) so disk I/O never blocks the render loop.
func (m Model) persistTagColors() tea.Msg {
	if len(m.tagColors) == 0 {
		return nil
	}
	viper.Set("ui.tag_colors", m.tagColors)
	if _, err := config.PrepareViperForWrite(viper.GetViper()); err != nil {
		log.Warn("failed to prepare config for tag colors", "error", err)
		return nil
	}
	if err := viper.WriteConfig(); err != nil {
		log.Warn("failed to persist tag colors", "error", err)
	}
	return nil
}

func (m Model) claimTask(id string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		user := core.GetCurrentUser()
		if err := m.service.ClaimTask(ctx, id, user, "Claimed via TUI"); err != nil {
			return fmt.Errorf("failed to claim task: %w", err)
		}
		return m.fetchTasks()
	}
}

func (m Model) unclaimTask(id string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		user := core.GetCurrentUser()
		if err := m.service.UnclaimTask(ctx, id, user, "Unclaimed via TUI"); err != nil {
			return fmt.Errorf("failed to unclaim task: %w", err)
		}
		return m.fetchTasks()
	}
}

func (m Model) rotateStatus(task *core.Task) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// nextRotationStatus offers only targets the task's workflow
		// accepts, so the transition below cannot be one the service
		// then refuses. No offer means no legal move out of this
		// status (every terminal status): do nothing visible rather
		// than raise a banner on a cycle keypress.
		next, ok := nextRotationStatus(task)
		if !ok {
			return nil
		}

		err := m.service.TransitionStatus(ctx, task.ID, next, core.GetCurrentUser(), "Rotated via TUI")
		if err != nil {
			return fmt.Errorf("failed to transition status: %w", err)
		}
		return m.fetchTasks()
	}
}

func (m Model) moveTask(task *core.Task, dir int) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		// The board's own columns, so a left/right move lands on a
		// column the user can actually see.
		columns := kanbanStatusOrder()

		currentIdx := -1
		for i, s := range columns {
			if s == task.Status {
				currentIdx = i
				break
			}
		}

		if currentIdx == -1 {
			return nil
		}

		newIdx := currentIdx + dir
		if newIdx < 0 || newIdx >= len(columns) {
			return nil
		}

		next := columns[newIdx]
		err := m.service.TransitionStatus(ctx, task.ID, next, core.GetCurrentUser(), "Moved via Kanban")
		if err != nil {
			return fmt.Errorf("failed to move task: %w", err)
		}
		return m.fetchTasks()
	}
}

func (m Model) createTask() (Model, tea.Cmd) {
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
	return m, m.form.Init()
}

func (m Model) saveTask(title, description string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		now := time.Now().UTC()

		proj := core.DetectProject()

		// Durable identity is the TypeID; storage allocates the
		// per-project Seq for the T-NNNN display alias on insert.
		// URIs embed the TypeID so they survive renames.
		id := core.NewTaskID()

		// The create default is the workflow's initial status, not a
		// hardcoded TODO. Same resolution the CLI create path and the
		// HTTP route use: a task created into a status the config never
		// declared can never be transitioned out of it.
		initial, err := core.DefaultWorkflow().InitialStatus()
		if err != nil {
			return fmt.Errorf(
				"no initial status to create into: %w; "+
					"set task.default_status or give a status role \"initial\"", err,
			)
		}

		task := &core.Task{
			ID:          id,
			Title:       title,
			Description: description,
			Status:      initial,
			Reference:   fmt.Sprintf("tlc:///%s", id),
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		if proj != nil && proj.ProjectID != "" {
			task.ProjectID = &proj.ProjectID
			task.Reference = fmt.Sprintf("tlc://%s/%s", proj.ProjectID, id)
		}

		// Retry on the (astronomically unlikely) TypeID collision so a
		// repeat insert still succeeds without surfacing a UNIQUE error.
		for {
			if err := m.service.CreateTask(ctx, task, core.GetCurrentUser(), "Created via TUI"); err == nil {
				break
			} else if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return fmt.Errorf("failed to create task: %w", err)
			}
			id = core.NewTaskID()
			task.ID = id
			if proj != nil && proj.ProjectID != "" {
				task.Reference = fmt.Sprintf("tlc://%s/%s", proj.ProjectID, id)
			} else {
				task.Reference = fmt.Sprintf("tlc:///%s", id)
			}
		}

		return m.fetchTasks()
	}
}
