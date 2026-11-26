package cli

// HTTP coverage for task meta: POST /tasks accepts `meta`; PATCH
// /tasks/{id} merges it per JSON Merge Patch (RFC 7396). Both go through
// the same core path as the CLI flags.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

// doServeRequestRaw sends body verbatim, for requests whose exact JSON
// matters (an explicit null) and which a marshalled Go value cannot say.
func doServeRequestRaw(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeTask(t *testing.T, body []byte) core.Task {
	t.Helper()
	var task core.Task
	if err := json.Unmarshal(body, &task); err != nil {
		t.Fatalf("decode task: %v\n%s", err, body)
	}
	return task
}

func TestServe_TaskCreateWithMeta(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()
		router := newTestServeRouter(t)

		rec := doServeRequest(t, router, "POST", "/tasks", map[string]any{
			"title": "with meta",
			"meta": map[string]any{
				"owner": "ana", "n": 3, "cfg": map[string]any{"a": true},
			},
		})
		if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
			t.Fatalf("POST /tasks: %d %s", rec.Code, rec.Body.String())
		}
		created := decodeTask(t, rec.Body.Bytes())

		rec = doServeRequest(t, router, "GET", "/tasks/"+created.ID, nil)
		got := decodeTask(t, rec.Body.Bytes())
		want := map[string]any{
			"owner": "ana", "n": 3.0, "cfg": map[string]any{"a": true},
		}
		if !reflect.DeepEqual(got.Meta, want) {
			t.Fatalf("stored meta = %v, want %v", got.Meta, want)
		}
	})
}

func TestServe_TaskCreateMetaKeepsBlockedBy(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()
		blocker := &core.Task{ID: core.NewTaskID(), Title: "blocker", Status: core.StatusTodo}
		if err := s.CreateTask(ctx, blocker); err != nil {
			t.Fatal(err)
		}
		router := newTestServeRouter(t)

		rec := doServeRequest(t, router, "POST", "/tasks", map[string]any{
			"title": "b", "blocked_by": []string{blocker.ID}, "meta": map[string]any{"owner": "ana"},
		})
		if rec.Code >= 300 {
			t.Fatalf("POST /tasks: %d %s", rec.Code, rec.Body.String())
		}
		got := decodeTask(t, rec.Body.Bytes())
		if got.Meta["owner"] != "ana" || len(got.BlockedBy()) != 1 {
			t.Fatalf("meta = %v, want owner and blocked_by both kept", got.Meta)
		}
	})
}

func TestServe_TaskCreateReservedMetaRejected(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		router := newTestServeRouter(t)

		for _, key := range []string{"eva", "blocked_by", "priority_source", "origin_system"} {
			rec := doServeRequest(t, router, "POST", "/tasks", map[string]any{
				"title": "bad", "meta": map[string]any{"ok": "1", key: "x"},
			})
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("meta.%s: expected 422, got %d: %s", key, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `reserved meta key \"`+key+`\"`) {
				t.Errorf("meta.%s: error should name the reserved key: %s", key, rec.Body.String())
			}
		}
		s, _ := getStorageRaw()
		defer s.Close()
		tasks, err := s.ListTasks(ctx, core.Query{})
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) != 0 {
			t.Fatalf("rejected creates left %d task(s)", len(tasks))
		}
	})
}

func TestServe_TaskPatchMetaMergePatch(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		task := &core.Task{
			ID: core.NewTaskID(), Title: "t", Status: core.StatusTodo,
			Meta: map[string]any{
				"keep": "k", "owner": "old", "gone": "x",
				"cfg": map[string]any{"a": "1", "b": "2"},
			},
		}
		task.AddEva([]string{"e1"})
		core.MarkPriorityManual(task)
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
		router := newTestServeRouter(t)

		// Raw JSON: null must reach the handler as null, which a Go struct
		// with omitempty cannot express.
		patch := func(body string) (int, string) {
			rec := doServeRequestRaw(t, router, "PATCH", "/tasks/"+task.ID, body)
			return rec.Code, rec.Body.String()
		}

		code, body := patch(`{"meta":{"owner":"new","added":1,"gone":null,"cfg":{"b":null,"c":"3"}}}`)
		if code != http.StatusOK {
			t.Fatalf("PATCH: %d %s", code, body)
		}
		got := decodeTask(t, []byte(body))
		want := map[string]any{
			"keep": "k", "owner": "new", "added": 1.0,
			"cfg":             map[string]any{"a": "1", "c": "3"},
			"eva":             []any{"e1"},
			"priority_source": task.Meta["priority_source"],
		}
		if !reflect.DeepEqual(got.Meta, want) {
			t.Fatalf("merged meta = %v, want %v", got.Meta, want)
		}

		// Absent and empty `meta` leave everything alone.
		for _, b := range []string{`{"title":"renamed"}`, `{"meta":{}}`, `{"meta":null}`} {
			if code, body := patch(b); code != http.StatusOK {
				t.Fatalf("PATCH %s: %d %s", b, code, body)
			}
			if after := decodeTask(t, []byte(getTaskBodyOverHTTP(t, router, task.ID))); !reflect.DeepEqual(after.Meta, want) {
				t.Fatalf("PATCH %s changed meta: %v", b, after.Meta)
			}
		}
	})
}

func TestServe_TaskPatchReservedMetaRejectedAtomically(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		task := &core.Task{ID: core.NewTaskID(), Title: "orig", Status: core.StatusTodo,
			Meta: map[string]any{"owner": "ana"}}
		task.AddEva([]string{"e1"})
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
		router := newTestServeRouter(t)
		before := getTaskBodyOverHTTP(t, router, task.ID)

		for _, body := range []string{
			`{"title":"renamed","meta":{"owner":"bob","eva":["x"]}}`,
			`{"title":"renamed","meta":{"eva":null}}`,
			`{"title":"renamed","meta":{"blocked_by_plan":null}}`,
			`{"title":"renamed","meta":{"origin_id":"1"}}`,
			`{"title":"renamed","status":"IN_PROGRESS","meta":{"last_sync_hash":"h"}}`,
		} {
			rec := doServeRequestRaw(t, router, "PATCH", "/tasks/"+task.ID, body)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("PATCH %s: expected 422, got %d: %s", body, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "reserved meta key") {
				t.Errorf("PATCH %s: error should say reserved meta key: %s", body, rec.Body.String())
			}
			if after := getTaskBodyOverHTTP(t, router, task.ID); after != before {
				t.Fatalf("PATCH %s partially applied:\nbefore %s\nafter  %s", body, before, after)
			}
		}
	})
}

func getTaskBodyOverHTTP(t *testing.T, router http.Handler, id string) string {
	t.Helper()
	rec := doServeRequestRaw(t, router, "GET", "/tasks/"+id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /tasks/%s: %d %s", id, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}
