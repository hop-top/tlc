package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

// seedSortHub stores three tasks whose titles sort differently from their
// creation order, so a sort that ran and one that did not look different.
func seedSortHub(t *testing.T) {
	t.Helper()
	ctx, _ := setupTestDir(t)
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for _, task := range []*core.Task{
		{ID: "T-0001", Title: "charlie", Status: core.StatusTodo},
		{ID: "T-0002", Title: "alpha", Status: core.StatusTodo},
		{ID: "T-0003", Title: "bravo", Status: core.StatusTodo},
	} {
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed %s: %v", task.ID, err)
		}
	}
}

// runSortCmd runs a command line and returns its combined output and error.
func runSortCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	resetTaskFlags()
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

// idOrder returns the T-NNNN ids in the order they appear in out.
func idOrder(out string) []string {
	var ids []string
	for _, line := range strings.Split(out, "\n") {
		for _, f := range strings.Fields(line) {
			if strings.HasPrefix(f, "T-") {
				ids = append(ids, f)
				break
			}
		}
	}
	return ids
}

var badSortKeys = []string{
	"title; SELECT 1",
	"(SELECT count(*) FROM tasks)",
	"(CASE WHEN title = 'bravo' THEN 0 ELSE 1 END)",
	"title DESC, id",
	"title--",
	"x'",
	"nosuchcolumn",
	"meta.priority",
	"title,",
}

func TestTaskList_SortByRejectsNonFields_E2E(t *testing.T) {
	for _, cmdName := range []string{"list", "graph"} {
		for _, bad := range badSortKeys {
			t.Run(cmdName+"/"+bad, func(t *testing.T) {
				seedSortHub(t)
				out, err := runSortCmd(t, "task", cmdName, "--sort-by", bad, "--sort-direction", "asc")
				if err == nil {
					t.Fatalf("--sort-by %q accepted; output:\n%s", bad, out)
				}
				if !strings.Contains(err.Error(), "--sort-by") {
					t.Errorf("error does not name the flag: %v", err)
				}
				if !strings.Contains(err.Error(), "created_at") {
					t.Errorf("error does not name the allowed fields: %v", err)
				}
				if ids := idOrder(out); len(ids) != 0 {
					t.Errorf("rows printed alongside the rejection: %v", ids)
				}
			})
		}
	}
}

func TestTaskList_SortDirectionRejectsNonDirections_E2E(t *testing.T) {
	for _, bad := range []string{"sideways", "asc; SELECT 1", "up"} {
		t.Run(bad, func(t *testing.T) {
			seedSortHub(t)
			out, err := runSortCmd(t, "task", "list", "--sort-by", "title", "--sort-direction", bad)
			if err == nil {
				t.Fatalf("--sort-direction %q accepted; output:\n%s", bad, out)
			}
			if !strings.Contains(err.Error(), "--sort-direction") {
				t.Errorf("error does not name the flag: %v", err)
			}
			if ids := idOrder(out); len(ids) != 0 {
				t.Errorf("rows printed alongside the rejection: %v", ids)
			}
		})
	}
}

func TestTaskList_SortByStillSorts_E2E(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"title asc", []string{"--sort-by", "title", "--sort-direction", "asc"}, []string{"T-0002", "T-0003", "T-0001"}},
		{"title desc", []string{"--sort-by", "title", "--sort-direction", "desc"}, []string{"T-0001", "T-0003", "T-0002"}},
		{"id asc", []string{"--sort-by", "id", "--sort-direction", "asc"}, []string{"T-0001", "T-0002", "T-0003"}},
		{"status then title", []string{"--sort-by", "status,title", "--sort-direction", "asc"}, []string{"T-0002", "T-0003", "T-0001"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seedSortHub(t)
			out, err := runSortCmd(t, append([]string{"task", "list"}, tc.args...)...)
			if err != nil {
				t.Fatalf("task list %v: %v\n%s", tc.args, err, out)
			}
			got := idOrder(out)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("order = %v, want %v\n%s", got, tc.want, out)
			}
		})
	}
}

// The REST list endpoint reads a fixed set of query parameters; none of
// them names a sort key or a filter field. A sort- or filter-shaped
// parameter must therefore be inert: same status, same rows, no SQL error.
func TestServe_E2E_TaskListIgnoresSortAndFilterFieldParams(t *testing.T) {
	withTestLock(func() {
		seedSortHub(t)
		router := newTestServeRouter(t)

		read := func(rawQuery string) []string {
			rec := doServeRequest(t, router, "GET", "/tasks?"+rawQuery, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET /tasks?%s: status %d: %s", rawQuery, rec.Code, rec.Body.String())
			}
			var resp taskListResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			ids := make([]string, 0, len(resp.Tasks))
			for _, task := range resp.Tasks {
				ids = append(ids, task.ID)
			}
			return ids
		}

		base := read("")
		if len(base) != 3 {
			t.Fatalf("baseline rows = %v, want 3", base)
		}
		payload := url.QueryEscape("(CASE WHEN title = 'bravo' THEN 0 ELSE 1 END)")
		for _, q := range []string{
			"sort=" + payload, "sort_by=" + payload, "sort-by=" + payload,
			"order=" + payload, "sort_direction=" + payload,
			payload + "=x", "title=" + payload, "field=" + payload,
		} {
			if got := read(q); strings.Join(got, ",") != strings.Join(base, ",") {
				t.Errorf("?%s changed the result: %v, want %v", q, got, base)
			}
		}
	})
}
