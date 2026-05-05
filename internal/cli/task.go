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
	"hop.top/tlc/internal/rpc"
	"hop.top/tlc/internal/storage"
	"hop.top/tlc/internal/uri"
)

var (
	taskID          string
	taskDescription string
	taskStatus      string
	taskAssignedTo  string
	taskEffort      string
	taskPriority    string
	taskBlockedBy   []string
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
	taskListOutput        string
	taskListIncludeLogs   bool

	taskShowOutput      string
	taskShowIncludeLogs bool

	taskShowLogs             bool
	taskShowLogSortDirection string

	taskUpdateTitle           string
	taskUpdateDescription     string
	taskUpdateStatus          string
	taskUpdateAssignedTo      string
	taskUpdateEffort          string
	taskUpdatePriority        string
	taskUpdateAddBlockedBy    []string
	taskUpdateRemoveBlockedBy []string
	taskUpdateClearBlockedBy  bool
	taskUpdateAddTags         []string
	taskUpdateRemoveTags      []string
	taskUpdateBlocked         string
	taskUpdateUnblock         bool
	taskUpdateTimeout         string

	taskCreateTimeout string

	taskDeleteYes  bool
	taskDeleteNote string

	taskUpdateNote string

	taskClaimNote        string
	taskUnclaimNote      string
	taskAssignNote       string
	taskCompleteNote     string
	taskCompleteNoVerify bool
	taskReopenNote       string
	taskUnassignNote     string
	taskUpdateForce      bool
	taskListSummary      bool
	taskListCounters     bool

	taskListWorkspace string
	taskListSpace     string
	taskListProfile   string
	taskListSquad     string

	taskListStale     bool
	taskListBlocked   bool
	taskListPriority  []string
	taskListBlockedBy []string

	taskNoPrompt bool

	taskTrack       string
	taskListTrack   string
	taskUpdateTrack string

	taskDue          string
	taskRemindAt     string
	taskRRule        string
	taskNoAutoRemind bool

	taskUpdateDue          string
	taskUpdateRemindAt     string
	taskUpdateRRule        string
	taskUpdateNoAutoRemind bool

	taskEva              []string
	taskUpdateAddEva     []string
	taskUpdateRemoveEva  []string
	taskUpdateClearEva   bool
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
		tmpl, _ = texttemplate.New("audit").Parse(defaultAuditTemplate) //nolint:errcheck // default template is compile-time constant
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

// saveTaskWithLog persists the task and audit log atomically, then (if
// the task is mirrored to an external system) attempts a best-effort
// push. Sync failures surface as warnings but never roll back local
// truth: the local DB is the source of truth, the remote is a downstream
// mirror. See T-0750.
func saveTaskWithLog(ctx context.Context, cmd *cobra.Command, task *core.Task, log *core.LogEntry, s interface {
	core.Repository
	core.LogRepository
}) error {
	// Step 1: atomic local commit — task row + audit log entry in one tx.
	// This is the source of truth; nothing downstream may invalidate it.
	if err := s.UpdateTaskWithLog(ctx, task, log); err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	// Step 2: if the task is mirrored to a remote system, attempt the
	// push as a best-effort side effect. Failures become warnings.
	if task.OriginSystem != nil && *task.OriginSystem != "" {
		if err := pushSyncedTask(ctx, task, s); err != nil {
			_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Warning: %v (local state saved; sync needs retry)\n", err)
		}
	}
	return nil
}

// syncPushFn pushes a task to its origin system. It is wired here so
// tests can inject a fake without spawning a plugin binary. Returns
// non-nil error on RPC failure, plugin error, or per-task failure.
var syncPushFn = defaultSyncPush

func defaultSyncPush(_ context.Context, system string, task *core.Task) error {
	binPath := getPluginPath(system)
	client, err := rpc.NewClient(binPath)
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
	return nil
}

// pushSyncedTask mirrors a previously-committed local change to the
// origin system. It MUST NOT mutate the source-of-truth fields (status,
// description, audit log) on failure — only LastSyncAt on success. The
// local DB has already committed when this runs.
func pushSyncedTask(ctx context.Context, task *core.Task, s core.Repository) error {
	if task.OriginSystem == nil || *task.OriginSystem == "" {
		return fmt.Errorf("task does not have an origin system")
	}

	system := *task.OriginSystem
	if system == syncSystemGitHub {
		ensureGitHubToken()
	}
	fmt.Printf("Syncing task %s to %s...\n", formatTaskAlias(task), system)

	if err := syncPushFn(ctx, system, task); err != nil {
		return err
	}

	now := time.Now().UTC()
	task.LastSyncAt = &now
	if err := s.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("failed to update task after sync: %w", err)
	}

	fmt.Printf("✓ Task %s synced to %s\n", formatTaskAlias(task), system)
	return nil
}

