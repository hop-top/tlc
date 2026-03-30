# Batch Task Operations Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Allow `claim`, `unclaim`, `assign`, `unassign`, `complete`, `reopen`, `update`, and `delete` to accept multiple task IDs and/or a regex pattern, with a rich confirmation prompt when a pattern matches multiple tasks.

**Architecture:** A new `resolveTaskIDs()` helper resolves args (exact IDs, regex patterns, or `*` glob) into `[]*uri.ResolvedTask` against the already-filtered `ListTasks` set. Each affected command loops over the resolved slice. A `confirmBatch()` helper uses `huh.NewConfirm()` for rich TTY prompts with plain-text fallback. `--no-prompt` skips confirmation and is added as a persistent flag on `TaskCmd`.

**Tech Stack:** Go, `github.com/charmbracelet/huh`, `github.com/charmbracelet/lipgloss`, existing `uri.Resolver`, `storage.SQLiteStorage`

---

### Task 1: Add `--no-prompt` persistent flag to TaskCmd

**Files:**
- Modify: `internal/cli/task.go`
- Modify: `internal/cli/test_helpers.go` (add `taskNoPrompt` to `resetTaskFlags`)

**Step 1: Write the failing test**

In `internal/cli/task_list_test.go`, add at the end of `TestTaskList`:

```go
t.Run("NoPromptFlagExists", func(t *testing.T) {
    _, cleanup := setupTestDir(t)
    defer cleanup()
    cmd := newTestCmd()
    cmd.AddCommand(TaskCmd)
    f := TaskCmd.PersistentFlags().Lookup("no-prompt")
    if f == nil {
        t.Error("expected --no-prompt persistent flag on TaskCmd")
    }
})
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/cli/ -run TestTaskList/NoPromptFlagExists -v
```
Expected: FAIL — `expected --no-prompt persistent flag on TaskCmd`

**Step 3: Add the flag var and register it**

In `internal/cli/task.go`, add to the `var (...)` block:

```go
taskNoPrompt bool
```

At the bottom of `func init()` in `task.go`:

```go
TaskCmd.PersistentFlags().BoolVar(&taskNoPrompt, "no-prompt", false, "Skip confirmation prompts")
```

**Step 4: Add to resetTaskFlags**

In `internal/cli/test_helpers.go`, in `resetTaskFlags()`, add:

```go
taskNoPrompt = false
```

Also add to the cobra flag reset loop in `resetTaskFlags`:
`TaskCmd` is already iterated via its subcommands — but `TaskCmd` itself needs its persistent flags reset. After the existing loop, add:

```go
TaskCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
    f.Changed = false
})
```

**Step 5: Run test to verify it passes**

```bash
go test ./internal/cli/ -run TestTaskList/NoPromptFlagExists -v
```
Expected: PASS

**Step 6: Run full suite**

```bash
go test ./internal/cli/
```
Expected: all pass

**Step 7: Commit**

```bash
git add internal/cli/task.go internal/cli/test_helpers.go internal/cli/task_list_test.go
git commit -m "feat(cli): add --no-prompt persistent flag to TaskCmd"
```

---

### Task 2: Create `task_resolve.go` with `resolveTaskIDs` and `confirmBatch`

**Files:**
- Create: `internal/cli/task_resolve.go`
- Create: `internal/cli/task_resolve_test.go`

**Context:**
- `resolveTaskIDs(ctx, args, s, query)` returns `([]*uri.ResolvedTask, bool, error)` — the bool is `requiresConfirmation` (true when a pattern was used and matched >1 task).
- Input detection: if any arg contains regex metacharacters (`[`, `(`, `*`, `?`, `+`, `\`, `.`, `^`, `$`, `{`, `|`) it is treated as a pattern. `*` alone is rewritten to `.*`.
- Pattern resolution: calls `s.ListTasks(ctx, query)` once, then filters IDs with `regexp.MatchString`.
- Exact IDs: resolved individually via `uri.NewResolver(s).ResolveTask()` — no confirmation needed.
- Mixed args (some exact, some patterns): not supported; return an error if both are present.
- `confirmBatch(cmd, ids []string, pattern string) error` renders a `huh.NewConfirm()` prompt listing matched IDs and returns `huh.ErrUserAborted` equivalent on rejection.

**Step 1: Write the failing tests**

Create `internal/cli/task_resolve_test.go`:

```go
package cli

import (
    "context"
    "testing"

    "hop.top/tlc/internal/core"
)

func TestResolveTaskIDs_ExactSingle(t *testing.T) {
    _, cleanup := setupTestDir(t)
    defer cleanup()
    s, _ := getStorageRaw()
    defer s.Close()
    ctx := context.Background()
    s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})

    resolved, confirm, err := resolveTaskIDs(ctx, []string{"T-0001"}, s, core.Query{})
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(resolved) != 1 || resolved[0].Task.ID != "T-0001" {
        t.Errorf("expected T-0001, got %v", resolved)
    }
    if confirm {
        t.Error("exact ID should not require confirmation")
    }
}

