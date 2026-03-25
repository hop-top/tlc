package cli

import (
	"context"
	"fmt"
	"strings"
	texttemplate "text/template"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/plugin"
)

var (
	taskID          string
	taskDescription string
	taskStatus      string
	taskAssignedTo  string
	taskEffort      string
	taskTags        []string
	taskReference   string
	taskInteractive bool

	taskListStatus        []string
	taskListAssignedTo    string
	taskListTag           []string
	taskListMine          bool
	taskListArchived      bool
	taskListAllProjects   bool
	taskListSortBy        string
	taskListSortDirection string
	taskListLimit         int
	taskListOffset        int

	taskShowLogs             bool
	taskShowLogSortDirection string

	taskUpdateTitle       string
	taskUpdateDescription string
	taskUpdateStatus      string
	taskUpdateAssignedTo  string
	taskUpdateEffort      string
	taskUpdateAddTags     []string
	taskUpdateRemoveTags  []string

	taskDeleteYes bool

	taskClaimNote        string
	taskUnclaimNote      string
	taskAssignNote       string
	taskCompleteNote     string
	taskCompleteNoVerify bool
	taskReopenNote       string
	taskUnassignNote     string
	taskUpdateForce      bool
	taskListSummary      bool

	taskListWorkspace string
	taskListSpace     string
	taskListProfile   string
	taskListSquad     string
)

// appendNote appends a note to the task's description, separated by a newline.
func appendNote(task *core.Task, note string) {
	if note == "" {
		return
	}
	if task.Description == "" {
		task.Description = note
	} else {
		task.Description = task.Description + "\n\n" + note
	}
}

// auditLogData holds the template context for an audit log entry.
type auditLogData struct {
	Timestamp string
	Author    string
	Action    string
	Details   string
	Note      string
}

const defaultAuditTemplate = "{{.Timestamp}} · @{{.Author}} · {{.Action}} {{.Details}}{{if .Note}}\n{{.Note}}{{end}}"
const defaultTimestampFormat = "2006-01-02 15:04"
const defaultAuditSeparator = "---"

// appendAuditLog appends a structured audit log entry to the task description.
// The first entry is always preceded by "---" (description/audit boundary).
// Subsequent entries are separated by the configured separator (default "---").
func appendAuditLog(task *core.Task, author, action, details, note string, ts time.Time) {
	tmplStr := viper.GetString("audit_log.template")
	if tmplStr == "" {
		tmplStr = defaultAuditTemplate
	}

	tsFmt := viper.GetString("audit_log.timestamp_format")
	if tsFmt == "" {
		tsFmt = defaultTimestampFormat
	}

	separator := defaultAuditSeparator
	if viper.IsSet("audit_log.separator") {
		separator = viper.GetString("audit_log.separator")
	}

	data := auditLogData{
		Timestamp: ts.Format(tsFmt),
		Author:    author,
		Action:    action,
		Details:   details,
		Note:      note,
	}

	tmpl, err := texttemplate.New("audit").Parse(tmplStr)
	if err != nil {
		// Fallback to default if custom template is invalid.
		tmpl, _ = texttemplate.New("audit").Parse(defaultAuditTemplate)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		// Fallback: plain format.
		buf.Reset()
		buf.WriteString(fmt.Sprintf("%s · @%s · %s %s", data.Timestamp, author, action, details))
		if note != "" {
			buf.WriteString("\n" + note)
		}
	}
	entry := buf.String()

	// Check if description already has audit entries (contains "---").
	hasAuditBlock := strings.Contains(task.Description, "\n---\n") || strings.HasPrefix(task.Description, "---")

	if task.Description == "" {
		task.Description = "---\n" + entry
	} else if hasAuditBlock {
		if separator == "" {
			task.Description = task.Description + "\n" + entry
		} else {
			task.Description = task.Description + "\n\n" + separator + "\n" + entry
		}
	} else {
		task.Description = task.Description + "\n\n---\n" + entry
	}
}