// validateBlockedByRefs resolves each blocked_by reference against the
// project registry. Cross-project refs (containing "/" or "://") are
// resolved through registryStorage with fuzzy project matching (label,
// id-prefix, label-prefix) and a shared DB handle cache, so a single
// command invocation reuses one cross-project SQLite handle per db_path.
//
// The cache is opened and closed inside this function: handles never
// outlive the call. registryStorage and localStorage may be the same
// *SQLiteStorage (the common case where the local DB also serves as the
// project registry).
func validateBlockedByRefs(ctx context.Context, registryStorage, localStorage *storage.SQLiteStorage, refs []string) ([]string, error) {
	normalized := core.NormalizeBlockedBy(refs)
	if len(normalized) == 0 {
		return nil, nil
	}

	cache := uri.NewProjectDBCache()
	defer cache.Close()

	validated := make([]string, 0, len(normalized))
	for _, ref := range normalized {
		normalizedRef := uri.NormalizeTaskID(ref)
		resolverStorage := localStorage
		if strings.Contains(normalizedRef, "/") || strings.Contains(normalizedRef, "://") {
			resolverStorage = registryStorage
		}

		resolver := uri.NewResolver(resolverStorage).WithDBCache(cache)
		resolved, err := resolver.ResolveTask(ctx, normalizedRef)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid blocked_by reference %q: %w; run 'tlc task list' to see available task IDs",
				ref, err,
			)
		}

		validated = append(validated, canonicalBlockedByRef(localStorage, normalizedRef, resolved))
	}

	return core.NormalizeBlockedBy(validated), nil
}

func canonicalBlockedByRef(base *storage.SQLiteStorage, fallback string, resolved *uri.ResolvedTask) string {
	if resolved == nil || resolved.Task == nil {
		return fallback
	}
	if resolved.Storage == base {
		return resolved.Task.ID
	}
	if resolved.Task.ProjectID != nil && *resolved.Task.ProjectID != "" {
		return *resolved.Task.ProjectID + "/" + resolved.Task.ID
	}
	return fallback
}

func deleteSyncedTask(_ context.Context, task *core.Task, _ core.Repository) error {
	if task.OriginSystem == nil || *task.OriginSystem == "" {
		return fmt.Errorf("task does not have an origin system")
	}

	system := *task.OriginSystem
	if system == syncSystemGitHub {
		ensureGitHubToken()
	}
	fmt.Printf("Deleting task %s from %s...\n", task.ID, system)

	binPath := getPluginPath(system)
	client, err := rpc.NewClient(binPath)
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
	TaskCmd.AddCommand(TaskGraphCmd)
	TaskCmd.AddCommand(TaskStaleCmd)
	TaskCmd.AddCommand(TaskShowCmd)
	TaskCmd.AddCommand(TaskUpdateCmd)
	TaskCmd.AddCommand(TaskDeleteCmd)
	TaskCmd.AddCommand(TaskClaimCmd)
	TaskCmd.AddCommand(TaskUnclaimCmd)
	TaskCmd.AddCommand(TaskAssignCmd)
	TaskCmd.AddCommand(TaskUnassignCmd)
	TaskCmd.AddCommand(TaskCompleteCmd)
	TaskCmd.AddCommand(TaskReopenCmd)

	TaskCmd.PersistentFlags().BoolVar(&taskNoPrompt, "no-prompt", false, "Skip confirmation prompts")

	RootCmd.AddCommand(TaskCmd)
}