func TestResolveTaskIDs_ExactMultiple(t *testing.T) {
    _, cleanup := setupTestDir(t)
    defer cleanup()
    s, _ := getStorageRaw()
    defer s.Close()
    ctx := context.Background()
    s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})
    s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "B", Status: core.StatusTodo})

    resolved, confirm, err := resolveTaskIDs(ctx, []string{"T-0001", "T-0002"}, s, core.Query{})
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(resolved) != 2 {
        t.Errorf("expected 2 tasks, got %d", len(resolved))
    }
    if confirm {
        t.Error("exact IDs should not require confirmation")
    }
}

func TestResolveTaskIDs_RegexPattern(t *testing.T) {
    _, cleanup := setupTestDir(t)
    defer cleanup()
    s, _ := getStorageRaw()
    defer s.Close()
    ctx := context.Background()
    s.CreateTask(ctx, &core.Task{ID: "T-0010", Title: "A", Status: core.StatusTodo})
    s.CreateTask(ctx, &core.Task{ID: "T-0011", Title: "B", Status: core.StatusTodo})
    s.CreateTask(ctx, &core.Task{ID: "T-0020", Title: "C", Status: core.StatusTodo})

    resolved, confirm, err := resolveTaskIDs(ctx, []string{"T-001[01]"}, s, core.Query{})
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(resolved) != 2 {
        t.Errorf("expected 2 tasks, got %d", len(resolved))
    }
    if !confirm {
        t.Error("regex matching >1 task should require confirmation")
    }
}

func TestResolveTaskIDs_GlobStar(t *testing.T) {
    _, cleanup := setupTestDir(t)
    defer cleanup()
    s, _ := getStorageRaw()
    defer s.Close()
    ctx := context.Background()
    s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})
    s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "B", Status: core.StatusTodo})

    resolved, confirm, err := resolveTaskIDs(ctx, []string{"*"}, s, core.Query{})
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(resolved) != 2 {
        t.Errorf("expected 2 tasks, got %d", len(resolved))
    }
    if !confirm {
        t.Error("glob * matching >1 task should require confirmation")
    }
}

func TestResolveTaskIDs_RegexNoMatch(t *testing.T) {
    _, cleanup := setupTestDir(t)
    defer cleanup()
    s, _ := getStorageRaw()
    defer s.Close()
    ctx := context.Background()
    s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})

    _, _, err := resolveTaskIDs(ctx, []string{"T-999[0-9]"}, s, core.Query{})
    if err == nil {
        t.Error("expected error for pattern matching no tasks")
    }
}

func TestResolveTaskIDs_RegexSingleMatchNoConfirm(t *testing.T) {
    _, cleanup := setupTestDir(t)
    defer cleanup()
    s, _ := getStorageRaw()
    defer s.Close()
    ctx := context.Background()
    s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})
    s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "B", Status: core.StatusTodo})

    resolved, confirm, err := resolveTaskIDs(ctx, []string{"T-0001"}, s, core.Query{})
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(resolved) != 1 {
        t.Errorf("expected 1 task, got %d", len(resolved))
    }
    // Exact ID — no confirmation even though it looks like it could be a pattern
    if confirm {
        t.Error("single exact match should not require confirmation")
    }
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/cli/ -run TestResolveTaskIDs -v
```
Expected: FAIL — `undefined: resolveTaskIDs`

**Step 3: Implement `task_resolve.go`**

Create `internal/cli/task_resolve.go`:

```go
package cli

import (
    "context"
    "fmt"
    "regexp"
    "strings"

    "github.com/charmbracelet/huh"
    "github.com/spf13/cobra"
    "hop.top/tlc/internal/core"
    "hop.top/tlc/internal/storage"
    "hop.top/tlc/internal/uri"
)

