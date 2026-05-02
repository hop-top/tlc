package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/markdown"
	"hop.top/kit/go/console/output"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

var (
	primaryColor = lipgloss.Color("39")
	successColor = lipgloss.Color("42")
	warningColor = lipgloss.Color("214")
	errorColor   = lipgloss.Color("196")
	mutedColor   = lipgloss.Color("241")

	titleStyle = lipgloss.NewStyle().Foreground(primaryColor).Bold(true)
	labelStyle = lipgloss.NewStyle().Foreground(mutedColor).Width(12)

	todoStyle       = lipgloss.NewStyle()
	inProgressStyle = lipgloss.NewStyle().Foreground(primaryColor)
	doneStyle       = lipgloss.NewStyle().Foreground(successColor).Bold(true)
	skippedStyle    = lipgloss.NewStyle().Foreground(warningColor)
)

// formatTaskAlias renders the human-readable task display alias
// (e.g. "T-0042"). When the durable Task.ID is already a legacy "T-NNNN"
// string (pre-typeid rows or CLI-allocated IDs) we surface it verbatim
// so it stays consistent with the seq the row was created under;
// otherwise we synthesise the alias from t.Seq via core.FormatTaskAlias.
// As a final fallback we return the raw ID so output never silently
// drops the identifier.
func formatTaskAlias(t *core.Task) string {
	if t == nil {
		return ""
	}
	if t.ID != "" && !core.IsTaskID(t.ID) {
		return t.ID
	}
	if alias := core.FormatTaskAlias(t); alias != "" {
		return alias
	}
	return t.ID
}

// formatTrackAlias renders the human-readable track display alias (the
// slug). Falls back to the durable typeid when Slug is unset.
func formatTrackAlias(t *core.Track) string {
	if t == nil {
		return ""
	}
	if t.Slug != "" {
		return t.Slug
	}
	return t.ID
}

// formatTaskTrackDisplay resolves a Task.TrackID value into its human-
// readable display form (the track's slug). On any lookup error the raw
// value is returned so the caller still sees something useful.
func formatTaskTrackDisplay(trackID string) string {
	if trackID == "" {
		return ""
	}
	s, err := getStorageRaw()
	if err != nil {
		return trackID
	}
	defer func() { _ = s.Close() }()
	track, err := s.GetTrack(context.Background(), trackID)
	if err != nil || track == nil {
		return trackID
	}
	return formatTrackAlias(track)
}

// isVerboseOutput reports whether the user requested verbose output via
// the --verbose / -V flag.
func isVerboseOutput() bool {
	return viper.GetBool("output.verbose")
}

func formatTasks(cmd *cobra.Command, tasks []*core.Task, format string) {
	out := cmd.OutOrStdout()
	switch format {
	case formatJSON, formatYAML:
		_ = output.Render(out, format, tasks) //nolint:errcheck // best-effort output
	case "tls":
		for _, t := range tasks {
			_, _ = fmt.Fprintln(out, formatTLS(t))
		}
	case formatSummary:
		renderSummary(out, tasks)
	case formatCounters:
		renderCounters(out, tasks)
	case formatVtodo:
		writeVtodo(cmd, tasks, nil, taskListOutput, taskListIncludeLogs)
	default: // table
		renderTable(out, tasks)
	}
}

// writeVtodo serialises the supplied tasks/tracks (and optionally logs)
// into a VCALENDAR and writes the .ics output. When outputPath is empty
// the calendar is written to cmd.OutOrStdout(); otherwise it is written
// to that file path. includeLogs gates VJOURNAL emission.
func writeVtodo(
	cmd *cobra.Command,
	tasks []*core.Task,
	tracks []*core.Track,
	outputPath string,
	includeLogs bool,
) {
	var logs []*core.LogEntry
	if includeLogs {
		logs = collectVtodoLogs(tasks)
	}
	opts := []vtodo.Option{}
	if includeLogs {
		opts = append(opts, vtodo.WithIncludeLogs(true))
	}
	cal, err := vtodo.BuildVCalendar(tasks, tracks, logs, opts...)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "vtodo encode failed: %v\n", err)
		return
	}
	body := cal.Serialize()
	if outputPath == "" {
		_, _ = fmt.Fprint(cmd.OutOrStdout(), body)
		return
	}
	if err := os.WriteFile(outputPath, []byte(body), 0o600); err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "failed to write %s: %v\n", outputPath, err)
	}
}

