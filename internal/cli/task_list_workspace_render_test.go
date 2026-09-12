package cli

// Coverage for the workspace table arm of `task list --workspace`.
//
// The cross-project view used to own a private row struct and a private
// renderer, which meant every feature landed on the shared `task list`
// table — column selection, grouping, truncation, priority-driven column
// dropping — stopped at the workspace boundary. The tests here pin the
// three things that collapse has to get right:
//
//   - The Project column survives. It is the ONLY column the workspace
//     view adds, and it is the reason the private renderer existed; a
//     collapse that drops it makes the cross-project listing unreadable
//     because two rows can carry the same T-NNNN.
//   - No ANSI escape reaches a non-TTY writer. The deleted renderer
//     carried a comment explaining why: kit's tabwriter passes cell
//     values through VERBATIM when the writer is not a terminal, so a
//     pre-styled lipgloss cell leaks escape bytes into a pipe. The
//     shared path must be plain in the same conditions.
//   - --cols and --group-by, silently inert under --workspace before,
//     now apply.

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"hop.top/tlc/internal/core"
)

// workspaceTasks builds a cross-project result set: two projects, two
// assignees, one unassigned row so the "(none)" section is exercised.
func workspaceTasks() []*core.Task {
	return []*core.Task{
		{
			ID: "T-0001", Seq: 1, Title: "Wire the alpha parser",
			Status: core.StatusTodo, AssignedTo: strptr("ana"),
			ProjectID: strptr("myorg/alpha"), Priority: core.PriorityP1,
		},
		{
			ID: "T-0002", Seq: 2, Title: "Alpha retry backoff",
			Status: core.StatusInProgress, AssignedTo: strptr("bo"),
			ProjectID: strptr("myorg/alpha"),
		},
		{
			ID: "T-0001", Seq: 1, Title: "Beta cache eviction",
			Status: core.StatusDone, AssignedTo: strptr("ana"),
			ProjectID: strptr("myorg/beta"),
		},
		{
			ID: "T-0002", Seq: 2, Title: "Beta docs pass",
			Status:    core.StatusTodo,
			ProjectID: strptr("myorg/beta"),
		},
	}
}

// workspaceRenderCmd returns a command whose output is captured, with
// viper reset so a --cols value set by one test cannot leak into the
// next. resolveEffectiveColumns reads the "cols" key off the global
// viper, so the reset is load-bearing, not hygiene.
func workspaceRenderCmd(t *testing.T) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	viper.Set("cols", nil)
	t.Cleanup(func() { viper.Set("cols", nil) })
	cmd := &cobra.Command{Use: "list"}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	return cmd, &buf
}

// renderWorkspace runs the workspace table arm and returns its text.
func renderWorkspace(t *testing.T, tasks []*core.Task, opts ...listOption) string {
	t.Helper()
	cmd, buf := workspaceRenderCmd(t)
	if err := formatWorkspaceTasks(cmd, tasks, formatTable, opts...); err != nil {
		t.Fatalf("formatWorkspaceTasks: %v", err)
	}
	return buf.String()
}

// TestWorkspaceTableKeepsProjectColumn pins the one column the workspace
// view adds. Without it a reader cannot tell myorg/alpha's T-0001 from
// myorg/beta's — the IDs are per-project sequences and collide by design.
func TestWorkspaceTableKeepsProjectColumn(t *testing.T) {
	got := renderWorkspace(t, workspaceTasks())

	if !strings.Contains(got, "Project") {
		t.Fatalf("workspace table lost its Project header:\n%s", got)
	}
	for _, label := range []string{"alpha", "beta"} {
		if !strings.Contains(got, label) {
			t.Errorf("workspace table missing project label %q:\n%s", label, got)
		}
	}
	// The label is the basename, not the full registered id: the column
	// exists to disambiguate rows on a terminal, and "myorg/alpha" spends
	// width on an org prefix every row repeats.
	if strings.Contains(got, "myorg/alpha") {
		t.Errorf("Project column should show the short label, not the full id:\n%s", got)
	}
}