// patternChars are regex metacharacters that indicate an arg is a pattern.
const patternChars = `[(*?+\.^${}|`

func isPattern(s string) bool {
    return strings.ContainsAny(s, patternChars)
}

// resolveTaskIDs resolves a slice of args (exact IDs or a single regex pattern)
// into ResolvedTask slice. Returns requiresConfirmation=true when a pattern
// matched more than one task.
//
// Rules:
//   - All args exact IDs → resolve each individually, no confirmation.
//   - Single arg is a pattern → list tasks via query, filter by regex.
//     requiresConfirmation=true if >1 matched.
//   - "*" is rewritten to ".*" before regex matching.
//   - Mixed exact + pattern args → error.
func resolveTaskIDs(ctx context.Context, args []string, s *storage.SQLiteStorage, query core.Query) ([]*uri.ResolvedTask, bool, error) {
    hasPattern := false
    hasExact := false
    for _, a := range args {
        if isPattern(a) {
            hasPattern = true
        } else {
            hasExact = true
        }
    }

    if hasPattern && hasExact {
        return nil, false, fmt.Errorf("cannot mix exact IDs and patterns; use one or the other")
    }

    if hasPattern {
        if len(args) > 1 {
            return nil, false, fmt.Errorf("only one pattern argument is supported")
        }
        return resolveByPattern(ctx, args[0], s, query)
    }

    // All exact IDs.
    resolver := uri.NewResolver(s)
    resolved := make([]*uri.ResolvedTask, 0, len(args))
    for _, id := range args {
        res, err := resolver.ResolveTask(ctx, id)
        if err != nil {
            return nil, false, err
        }
        resolved = append(resolved, res)
    }
    return resolved, false, nil
}

func resolveByPattern(ctx context.Context, pattern string, s *storage.SQLiteStorage, query core.Query) ([]*uri.ResolvedTask, bool, error) {
    if pattern == "*" {
        pattern = ".*"
    }
    re, err := regexp.Compile(pattern)
    if err != nil {
        return nil, false, fmt.Errorf("invalid pattern %q: %w", pattern, err)
    }

    // Use a generous limit; patterns operate on the already-filtered set.
    q := query
    if q.Limit == 0 {
        q.Limit = 10000
    }
    tasks, err := s.ListTasks(ctx, q)
    if err != nil {
        return nil, false, fmt.Errorf("failed to list tasks: %w", err)
    }

    var matched []*uri.ResolvedTask
    for _, t := range tasks {
        if re.MatchString(t.ID) {
            matched = append(matched, &uri.ResolvedTask{Task: t, Storage: s})
        }
    }

    if len(matched) == 0 {
        return nil, false, fmt.Errorf("pattern %q matched no tasks", pattern)
    }

    requiresConfirmation := len(matched) > 1
    return matched, requiresConfirmation, nil
}

// confirmBatch shows a rich confirmation prompt listing matched task IDs.
// Returns nil if confirmed, error if rejected or aborted.
// Skipped entirely when --no-prompt is set.
func confirmBatch(cmd *cobra.Command, tasks []*uri.ResolvedTask, pattern string) error {
    if taskNoPrompt {
        return nil
    }

    ids := make([]string, len(tasks))
    for i, t := range tasks {
        ids[i] = t.Task.ID
    }

    msg := fmt.Sprintf("%d tasks matched %q: %s\nProceed?",
        len(tasks), pattern, strings.Join(ids, ", "))

    var confirmed bool
    err := huh.NewForm(
        huh.NewGroup(
            huh.NewConfirm().
                Title(msg).
                Value(&confirmed),
        ),
    ).WithOutput(cmd.OutOrStdout()).Run()

    if err != nil {
        return err
    }
    if !confirmed {
        return fmt.Errorf("aborted")
    }
    return nil
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/cli/ -run TestResolveTaskIDs -v
```
Expected: all PASS

**Step 5: Run full suite**

```bash
go test ./internal/cli/
```
Expected: all pass

**Step 6: Commit**

```bash
git add internal/cli/task_resolve.go internal/cli/task_resolve_test.go
git commit -m "feat(cli): add resolveTaskIDs helper with regex/glob support and confirmBatch"
```

---

### Task 3: Update `claim`, `unclaim`, `complete`, `reopen`, `unassign` for batch

**Files:**
- Modify: `internal/cli/task_lifecycle.go`
- Modify: `internal/cli/task_lifecycle_test.go`

**Context:** Each command changes from `cobra.ExactArgs(1)` to `cobra.MinimumNArgs(1)`, replaces `args[0]` + single `ResolveTask` with `resolveTaskIDs`, and wraps the operation body in a loop. Errors are collected; all tasks are attempted before returning any error.

For commands that check `args[0]` before resolving (e.g. `unassign` checks note before using id, `reopen` checks note), move the note check to before the resolve call.

**Step 1: Write the failing tests**

Add to `internal/cli/task_lifecycle_test.go`:

```go
func TestTaskClaim_MultipleIDs(t *testing.T) {
    ctx, cleanup := setupTestDir(t)
    defer cleanup()
    s, _ := getStorageRaw()
    defer s.Close()
    s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})
    s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "B", Status: core.StatusTodo})

    cmd := newTestCmd()
    cmd.AddCommand(TaskCmd)
    buf := new(bytes.Buffer)
    cmd.SetOut(buf)
    cmd.SetArgs([]string{"task", "claim", "T-0001", "T-0002"})
    if err := cmd.Execute(); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    for _, id := range []string{"T-0001", "T-0002"} {
        task, _ := s.GetTask(ctx, id)
        if task.Status != core.StatusInProgress {
            t.Errorf("expected %s IN_PROGRESS, got %s", id, task.Status)
        }
    }
}