// collectVtodoLogs fetches log entries for each task that has an
// established storage handle. Failures degrade silently — vtodo export
// is a best-effort read path, not a write critical path.
func collectVtodoLogs(tasks []*core.Task) []*core.LogEntry {
	if len(tasks) == 0 {
		return nil
	}
	s, err := getStorageRaw()
	if err != nil {
		return nil
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	var out []*core.LogEntry
	for _, t := range tasks {
		if t == nil || t.ID == "" {
			continue
		}
		entries, gErr := s.GetLogs(ctx, t.ID, "asc")
		if gErr != nil {
			continue
		}
		out = append(out, entries...)
	}
	return out
}

func printTask(cmd *cobra.Command, task *core.Task, logs []*core.LogEntry, format string) {
	out := cmd.OutOrStdout()
	switch format {
	case formatJSON, formatYAML:
		result := map[string]interface{}{
			"task": task,
			"logs": logs,
		}
		_ = output.Render(out, format, result) //nolint:errcheck // best-effort output
	default:
		renderTaskDetail(out, task, logs)
	}
}

func formatTLS(t *core.Task) string {
	wm := core.DefaultWorkflow()
	marker := " "
	def, err := wm.GetStatusDef(t.Status)
	if err == nil && def.TLSMarker != "" {
		marker = def.TLSMarker
	}
	status := fmt.Sprintf("[%s]", marker)

	quotedTitle := fmt.Sprintf("%q", t.Title)
	parts := []string{status, formatTaskAlias(t), quotedTitle}

	if t.AssignedTo != nil && *t.AssignedTo != "" {
		parts = append(parts, "@"+*t.AssignedTo)
	}

	for _, tag := range t.Tags {
		parts = append(parts, "#"+tag)
	}

	if t.Effort != "" {
		parts = append(parts, "effort:"+string(t.Effort))
	}

	if t.Priority != "" {
		parts = append(parts, "prio:"+string(t.Priority))
	}

	if t.Reference != "" && !isDefaultRef(t.Reference, t.ID) {
		parts = append(parts, "ref:"+t.Reference)
	}
	if t.ProjectID != nil && *t.ProjectID != "" {
		parts = append(parts, "project_id="+*t.ProjectID)
	}

	if !t.CreatedAt.IsZero() {
		parts = append(parts, "created_at="+t.CreatedAt.Format(time.RFC3339))
	}
	if !t.UpdatedAt.IsZero() {
		parts = append(parts, "updated_at="+t.UpdatedAt.Format(time.RFC3339))
	}

	for k, v := range t.Meta {
		switch k {
		case "blocked_by":
			if blockedBy := t.BlockedBy(); len(blockedBy) > 0 {
				parts = append(parts, "blocked_by="+strings.Join(blockedBy, ","))
			}
		case "eva":
			if eva := t.Eva(); len(eva) > 0 {
				parts = append(parts, "eva="+strings.Join(eva, ","))
			}
		case "prio":
			// Skip: priority is now a first-class field serialised above.
		case "domain":
			parts = append(parts, "domain:"+fmt.Sprintf("%v", v))
		case "due":
			parts = append(parts, "due:"+fmt.Sprintf("%v", v))
		default:
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}

	return strings.Join(parts, " ")
}

// formatDuration returns a short human-readable duration using the largest unit only.
// e.g. 2d, 3h, 45m
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	days := int(d.Hours()) / 24
	if days > 0 {
		return fmt.Sprintf("%dd", days)
	}
	hours := int(d.Hours())
	if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

func renderTable(w io.Writer, tasks []*core.Task) {
	headers := []string{"ID", "Title", "Status", "Assigned", "Due", "Stale", "Blocked"}

	rows := make([][]string, 0, len(tasks))
	for _, t := range tasks {
		assignee := "-"
		if t.AssignedTo != nil {
			assignee = *t.AssignedTo
		}

		dueCol := "-"
		if t.DueAt != nil {
			if t.IsOverdue() {
				dueCol = "! " + t.DueAt.Format("2006-01-02")
			} else {
				dueCol = t.DueAt.Format("2006-01-02")
			}
		}

		staleCol := "-"
		if t.IsStale() {
			if s := t.StaleSince(); s != nil {
				staleCol = "! " + formatDuration(*s)
			}
		}

		blockedCol := "-"
		if t.IsBlocked() {
			blockedCol = *t.BlockedReason
		}

		rows = append(rows, []string{
			formatTaskAlias(t),
			t.Title,
			formatStatusPlain(t.Status),
			assignee,
			dueCol,
			staleCol,
			blockedCol,
		})
	}

	// Color is a pure function of status + state:
	//   DONE / SKIPPED → white (terminal, always)
	//   IN_PROGRESS → green
	//   blocker (ID in another task's blocked-by) → pink
	//   blocked (has BlockedReason) → muted
	//   everything else → white
	blockerIDs := make(map[string]bool)
	for _, t := range tasks {
		for _, dep := range t.BlockedBy() {
			blockerIDs[dep] = true
		}
	}

	primary := make(map[int]bool)
	blockers := make(map[int]bool)
	blocked := make(map[int]bool)
	for i, t := range tasks {
		switch {
		case t.Status == core.StatusDone,
			t.Status == core.StatusSkipped:
			// White — terminal statuses stay neutral.
		case t.Status == core.StatusInProgress:
			primary[i] = true
		case blockerIDs[t.ID]:
			blockers[i] = true
		case t.IsBlocked():
			blocked[i] = true
		}
	}

	var opts []TableOption
	if len(primary) > 0 || len(blockers) > 0 || len(blocked) > 0 {
		opts = append(opts,
			WithPrimaryRows(primary),
			WithSecondaryRows(blockers),
			WithMutedRows(blocked),
		)
	}
	renderTTYTable(w, headers, rows, termWidth(), opts...)
}

// renderWorkspaceTable renders a task table with an additional Project column.
func renderWorkspaceTable(w io.Writer, tasks []*core.Task) {
	headers := []string{"Project", "ID", "Title", "Status", "Assigned"}

	rows := make([][]string, 0, len(tasks))
	for _, t := range tasks {
		assignee := "-"
		if t.AssignedTo != nil {
			assignee = *t.AssignedTo
		}
		proj := "-"
		if t.ProjectID != nil && *t.ProjectID != "" {
			proj = projectLabel(*t.ProjectID)
		}

		rows = append(rows, []string{
			proj,
			formatTaskAlias(t),
			t.Title,
			formatStatus(t.Status),
			assignee,
		})
	}

	renderTTYTable(w, headers, rows, termWidth())
}

// formatStatusPlain returns the human-readable status label without ANSI
// escape sequences, safe to embed in table cell data.
func formatStatusPlain(status core.TaskStatus) string {
	wm := core.DefaultWorkflow()
	def, err := wm.GetStatusDef(status)
	if err != nil {
		return string(status)
	}
	if def.Label != "" {
		return def.Label
	}
	return def.Name
}

func formatStatus(status core.TaskStatus) string {
	wm := core.DefaultWorkflow()
	def, err := wm.GetStatusDef(status)
	if err != nil {
		return string(status)
	}
	label := def.Name
	if def.Label != "" {
		label = def.Label
	}
	if def.Color != "" {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(def.Color)).Render(label)
	}
	// Fallback to legacy styles for default statuses without explicit color
	switch status {
	case core.StatusTodo:
		return todoStyle.Render(label)
	case core.StatusInProgress:
		return inProgressStyle.Render(label)
	case core.StatusDone:
		return doneStyle.Render(label)
	case core.StatusSkipped:
		return skippedStyle.Render(label)
	default:
		return label
	}
}

// resolveTaskReference returns the absolute reference URI for a task.
// If the stored reference is relative (tlc:///T-XXXX), it is expanded using the
// task's project_id or the currently detected project.
func resolveTaskReference(t *core.Task) string {
	ref := t.Reference
	if ref == "" {
		ref = fmt.Sprintf("tlc:///%s", t.ID)
	}
	// Already absolute: contains a project host segment beyond the task ID.
	if !isLocalTaskRef(ref) {
		return ref
	}
	// Local form: tlc:///T-XXXX — expand with project ID.
	taskID := extractLocalTaskID(ref)
	if t.ProjectID != nil && *t.ProjectID != "" {
		return fmt.Sprintf("tlc://%s/%s", *t.ProjectID, taskID)
	}
	if proj := core.DetectProject(); proj != nil && proj.ProjectID != "" {
		return fmt.Sprintf("tlc://%s/%s", proj.ProjectID, taskID)
	}
	return ref
}

// isLocalTaskRef returns true if ref is a local (no-project) task reference
// in any of the known default forms: tlc:///T-..., tlc://T-..., task://T-....
func isLocalTaskRef(ref string) bool {
	switch {
	case strings.HasPrefix(ref, "tlc:///T-"):
		return true
	case strings.HasPrefix(ref, "tlc://T-"):
		return true
	case strings.HasPrefix(ref, "task://T-"):
		return true
	default:
		return false
	}
}

// extractLocalTaskID strips the scheme+authority from a local task reference,
// returning the bare task ID (e.g. "T-0042").
func extractLocalTaskID(ref string) string {
	for _, prefix := range []string{"tlc:///", "tlc://", "task://"} {
		if strings.HasPrefix(ref, prefix) {
			return strings.TrimPrefix(ref, prefix)
		}
	}
	return ref
}

// isDefaultRef returns true if ref is a default-generated local reference
// for the given task ID (any of the known local forms).
func isDefaultRef(ref, taskID string) bool {
	switch ref {
	case "tlc:///" + taskID,
		"tlc://" + taskID,
		"task://" + taskID:
		return true
	default:
		return false
	}
}

func renderTaskDetail(w io.Writer, t *core.Task, logs []*core.LogEntry) {
	_, _ = fmt.Fprintln(w, titleStyle.Render(fmt.Sprintf("Task: %s", formatTaskAlias(t))))
	if isVerboseOutput() && t.ID != "" && t.ID != formatTaskAlias(t) {
		_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("ID:"), t.ID)
	}
	_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Title:"), t.Title)
	_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Status:"), formatStatus(t.Status))

	assignee := "-"
	if t.AssignedTo != nil {
		assignee = *t.AssignedTo
	}
	_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Assigned:"), assignee)
	if t.Effort != "" {
		_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Effort:"), string(t.Effort))
	}
	if t.Priority != "" {
		_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Priority:"), string(t.Priority))
	}
	if t.StaleTimeout != nil {
		_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Stale Timeout:"), t.StaleTimeout.String())
	}
	if t.BlockedReason != nil && *t.BlockedReason != "" {
		_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Blocked Reason:"), *t.BlockedReason)
	}
	if t.StaleFiredAt != nil {
		_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Stale Fired At:"), t.StaleFiredAt.Format(time.RFC3339))
	}
	if t.TrackID != nil && *t.TrackID != "" {
		_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Track:"), formatTaskTrackDisplay(*t.TrackID))
	}
	if t.DueAt != nil {
		dueLabel := "Due:"
		if t.IsOverdue() {
			dueLabel = "Due (OVERDUE):"
		}
		_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render(dueLabel), t.DueAt.Format(time.RFC3339))
	}
	if t.RemindAt != nil {
		_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Remind At:"), t.RemindAt.Format(time.RFC3339))
	}
	if t.RRule != "" {
		_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("RRule:"), t.RRule)
	}
	if t.NoAutoRemind {
		_, _ = fmt.Fprintf(w, "%s yes\n", labelStyle.Render("No Auto-Remind:"))
	}
	if eva := t.Eva(); len(eva) > 0 {
		currentUser := core.GetCurrentUser()
		assignee := ""
		if t.AssignedTo != nil {
			assignee = *t.AssignedTo
		}
		if currentUser != assignee {
			_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Eva:"), strings.Join(eva, ", "))
		}
	}
	_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Tags:"), strings.Join(t.Tags, ", "))
	_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Reference:"), resolveTaskReference(t))
	_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Created:"), t.CreatedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Updated:"), t.UpdatedAt.Format(time.RFC3339))

	if t.Description != "" {
		_, _ = fmt.Fprintln(w, "\nDescription:")
		noColor := viper.GetBool("output.color") // true means no-color
		out, err := markdown.Render(t.Description, noColor)
		if err != nil {
			_, _ = fmt.Fprintln(w, t.Description)
		} else {
			_, _ = fmt.Fprint(w, out)
		}
	}

	if len(logs) > 0 {
		_, _ = fmt.Fprintln(w, "\nLogs:")
		for _, l := range logs {
			_, _ = fmt.Fprintf(w, "  %s  %-15s (%s) %s\n",
				l.Timestamp.Format("2006-01-02 15:04:05"),
				l.Action,
				l.By,
				l.Note,
			)
		}
	}
}
