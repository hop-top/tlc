package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
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
// the --verbose / -V flag (or output.verbose in config).
func isVerboseOutput() bool {
	return viper.GetBool("output.verbose")
}

// isShowTypeIDOutput reports whether the user wants durable TypeIDs
// rendered alongside human-readable aliases in default output.
//
// Distinct from isVerboseOutput on purpose: a user enabling
// `output.verbose: true` in config typically wants extra debug
// logging, not typeids polluting every "Created task T-NNNN" echo.
//
// Returns true when ANY of:
//   - output.show_typeid is set in config (explicit opt-in)
//   - TLC_SHOW_TYPEID env parses as truthy via strconv.ParseBool
//     (accepts 1/t/T/TRUE/true/True/etc.; trims whitespace)
//   - -VV or higher (verbose count flag, level >= 2) is set
//
// Default false. Use this to gate `ID:` companion lines and any other
// site that prints a durable typeid next to its human-readable
// counterpart.
func isShowTypeIDOutput() bool {
	if viper.GetBool("output.show_typeid") {
		return true
	}
	if v := strings.TrimSpace(os.Getenv("TLC_SHOW_TYPEID")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil && b {
			return true
		}
	}
	// kit registers --verbose / -V as a stackable Count flag and binds
	// it as int on the global viper at "output.verbose" (see kitRoot in
	// root.go). -V = 1, -VV = 2, etc. Level >= 2 opts in to typeid
	// display alongside debug logging.
	if viper.GetInt("output.verbose") >= 2 {
		return true
	}
	return false
}

func formatTasks(cmd *cobra.Command, tasks []*core.Task, format string, statusProvided bool) error {
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
		return writeVtodo(cmd, tasks, nil, taskListOutput, taskListIncludeLogs)
	default: // table
		cols := effectiveTaskColumns(cmd, statusProvided)
		renderTable(out, tasks, cols)
	}
	return nil
}

// effectiveTaskColumns resolves the table header list for `task list`.
// See resolveEffectiveColumns for the full ladder and pruning logic.
func effectiveTaskColumns(cmd *cobra.Command, statusProvided bool) []string {
	return resolveEffectiveColumns(cmd, taskListDefaultColumns, taskColumnHeaders, statusProvided, nil)
}

// writeVtodo serialises the supplied tasks/tracks (and optionally logs)
// into a VCALENDAR and writes the .ics output. When outputPath is empty
// the calendar is written to cmd.OutOrStdout(); otherwise it is written
// to that file path. Returns non-nil on encode/write failure so callers
// (and downstream scripts) can distinguish a real export from a silent
// no-op.
func writeVtodo(
	cmd *cobra.Command,
	tasks []*core.Task,
	tracks []*core.Track,
	outputPath string,
	includeLogs bool,
) error {
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
		return fmt.Errorf("vtodo encode failed: %w", err)
	}
	body, err := vtodo.Serialize(cal)
	if err != nil {
		return fmt.Errorf("vtodo encode failed: %w", err)
	}
	if outputPath == "" {
		_, _ = fmt.Fprint(cmd.OutOrStdout(), body)
		return nil
	}
	if err := os.WriteFile(outputPath, []byte(body), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", outputPath, err)
	}
	return nil
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