func TestTaskComplete_RegexPattern(t *testing.T) {
    ctx, cleanup := setupTestDir(t)
    defer cleanup()
    s, _ := getStorageRaw()
    defer s.Close()
    user := "testuser"
    s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusInProgress, AssignedTo: &user})
    s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "B", Status: core.StatusInProgress, AssignedTo: &user})
    s.CreateTask(ctx, &core.Task{ID: "T-0010", Title: "C", Status: core.StatusInProgress, AssignedTo: &user})

    cmd := newTestCmd()
    cmd.AddCommand(TaskCmd)
    buf := new(bytes.Buffer)
    cmd.SetOut(buf)
    cmd.SetArgs([]string{"task", "complete", "T-000[12]", "--no-prompt"})
    if err := cmd.Execute(); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    for _, id := range []string{"T-0001", "T-0002"} {
        task, _ := s.GetTask(ctx, id)
        if task.Status != core.StatusDone {
            t.Errorf("expected %s DONE, got %s", id, task.Status)
        }
    }
    // T-0010 should be untouched
    task, _ := s.GetTask(ctx, "T-0010")
    if task.Status != core.StatusInProgress {
        t.Errorf("expected T-0010 still IN_PROGRESS, got %s", task.Status)
    }
}

func TestTaskReopen_MultipleIDs(t *testing.T) {
    ctx, cleanup := setupTestDir(t)
    defer cleanup()
    s, _ := getStorageRaw()
    defer s.Close()
    s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusDone})
    s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "B", Status: core.StatusDone})

    cmd := newTestCmd()
    cmd.AddCommand(TaskCmd)
    buf := new(bytes.Buffer)
    cmd.SetOut(buf)
    cmd.SetArgs([]string{"task", "reopen", "T-0001", "T-0002", "--note", "retry"})
    if err := cmd.Execute(); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    for _, id := range []string{"T-0001", "T-0002"} {
        task, _ := s.GetTask(ctx, id)
        if task.Status != core.StatusTodo {
            t.Errorf("expected %s TODO, got %s", id, task.Status)
        }
    }
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/cli/ -run "TestTaskClaim_MultipleIDs|TestTaskComplete_RegexPattern|TestTaskReopen_MultipleIDs" -v
```
Expected: FAIL — args validation rejects multiple IDs

**Step 3: Update lifecycle commands**

For each command in `internal/cli/task_lifecycle.go`:
1. Change `Args: cobra.ExactArgs(1)` → `Args: cobra.MinimumNArgs(1)`
2. Change `Use` to `<task-id|pattern>...` (e.g. `"claim <task-id|pattern>..."`)
3. Replace the body with the pattern below (shown for `claim`, apply same to all):

```go
RunE: func(cmd *cobra.Command, args []string) error {
    s, err := getStorage()
    if err != nil {
        return err
    }
    defer func() { _ = s.Close() }()
    ctx := context.Background()

    resolved, needsConfirm, err := resolveTaskIDs(ctx, args, s, core.Query{})
    if err != nil {
        return err
    }
    if needsConfirm {
        if err := confirmBatch(cmd, resolved, args[0]); err != nil {
            return err
        }
    }

    var errs []string
    for _, res := range resolved {
        task := res.Task
        if res.Storage != s {
            defer func() { _ = res.Storage.Close() }()
        }
        // ... original single-task operation body unchanged ...
        if err := saveTaskWithLog(ctx, cmd, task, log, res.Storage); err != nil {
            errs = append(errs, fmt.Sprintf("%s: %v", task.ID, err))
            continue
        }
        _, _ = fmt.Fprintf(cmd.OutOrStdout(), "Claimed task %s\n", task.ID)
    }
    if len(errs) > 0 {
        return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
    }
    return syncTODOAll()
},
```

For `unassign` and `reopen` (which require `--note`): move the note check above `resolveTaskIDs`. Change the error message to not reference `args[0]` since there may be multiple:

```go
if taskUnassignNote == "" {
    return errNoteRequired("tlc task unassign <task-id|pattern>...")
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/cli/ -run "TestTaskClaim_MultipleIDs|TestTaskComplete_RegexPattern|TestTaskReopen_MultipleIDs" -v
```
Expected: all PASS

**Step 5: Run full suite**

```bash
go test ./internal/cli/
```
Expected: all pass

**Step 6: Commit**

```bash
git add internal/cli/task_lifecycle.go internal/cli/task_lifecycle_test.go
git commit -m "feat(cli): batch claim/unclaim/complete/reopen/unassign via multiple IDs or regex"
```

---

### Task 4: Update `assign` — flip arg order to `<assignee> <task-id|pattern>...`

**Files:**
- Modify: `internal/cli/task_lifecycle.go` (assign command only)
- Modify: `internal/cli/task_lifecycle_test.go`

**Context:** Old signature: `assign <task-id> <assignee>`. New: `assign <assignee> <task-id|pattern>...`. `args[0]` is now the assignee; `args[1:]` are the IDs/pattern.

**Step 1: Write the failing tests**

Add to `internal/cli/task_lifecycle_test.go`:

```go
func TestTaskAssign_NewArgOrder(t *testing.T) {
    ctx, cleanup := setupTestDir(t)
    defer cleanup()
    s, _ := getStorageRaw()
    defer s.Close()
    s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})
    s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "B", Status: core.StatusTodo})

    cmd := newTestCmd()
    cmd.AddCommand(TaskCmd)
    cmd.SetArgs([]string{"task", "assign", "alice", "T-0001", "T-0002"})
    if err := cmd.Execute(); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    for _, id := range []string{"T-0001", "T-0002"} {
        task, _ := s.GetTask(ctx, id)
        if task.AssignedTo == nil || *task.AssignedTo != "alice" {
            t.Errorf("expected %s assigned to alice", id)
        }
    }
}

