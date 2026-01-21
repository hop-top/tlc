package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/oss-tlc-cli/internal/core"
	"github.com/spf13/viper"
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
	defer s.Close()

	ctx := context.Background()
	tasks, err := s.ListTasks(ctx, core.Query{
		SortBy:        "created_at",
		SortDirection: "asc",
		AllProjects:   true,
	})
	if err != nil {
		return err
	}

	todoFile := viper.GetString("task.todo_file")
	os.MkdirAll(filepath.Dir(todoFile), 0755)
	f, err := os.Create(todoFile)
	if err != nil {
		return fmt.Errorf("failed to create TODO file: %w", err)
	}
	defer f.Close()

	for _, t := range tasks {
		_, err := f.WriteString(formatTLS(t) + "\n")
		if err != nil {
			return err
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
	defer s.Close()

	ctx := context.Background()
	tasks, err := s.ListTasks(ctx, core.Query{
		SortBy:        "created_at",
		SortDirection: "asc",
	})
	if err != nil {
		return err
	}

	todoFile := filepath.Join(filepath.Dir(proj.ConfigPath), "todo.txt")
	os.MkdirAll(filepath.Dir(todoFile), 0755)
	f, err := os.Create(todoFile)
	if err != nil {
		return fmt.Errorf("failed to create project TODO file: %w", err)
	}
	defer f.Close()

	for _, t := range tasks {
		_, err := f.WriteString(formatTLS(t) + "\n")
		if err != nil {
			return err
		}
	}

	return nil
}

// ingestTODO reads the TODO file and updates SQLite if changes are found.
func ingestTODO() error {
	f, err := os.Open(viper.GetString("task.todo_file"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	s, err := getStorage()
	if err != nil {
		return err
	}
	defer s.Close()

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

		// Check if task exists
		existing, _ := s.GetTask(ctx, task.ID)
		if existing == nil {
			if task.CreatedAt.IsZero() {
				task.CreatedAt = time.Now()
			}
			if task.UpdatedAt.IsZero() {
				task.UpdatedAt = task.CreatedAt
			}
			s.CreateTask(ctx, task)
		} else {
			// Update if different (simplified check)
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
				// Merge meta
				for k, v := range task.Meta {
					existing.Meta[k] = v
				}
				s.UpdateTask(ctx, existing)
			}
		}
	}

	return scanner.Err()
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

	// Status mapping
	switch statusContent {
	case "":
		task.Status = core.StatusTodo
	case "~":
		task.Status = core.StatusInProgress
	case "x":
		task.Status = core.StatusDone
	case "-":
		task.Status = core.StatusSkipped
	default:
		// Fallback for cases like "[DONE]" or "[ ]"
		sc := strings.ToLower(statusContent)
		if strings.Contains(sc, "x") || strings.Contains(sc, "done") {
			task.Status = core.StatusDone
		} else if strings.Contains(sc, "~") || strings.Contains(sc, "progress") {
			task.Status = core.StatusInProgress
		} else {
			task.Status = core.StatusTodo
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
			if key == "created_at" {
				if t, err := time.Parse(time.RFC3339, val); err == nil {
					task.CreatedAt = t
				}
			} else if key == "updated_at" {
				if t, err := time.Parse(time.RFC3339, val); err == nil {
					task.UpdatedAt = t
				}
			} else {
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
