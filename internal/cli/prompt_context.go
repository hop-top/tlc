package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
	"hop.top/tlc/internal/uri"
)

var promptJSON bool

// PromptCmd is the top-level command group for context dumps and enriched
// agent output under `tlc prompt`.
var PromptCmd = &cobra.Command{
	Use:   "prompt",
	Short: "Context dump and enriched output for agent consumption",
}

// PromptTaskCmd renders an enriched context dump for a task.
var PromptTaskCmd = &cobra.Command{
	Use:   "task <id>",
	Short: "Render enriched task context for agent consumption",
	Args:  cobra.ExactArgs(1),
	RunE:  runTaskPrompt,
}

func runTaskPrompt(cmd *cobra.Command, args []string) error {
	s, err := getStorage()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	ctx := cmd.Context()

	res, err := uri.NewResolver(s).ResolveTask(ctx, args[0])
	if err != nil {
		return fmt.Errorf("task %s not found; run 'tlc task list' to see available tasks", args[0])
	}
	task := res.Task
	if res.Storage != s {
		defer func() { _ = res.Storage.Close() }()
	}

	data := buildPromptContext(ctx, s, res.Storage, task)

	if promptJSON {
		return renderPromptJSON(cmd.OutOrStdout(), data)
	}
	renderPromptMarkdown(cmd.OutOrStdout(), data)
	return nil
}

// promptContext holds the enriched context for a task prompt.
type promptContext struct {
	Task         *core.Task          `json:"task"`
	Description  string              `json:"description"`
	BlockedBy    []promptDep         `json:"blocked_by,omitempty"`
	Blocking     []promptDep         `json:"blocking,omitempty"`
	Track        *promptTrack        `json:"track,omitempty"`
	RecentLogs   []promptLogEntry    `json:"recent_activity,omitempty"`
}

// promptDep is a dependency summary for the prompt output.
type promptDep struct {
	ID     string          `json:"id"`
	Title  string          `json:"title"`
	Status core.TaskStatus `json:"status"`
}

// promptTrack holds track context for the prompt output.
type promptTrack struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	CurrentPhase int    `json:"current_phase"`
	TotalPhases  int    `json:"total_phases"`
}

// promptLogEntry is a single audit log entry for the prompt output.
type promptLogEntry struct {
	Timestamp string `json:"timestamp"`
	By        string `json:"by"`
	Action    string `json:"action"`
}

const promptLogLimit = 10

func buildPromptContext(ctx context.Context, registryStorage, taskStorage *storage.SQLiteStorage, task *core.Task) promptContext {
	desc := splitDescription(task.Description)
	blockedBy, blocking := collectTaskRelations(ctx, registryStorage, taskStorage, task)

	pc := promptContext{
		Task:        task,
		Description: desc,
		BlockedBy:   toPromptDeps(blockedBy),
		Blocking:    toPromptDeps(blocking),
	}

	// Load track info if present.
	if task.TrackID != nil && *task.TrackID != "" {
		pc.Track = loadPromptTrack(ctx, taskStorage, *task.TrackID)
	}

	// Load recent log entries.
	logs, err := taskStorage.GetLogs(ctx, task.ID, "desc")
	if err == nil && len(logs) > 0 {
		limit := promptLogLimit
		if len(logs) < limit {
			limit = len(logs)
		}
		pc.RecentLogs = make([]promptLogEntry, limit)
		for i, log := range logs[:limit] {
			pc.RecentLogs[i] = promptLogEntry{
				Timestamp: log.Timestamp.Format("2006-01-02 15:04"),
				By:        log.By,
				Action:    log.Action,
			}
		}
	}

	return pc
}

// splitDescription separates the user description from the audit block
// (delimited by "---" on its own line).
func splitDescription(raw string) string {
	idx := strings.Index(raw, "\n---\n")
	if idx >= 0 {
		return strings.TrimSpace(raw[:idx])
	}
	if strings.HasPrefix(raw, "---") {
		return ""
	}
	return strings.TrimSpace(raw)
}

func toPromptDeps(summaries []relatedTaskSummary) []promptDep {
	if len(summaries) == 0 {
		return nil
	}
	deps := make([]promptDep, len(summaries))
	for i, s := range summaries {
		deps[i] = promptDep{
			ID:     s.Ref,
			Title:  s.Title,
			Status: s.Status,
		}
	}
	return deps
}

