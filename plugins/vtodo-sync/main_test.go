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