func TestTaskAssign_RegexPattern(t *testing.T) {
    ctx, cleanup := setupTestDir(t)
    defer cleanup()
    s, _ := getStorageRaw()
    defer s.Close()
    s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})
    s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "B", Status: core.StatusTodo})
    s.CreateTask(ctx, &core.Task{ID: "T-0010", Title: "C", Status: core.StatusTodo})

    cmd := newTestCmd()
    cmd.AddCommand(TaskCmd)
    cmd.SetArgs([]string{"task", "assign", "bob", "T-000[12]", "--no-prompt"})
    if err := cmd.Execute(); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    for _, id := range []string{"T-0001", "T-0002"} {
        task, _ := s.GetTask(ctx, id)
        if task.AssignedTo == nil || *task.AssignedTo != "bob" {
            t.Errorf("expected %s assigned to bob", id)
        }
    }
    task, _ := s.GetTask(ctx, "T-0010")
    if task.AssignedTo != nil {
        t.Errorf("expected T-0010 unassigned")
    }
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/cli/ -run "TestTaskAssign_NewArgOrder|TestTaskAssign_RegexPattern" -v
```
Expected: FAIL

**Step 3: Update assign command**

In `internal/cli/task_lifecycle.go`, update `TaskAssignCmd`:

```go
var TaskAssignCmd = &cobra.Command{
    Use:   "assign <assignee> <task-id|pattern>...",
    Short: "Assign tasks to someone",
    Args:  cobra.MinimumNArgs(2),
    RunE: func(cmd *cobra.Command, args []string) error {
        assignee := args[0]
        taskArgs := args[1:]
        s, err := getStorage()
        if err != nil {
            return err
        }
        defer func() { _ = s.Close() }()
        ctx := context.Background()

        resolved, needsConfirm, err := resolveTaskIDs(ctx, taskArgs, s, core.Query{})
        if err != nil {
            return err
        }
        if needsConfirm {
            if err := confirmBatch(cmd, resolved, taskArgs[0]); err != nil {
                return err
            }
        }

        var errs []string
        for _, res := range resolved {
            task := res.Task
            if res.Storage != s {
                defer func() { _ = res.Storage.Close() }()
            }
            user := core.GetCurrentUser()
            prevAssignee := ""
            if task.AssignedTo != nil {
                prevAssignee = *task.AssignedTo
            }
            task.AssignedTo = &assignee
            task.UpdatedAt = time.Now().UTC()
            details := fmt.Sprintf("(assigned to @%s)", assignee)
            if prevAssignee != "" {
                details = fmt.Sprintf("(reassigned from @%s to @%s)", prevAssignee, assignee)
            }
            appendAuditLog(task, user, "ASSIGNED", details, taskAssignNote, task.UpdatedAt)
            logEntry := &core.LogEntry{
                TaskID:    task.ID,
                Timestamp: task.UpdatedAt,
                By:        user,
                Action:    core.ActionReassigned,
                Note:      taskAssignNote,
            }
            if err := saveTaskWithLog(ctx, cmd, task, logEntry, res.Storage); err != nil {
                errs = append(errs, fmt.Sprintf("%s: %v", task.ID, err))
                continue
            }
            _, _ = fmt.Fprintf(cmd.OutOrStdout(), "Assigned task %s to %s\n", task.ID, assignee)
        }
        if len(errs) > 0 {
            return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
        }
        return syncTODOAll()
    },
}
```

**Step 4: Update existing assign tests**

In `task_lifecycle_test.go`, find any existing `TestTaskAssign` tests using old arg order `assign T-0001 alice` and update to `assign alice T-0001`.

**Step 5: Run tests**

```bash
go test ./internal/cli/ -run "TestTaskAssign" -v
```
Expected: all PASS

**Step 6: Run full suite**

```bash
go test ./internal/cli/
```
Expected: all pass

**Step 7: Commit**

```bash
git add internal/cli/task_lifecycle.go internal/cli/task_lifecycle_test.go
git commit -m "feat(cli): assign takes <assignee> first, supports multiple IDs and regex"
```

---

### Task 5: Update `update` and `delete` for batch

**Files:**
- Modify: `internal/cli/task_update.go`
- Modify: `internal/cli/task_update_test.go`

**Context:** `update` is trickier because it has many flags. Multiple IDs/pattern apply the same flag changes to all matched tasks. `delete` gets the same multi-ID/pattern treatment with a confirmation prompt (even for explicit IDs when >1, since delete is destructive).

For `delete`: always require confirmation when >1 ID given (exact or pattern), unless `--no-prompt` or `-y` is set. Existing `-y` flag already exists — treat `-y` the same as `--no-prompt` for delete.

**Step 1: Write the failing tests**

Add to `internal/cli/task_update_test.go`:

```go
func TestTaskUpdate_MultipleIDs(t *testing.T) {
    ctx, cleanup := setupTestDir(t)
    defer cleanup()
    s, _ := getStorageRaw()
    defer s.Close()
    s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})
    s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "B", Status: core.StatusTodo})

    cmd := newTestCmd()
    cmd.AddCommand(TaskCmd)
    cmd.SetArgs([]string{"task", "update", "T-0001", "T-0002", "--assigned-to", "alice"})
    if err := cmd.Execute(); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    for _, id := range []string{"T-0001", "T-0002"} {
        task, _ := s.GetTask(ctx, id)
        if task.AssignedTo == nil || *task.AssignedTo != "alice" {
            t.Errorf("expected %s assigned to alice", id)
        }
    }
}

