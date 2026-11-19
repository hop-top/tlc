package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// seedCrossDBBlocker creates a local task T-0001 plus a registered
// sibling project "hop-top/kit" holding blocker T-0007.
func seedCrossDBBlocker(t *testing.T, ctx context.Context) {
	t.Helper()
	local, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	t.Cleanup(func() { _ = local.Close() })

	if err := local.CreateTask(ctx, &core.Task{
		ID: "T-0001", Title: "local task", Status: core.StatusTodo,
	}); err != nil {
		t.Fatalf("CreateTask local: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "kit.sqlite")
	sib, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	projID := "hop-top/kit"
	if err := sib.CreateTask(ctx, &core.Task{
		ID: "T-0007", Title: "kit blocker", Status: core.StatusTodo, ProjectID: &projID,
	}); err != nil {
		t.Fatalf("CreateTask sibling: %v", err)
	}
	_ = sib.Close()
	if err := local.RegisterProject(ctx, projID, dbPath, "", "kit"); err != nil {
		t.Fatalf("RegisterProject: %v", err)
	}
}

func runTaskUpdate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(append([]string{"task", "update", "T-0001"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// Every ref form --add-blocked-by accepts must remove the edge it added.
func TestTaskUpdate_RemoveBlockedBy_AcceptsAllAddForms(t *testing.T) {
	forms := []string{
		"tlc://hop-top/kit/T-0007",
		"hop-top/kit/T-0007",
		"kit/T-0007",
	}
	for _, form := range forms {
		t.Run(form, func(t *testing.T) {
			ctx, cleanup := setupTestDir(t)
			defer cleanup()
			seedCrossDBBlocker(t, ctx)

			if out, err := runTaskUpdate(t, "--add-blocked-by", form); err != nil {
				t.Fatalf("add %q: %v\n%s", form, err, out)
			}
			if got := getTaskByAlias(t, ctx, "T-0001").BlockedBy(); len(got) != 1 {
				t.Fatalf("after add: blocked_by = %v, want one edge", got)
			}

			if out, err := runTaskUpdate(t, "--remove-blocked-by", form); err != nil {
				t.Fatalf("remove %q: %v\n%s", form, err, out)
			}
			if got := getTaskByAlias(t, ctx, "T-0001").BlockedBy(); len(got) != 0 {
				t.Fatalf("after remove %q: blocked_by = %v, want empty", form, got)
			}
		})
	}
}

// A remove ref matching no edge must fail loudly, naming the ref.
func TestTaskUpdate_RemoveBlockedBy_NoMatchErrors(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	seedCrossDBBlocker(t, ctx)

	if out, err := runTaskUpdate(t, "--add-blocked-by", "tlc://hop-top/kit/T-0007"); err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}

	out, err := runTaskUpdate(t, "--remove-blocked-by", "tlc://hop-top/kit/T-0999")
	if err == nil {
		t.Fatalf("expected error removing non-edge, got nil\n%s", out)
	}
	if !strings.Contains(err.Error(), "tlc://hop-top/kit/T-0999") {
		t.Fatalf("error %q does not name the ref", err)
	}
	if got := getTaskByAlias(t, ctx, "T-0001").BlockedBy(); len(got) != 1 {
		t.Fatalf("existing edge disturbed: %v", got)
	}
}