// TestWorkspaceTableCarriesSharedColumns proves the collapse actually
// happened: the workspace row is the shared task row, so the columns the
// shared default carries (Due, Stale, Blocked) render here too. The old
// private struct had exactly five fields and could not.
func TestWorkspaceTableCarriesSharedColumns(t *testing.T) {
	got := renderWorkspace(t, workspaceTasks())

	for _, header := range []string{"Project", "ID", "Title", "Status", "Assigned", "Due", "Stale", "Blocked"} {
		if !strings.Contains(got, header) {
			t.Errorf("workspace table missing shared column %q:\n%s", header, got)
		}
	}
}

// TestWorkspaceTableNoANSIInNonTTY is the trap the deleted renderer
// documented. kit/output's tabwriter writes cell values verbatim on a
// non-TTY writer, so a status cell styled with lipgloss would ship raw
// escape bytes into a pipe, a file, or a script's stdin. A bytes.Buffer
// is a non-TTY writer, so this asserts exactly that condition.
func TestWorkspaceTableNoANSIInNonTTY(t *testing.T) {
	got := renderWorkspace(t, workspaceTasks())
	assertNoANSI(t, got, "workspace table")
}

// TestWorkspaceGroupedNoANSIInNonTTY covers the grouped variant: group
// headings go through titleStyle, which is a lipgloss style. lipgloss
// suppresses color when it detects no terminal, and this pins that the
// suppression actually holds for the writer the workspace path uses.
func TestWorkspaceGroupedNoANSIInNonTTY(t *testing.T) {
	got := renderWorkspace(t, workspaceTasks(), withGroupBy("project"))
	assertNoANSI(t, got, "grouped workspace table")
}

// TestTableCellsNeverStyleStatus is the cell half of the ANSI guard, and
// it reads the SOURCE rather than the rendered bytes. That is deliberate,
// and the reason is worth stating because the obvious test does not work:
//
// The default workflow names its status colors ("yellow", "blue",
// "green"). lipgloss v2 parses ANSI indices and #hex and ignores names,
// so formatStatus returns a BARE label for every status a default-config
// test can produce — byte-identical to formatStatusPlain. A cell rewired
// to formatStatus therefore passes both a no-escape assertion and an
// output-comparison assertion, while being one `color: "42"` in a user's
// config away from shipping escape bytes into every pipe and, worse,
// mismeasuring the tabwriter's column widths on the way out.
//
// Byte-level output cannot see that difference, so this asserts the
// wiring that creates it: computeTaskCells is the single place table
// cells are built, and it must never call the styling formatter.
func TestTableCellsNeverStyleStatus(t *testing.T) {
	// runtime.Caller, not a relative path: TestMain chdirs the suite into
	// a temp directory, so "formatter.go" resolves to nothing there.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed; cannot locate package source")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "formatter.go"))
	if err != nil {
		t.Fatalf("read formatter.go: %v", err)
	}
	body := funcBody(t, string(src), "func computeTaskCells(")

	if !strings.Contains(body, "formatStatusPlain(") {
		t.Errorf("computeTaskCells must derive the status cell via formatStatusPlain:\n%s", body)
	}
	// The plain call contains "formatStatus" as a prefix, so the styled
	// call has to be matched as a call that is NOT the plain one.
	if styledCalls(body) > 0 {
		t.Errorf("computeTaskCells must not call formatStatus: kit's non-TTY tabwriter "+
			"passes cell values through verbatim, so a styled cell leaks escapes into "+
			"piped output.\n%s", body)
	}
}

// styledCalls counts calls to formatStatus( that are not
// formatStatusPlain(.
func styledCalls(body string) int {
	n := 0
	for i := 0; ; {
		j := strings.Index(body[i:], "formatStatus")
		if j < 0 {
			return n
		}
		i += j + len("formatStatus")
		if strings.HasPrefix(body[i:], "(") {
			n++
		}
	}
}