func TestTaskDelete_MultipleIDs_WithNoPrompt(t *testing.T) {
    ctx, cleanup := setupTestDir(t)
    defer cleanup()
    s, _ := getStorageRaw()
    defer s.Close()
    s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})
    s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "B", Status: core.StatusTodo})

    cmd := newTestCmd()
    cmd.AddCommand(TaskCmd)
    cmd.SetArgs([]string{"task", "delete", "T-0001", "T-0002", "--no-prompt"})
    if err := cmd.Execute(); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    for _, id := range []string{"T-0001", "T-0002"} {
        task, _ := s.GetTask(ctx, id)
        if task != nil {
            t.Errorf("expected %s to be deleted", id)
        }
    }
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/cli/ -run "TestTaskUpdate_MultipleIDs|TestTaskDelete_MultipleIDs" -v
```
Expected: FAIL

**Step 3: Update `update` command**

In `internal/cli/task_update.go`:
1. Change `Args: cobra.ExactArgs(1)` → `cobra.MinimumNArgs(1)`
2. Change `Use` to `"update <task-id|pattern>..."`
3. Replace single-task body with loop over `resolveTaskIDs` result

For `update`, the `--title` flag only makes sense for a single task. Add a check:
```go
if cmd.Flags().Changed("title") && len(resolved) > 1 {
    return fmt.Errorf("--title can only be set when updating a single task")
}
```

**Step 4: Update `delete` command**

In `internal/cli/task_update.go`:
1. Change `Args: cobra.ExactArgs(1)` → `cobra.MinimumNArgs(1)`
2. Change `Use` to `"delete <task-id|pattern>..."`
3. Use `resolveTaskIDs`; treat `taskDeleteYes` as equivalent to `taskNoPrompt`:

```go
resolved, needsConfirm, err := resolveTaskIDs(ctx, args, s, core.Query{})
// For delete: confirm if >1 regardless of whether it was a pattern
if (needsConfirm || len(resolved) > 1) && !taskDeleteYes {
    if err := confirmBatch(cmd, resolved, strings.Join(args, " ")); err != nil {
        return err
    }
}
```

**Step 5: Run tests**

```bash
go test ./internal/cli/ -run "TestTaskUpdate_MultipleIDs|TestTaskDelete_MultipleIDs" -v
```
Expected: PASS

**Step 6: Run full suite**

```bash
go test ./internal/cli/
```
Expected: all pass

**Step 7: Commit**

```bash
git add internal/cli/task_update.go internal/cli/task_update_test.go
git commit -m "feat(cli): batch update/delete via multiple IDs or regex"
```

---

### Task 6: E2E tests

**Files:**
- Create: `internal/cli/task_batch_e2e_test.go`

**Step 1: Write the e2e tests**

```go
package cli

