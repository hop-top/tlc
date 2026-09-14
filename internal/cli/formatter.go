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
//
// This is the *prose* alias — what reads best mid-sentence ("Created track
// browser-rendering"). Table ID cells want formatTrackDisplayID instead:
// a slug is unbounded and pushes every other column off the screen.
func formatTrackAlias(t *core.Track) string {
	if t == nil {
		return ""
	}
	if t.Slug != "" {
		return t.Slug
	}
	return t.ID
}

// formatTrackDisplayID renders the short "L-NNNN" identifier for an ID
// column. Delegates to core.FormatTrackDisplay, which falls back to the
// durable typeid when Seq is 0 — rows predating the sequence backfill, and
// structs assembled in memory, still get an identifier rather than a hole
// in the column.
func formatTrackDisplayID(t *core.Track) string {
	if t == nil {
		return ""
	}
	return core.FormatTrackDisplay(t)
}

// formatTrackSlug renders the slug cell for a table row. The cell carries
// the FULL slug: clipping it to the terminal is the table renderer's job
// (kit/output measures the whole table and elides the widest cells), and
// keeping it out of here is what stops a truncated slug from ever reaching
// the JSON/YAML projections, where it would be an unresolvable reference
// rather than a cosmetic elision.
func formatTrackSlug(t *core.Track) string {
	if t == nil {
		return ""
	}
	return t.Slug
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

// listOptions carries the presentation choices that vary between the
// callers of formatTasks. Threaded as options rather than positional
// parameters so the callers that want none of them (task stale, tag
// list) keep their existing call shape and cannot accidentally opt in.
type listOptions struct {
	// groupBy is the --group-by dimension, empty for an ungrouped
	// listing. Validated by the caller via ValidGroupByKey; an unknown
	// key here degrades to ungrouped rather than rendering nothing.
	groupBy string

	// groupLimit caps the rows rendered WITHIN each group. Zero or
	// negative means no cap — the zero value must mean "unset", not
	// "render nothing", or a config-supplied default would silently
	// blank every section.
	//
	// Deliberately NOT --limit. --limit caps the match set fetched from
	// the store, before grouping; this caps what each group shows after
	// it, which is the only way every group stays represented when one
	// of them holds most of the rows.
	groupLimit int

	// partialMatchSet records that the fetched rows are a --limit page
	// of the matches, not all of them. It changes nothing about what
	// renders — only whether the per-group totals are qualified. See
	// renderGroupedTables.
	partialMatchSet bool

	// labeler renames group headings from the stored KEY to a name a
	// reader recognizes — a track's title, an assignee's resolved
	// profile. Nil for the dimensions whose keys are already names, and
	// nil for every caller that does not group at all.
	//
	// Built by the caller, not here: resolving a track title needs a
	// store handle, and formatTasks is a rendering chokepoint that
	// deliberately holds none. See groupLabelerFor.
	labeler groupLabeler

	// projectColumn adds the Project column to the default table column
	// set. Set by the cross-project view (--workspace), where a bare
	// T-NNNN is ambiguous: the sequence is per project, so two rows in one
	// listing legitimately carry the same id.
	//
	// A DEFAULT, not a floor. An explicit --cols is the user naming the
	// whole set, and it wins here exactly as it wins everywhere else —
	// "project" is a registry key they can name themselves.
	projectColumn bool
}

// listOption mutates listOptions. See withGroupBy.
type listOption func(*listOptions)

// withGroupBy renders the table format as one titled table per group
// along key. Empty key means ungrouped — the default — so callers can
// forward an unset flag unconditionally.
func withGroupBy(key string) listOption {
	return func(o *listOptions) { o.groupBy = key }
}

// withGroupLimit caps the rows rendered within each group at n. Zero or
// negative is no cap, so callers forward an unset flag unconditionally.
// Meaningless without withGroupBy, and the command rejects that
// combination before reaching here.
func withGroupLimit(n int) listOption {
	return func(o *listOptions) { o.groupLimit = n }
}

// withPartialMatchSet declares that --limit truncated the match set, so
// per-group totals count only the fetched rows.
func withPartialMatchSet(partial bool) listOption {
	return func(o *listOptions) { o.partialMatchSet = partial }
}

// withProjectColumn adds Project to the default table columns. Used by
// the cross-project listing; false everywhere else, so a single-project
// listing does not grow a column whose every cell is the same value.
func withProjectColumn(show bool) listOption {
	return func(o *listOptions) { o.projectColumn = show }
}

// withGroupLabeler supplies the key-to-name mapping for group headings.
// Nil is the identity pass, so callers forward an unresolved dimension
// unconditionally.
func withGroupLabeler(l groupLabeler) listOption {
	return func(o *listOptions) { o.labeler = l }
}

// formatTasks is the SINGLE chokepoint for `task list`-shaped row
// output. Every path — plain, aggregate, stale, tag — renders through
// here, so a presentation change lands once. Grouping in particular is
// threaded through opts rather than branched at the call sites: a second
// branch is a second chance for the two to diverge.
func formatTasks(
	cmd *cobra.Command,
	tasks []*core.Task,
	format string,
	statusProvided bool,
	opts ...listOption,
) error {
	var o listOptions
	for _, opt := range opts {
		opt(&o)
	}

	out := cmd.OutOrStdout()
	switch format {
	case formatJSON, formatYAML:
		// Grouped structured output NESTS, so a script consumes the same
		// shape a human reads. The wrap is conditional on purpose: with
		// no grouping key the payload stays the top-level array every
		// existing consumer iterates. Wrapping unconditionally would
		// break all of them at once and silently — `jq '.[]'` on an
		// object fails exactly the way it failed on the `null` the
		// empty-slice fix removed.
		if groups := groupTasks(tasks, o.groupBy); len(groups) > 0 {
			// --group-limit is NOT applied here. It caps what is
			// DISPLAYED; a truncated task list inside a JSON payload
			// carries no marker of its truncation, so a consumer that
			// re-serializes it persists a subset as the whole. That is
			// data corruption, not a display choice. The omission is
			// announced instead — see noteGroupLimitIgnored.
			//
			// Labeled the SAME way the table path labels, so a script
			// reading `.groups[].name` and a human reading the headings
			// see the same names. Emitting raw typeids here while the
			// table shows titles would make the two views of one listing
			// disagree about what the groups are called.
			groups = applyGroupLabels(groups, o.labeler)
			_ = output.Render(out, format, groupedPayload(groups)) //nolint:errcheck // best-effort output
			return nil
		}
		_ = output.Render(out, format, normalizeEmptySlices(tasks)) //nolint:errcheck // best-effort output
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
		// Column resolution runs ONCE and is shared by every group
		// table, so the headers cannot differ between sections of the
		// same listing.
		cols := effectiveTaskColumns(cmd, statusProvided, o.projectColumn)
		if groups := groupTasks(tasks, o.groupBy); len(groups) > 0 {
			// Headings show NAMES, not the stored keys grouping ran on:
			// a track's title rather than its typeid, an assignee's
			// resolved profile rather than whichever alias the row
			// happens to carry. Applied after grouping so the ordering
			// groupTasks established survives — see applyGroupLabels.
			renderGroupedTables(out, applyGroupLabels(groups, o.labeler), cols, o)
			return nil
		}
		// --group-limit is NOT consulted here. It caps rows within a
		// group, and an ungrouped listing has none; applying it would
		// silently truncate a plain `task list`, which is a data loss
		// the user did not ask for and cannot see.
		renderTable(out, tasks, cols)
	}
	return nil
}

// renderGroupedTables writes one titled table per group: the group name
// as a heading, its rows beneath it, a blank line between sections.
// Headers repeat per table because each table stands alone — a reader
// scrolled to the third section should not have to scroll back up to
// learn what the columns mean.
//
// A distinct-count footer follows ONLY when the rendered row count
// exceeds the distinct task count, which happens under `--group-by tag`
// because tag membership is many-to-many. Without the footer "3 rows"
// over 2 tasks reads as a duplication bug; with it, the duplication is
// stated. When the counts agree the line carries no information and is
// omitted rather than printed as redundant noise.
//
// TRUNCATION IS ANNOUNCED, never silent. o.groupLimit caps each group's
// rows, and a capped group's heading carries "(shown of total)" so the
// reader can see rows were withheld. A group that fits under the cap is
// NOT annotated: "(2 of 2)" states nothing, and an annotation printed
// unconditionally stops meaning "there is more".
//
// The counts the footer and the headings report are counts of RENDERED
// rows and of the FETCHED match set respectively — never of the store.
// When --limit truncated that match set, o.partialMatchSet is set and a
// trailing note says the totals are partial. Printing "2 of 5" off a
// truncated fetch would state a group size that is not the group's size,
// the same defect noteIgnoredPagination exists to prevent for counts.
func renderGroupedTables(w io.Writer, groups []TaskGroup, cols []string, o listOptions) {
	rows := 0
	truncated := false
	distinct := make(map[string]struct{})
	for i, g := range groups {
		if i > 0 {
			_, _ = fmt.Fprintln(w)
		}
		shown := g.Tasks
		if o.groupLimit > 0 && len(shown) > o.groupLimit {
			shown = shown[:o.groupLimit]
			truncated = true
		}
		_, _ = fmt.Fprintln(w, groupHeading(w, groupTitle(g.Name, len(shown), len(g.Tasks))))
		renderTable(w, shown, cols)
		rows += len(shown)
		for _, t := range shown {
			if t != nil {
				distinct[t.ID] = struct{}{}
			}
		}
	}
	if rows > len(distinct) {
		_, _ = fmt.Fprintf(w, "\n%d rows, %d distinct tasks\n", rows, len(distinct))
	}
	// Only when a total was actually printed: with nothing truncated
	// there is no "of N" on screen for the note to qualify, and an
	// unconditional disclaimer is noise a reader learns to skip.
	if truncated && o.partialMatchSet {
		_, _ = fmt.Fprintf(w,
			"\nnote: group totals are partial; --limit truncated the match set before grouping\n")
	}
}

// groupTitle renders a group's heading text, appending the shown/total
// count only when rows were withheld.
func groupTitle(name string, shown, total int) string {
	if shown >= total {
		return name
	}
	return fmt.Sprintf("%s (%d of %d)", name, shown, total)
}

// groupHeading styles a group name as a section title. titleStyle is the
// same style `task show` uses for its heading, so grouped output reads
// as part of the same CLI rather than a bolt-on.
//
// STYLED ONLY ON A TERMINAL, and gated on the destination writer rather
// than on a global. lipgloss renders escape sequences whatever it is
// writing into, so an unguarded heading ships raw escape bytes down a
// pipe, into a redirect, and into a script's stdin — the same hazard the
// table cells avoid by carrying plain values (see formatStatusPlain).
// The gate is the one kit/output applies to its own styled renderer:
// *os.File plus a character device. Everything else gets the bare name.
func groupHeading(w io.Writer, name string) string {
	if !styledWriter(w) {
		return name
	}
	return titleStyle.Render(name)
}

// styledWriter reports whether w is a terminal that can render ANSI.
//
// Deliberately NOT writerInteractive: that one additionally requires
// /dev/tty to be openable, which is the gate for prompting a human. A
// heading only needs to know whether escapes will be displayed or stored,
// and a writer can be a perfectly good terminal with no controlling tty.
func styledWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && charDevice(f)
}

