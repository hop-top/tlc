package cli

// End-to-end tests against a store holding legacy status labels (TODO,
// IN_PROGRESS, DONE, SKIPPED) under the default vocabulary. Fresh test
// stores only ever hold current spellings, so every status predicate
// passes against them whether or not legacy rows still match; this
// fixture is the one to run status work against.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	_ "modernc.org/sqlite" // registers the "sqlite" driver for the raw rewrite
)

const legacyHubUser = "legacy-user"

// legacyHubTasks: ID -> stored label after the rewrite. The seed uses the
// current names (UpdatedAt kept recent so nothing auto-archives or reads
// as stale); the rewrite below turns them into what an older build
// stored.
//
//	T-0001 todo        mine  track
//	T-0002 in_progress mine  track, stale
//	T-0003 todo        other
//	T-0004 done        mine
//	T-0005 cancelled   other (stored SKIPPED)
func seedLegacyHub(t *testing.T) {
	t.Helper()
	ctx, _ := setupTestDir(t)
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	viper.Set("runtime.profile", legacyHubUser)
	t.Cleanup(func() { viper.Set("runtime.profile", nil) })

	trackID := seedTrack(t, ctx, s, s, &core.Track{Slug: "legacy-track", Title: "Legacy", Type: "feature"})
	now := time.Now().UTC()
	stale := 5 * time.Minute
	me, other := legacyHubUser, "someone-else"
	seed := []*core.Task{
		{ID: "T-0001", Title: "alpha", Status: core.StatusTodo, AssignedTo: &me, TrackID: &trackID, UpdatedAt: now},
		{
			ID: "T-0002", Title: "bravo", Status: core.StatusInProgress, AssignedTo: &me, TrackID: &trackID,
			UpdatedAt: now.Add(-4 * time.Hour), StaleTimeout: &stale,
		},
		{ID: "T-0003", Title: "charlie", Status: core.StatusTodo, AssignedTo: &other, UpdatedAt: now},
		{ID: "T-0004", Title: "delta", Status: core.StatusDone, AssignedTo: &me, UpdatedAt: now},
		{ID: "T-0005", Title: "echo", Status: core.StatusCancelled, AssignedTo: &other, UpdatedAt: now},
	}
	for _, task := range seed {
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed %s: %v", task.ID, err)
		}
	}

	raw, err := sql.Open("sqlite", viper.GetString("storage.db_path"))
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	defer func() { _ = raw.Close() }()
	if _, err := raw.ExecContext(ctx, `UPDATE tasks SET status =
		CASE status WHEN 'cancelled' THEN 'SKIPPED' ELSE UPPER(status) END`); err != nil {
		t.Fatalf("rewrite to legacy labels: %v", err)
	}
	var lower int
	if err := raw.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE status != UPPER(status)`).Scan(&lower); err != nil || lower != 0 {
		t.Fatalf("fixture not legacy: %d lowercase rows, err %v", lower, err)
	}
}

func runLegacyHubCmd(t *testing.T, args ...string) string {
	t.Helper()
	resetTaskFlags()
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, buf.String())
	}
	return buf.String()
}

func assertIDs(t *testing.T, out string, want, notWant []string) {
	t.Helper()
	for _, id := range want {
		if !strings.Contains(out, id) {
			t.Errorf("missing %s in:\n%s", id, out)
		}
	}
	for _, id := range notWant {
		if strings.Contains(out, id) {
			t.Errorf("unexpected %s in:\n%s", id, out)
		}
	}
}

func TestLegacyStatusLabels_TaskList_E2E(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    []string
		notWant []string
	}{
		// Default view: unfinished work.
		{"default", []string{"task", "list"}, []string{"T-0001", "T-0002", "T-0003"}, []string{"T-0004", "T-0005"}},
		{"status lowercase", []string{"task", "list", "--status", "todo"}, []string{"T-0001", "T-0003"}, []string{"T-0002", "T-0004", "T-0005"}},
		{"status uppercase", []string{"task", "list", "--status", "TODO"}, []string{"T-0001", "T-0003"}, []string{"T-0002", "T-0004", "T-0005"}},
		{"status list", []string{"task", "list", "--status", "in_progress,done"}, []string{"T-0002", "T-0004"}, []string{"T-0001", "T-0003", "T-0005"}},
		{"status cancelled reads legacy skipped", []string{"task", "list", "--status", "cancelled"}, []string{"T-0005"}, []string{"T-0001", "T-0004"}},
		{"mine", []string{"task", "list", "--mine"}, []string{"T-0001", "T-0002"}, []string{"T-0003", "T-0004", "T-0005"}},
		{"mine all statuses", []string{"task", "list", "--mine", "--status", "todo,in_progress,done"}, []string{"T-0001", "T-0002", "T-0004"}, []string{"T-0003", "T-0005"}},
		{"track", []string{"task", "list", "--track", "legacy-track"}, []string{"T-0001", "T-0002"}, []string{"T-0003", "T-0004", "T-0005"}},
		{"all projects", []string{"task", "list", "--all-projects"}, []string{"T-0001", "T-0002", "T-0003"}, []string{"T-0004", "T-0005"}},
		{"sort by status", []string{"task", "list", "--sort-by", "status", "--status", "todo,in_progress"}, []string{"T-0001", "T-0002", "T-0003"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedLegacyHub(t)
			assertIDs(t, runLegacyHubCmd(t, c.args...), c.want, c.notWant)
		})
	}
}

func TestLegacyStatusLabels_TaskStale_E2E(t *testing.T) {
	seedLegacyHub(t)
	out := runLegacyHubCmd(t, "task", "stale")
	assertIDs(t, out, []string{"T-0002"}, []string{"T-0001", "T-0003", "T-0004", "T-0005"})
}

func TestLegacyStatusLabels_TaskGraph_E2E(t *testing.T) {
	for _, args := range [][]string{
		{"task", "graph"},
		{"task", "graph", "--status", "todo"},
		{"task", "graph", "--status", "TODO"},
	} {
		seedLegacyHub(t)
		out := runLegacyHubCmd(t, args...)
		assertIDs(t, out, []string{"T-0001", "T-0003"}, []string{"T-0004", "T-0005"})
	}
}

func TestLegacyStatusLabels_ServeTaskList_E2E(t *testing.T) {
	seedLegacyHub(t)
	router := newTestServeRouter(t)
	for _, c := range []struct {
		path string
		want int
	}{
		{"/tasks", 5},
		{"/tasks?status=todo", 2},
		{"/tasks?status=TODO", 2},
		{"/tasks?status=done", 1},
		{"/tasks?status=cancelled", 1},
	} {
		rec := doServeRequest(t, router, "GET", c.path, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", c.path, rec.Code, rec.Body.String())
		}
		var resp taskListResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("GET %s: decode: %v", c.path, err)
		}
		if resp.Total != c.want {
			t.Errorf("GET %s: %d tasks, want %d", c.path, resp.Total, c.want)
		}
	}
}