// funcBody returns the source text of the function whose declaration
// starts with decl, from the declaration to the first line that is a bare
// closing brace.
func funcBody(t *testing.T, src, decl string) string {
	t.Helper()
	i := strings.Index(src, decl)
	if i < 0 {
		t.Fatalf("declaration %q not found", decl)
	}
	rest := src[i:]
	if j := strings.Index(rest, "\n}\n"); j >= 0 {
		return rest[:j]
	}
	return rest
}

// TestGroupHeadingPlainForNonTTY pins the heading half at its source. A
// bytes.Buffer is not a terminal, so the heading must come back bare —
// titleStyle is bold, and lipgloss emits the bold escape even when it
// resolves no color, which is how grouped output leaked before the gate.
func TestGroupHeadingPlainForNonTTY(t *testing.T) {
	var buf bytes.Buffer
	if got := groupHeading(&buf, "myorg/alpha"); got != "myorg/alpha" {
		t.Errorf("group heading on a non-TTY writer must be plain, got %q", got)
	}
	// And the style it would have applied really does emit escapes, so
	// the assertion above is testing the gate rather than a no-op style.
	if styled := titleStyle.Render("myorg/alpha"); !strings.ContainsRune(styled, '\x1b') {
		t.Skip("titleStyle emits no escapes in this environment; gate is untestable here")
	}
}

// assertNoANSI fails when s carries an escape sequence introducer.
func assertNoANSI(t *testing.T, s, what string) {
	t.Helper()
	if strings.ContainsRune(s, '\x1b') {
		t.Errorf("%s leaked an ANSI escape into non-TTY output: %q", what, s)
	}
}

