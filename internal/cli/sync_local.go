package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// syncTODOAll syncs tasks to both global and project-specific todo.txt files.
func syncTODOAll() error {
	if err := syncToTODO(); err != nil {
		return err
	}
	return syncToProjectTODO()
}

// syncToTODO exports all tasks from SQLite to the TODO file in TLS format.
func syncToTODO() error {
	s, err := getStorage()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	tasks, err := s.ListTasks(ctx, core.Query{
		SortBy:        "created_at",
		SortDirection: "asc",
		AllProjects:   true,
	})
	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	todoFile := viper.GetString("task.todo_file")
	if err := os.MkdirAll(filepath.Dir(todoFile), 0o750); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	f, err := os.Create(todoFile)
	if err != nil {
		return fmt.Errorf("failed to create TODO file: %w", err)
	}
	defer func() { _ = f.Close() }()

	for _, t := range tasks {
		_, err := f.WriteString(formatTLS(t) + "\n")
		if err != nil {
			return fmt.Errorf("failed to write task to TODO file: %w", err)
		}
	}

	return nil
}

// syncToProjectTODO exports project-scoped tasks to the local .tlc/todo.txt file.
func syncToProjectTODO() error {
	proj := core.DetectProject()
	if proj == nil || !proj.InProject {
		return nil
	}

	s, err := getStorage()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	tasks, err := s.ListTasks(ctx, core.Query{
		SortBy:        "created_at",
		SortDirection: "asc",
	})
	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	todoFile := filepath.Join(filepath.Dir(proj.ConfigPath), "todo.txt")
	if err := os.MkdirAll(filepath.Dir(todoFile), 0o750); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	f, err := os.Create(todoFile)
	if err != nil {
		return fmt.Errorf("failed to create project TODO file: %w", err)
	}
	defer func() { _ = f.Close() }()

	for _, t := range tasks {
		_, err := f.WriteString(formatTLS(t) + "\n")
		if err != nil {
			return fmt.Errorf("failed to write task to project TODO file: %w", err)
		}
	}

	return nil
}

// ingestTODOWith reads the TODO file and updates the given storage if changes are found.
// This variant accepts a pre-opened storage to avoid opening a second connection
// during lazy initialization in getStorage().
func ingestTODOWith(s *storage.SQLiteStorage) error {
	f, err := os.Open(viper.GetString("task.todo_file"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to open TODO file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Determine current project ID so ingested tasks are scoped correctly.
	proj := core.DetectProject()
	var projectID string
	if proj != nil && proj.InProject && proj.ProjectID != "" {
		projectID = proj.ProjectID
	}

	ctx := context.Background()
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		task, err := parseTLS(line)
		if err != nil {
			fmt.Printf("Warning: failed to parse line: %s (%v)\n", line, err)
			continue
		}

		lineProjectID := ""
		if task.ProjectID != nil {
			lineProjectID = *task.ProjectID
		}

		// In project context, only accept lines that explicitly belong to the
		// current project. Shared global TODO lines from other projects must not
		// update same-ID tasks in the current project.
		if projectID != "" {
			if lineProjectID == "" {
				continue
			}
			if lineProjectID != projectID {
				continue
			}
		} else if lineProjectID == "" {
			// Outside project context, nil project IDs map to the default bucket.
			task.ProjectID = nil
		}

		lookupProjectID := ""
		if task.ProjectID != nil {
			lookupProjectID = *task.ProjectID
		}

		existing, _ := s.GetTaskInProject(ctx, task.ID, lookupProjectID)
		if existing == nil {
			if task.CreatedAt.IsZero() {
				task.CreatedAt = time.Now()
			}
			if task.UpdatedAt.IsZero() {
				task.UpdatedAt = task.CreatedAt
			}
			if err := s.CreateTask(ctx, task); err != nil {
				fmt.Printf("Warning: failed to create task %s: %v\n", task.ID, err)
			}
		} else {
			if existing.Status != task.Status || existing.Title != task.Title {
				existing.Status = task.Status
				existing.Title = task.Title
				existing.AssignedTo = task.AssignedTo
				existing.Tags = task.Tags
				if task.UpdatedAt.IsZero() || task.UpdatedAt.Equal(existing.UpdatedAt) {
					existing.UpdatedAt = time.Now()
				} else {
					existing.UpdatedAt = task.UpdatedAt
				}
				for k, v := range task.Meta {
					existing.Meta[k] = v
				}
				if err := s.UpdateTask(ctx, existing); err != nil {
					fmt.Printf("Warning: failed to update task %s: %v\n", existing.ID, err)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan TODO file: %w", err)
	}
	return nil
}

// parseTLS is a basic parser for Task Line Syntax.
func parseTLS(line string) (*core.Task, error) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "[") {
		return nil, fmt.Errorf("invalid TLS format: missing status bracket")
	}

	bracketEnd := strings.Index(line, "]")
	if bracketEnd == -1 {
		return nil, fmt.Errorf("invalid TLS format: unclosed status bracket")
	}

	statusContent := strings.TrimSpace(line[1:bracketEnd])
	remaining := strings.TrimSpace(line[bracketEnd+1:])

	tokens := strings.Fields(remaining)
	if len(tokens) < 2 {
		return nil, fmt.Errorf("invalid TLS format: missing ID or Title")
	}

	id := tokens[0]
	now := time.Now()
	task := &core.Task{
		ID:        id,
		CreatedAt: now,
		UpdatedAt: now,
		Meta:      make(map[string]interface{}),
	}

	// Status mapping via WorkflowManager
	wm := core.DefaultWorkflow()
	switch statusContent {
	case "":
		task.Status = core.StatusTodo // default
	default:
		if s, ok := wm.StatusForTLSMarker(statusContent); ok {
			task.Status = s
		} else {
			// Legacy fallback
			sc := strings.ToLower(statusContent)
			if strings.Contains(sc, "x") || strings.Contains(sc, "done") {
				task.Status = core.StatusDone
			} else if strings.Contains(sc, "~") || strings.Contains(sc, "progress") {
				task.Status = core.StatusInProgress
			} else {
				task.Status = core.StatusTodo
			}
		}
	}

	// Title and Metadata
	var titleParts []string
	for i := 1; i < len(tokens); i++ {
		token := tokens[i]
		if strings.HasPrefix(token, "@") {
			assignee := strings.TrimPrefix(token, "@")
			task.AssignedTo = &assignee
		} else if strings.HasPrefix(token, "#") {
			task.Tags = append(task.Tags, strings.TrimPrefix(token, "#"))
		} else if strings.HasPrefix(token, "prio:") {
			task.Meta["prio"] = strings.TrimPrefix(token, "prio:")
		} else if strings.HasPrefix(token, "domain:") {
			task.Meta["domain"] = strings.TrimPrefix(token, "domain:")
		} else if strings.Contains(token, "=") {
			kv := strings.SplitN(token, "=", 2)
			key, val := kv[0], kv[1]
			switch key {
			case "project_id":
				task.ProjectID = &val
			case "created_at":
				if t, err := time.Parse(time.RFC3339, val); err == nil {
					task.CreatedAt = t
				}
			case "updated_at":
				if t, err := time.Parse(time.RFC3339, val); err == nil {
					task.UpdatedAt = t
				}
			default:
				task.Meta[key] = val
			}
		} else if strings.HasPrefix(token, "ref:") {
			task.Reference = strings.TrimPrefix(token, "ref:")
		} else {
			titleParts = append(titleParts, token)
		}
	}
	task.Title = strings.Join(titleParts, " ")

	return task, nil
}
