package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestHandlePush_FileMode_RoundTrip confirms a sync.push followed by a
// sync.pull on the same file:// source returns the same task ID and
// title, with the tasks list and updated counts as expected.
func TestHandlePush_FileMode_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	icsPath := filepath.Join(dir, "cal.ics")
	source := "file://" + icsPath

	now := time.Now().UTC().Truncate(time.Second)
	taskID := "task_01h455vb4pex5vsknk084sn02q"

	// PUSH ----------------------------------------------------------------
	pushParams := SyncPushParams{
		Source: source,
		Tasks: []Task{{
			ID:        taskID,
			Title:     "Replace JWT signer",
			Status:    "IN_PROGRESS",
			Tags:      []string{"security"},
			Priority:  "P1",
			Effort:    "M",
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}
	pushReq := newRequest(t, 1, "sync.push", pushParams)
	pushResp := handleRequest(pushReq)
	if pushResp.Error != nil {
		t.Fatalf("sync.push error: %+v", pushResp.Error)
	}
	pushResult, _ := pushResp.Result.(map[string]interface{})
	updated, _ := pushResult["updated"].([]string)
	if len(updated) != 1 || updated[0] != taskID {
		t.Errorf("updated = %v, want [%s]", updated, taskID)
	}
	if _, err := os.Stat(icsPath); err != nil {
		t.Fatalf("expected ics file to exist after push: %v", err)
	}

	// PULL ----------------------------------------------------------------
	pullReq := newRequest(t, 2, "sync.pull",
		SyncPullParams{Source: source})
	pullResp := handleRequest(pullReq)
	if pullResp.Error != nil {
		t.Fatalf("sync.pull error: %+v", pullResp.Error)
	}
	pullResult, _ := pullResp.Result.(map[string]interface{})
	tasksRaw, _ := pullResult["tasks"].([]Task)
	if len(tasksRaw) != 1 {
		t.Fatalf("pulled tasks = %d, want 1", len(tasksRaw))
	}
	if tasksRaw[0].ID != taskID {
		t.Errorf("pulled task ID = %q, want %q", tasksRaw[0].ID, taskID)
	}
	if tasksRaw[0].Title != "Replace JWT signer" {
		t.Errorf("pulled title = %q, want %q",
			tasksRaw[0].Title, "Replace JWT signer")
	}
}

// TestHandlePull_MissingFileReturnsEmpty confirms the first-time-pull
// behaviour: a file:// pointing at a non-existent file returns an empty
// task list rather than an error.
func TestHandlePull_MissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	source := "file://" + filepath.Join(dir, "does-not-exist.ics")

	resp := handleRequest(newRequest(t, 1, "sync.pull",
		SyncPullParams{Source: source}))
	if resp.Error != nil {
		t.Fatalf("sync.pull on missing file errored: %+v", resp.Error)
	}
	result, _ := resp.Result.(map[string]interface{})
	tasks, _ := result["tasks"].([]Task)
	if len(tasks) != 0 {
		t.Errorf("missing-file pull returned %d tasks, want 0", len(tasks))
	}
}

// TestHandlePull_HTTPSReturnsCalDAVError confirms https:// (and any
// non-file scheme) is rejected with the documented -32600 error.
func TestHandlePull_HTTPSReturnsCalDAVError(t *testing.T) {
	resp := handleRequest(newRequest(t, 1, "sync.pull",
		SyncPullParams{Source: "https://cal.example.com/cal.ics"}))
	if resp.Error == nil {
		t.Fatal("expected error for https:// source, got nil")
	}
	if resp.Error.Code != -32600 {
		t.Errorf("Code = %d, want -32600", resp.Error.Code)
	}
	if want := "CalDAV mode not yet implemented"; !strings.Contains(resp.Error.Message, want) {
		t.Errorf("Message = %q, want substring %q",
			resp.Error.Message, want)
	}
}

// TestHandleRequest_UnknownMethod returns -32601.
func TestHandleRequest_UnknownMethod(t *testing.T) {
	resp := handleRequest(Request{
		JSONRPC: "2.0",
		Method:  "sync.frobnicate",
		ID:      1,
	})
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Errorf("error = %+v, want code -32601", resp.Error)
	}
}

// TestHandleDelete_RemovesMatchingTasks confirms sync.delete drops the
// task by ID and leaves the rest intact.
func TestHandleDelete_RemovesMatchingTasks(t *testing.T) {
	dir := t.TempDir()
	icsPath := filepath.Join(dir, "cal.ics")
	source := "file://" + icsPath
	now := time.Now().UTC().Truncate(time.Second)

	keep := Task{
		ID:        "task_01h455vb4pex5vsknk084sn02a",
		Title:     "Keeper",
		Status:    "TODO",
		CreatedAt: now,
		UpdatedAt: now,
	}
	drop := Task{
		ID:        "task_01h455vb4pex5vsknk084sn02b",
		Title:     "Doomed",
		Status:    "TODO",
		CreatedAt: now,
		UpdatedAt: now,
	}
	pushResp := handleRequest(newRequest(t, 1, "sync.push",
		SyncPushParams{Source: source, Tasks: []Task{keep, drop}}))
	if pushResp.Error != nil {
		t.Fatalf("seed push error: %+v", pushResp.Error)
	}

	delResp := handleRequest(newRequest(t, 2, "sync.delete",
		SyncDeleteParams{Source: source, Tasks: []Task{drop}}))
	if delResp.Error != nil {
		t.Fatalf("sync.delete error: %+v", delResp.Error)
	}
	delResult, _ := delResp.Result.(map[string]interface{})
	deleted, _ := delResult["deleted"].([]string)
	if len(deleted) != 1 || deleted[0] != drop.ID {
		t.Errorf("deleted = %v, want [%s]", deleted, drop.ID)
	}

	pullResp := handleRequest(newRequest(t, 3, "sync.pull",
		SyncPullParams{Source: source}))
	pullResult, _ := pullResp.Result.(map[string]interface{})
	tasks, _ := pullResult["tasks"].([]Task)
	if len(tasks) != 1 || tasks[0].ID != keep.ID {
		t.Errorf("post-delete tasks = %+v, want only %s", tasks, keep.ID)
	}
}