// printTaskWithBlockers renders a task, adding a resolved "blocked_by"
// array to the structured views. Every pre-existing key is preserved —
// blockers are an additive field, not a restructuring.
func printTaskWithBlockers(
	cmd *cobra.Command,
	task *core.Task,
	logs []*core.LogEntry,
	format string,
	blockers []blockerJSON,
) {
	out := cmd.OutOrStdout()
	switch format {
	case formatJSON, formatYAML:
		result := map[string]interface{}{
			"task": taskShowJSON{Task: task, BlockedBy: blockers},
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
		parts = append(parts, "created_at="+DisplayTime(t.CreatedAt, LayoutRFC3339))
	}
	if !t.UpdatedAt.IsZero() {
		parts = append(parts, "updated_at="+DisplayTime(t.UpdatedAt, LayoutRFC3339))
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

// taskTableRow is the row schema for `tlc task list` table output.
// The struct holds all possible columns; effectiveTaskColumns selects
// which headers are rendered so the default view stays identical to
// the pre-widening layout.
type taskTableRow struct {
	ID       string `table:"ID"`
	Title    string `table:"Title"`
	Status   string `table:"Status"`
	Priority string `table:"Priority"`
	Assigned string `table:"Assigned"`
	Track    string `table:"Track"`
	Effort   string `table:"Effort"`
	Due      string `table:"Due"`
	Stale    string `table:"Stale"`
	Blocked  string `table:"Blocked"`
}

func renderTable(w io.Writer, tasks []*core.Task, cols []string) {
	// Color is a pure function of status + state:
	//   DONE / SKIPPED → none (terminal, always)
	//   IN_PROGRESS    → primary (green)
	//   blocker (ID in another task's blocked-by) → secondary (pink)
	//   blocked (has BlockedReason) → muted
	//   everything else → none
	blockerIDs := make(map[string]bool)
	for _, t := range tasks {
		for _, dep := range t.BlockedBy() {
			blockerIDs[dep] = true
		}
	}
	unmet := unmetBlockersByTask(tasks)
	emphasis := make(map[int]output.EmphasisKind)
	for i, t := range tasks {
		switch {
		case t.Status == core.StatusDone, t.Status == core.StatusSkipped:
			// none — terminal statuses stay neutral.
		case t.Status == core.StatusInProgress:
			emphasis[i] = output.EmphasisPrimary
		case blockerIDs[t.ID]:
			emphasis[i] = output.EmphasisSecondary
		case t.IsBlocked():
			emphasis[i] = output.EmphasisMuted
		}
	}

	// When column customization is active (cols non-nil), build wide rows and
	// use the cols-aware plain formatter so only the requested headers render.
	// When cols is nil (default view), build narrow rows and use the styled
	// path so TTY gets box-drawing + row emphasis unchanged from before.
	if cols != nil {
		rows := make([]taskTableRow, len(tasks))
		for i, t := range tasks {
			rows[i] = buildWideRow(t, unmet[t.ID])
		}
		_ = renderStyledListCols(w, formatTable, rows, emphasis, cols) //nolint:errcheck // best-effort output
		return
	}

	type narrowRow struct {
		ID       string `table:"ID"`
		Title    string `table:"Title"`
		Status   string `table:"Status"`
		Assigned string `table:"Assigned"`
		Due      string `table:"Due"`
		Stale    string `table:"Stale"`
		Blocked  string `table:"Blocked"`
	}
	rows := make([]narrowRow, len(tasks))
	for i, t := range tasks {
		c := computeTaskCells(t, unmet[t.ID])
		rows[i] = narrowRow{
			ID:       c.id,
			Title:    c.title,
			Status:   c.status,
			Assigned: c.assigned,
			Due:      c.due,
			Stale:    c.stale,
			Blocked:  c.blocked,
		}
	}
	_ = renderStyledList(w, formatTable, rows, emphasis) //nolint:errcheck // best-effort output
}

// unmetBlockersByTask maps each task ID to the display refs of its
// blockers that are not yet satisfied.
//
// A blocker is met when its status is terminal (DONE/SKIPPED). Blockers
// are resolved against the listed tasks first; anything not on the page
// (including cross-project refs) is looked up once via storage. A
// blocker that cannot be resolved at all is reported as "<ref>?" so a
// dangling edge stays visible rather than silently reading as met.
func unmetBlockersByTask(tasks []*core.Task) map[string][]string {
	wm := core.DefaultWorkflow()

	// Index the page by every identifier a blocked_by entry may use.
	onPage := make(map[string]*core.Task, len(tasks)*2)
	for _, t := range tasks {
		onPage[t.ID] = t
		if alias := formatTaskAlias(t); alias != "" {
			onPage[alias] = t
		}
	}

	// Collect refs needing a storage lookup, then resolve them in one
	// pass so the common case (blockers on the same page) costs nothing.
	offPage := make(map[string]*core.Task)
	var missing []string
	for _, t := range tasks {
		for _, ref := range t.BlockedBy() {
			if _, ok := onPage[ref]; ok {
				continue
			}
			if _, ok := offPage[ref]; ok {
				continue
			}
			missing = append(missing, ref)
		}
	}
	if len(missing) > 0 {
		resolveOffPageBlockers(missing, offPage)
	}

	out := make(map[string][]string)
	for _, t := range tasks {
		var unmet []string
		for _, ref := range t.BlockedBy() {
			blocker, ok := onPage[ref]
			if !ok {
				blocker, ok = offPage[ref]
			}
			if !ok || blocker == nil {
				// Unresolvable edge — surface it, flagged.
				unmet = append(unmet, ref+"?")
				continue
			}
			if wm.IsTerminal(blocker.Status) {
				continue
			}
			unmet = append(unmet, formatTaskAlias(blocker))
		}
		if len(unmet) > 0 {
			out[t.ID] = unmet
		}
	}
	return out
}

// resolveOffPageBlockers looks up blocker refs that were not part of the
// listed tasks, filling found entries into dst. Lookup failures leave
// the ref absent so the caller renders it as unresolved.
func resolveOffPageBlockers(refs []string, dst map[string]*core.Task) {
	s, err := getStorageRaw()
	if err != nil {
		return
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	for _, ref := range refs {
		lookup := ref
		if translated, tErr := parseTaskRefForCLI(ctx, s, ref); tErr == nil && translated != "" {
			lookup = translated
		}
		if blocker, gErr := s.GetTask(ctx, lookup); gErr == nil && blocker != nil {
			dst[ref] = blocker
		}
	}
}

// taskCells holds the computed display strings for all table columns.
type taskCells struct {
	id, title, status, assigned, due, stale, blocked string
	priority, track, effort                          string
}

// computeTaskCells derives every display cell for a task in one place,
// eliminating duplicated cell-derivation logic across row builders.
func computeTaskCells(t *core.Task, unmetBlockers []string) taskCells {
	assignee := "-"
	if t.AssignedTo != nil {
		assignee = *t.AssignedTo
	}
	// Table column: humanize DueAt relative to now ("in 3d", "2h ago").
	// JSON/YAML output paths stay on RFC3339 via the structured marshaller.
	dueCol := "-"
	if t.DueAt != nil {
		if t.IsOverdue() {
			dueCol = "! " + DisplayTimePtrRelative(t.DueAt)
		} else {
			dueCol = DisplayTimePtrRelative(t.DueAt)
		}
	}
	staleCol := "-"
	if t.IsStale() {
		if s := t.StaleSince(); s != nil {
			staleCol = "! " + formatDuration(*s)
		}
	}
	// The Blocked column reports both kinds of blocker: the free-text
	// BlockedReason and unmet dependency edges. Dependencies used to be
	// invisible here, which hid broken/destroyed blocked-by edges from
	// every non-interactive view.
	blockedCol := "-"
	switch {
	case t.IsBlocked() && len(unmetBlockers) > 0:
		blockedCol = *t.BlockedReason + "; " + strings.Join(unmetBlockers, ",")
	case t.IsBlocked():
		blockedCol = *t.BlockedReason
	case len(unmetBlockers) > 0:
		blockedCol = strings.Join(unmetBlockers, ",")
	}
	priorityCol := "-"
	if t.Priority != "" {
		priorityCol = string(t.Priority)
	}
	trackCol := "-"
	if t.TrackID != nil && *t.TrackID != "" {
		trackCol = *t.TrackID
	}
	effortCol := "-"
	if t.Effort != "" {
		effortCol = string(t.Effort)
	}
	return taskCells{
		id:       formatTaskAlias(t),
		title:    t.Title,
		status:   formatStatusPlain(t.Status),
		assigned: assignee,
		due:      dueCol,
		stale:    staleCol,
		blocked:  blockedCol,
		priority: priorityCol,
		track:    trackCol,
		effort:   effortCol,
	}
}

// buildWideRow populates a taskTableRow with all available fields.
func buildWideRow(t *core.Task, unmetBlockers []string) taskTableRow {
	c := computeTaskCells(t, unmetBlockers)
	return taskTableRow{
		ID:       c.id,
		Title:    c.title,
		Status:   c.status,
		Priority: c.priority,
		Assigned: c.assigned,
		Track:    c.track,
		Effort:   c.effort,
		Due:      c.due,
		Stale:    c.stale,
		Blocked:  c.blocked,
	}
}

// workspaceTaskRow is the row schema for the workspace tasks table
// (cross-project view, includes Project column).
type workspaceTaskRow struct {
	Project  string `table:"Project"`
	ID       string `table:"ID"`
	Title    string `table:"Title"`
	Status   string `table:"Status"`
	Assigned string `table:"Assigned"`
}

// renderWorkspaceTable renders a task table with an additional Project column.
func renderWorkspaceTable(w io.Writer, tasks []*core.Task) {
	rows := make([]workspaceTaskRow, len(tasks))
	for i, t := range tasks {
		assignee := "-"
		if t.AssignedTo != nil {
			assignee = *t.AssignedTo
		}
		proj := "-"
		if t.ProjectID != nil && *t.ProjectID != "" {
			proj = projectLabel(*t.ProjectID)
		}
		rows[i] = workspaceTaskRow{
			Project: proj,
			ID:      formatTaskAlias(t),
			Title:   t.Title,
			// Cell values must be plain — kit/output's tabwriter (non-TTY)
			// passes them through verbatim, so any pre-styled lipgloss
			// escapes would leak into piped output.
			Status:   formatStatusPlain(t.Status),
			Assigned: assignee,
		}
	}

	_ = renderStyledList(w, formatTable, rows, nil) //nolint:errcheck // best-effort output
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
	if isShowTypeIDOutput() && t.ID != "" && t.ID != formatTaskAlias(t) {
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
		// Detail view: humanise past timestamps ("2d ago"). T-1384.
		_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Stale Fired At:"), DisplayTimePtrRelative(t.StaleFiredAt))
	}
	if t.TrackID != nil && *t.TrackID != "" {
		_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Track:"), formatTaskTrackDisplay(*t.TrackID))
	}
	if t.DueAt != nil {
		dueLabel := "Due:"
		if t.IsOverdue() {
			dueLabel = "Due (OVERDUE):"
		}
		_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render(dueLabel), DisplayTimePtr(t.DueAt, LayoutRFC3339))
	}
	if t.RemindAt != nil {
		_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Remind At:"), DisplayTimePtr(t.RemindAt, LayoutRFC3339))
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
	// Reference: hide auto-generated internal refs (tlc://<project>/<typeid>)
	// from default human output — the embedded typeid only surfaces when
	// the user explicitly opts in. External refs (github:..., docs/...,
	// https://...) remain visible because they carry information the alias
	// does not.
	if isShowTypeIDOutput() || !core.IsInternalTaskRef(t.Reference, t) {
		_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Reference:"), resolveTaskReference(t))
	}
	// Detail view: CreatedAt/UpdatedAt humanise ("2d ago", "5m ago").
	// DueAt/RemindAt above stay absolute because a specific deadline
	// is more useful than a relative one when deep-inspecting a task.
	// T-1384.
	_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Created:"), DisplayTimeRelative(t.CreatedAt))
	_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Updated:"), DisplayTimeRelative(t.UpdatedAt))

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
			_, _ = fmt.Fprintf(
				w, "  %s  %-15s (%s) %s\n",
				DisplayTime(l.Timestamp, LayoutDateTime),
				l.Action,
				l.By,
				l.Note,
			)
		}
	}
}
