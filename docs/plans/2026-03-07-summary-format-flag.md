# `--summary` Output Format Flag

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement
> this plan task-by-task.

**Goal:** Add `--summary` as a global output modifier that groups any task list
by project + status, showing counts and assignee breakdown — no LLM needed.

**Architecture:** New format constant `"summary"` handled in `formatTasks()`.
Aggregation logic lives in a new `renderSummary()` function in `formatter.go`.
The flag is a bool alias that sets `output.format = "summary"`, same pattern as
`--all-projects` or `--mine`.

**Tech Stack:** Go, Charmbracelet lipgloss (styling), cobra/viper (flags)

---

### Task 1: Add `formatSummary` constant and `renderSummary` function

**Files:**
- Modify: `internal/cli/log.go:17-22` (add constant)
- Modify: `internal/cli/formatter.go:34-50` (add case to switch)
- Create: `internal/cli/summary.go` (aggregation + rendering)
- Test: `internal/cli/summary_test.go`

**Step 1: Write the failing test**

Create `internal/cli/summary_test.go`:

```go
package cli

import (
    "bytes"
    "testing"

    "hop.top/tlc/internal/core"
)

func TestRenderSummary(t *testing.T) {
    p1 := "proj/alpha"
    p2 := "proj/beta"
    a1 := "alice"
    a2 := "bob"

    tasks := []*core.Task{
        {ID: "T-0001", Status: core.StatusTodo, ProjectID: &p1},
        {ID: "T-0002", Status: core.StatusTodo, ProjectID: &p1, AssignedTo: &a1},
        {ID: "T-0003", Status: core.StatusInProgress, ProjectID: &p1, AssignedTo: &a1},
        {ID: "T-0004", Status: core.StatusDone, ProjectID: &p1, AssignedTo: &a2},
        {ID: "T-0005", Status: core.StatusTodo, ProjectID: &p2},
        {ID: "T-0006", Status: core.StatusInProgress, ProjectID: &p2, AssignedTo: &a2},
    }

    var buf bytes.Buffer
    renderSummary(&buf, tasks)
    out := buf.String()

    // Must contain project headers
    if !containsAll(out, "proj/alpha", "proj/beta") {
        t.Errorf("missing project headers in:\n%s", out)
    }
    // Must contain status counts
    if !containsAll(out, "To Do", "In Progress", "Done") {
        t.Errorf("missing status labels in:\n%s", out)
    }
    // Must contain assignees
    if !containsAll(out, "alice", "bob") {
        t.Errorf("missing assignees in:\n%s", out)
    }
}

func TestRenderSummaryNilProject(t *testing.T) {
    tasks := []*core.Task{
        {ID: "T-0001", Status: core.StatusTodo},
        {ID: "T-0002", Status: core.StatusInProgress},
    }

    var buf bytes.Buffer
    renderSummary(&buf, tasks)
    out := buf.String()

    // Tasks with nil ProjectID should group under a fallback label
    if !containsAll(out, "(no project)") {
        t.Errorf("expected fallback project label in:\n%s", out)
    }
}

func TestRenderSummaryEmpty(t *testing.T) {
    var buf bytes.Buffer
    renderSummary(&buf, []*core.Task{})
    out := buf.String()
    if out == "" {
        t.Error("expected some output even for empty list")
    }
}

func containsAll(s string, subs ...string) bool {
    for _, sub := range subs {
        if !contains(s, sub) {
            return false
        }
    }
    return true
}

func contains(s, sub string) bool {
    return len(s) >= len(sub) && (s == sub ||
        len(s) > 0 && containsString(s, sub))
}

func containsString(s, sub string) bool {
    return bytes.Contains([]byte(s), []byte(sub))
}
```

**Step 2: Run test to verify it fails**

Run: `just test internal/cli/summary_test.go` or
`cd /path/to/tlc && go test ./internal/cli/ -run TestRenderSummary -v`
Expected: FAIL — `renderSummary` undefined

**Step 3: Add format constant**

In `internal/cli/log.go:17-22`, add `formatSummary`:

```go
const (
    formatJSON    = "json"
    formatYAML    = "yaml"
    formatTable   = "table"
    formatSummary = "summary"
    sortDesc      = "desc"
)
```

**Step 4: Create `internal/cli/summary.go`**