import (
    "bytes"
    "context"
    "fmt"
    "testing"

    "hop.top/tlc/internal/core"
)

func TestBatchE2E_CompleteAllWithGlob(t *testing.T) {
    ctx, cleanup := setupTestDir(t)
    defer cleanup()

    // Create 4 tasks via CLI
    for i := 1; i <= 4; i++ {
        cmd := newTestCmd()
        cmd.AddCommand(TaskCmd)
        cmd.SetArgs([]string{"task", "create", fmt.Sprintf("Task %d", i), "--status", "IN_PROGRESS"})
        if err := cmd.Execute(); err != nil {
            t.Fatalf("create: %v", err)
        }
    }

    // Complete all with *
    cmd := newTestCmd()
    cmd.AddCommand(TaskCmd)
    buf := new(bytes.Buffer)
    cmd.SetOut(buf)
    cmd.SetArgs([]string{"task", "complete", "*", "--no-prompt", "--status", "IN_PROGRESS"})
    if err := cmd.Execute(); err != nil {
        t.Fatalf("complete *: %v", err)
    }

    // List and verify all DONE
    s, _ := getStorageRaw()
    defer s.Close()
    tasks, _ := s.ListTasks(context.Background(), core.Query{Limit: 100})
    for _, task := range tasks {
        if task.Status != core.StatusDone {
            t.Errorf("expected %s DONE, got %s", task.ID, task.Status)
        }
    }
}

func TestBatchE2E_AssignRegexNoPrompt(t *testing.T) {
    ctx, cleanup := setupTestDir(t)
    defer cleanup()
    s, _ := getStorageRaw()
    defer s.Close()

    for _, id := range []string{"T-0010", "T-0011", "T-0020"} {
        s.CreateTask(ctx, &core.Task{ID: id, Title: id, Status: core.StatusTodo})
    }

    cmd := newTestCmd()
    cmd.AddCommand(TaskCmd)
    cmd.SetArgs([]string{"task", "assign", "carol", `T-001\d`, "--no-prompt"})
    if err := cmd.Execute(); err != nil {
        t.Fatalf("assign: %v", err)
    }

    for _, id := range []string{"T-0010", "T-0011"} {
        task, _ := s.GetTask(ctx, id)
        if task.AssignedTo == nil || *task.AssignedTo != "carol" {
            t.Errorf("expected %s → carol", id)
        }
    }
    task, _ := s.GetTask(ctx, "T-0020")
    if task.AssignedTo != nil {
        t.Errorf("expected T-0020 unassigned")
    }
}

