package core

import (
	"bytes"
	"fmt"
	"os/exec"
	"text/template"
	"time"

	"hop.top/tlc/internal/config"
)

// StaleHookData is the template context passed to each stale hook command.
type StaleHookData struct {
	ID         string
	Title      string
	AssignedTo string
	UpdatedAt  time.Time
	Timeout    time.Duration
	StaleSince time.Duration
}

// RunStaleHooks executes all configured hooks for a stale task.
// Template vars available: {{.ID}}, {{.Title}}, {{.AssignedTo}},
// {{.UpdatedAt}}, {{.Timeout}}, {{.StaleSince}}.
// Hook failures emit a warning and continue; all hooks run regardless.
// Always returns nil.
func RunStaleHooks(task *Task, hooks []config.StaleHook) error {
	staleSince := time.Duration(0)
	if s := task.StaleSince(); s != nil {
		staleSince = *s
	}
	timeout := time.Duration(0)
	if task.StaleTimeout != nil {
		timeout = *task.StaleTimeout
	}
	assignee := ""
	if task.AssignedTo != nil {
		assignee = *task.AssignedTo
	}
	data := StaleHookData{
		ID:         task.ID,
		Title:      task.Title,
		AssignedTo: assignee,
		UpdatedAt:  task.UpdatedAt,
		Timeout:    timeout,
		StaleSince: staleSince,
	}
	for _, h := range hooks {
		if err := runStaleHook(h.Command, data); err != nil {
			fmt.Printf("stale hook failed (continuing): %v\n", err)
		}
	}
	return nil
}

func runStaleHook(command string, data StaleHookData) error {
	tmpl, err := template.New("hook").Parse(command)
	if err != nil {
		return fmt.Errorf("hook template parse error: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("hook template execute error: %w", err)
	}
	cmd := exec.Command("sh", "-c", buf.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("hook command %q failed: %w (output: %s)", buf.String(), err, out)
	}
	return nil
}