```go
package cli

import (
    "fmt"
    "io"
    "sort"
    "strings"

    "github.com/charmbracelet/lipgloss"
    "hop.top/tlc/internal/core"
)

type projectSummary struct {
    name     string
    total    int
    byStatus map[core.TaskStatus]int
    byOwner  map[string]int
}

func renderSummary(w io.Writer, tasks []*core.Task) {
    if len(tasks) == 0 {
        fmt.Fprintln(w, "No tasks found.")
        return
    }

    grouped := groupByProject(tasks)
    keys := sortedKeys(grouped)

    totals := map[core.TaskStatus]int{}

    for _, key := range keys {
        ps := grouped[key]
        header := lipgloss.NewStyle().Bold(true).
            Foreground(primaryColor).Render(ps.name)
        fmt.Fprintf(w, "\n%s (%d)\n", header, ps.total)

        statuses := []core.TaskStatus{
            core.StatusTodo,
            core.StatusInProgress,
            core.StatusDone,
            core.StatusSkipped,
        }
        for _, st := range statuses {
            count := ps.byStatus[st]
            if count == 0 {
                continue
            }
            totals[st] += count
            fmt.Fprintf(w, "  %s %d\n", formatStatus(st), count)
        }

        if len(ps.byOwner) > 0 {
            owners := sortedStringKeys(ps.byOwner)
            parts := make([]string, 0, len(owners))
            for _, o := range owners {
                parts = append(parts, fmt.Sprintf("%s:%d", o, ps.byOwner[o]))
            }
            label := lipgloss.NewStyle().
                Foreground(mutedColor).Render("  assignees: ")
            fmt.Fprintf(w, "%s%s\n", label, strings.Join(parts, "  "))
        }
    }

    // Grand totals
    fmt.Fprintf(w, "\n%s\n",
        lipgloss.NewStyle().Bold(true).Render("Total"))
    fmt.Fprintf(w, "  %d tasks across %d projects\n",
        len(tasks), len(keys))
    statuses := []core.TaskStatus{
        core.StatusTodo,
        core.StatusInProgress,
        core.StatusDone,
        core.StatusSkipped,
    }
    for _, st := range statuses {
        if totals[st] > 0 {
            fmt.Fprintf(w, "  %s %d\n", formatStatus(st), totals[st])
        }
    }
}

func groupByProject(tasks []*core.Task) map[string]*projectSummary {
    grouped := map[string]*projectSummary{}
    for _, t := range tasks {
        key := "(no project)"
        if t.ProjectID != nil && *t.ProjectID != "" {
            key = *t.ProjectID
        }
        ps, ok := grouped[key]
        if !ok {
            ps = &projectSummary{
                name:     key,
                byStatus: map[core.TaskStatus]int{},
                byOwner:  map[string]int{},
            }
            grouped[key] = ps
        }
        ps.total++
        ps.byStatus[t.Status]++
        if t.AssignedTo != nil && *t.AssignedTo != "" {
            ps.byOwner[*t.AssignedTo]++
        }
    }
    return grouped
}

func sortedKeys(m map[string]*projectSummary) []string {
    keys := make([]string, 0, len(m))
    for k := range m {
        keys = append(keys, k)
    }
    sort.Strings(keys)
    return keys
}

func sortedStringKeys(m map[string]int) []string {
    keys := make([]string, 0, len(m))
    for k := range m {
        keys = append(keys, k)
    }
    sort.Strings(keys)
    return keys
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/cli/ -run TestRenderSummary -v`
Expected: PASS

**Step 6: Commit**

```
feat(cli): add renderSummary for grouped task output
```

---

### Task 2: Wire `--summary` flag into CLI

**Files:**
- Modify: `internal/cli/formatter.go:34-50` (add case)
- Modify: `internal/cli/task.go` (add flag + registration)
- Test: `internal/cli/task_test.go`

**Step 1: Write the failing test**

Add to `internal/cli/task_test.go`:

```go
func TestTaskListSummaryFormat(t *testing.T) {
    defer resetTestDB(t)()

    // Seed tasks
    s, _ := getStorageRaw()
    ctx := context.Background()
    p := "test/proj"
    for i := 0; i < 3; i++ {
        task := &core.Task{
            ID:        fmt.Sprintf("T-%04d", i+1),
            Title:     fmt.Sprintf("Task %d", i+1),
            Status:    core.StatusTodo,
            ProjectID: &p,
            CreatedAt: time.Now(),
            UpdatedAt: time.Now(),
        }
        _ = s.CreateTask(ctx, task)
    }
    s.Close()

    cmd := newTestCmd()
    cmd.AddCommand(taskCmd)
    buf := new(bytes.Buffer)
    cmd.SetOut(buf)
    cmd.SetErr(buf)
    cmd.SetArgs([]string{"task", "list", "--summary"})
    viper.Set("output.format", "")

    if err := cmd.Execute(); err != nil {
        t.Fatalf("task list --summary failed: %v", err)
    }

    out := buf.String()
    if !bytes.Contains([]byte(out), []byte("test/proj")) {
        t.Errorf("expected project name in summary output:\n%s", out)
    }
    if !bytes.Contains([]byte(out), []byte("Total")) {
        t.Errorf("expected Total section in summary output:\n%s", out)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestTaskListSummaryFormat -v`
Expected: FAIL — unknown flag `--summary`

**Step 3: Add `--summary` flag to task list**

In `internal/cli/task.go`, add variable near other list flag vars:

```go
var taskListSummary bool
```

In the `taskListCmd.RunE`, before `formatTasks` call, add override:

```go
if taskListSummary {
    viper.Set("output.format", formatSummary)
}
```

Register the flag (near other `taskListCmd.Flags()` calls):

```go
taskListCmd.Flags().BoolVar(
    &taskListSummary, "summary", false,
    "Show grouped summary by project and status",
)
```

**Step 4: Add case to `formatTasks` switch**

In `internal/cli/formatter.go:34-50`:

```go
case formatSummary:
    renderSummary(out, tasks)
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/cli/ -run TestTaskListSummaryFormat -v`
Expected: PASS

**Step 6: Run full test suite**

Run: `go test ./internal/cli/ -v`
Expected: All pass

**Step 7: Commit**

```
feat(cli): wire --summary flag to task list command
```

---

### Task 3: Support `-f summary` as format value

**Files:**
- Test: `internal/cli/task_test.go`

**Step 1: Write the failing test**

```go
func TestTaskListFormatSummary(t *testing.T) {
    defer resetTestDB(t)()

    s, _ := getStorageRaw()
    ctx := context.Background()
    p := "test/proj"
    task := &core.Task{
        ID: "T-0001", Title: "Task 1", Status: core.StatusTodo,
        ProjectID: &p, CreatedAt: time.Now(), UpdatedAt: time.Now(),
    }
    _ = s.CreateTask(ctx, task)
    s.Close()

    cmd := newTestCmd()
    cmd.AddCommand(taskCmd)
    buf := new(bytes.Buffer)
    cmd.SetOut(buf)
    cmd.SetErr(buf)
    cmd.SetArgs([]string{"task", "list", "-f", "summary"})

    if err := cmd.Execute(); err != nil {
        t.Fatalf("task list -f summary failed: %v", err)
    }

    out := buf.String()
    if !bytes.Contains([]byte(out), []byte("Total")) {
        t.Errorf("expected summary output with -f summary:\n%s", out)
    }
}
```

**Step 2: Run test — should already pass**

Since `formatSummary = "summary"` is already a case in the `formatTasks`
switch (added in Task 2), `-f summary` should work automatically. This test
just confirms it.

Run: `go test ./internal/cli/ -run TestTaskListFormatSummary -v`
Expected: PASS

**Step 3: Commit**

```
test(cli): verify -f summary works as format value
```

---

### Task 4: Update help text and `tlc help llm`

**Files:**
- Modify: `internal/cli/root.go:45` (update format flag help)
- Modify: wherever `tlc help llm` template lives (if present)

**Step 1: Update format flag description**

In `root.go:45`:
```go
rootCmd.PersistentFlags().StringP(
    "format", "f", "",
    "output format (table, json, yaml, tls, summary)",
)
```

**Step 2: Update `tlc help llm` if applicable**

Search for `llm` subcommand and update its output to document `--summary`.

**Step 3: Commit**

```
docs(cli): document summary format in help text
```

---

**Plan complete and saved to
`docs/plans/2026-03-07-summary-format-flag.md`. Two execution options:**

**1. Subagent-Driven (this session)** — I dispatch fresh subagent per task,
review between tasks, fast iteration

**2. Parallel Session (separate)** — Open new session with executing-plans,
batch execution with checkpoints

Which approach?