func saveTaskWithLog(ctx context.Context, cmd *cobra.Command, task *core.Task, log *core.LogEntry, s interface {
	core.Repository
	core.LogRepository
}) error {
	if task.OriginSystem != nil && *task.OriginSystem != "" {
		if err := updateSyncedTask(ctx, task, s); err != nil {
			return err
		}
		if err := s.AddLog(ctx, log); err != nil {
			_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to write log: %v\n", err)
		}
	} else {
		if err := s.UpdateTask(ctx, task); err != nil {
			return fmt.Errorf("failed to update task: %w", err)
		}
		if err := s.AddLog(ctx, log); err != nil {
			_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to write log: %v\n", err)
		}
	}
	return nil
}

func updateSyncedTask(ctx context.Context, task *core.Task, s core.Repository) error {
	if task.OriginSystem == nil || *task.OriginSystem == "" {
		return fmt.Errorf("task does not have an origin system")
	}

	system := *task.OriginSystem
	fmt.Printf("Syncing task %s to %s...\n", task.ID, system)

	if err := s.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	binPath := getPluginPath(system)
	client, err := plugin.NewRPCClient(binPath)
	if err != nil {
		return fmt.Errorf("failed to start plugin %s: %w", system, err)
	}
	defer func() { _ = client.Close() }()

	params := map[string]interface{}{
		"repo":  viper.GetString(fmt.Sprintf("sync.%s.repo", system)),
		"tasks": []core.Task{*task},
	}

	var result struct {
		Updated []string          `json:"updated"`
		Failed  map[string]string `json:"failed"`
	}

	if err := client.Call("sync.push", params, &result); err != nil {
		return fmt.Errorf("sync push RPC failed: %w", err)
	}

	if len(result.Failed) > 0 {
		if errMsg, ok := result.Failed[task.ID]; ok {
			return fmt.Errorf("failed to push to %s: %s", system, errMsg)
		}
	}

	now := time.Now().UTC()
	task.LastSyncAt = &now
	if err := s.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("failed to update task after sync: %w", err)
	}

	fmt.Printf("✓ Task %s synced to %s\n", task.ID, system)
	return nil
}

func deleteSyncedTask(_ context.Context, task *core.Task, _ core.Repository) error {
	if task.OriginSystem == nil || *task.OriginSystem == "" {
		return fmt.Errorf("task does not have an origin system")
	}

	system := *task.OriginSystem
	fmt.Printf("Deleting task %s from %s...\n", task.ID, system)

	binPath := getPluginPath(system)
	client, err := plugin.NewRPCClient(binPath)
	if err != nil {
		return fmt.Errorf("failed to start plugin %s: %w", system, err)
	}
	defer func() { _ = client.Close() }()

	params := map[string]interface{}{
		"repo":  viper.GetString(fmt.Sprintf("sync.%s.repo", system)),
		"tasks": []core.Task{*task},
	}

	var result struct {
		Deleted []string          `json:"deleted"`
		Failed  map[string]string `json:"failed"`
	}

	if err := client.Call("sync.delete", params, &result); err != nil {
		return fmt.Errorf("sync delete RPC failed: %w", err)
	}

	if len(result.Failed) > 0 {
		if errMsg, ok := result.Failed[task.ID]; ok {
			return fmt.Errorf("failed to delete from %s: %s", system, errMsg)
		}
	}

	fmt.Printf("✓ Task %s deleted from %s\n", task.ID, system)
	return nil
}

var TaskCmd = &cobra.Command{
	Use:   "task",
	Short: "Task operations",
}

func init() {
	TaskCmd.AddCommand(TaskCreateCmd)
	TaskCmd.AddCommand(TaskListCmd)
	TaskCmd.AddCommand(TaskShowCmd)
	TaskCmd.AddCommand(TaskUpdateCmd)
	TaskCmd.AddCommand(TaskDeleteCmd)
	TaskCmd.AddCommand(TaskClaimCmd)
	TaskCmd.AddCommand(TaskUnclaimCmd)
	TaskCmd.AddCommand(TaskAssignCmd)
	TaskCmd.AddCommand(TaskUnassignCmd)
	TaskCmd.AddCommand(TaskCompleteCmd)
	TaskCmd.AddCommand(TaskReopenCmd)

	RootCmd.AddCommand(TaskCmd)
}

