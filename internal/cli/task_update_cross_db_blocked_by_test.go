package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// TestTaskUpdate_CrossDBBlockedBy_Label seeds two project DBs (the local
// "current" project plus a sibling kit DB), registers the sibling under
// the canonical id "hop-top/kit" with label "kit", and verifies that
//
//	tlc task update T-0001 --add-blocked-by kit/T-0001
//
// resolves the label "kit", opens the cross-project DB, finds the
// blocking task, and stamps the canonical "hop-top/kit/T-0001" reference
// onto the local task's blocked_by list.
//
// This is the regression test for T-1123: previously the registry
// resolver only matched exact project IDs, so the label form would
// surface "project 'kit' not found in registry"; and even with the full
// id the cross-DB lookup was missing.
func TestTaskUpdate_CrossDBBlockedBy_Label(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	// Local DB (also the registry).
	local, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer func() { _ = local.Close() }()

	// Seed the local task we'll update.
	if err := local.CreateTask(ctx, &core.Task{
		ID:     "T-0001",
		Title:  "local task",
		Status: core.StatusTodo,
	}); err != nil {
		t.Fatalf("CreateTask local: %v", err)
	}

	// Sibling project DB lives in its own tmp dir to prove handles are
	// distinct.
	sibTmp := t.TempDir()
	sibDBPath := filepath.Join(sibTmp, "kit.sqlite")
	sib, err := storage.NewSQLiteStorage(sibDBPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage(sibling): %v", err)
	}
	// Seed the blocking task scoped to the sibling project so the
	// resolver finds it via GetTaskInProject.
	projID := "hop-top/kit"
	blockingID := "T-0001"
	if err := sib.CreateTask(ctx, &core.Task{
		ID:        blockingID,
		Title:     "kit blocker",
		Status:    core.StatusTodo,
		ProjectID: &projID,
	}); err != nil {
		t.Fatalf("CreateTask sibling: %v", err)
	}
	_ = sib.Close()

	// Register the sibling under the canonical id with a friendly label.
	if err := local.RegisterProject(ctx, projID, sibDBPath, "", "kit"); err != nil {
		t.Fatalf("RegisterProject: %v", err)
	}

	// Run `tlc task update T-0001 --add-blocked-by kit/T-0001` — note
	// the *label* form, which the resolver must fuzzy-match to
	// "hop-top/kit".
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{
		"task", "update", "T-0001",
		"--add-blocked-by", "kit/T-0001",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update with label cross-DB ref failed: %v\noutput: %s", err, buf.String())
	}

	// Verify the canonical (full project id + task id) form was
	// persisted.
	updated := getTaskByAlias(t, ctx, "T-0001")
	if updated == nil {
		t.Fatal("task not found after update")
	}
	got := updated.BlockedBy()
	if len(got) != 1 {
		t.Fatalf("blocked_by = %v, want exactly one entry", got)
	}
	want := "hop-top/kit/T-0001"
	if got[0] != want {
		t.Fatalf("blocked_by[0] = %q, want %q", got[0], want)
	}
}

// TestTaskUpdate_CrossDBBlockedBy_FullID also exercises the cross-DB
// lookup but using the full canonical project id, ensuring the resolver
// path that *bypasses* fuzzy matching still opens the sibling database
// and finds the task.
func TestTaskUpdate_CrossDBBlockedBy_FullID(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	local, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer func() { _ = local.Close() }()

	if err := local.CreateTask(ctx, &core.Task{
		ID:     "T-0001",
		Title:  "local task",
		Status: core.StatusTodo,
	}); err != nil {
		t.Fatalf("CreateTask local: %v", err)
	}

	sibTmp := t.TempDir()
	sibDBPath := filepath.Join(sibTmp, "kit.sqlite")
	sib, err := storage.NewSQLiteStorage(sibDBPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage(sibling): %v", err)
	}
	projID := "hop-top/kit"
	blockingID := "T-0007"
	if err := sib.CreateTask(ctx, &core.Task{
		ID:        blockingID,
		Title:     "kit blocker",
		Status:    core.StatusTodo,
		ProjectID: &projID,
	}); err != nil {
		t.Fatalf("CreateTask sibling: %v", err)
	}
	_ = sib.Close()

	if err := local.RegisterProject(ctx, projID, sibDBPath, "", "kit"); err != nil {
		t.Fatalf("RegisterProject: %v", err)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{
		"task", "update", "T-0001",
		"--add-blocked-by", "hop-top/kit/T-0007",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update with full-id cross-DB ref failed: %v\noutput: %s", err, buf.String())
	}

	updated := getTaskByAlias(t, ctx, "T-0001")
	if updated == nil {
		t.Fatal("task not found after update")
	}
	got := updated.BlockedBy()
	if len(got) != 1 || got[0] != "hop-top/kit/T-0007" {
		t.Fatalf("blocked_by = %v, want [hop-top/kit/T-0007]", got)
	}
}

// TestTaskUpdate_CrossDBBlockedBy_UnknownProject verifies that a label
// that doesn't match any registered project surfaces the actionable
// "project not found" error from the resolver rather than crashing or
// silently no-oping.
func TestTaskUpdate_CrossDBBlockedBy_UnknownProject(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	local, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer func() { _ = local.Close() }()

	if err := local.CreateTask(ctx, &core.Task{
		ID:     "T-0001",
		Title:  "local task",
		Status: core.StatusTodo,
	}); err != nil {
		t.Fatalf("CreateTask local: %v", err)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{
		"task", "update", "T-0001",
		"--add-blocked-by", "ghost/T-0001",
	})

	err = cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for unknown project, got nil; output=%s", buf.String())
	}
	combined := err.Error() + buf.String()
	if !contains(combined, "ghost") || !contains(combined, "not found") {
		t.Fatalf("expected error to mention unknown project; got: %s", combined)
	}
}

// Compile-time guard: these tests rely on validateBlockedByRefs which is
// in the same package, but we also confirm the cross-DB cache plumbing
// builds.
var _ = context.Background