// TestHandlePush_PreservesUnmentionedTasks ensures incremental pushes
// merge with the existing calendar instead of overwriting it. tlc's
// sync layer sends only changed tasks per call; if push replaced the
// whole file with just those tasks, every other VTODO would vanish.
func TestHandlePush_PreservesUnmentionedTasks(t *testing.T) {
	dir := t.TempDir()
	icsPath := filepath.Join(dir, "cal.ics")
	source := "file://" + icsPath
	now := time.Now().UTC().Truncate(time.Second)

	keep := Task{
		ID:        "task_01h455vb4pex5vsknk084sn02a",
		Title:     "Existing — must survive",
		Status:    "TODO",
		CreatedAt: now,
		UpdatedAt: now,
	}
	incoming := Task{
		ID:        "task_01h455vb4pex5vsknk084sn02b",
		Title:     "Newly synced",
		Status:    "TODO",
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Seed the file with `keep`.
	seed := handleRequest(newRequest(t, 1, "sync.push",
		SyncPushParams{Source: source, Tasks: []Task{keep}}))
	if seed.Error != nil {
		t.Fatalf("seed push: %+v", seed.Error)
	}

	// Incremental push for `incoming` only.
	resp := handleRequest(newRequest(t, 2, "sync.push",
		SyncPushParams{Source: source, Tasks: []Task{incoming}}))
	if resp.Error != nil {
		t.Fatalf("incremental push: %+v", resp.Error)
	}

	// Pull and assert both tasks present.
	pullResp := handleRequest(newRequest(t, 3, "sync.pull",
		SyncPullParams{Source: source}))
	result, _ := pullResp.Result.(map[string]interface{})
	tasks, _ := result["tasks"].([]Task)
	if len(tasks) != 2 {
		t.Fatalf("after incremental push: %d tasks, want 2 (got %+v)",
			len(tasks), tasks)
	}
	ids := map[string]bool{}
	for _, t := range tasks {
		ids[t.ID] = true
	}
	if !ids[keep.ID] || !ids[incoming.ID] {
		t.Errorf("missing IDs: have=%v want={%s,%s}",
			ids, keep.ID, incoming.ID)
	}

	// And on a re-push of the same incoming task, content updates without
	// duplicating or wiping `keep`.
	updated := incoming
	updated.Title = "Newly synced (updated)"
	resp2 := handleRequest(newRequest(t, 4, "sync.push",
		SyncPushParams{Source: source, Tasks: []Task{updated}}))
	if resp2.Error != nil {
		t.Fatalf("update push: %+v", resp2.Error)
	}
	pullResp2 := handleRequest(newRequest(t, 5, "sync.pull",
		SyncPullParams{Source: source}))
	result2, _ := pullResp2.Result.(map[string]interface{})
	tasks2, _ := result2["tasks"].([]Task)
	if len(tasks2) != 2 {
		t.Fatalf("after update push: %d tasks, want 2", len(tasks2))
	}
	for _, ts := range tasks2 {
		if ts.ID == updated.ID && ts.Title != "Newly synced (updated)" {
			t.Errorf("update push didn't refresh title: %q", ts.Title)
		}
		if ts.ID == keep.ID && ts.Title != keep.Title {
			t.Errorf("keep title mutated: %q", ts.Title)
		}
	}
}

// TestRPCWireContract_AcceptsRepoField simulates the actual tlc sync
// client which sends params with a "repo" field (mirroring github-sync).
// Plugins are expected to treat that as the source endpoint. Without it,
// every sync.pull/push/delete from tlc fails with "source is required".
func TestRPCWireContract_AcceptsRepoField(t *testing.T) {
	dir := t.TempDir()
	icsPath := filepath.Join(dir, "cal.ics")
	endpoint := "file://" + icsPath

	// Seed a task using the wire shape the tlc client actually sends.
	now := time.Now().UTC().Truncate(time.Second)
	rawPush, err := json.Marshal(map[string]interface{}{
		"repo": endpoint,
		"tasks": []Task{{
			ID:        "task_01h455vb4pex5vsknk084sn02q",
			Title:     "Wire-format check",
			Status:    "TODO",
			CreatedAt: now,
			UpdatedAt: now,
		}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	pushResp := handleRequest(Request{
		JSONRPC: "2.0", Method: "sync.push", Params: rawPush, ID: 1,
	})
	if pushResp.Error != nil {
		t.Fatalf("sync.push with repo field failed: %+v", pushResp.Error)
	}

	// Pull using the same wire shape.
	rawPull, _ := json.Marshal(map[string]interface{}{"repo": endpoint})
	pullResp := handleRequest(Request{
		JSONRPC: "2.0", Method: "sync.pull", Params: rawPull, ID: 2,
	})
	if pullResp.Error != nil {
		t.Fatalf("sync.pull with repo field failed: %+v", pullResp.Error)
	}
	result, _ := pullResp.Result.(map[string]interface{})
	tasks, _ := result["tasks"].([]Task)
	if len(tasks) != 1 || tasks[0].ID != "task_01h455vb4pex5vsknk084sn02q" {
		t.Errorf("pull returned %+v; want exactly the seeded task", tasks)
	}
}

// --- helpers ---

func newRequest(t *testing.T, id int, method string, params interface{}) Request {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return Request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  raw,
		ID:      id,
	}
}