func TestBatchE2E_DeleteRequiresConfirmOrNoPrompt(t *testing.T) {
    ctx, cleanup := setupTestDir(t)
    defer cleanup()
    s, _ := getStorageRaw()
    defer s.Close()
    s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})
    s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "B", Status: core.StatusTodo})

    // Without --no-prompt in non-TTY, huh should still work or return error.
    // In test (non-TTY), confirm defaults to false → aborted.
    cmd := newTestCmd()
    cmd.AddCommand(TaskCmd)
    cmd.SetArgs([]string{"task", "delete", "T-0001", "T-0002"})
    err := cmd.Execute()
    if err == nil {
        t.Error("expected error without --no-prompt in non-TTY batch delete")
    }

    // With --no-prompt it should succeed.
    cmd2 := newTestCmd()
    cmd2.AddCommand(TaskCmd)
    cmd2.SetArgs([]string{"task", "delete", "T-0001", "T-0002", "--no-prompt"})
    if err := cmd2.Execute(); err != nil {
        t.Fatalf("delete with --no-prompt: %v", err)
    }
}
```

**Step 2: Run to verify they fail**

```bash
go test ./internal/cli/ -run TestBatchE2E -v
```
Expected: FAIL (feature not yet complete — run after Tasks 3-5)

**Step 3: Run to verify they pass (after Tasks 3-5)**

```bash
go test ./internal/cli/ -run TestBatchE2E -v
```
Expected: all PASS

**Step 4: Commit**

```bash
git add internal/cli/task_batch_e2e_test.go
git commit -m "test(cli): add batch operations e2e tests"
```

---

### Task 7: Docs — story, spec updates, GETTING-STARTED

**Files:**
- Create: `docs/stories/066-batch-task-operations.md`
- Modify: `docs/task-crud-spec-0.1.md` — add batch operations section
- Modify: `docs/tlc-cli-spec-0.1.md` — update command signatures
- Modify: `docs/GETTING-STARTED.md` — add batch examples
- Modify: `docs/stories/003-task-claiming.md`, `005-task-update.md`, `006-task-deletion.md`, `007-task-reopen.md`, `008-task-assignment.md` — add batch acceptance criteria

**Step 1: Create story 066**

`docs/stories/066-batch-task-operations.md` — document the feature from a user perspective with acceptance criteria covering:
- Multiple exact IDs on any lifecycle command
- Regex pattern matching against current filtered set
- `*` glob as match-all
- Confirmation prompt for regex matching >1 task
- `--no-prompt` skips confirmation
- `assign` takes assignee as first arg
- `delete` requires confirmation or `--no-prompt`/`-y` for >1 task

**Step 2: Update existing stories**

Add to each story's acceptance criteria:
- `003`: `claim` / `unclaim` accept multiple IDs and regex
- `005`: `update` accepts multiple IDs and regex; `--title` blocked for multi-target
- `006`: `delete` accepts multiple IDs; requires confirmation for >1
- `007`: `reopen` accepts multiple IDs and regex
- `008`: `assign` new signature `<assignee> <task-id|pattern>...`; `unassign` accepts multiple IDs and regex

**Step 3: Update specs**

In `docs/task-crud-spec-0.1.md`, add a **Batch Operations** section documenting ID forms, pattern syntax, confirmation behaviour.

In `docs/tlc-cli-spec-0.1.md`, update `Use` signatures for all affected commands.

In `docs/GETTING-STARTED.md`, add examples under a new "Batch Operations" subsection.

**Step 4: Commit**

```bash
git add docs/
git commit -m "docs: add batch task operations story, spec updates, and examples"
```

---

### Task 8: Final verification

**Step 1: Run full test suite**

```bash
go test ./...
```
Expected: all pass

**Step 2: Build and smoke test**

```bash
make build
bin/tlc task complete T-0001 T-0002 --no-prompt
bin/tlc task assign alice T-0001 T-0002
bin/tlc task complete "T-00[12]\d" --no-prompt
```

**Step 3: Commit any fixups**

```bash
git add -p
git commit -m "fix(cli): batch operations follow-up"
```