// TestWorkspaceTableHonorsCols pins that --cols reaches the workspace
// arm. Before the collapse the flag parsed, validated, and was then
// discarded by the private renderer — the listing came back with its
// five fixed columns and exit code 0.
func TestWorkspaceTableHonorsCols(t *testing.T) {
	viper.Set("cols", []string{"id", "title"})
	t.Cleanup(func() { viper.Set("cols", nil) })

	cmd := &cobra.Command{Use: "list"}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := formatWorkspaceTasks(cmd, workspaceTasks(), formatTable); err != nil {
		t.Fatalf("formatWorkspaceTasks: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, "ID") || !strings.Contains(got, "Title") {
		t.Fatalf("--cols id,title dropped a requested column:\n%s", got)
	}
	// An explicit --cols is the user naming the whole set. The workspace
	// Project column is a default, not a floor: naming columns without it
	// must not re-add it, or --cols means something different here than
	// it does on every other listing.
	if strings.Contains(got, "Assigned") {
		t.Errorf("--cols id,title should have dropped Assigned:\n%s", got)
	}
	if strings.Contains(got, "Project") {
		t.Errorf("explicit --cols without project should not re-inject Project:\n%s", got)
	}
}

// TestWorkspaceTableColsCanRequestProject is the other half: "project"
// has to be a real key in the task column registry, not a column the
// workspace path bolts on outside it. A user who wants it alongside a
// custom set must be able to name it.
func TestWorkspaceTableColsCanRequestProject(t *testing.T) {
	viper.Set("cols", []string{"project", "id", "title"})
	t.Cleanup(func() { viper.Set("cols", nil) })

	cmd := &cobra.Command{Use: "list"}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := formatWorkspaceTasks(cmd, workspaceTasks(), formatTable); err != nil {
		t.Fatalf("formatWorkspaceTasks: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, "Project") {
		t.Errorf("--cols project,id,title should render Project:\n%s", got)
	}
	if strings.Contains(got, "warning: unknown column") {
		t.Errorf("%q must be a known task column key:\n%s", "project", got)
	}
}

// TestWorkspaceTableHonorsGroupBy pins the grouping the workspace path
// could not reach. Sections are titled and each renders its own header
// row, per renderGroupedTables.
func TestWorkspaceTableHonorsGroupBy(t *testing.T) {
	got := renderWorkspace(t, workspaceTasks(), withGroupBy("assignee"))

	for _, section := range []string{"ana", "bo", noneGroupName} {
		if !strings.Contains(got, section) {
			t.Errorf("--group-by assignee missing section %q:\n%s", section, got)
		}
	}
	// One header row per section is what distinguishes a grouped listing
	// from a flat one that happens to mention the names in its cells.
	if n := strings.Count(got, "Title"); n < 3 {
		t.Errorf("expected one header row per group (>=3), got %d:\n%s", n, got)
	}
}

// TestWorkspaceTableGroupByProject is the dimension that actually means
// something in a cross-project listing, and the one that interacts with
// the injected Project column: the heading names the group, the column
// still labels every row.
func TestWorkspaceTableGroupByProject(t *testing.T) {
	got := renderWorkspace(t, workspaceTasks(), withGroupBy("project"))

	// Headings carry the stored project id, which is what groupTasks
	// buckets on — the column's short label is a display choice and does
	// not change what the group IS.
	for _, heading := range []string{"myorg/alpha", "myorg/beta"} {
		if !strings.Contains(got, heading) {
			t.Errorf("--group-by project missing heading %q:\n%s", heading, got)
		}
	}
	if !strings.Contains(got, "Project") {
		t.Errorf("--group-by project must keep the Project column:\n%s", got)
	}
	if n := strings.Count(got, "Title"); n < 2 {
		t.Errorf("expected one header row per project group (>=2), got %d:\n%s", n, got)
	}
}

// TestWorkspaceTableHonorsGroupLimit pins that the per-group row cap
// threads through too, heading annotation included.
func TestWorkspaceTableHonorsGroupLimit(t *testing.T) {
	got := renderWorkspace(t, workspaceTasks(), withGroupBy("project"), withGroupLimit(1))

	if !strings.Contains(got, "(1 of 2)") {
		t.Errorf("--group-limit 1 should annotate a capped group heading:\n%s", got)
	}
}

// TestWorkspaceStructuredArmsStillDelegate guards the arms that were
// ALREADY shared. The collapse touched only the table arm; if a refactor
// rewires json/yaml/tls it has changed a contract nobody asked it to.
func TestWorkspaceStructuredArmsStillDelegate(t *testing.T) {
	tasks := workspaceTasks()

	for _, format := range []string{formatJSON, formatYAML} {
		cmd, buf := workspaceRenderCmd(t)
		if err := formatWorkspaceTasks(cmd, tasks, format); err != nil {
			t.Fatalf("formatWorkspaceTasks(%s): %v", format, err)
		}
		ws := buf.String()

		cmd2, buf2 := workspaceRenderCmd(t)
		if err := formatTasks(cmd2, tasks, format, false); err != nil {
			t.Fatalf("formatTasks(%s): %v", format, err)
		}
		if ws != buf2.String() {
			t.Errorf("%s arm diverged from the shared formatter:\nworkspace:\n%s\nshared:\n%s",
				format, ws, buf2.String())
		}
	}
}

// TestWorkspaceGroupedStructuredNests pins that --group-by reaches the
// structured arms of the workspace path as well, so a script consuming
// the cross-project listing reads the same nested shape a script
// consuming a single-project listing does.
func TestWorkspaceGroupedStructuredNests(t *testing.T) {
	got := renderWorkspace2(t, workspaceTasks(), formatJSON, withGroupBy("project"))

	if !strings.Contains(got, `"groups"`) {
		t.Errorf("grouped workspace JSON should nest under \"groups\":\n%s", got)
	}
}

// renderWorkspace2 is renderWorkspace with an explicit format.
func renderWorkspace2(t *testing.T, tasks []*core.Task, format string, opts ...listOption) string {
	t.Helper()
	cmd, buf := workspaceRenderCmd(t)
	if err := formatWorkspaceTasks(cmd, tasks, format, opts...); err != nil {
		t.Fatalf("formatWorkspaceTasks(%s): %v", format, err)
	}
	return buf.String()
}