func loadPromptTrack(ctx context.Context, s *storage.SQLiteStorage, trackID string) *promptTrack {
	staleThreshold := viper.GetDuration("tracks.stale_threshold")
	if staleThreshold == 0 {
		staleThreshold = 48 * time.Hour
	}
	svc := core.NewTrackService(s, s)
	track, _, progress, err := svc.GetTrackWithState(ctx, trackID, staleThreshold)
	if err != nil {
		return nil
	}
	return &promptTrack{
		ID:           track.ID,
		Title:        track.Title,
		Status:       string(track.Status),
		CurrentPhase: progress.CurrentPhase,
		TotalPhases:  progress.TotalPhases,
	}
}

// renderPromptMarkdown writes the enriched context in a markdown format
// optimized for LLM consumption.
func renderPromptMarkdown(w io.Writer, pc promptContext) {
	t := pc.Task

	// Header line.
	_, _ = fmt.Fprintf(w, "# %s: %s\n", t.ID, t.Title)

	// Status line.
	parts := []string{fmt.Sprintf("Status: %s", t.Status)}
	if t.AssignedTo != nil && *t.AssignedTo != "" {
		parts = append(parts, fmt.Sprintf("Assigned: @%s", *t.AssignedTo))
	}
	if t.Priority != "" {
		parts = append(parts, fmt.Sprintf("Priority: %s", t.Priority))
	}
	_, _ = fmt.Fprintf(w, "%s\n", strings.Join(parts, " | "))

	// Tags.
	if len(t.Tags) > 0 {
		_, _ = fmt.Fprintf(w, "Tags: %s\n", strings.Join(t.Tags, ", "))
	}

	// Track.
	if pc.Track != nil {
		_, _ = fmt.Fprintf(w, "Track: %s (%s, phase %d/%d)\n",
			pc.Track.ID, pc.Track.Status,
			pc.Track.CurrentPhase, pc.Track.TotalPhases)
	}

	// Description.
	if pc.Description != "" {
		_, _ = fmt.Fprintf(w, "\n## Description\n%s\n", pc.Description)
	}

	// Dependencies.
	if len(pc.BlockedBy) > 0 || len(pc.Blocking) > 0 {
		_, _ = fmt.Fprint(w, "\n## Dependencies\n")
		for _, dep := range pc.BlockedBy {
			_, _ = fmt.Fprintf(w, "- blocked-by %s [%s]: %s\n", dep.ID, dep.Status, dep.Title)
		}
		for _, dep := range pc.Blocking {
			_, _ = fmt.Fprintf(w, "- blocks %s [%s]: %s\n", dep.ID, dep.Status, dep.Title)
		}
	}

	// Recent activity.
	if len(pc.RecentLogs) > 0 {
		_, _ = fmt.Fprint(w, "\n## Recent Activity\n")
		for _, entry := range pc.RecentLogs {
			by := entry.By
			if by != "" {
				by = "@" + by
			}
			_, _ = fmt.Fprintf(w, "- %s · %s · %s\n", entry.Timestamp, by, entry.Action)
		}
	}
}

// renderPromptJSON writes the enriched context as a structured JSON object.
func renderPromptJSON(w io.Writer, pc promptContext) error {
	out := promptJSONOutput{
		ID:          pc.Task.ID,
		Title:       pc.Task.Title,
		Status:      string(pc.Task.Status),
		Description: pc.Description,
		Tags:        pc.Task.Tags,
		Priority:    string(pc.Task.Priority),
		Effort:      string(pc.Task.Effort),
		BlockedBy:   pc.BlockedBy,
		Blocking:    pc.Blocking,
		Track:       pc.Track,
		RecentLogs:  pc.RecentLogs,
	}
	if pc.Task.AssignedTo != nil {
		out.AssignedTo = *pc.Task.AssignedTo
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal prompt context: %w", err)
	}
	_, _ = fmt.Fprintln(w, string(data))
	return nil
}

// promptJSONOutput is the typed JSON output for --json mode.
type promptJSONOutput struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Status      string           `json:"status"`
	Description string           `json:"description,omitempty"`
	AssignedTo  string           `json:"assigned_to,omitempty"`
	Tags        []string         `json:"tags,omitempty"`
	Priority    string           `json:"priority,omitempty"`
	Effort      string           `json:"effort,omitempty"`
	BlockedBy   []promptDep      `json:"blocked_by,omitempty"`
	Blocking    []promptDep      `json:"blocking,omitempty"`
	Track       *promptTrack     `json:"track,omitempty"`
	RecentLogs  []promptLogEntry `json:"recent_activity,omitempty"`
}

func init() {
	PromptTaskCmd.Flags().BoolVar(&promptJSON, "json", false, "Output as JSON")
	PromptCmd.AddCommand(PromptTaskCmd)
	RootCmd.AddCommand(PromptCmd)
}