// effectiveTaskColumns resolves the table header list for `task list`.
// See resolveEffectiveColumns for the full ladder and pruning logic.
//
// showProject injects the Project column for the cross-project view, the
// same shape effectiveTrackColumns uses for --all-projects. The injection
// is a transform over the KEY list rather than a second row struct, so
// the cross-project table is the same table with one more column — not a
// parallel renderer free to drift away from this one.
func effectiveTaskColumns(cmd *cobra.Command, statusProvided, showProject bool) []string {
	return resolveEffectiveColumns(cmd, taskListDefaultColumns, taskColumnHeaders, statusProvided,
		func(keys []string, explicit bool) ([]string, bool) {
			// An explicit --cols is the user naming the whole set, and a
			// default does not get to append itself to it. They can still
			// ask for the column by name — "project" is a registry key,
			// not a mode this path switches on.
			if showProject && !explicit && !containsKey(keys, "project") {
				return injectFirst(keys, "project"), true
			}
			return keys, false
		})
}

// vtodoConfigOptions resolves the `output.vtodo.*` config keys into
// vtodo Options. Both keys are seeded with the vtodo package defaults
// in setDefaults, and the Option constructors ignore empty strings, so
// an unset or blank key leaves the package default in force.
//
// uid_domain is not cosmetic on the decode side: it is the domain tlc
// recognizes as its own, so a calendar exported under one domain and
// reimported under another reads as foreign. The CLI only encodes today
// (the plugin owns the decode path), but any decode added here must
// resolve the domain through this same helper.
func vtodoConfigOptions() []vtodo.Option {
	return []vtodo.Option{
		vtodo.WithProductID(viper.GetString("output.vtodo.product_id")),
		vtodo.WithUIDDomain(viper.GetString("output.vtodo.uid_domain")),
	}
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
	opts := vtodoConfigOptions()
	if includeLogs {
		opts = append(opts, vtodo.WithIncludeLogs(true))
	}
	if runs := collectVtodoRuns(tasks); len(runs) > 0 {
		opts = append(opts, vtodo.WithRecipeRuns(runs))
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

// collectVtodoRuns fetches the recipe run behind every exported task
// that was materialised by one (Task.RunID), each once, so the export
// carries the playthrough VEVENT the task's X-TLC-RUN edge points at.
// Best-effort like collectVtodoLogs: no storage or a missing run row
// drops the run, never the export.
func collectVtodoRuns(tasks []*core.Task) []*core.RecipeRun {
	seen := make(map[string]bool)
	var ids []string
	for _, t := range tasks {
		if t == nil || t.RunID == "" || seen[t.RunID] {
			continue
		}
		seen[t.RunID] = true
		ids = append(ids, t.RunID)
	}
	if len(ids) == 0 {
		return nil
	}
	s, err := getStorageRaw()
	if err != nil {
		return nil
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	var out []*core.RecipeRun
	for _, id := range ids {
		run, gErr := s.GetRecipeRun(ctx, id)
		if gErr != nil || run == nil {
			continue
		}
		out = append(out, run)
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
		_ = output.Render(out, format, normalizeEmptySlices(result)) //nolint:errcheck // best-effort output
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
	// Project leads because the cross-project view (--workspace) has
	// always led with it, and because the ID that follows is a per-project
	// sequence: two projects hand out the same T-NNNN, so the scope has to
	// be adjacent to the identifier it scopes.
	Project  string `table:"Project"`
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
	priority, track, effort, project                 string
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
	// The SHORT label, not the registered id: an org prefix every row
	// repeats buys nothing and costs width the titles need. Grouping still
	// buckets on the full id — see groupTasks — so the heading names the
	// project unambiguously while the column only has to disambiguate rows.
	projectCol := "-"
	if t.ProjectID != nil && *t.ProjectID != "" {
		projectCol = projectLabel(*t.ProjectID)
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
		project:  projectCol,
	}
}

// buildWideRow populates a taskTableRow with all available fields.
func buildWideRow(t *core.Task, unmetBlockers []string) taskTableRow {
	c := computeTaskCells(t, unmetBlockers)
	return taskTableRow{
		Project:  c.project,
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

// formatStatusPlain returns the human-readable status label without ANSI
// escape sequences, safe to embed in table cell data.
//
// EVERY table cell goes through this, never formatStatus. kit/output's
// non-TTY renderer is a tabwriter that passes cell values through
// VERBATIM — it neither strips nor re-measures escape sequences — so a
// pre-styled lipgloss cell ships raw escape bytes into a pipe, a
// redirect, or a script's stdin, and mismeasures the column width on the
// way out. Row color is applied by the renderer instead, which knows
// whether it is writing to a terminal; formatStatus is for prose
// (`task show`), not for cells.
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
