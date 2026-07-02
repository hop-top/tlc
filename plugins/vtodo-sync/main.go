package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

// JSON-RPC envelope shapes. Mirrored from plugins/github-sync so the
// host treats every external-sync plugin uniformly.

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      interface{}     `json:"id"`
}

type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Method-specific param shapes.
//
// The tlc sync client sends the configured endpoint under "repo" to
// match the existing github/jira/linear plugin contract. We accept it
// there and keep "source" as an alias for direct callers (manifest +
// docs use "source" historically). UnmarshalJSON copies "repo" into
// Source when "source" is absent.

type SyncPullParams struct {
	Source      string `json:"source"`
	IncludeLogs bool   `json:"include_logs,omitempty"`
	LastSyncAt  string `json:"last_sync_at,omitempty"`
}

func (p *SyncPullParams) UnmarshalJSON(b []byte) error {
	type raw SyncPullParams
	var aux struct {
		raw
		Repo string `json:"repo,omitempty"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	*p = SyncPullParams(aux.raw)
	if p.Source == "" {
		p.Source = aux.Repo
	}
	return nil
}

type SyncPushParams struct {
	Source      string `json:"source"`
	IncludeLogs bool   `json:"include_logs,omitempty"`
	Tasks       []Task `json:"tasks"`
}

func (p *SyncPushParams) UnmarshalJSON(b []byte) error {
	type raw SyncPushParams
	var aux struct {
		raw
		Repo string `json:"repo,omitempty"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	*p = SyncPushParams(aux.raw)
	if p.Source == "" {
		p.Source = aux.Repo
	}
	return nil
}

type SyncDeleteParams struct {
	Source string `json:"source"`
	Tasks  []Task `json:"tasks"`
}

func (p *SyncDeleteParams) UnmarshalJSON(b []byte) error {
	type raw SyncDeleteParams
	var aux struct {
		raw
		Repo string `json:"repo,omitempty"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	*p = SyncDeleteParams(aux.raw)
	if p.Source == "" {
		p.Source = aux.Repo
	}
	return nil
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	// Allow large payloads (the default 64K is too small for big
	// .ics dumps).
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 16*1024*1024)
	for scanner.Scan() {
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			sendError(nil, -32700, "Parse error", nil)
			continue
		}
		sendResponse(handleRequest(req))
	}
}

func handleRequest(req Request) Response {
	switch req.Method {
	case "sync.pull":
		return handlePull(req)
	case "sync.push":
		return handlePush(req)
	case "sync.delete":
		return handleDelete(req)
	default:
		return errorResponse(req.ID, -32601, "Method not found", nil)
	}
}

func handlePull(req Request) Response {
	var params SyncPullParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "Invalid params", nil)
	}

	path, err := resolveFileSource(params.Source)
	if err != nil {
		return errorResponse(req.ID, err.code, err.msg, nil)
	}

	tasks, err2 := readTasksFromFile(path)
	if err2 != nil {
		return errorResponse(req.ID, -32603,
			fmt.Sprintf("failed to read %s: %v", path, err2), nil)
	}

	return Response{
		JSONRPC: "2.0",
		Result: map[string]interface{}{
			"tasks":     tasks,
			"conflicts": []interface{}{},
			"sync_at":   time.Now().UTC().Format(time.RFC3339),
		},
		ID: req.ID,
	}
}

func handlePush(req Request) Response {
	var params SyncPushParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "Invalid params", nil)
	}

	path, perr := resolveFileSource(params.Source)
	if perr != nil {
		return errorResponse(req.ID, perr.code, perr.msg, nil)
	}

	updated := []string{}
	failed := map[string]string{}

	if err := writeTasksToFile(path, params.Tasks); err != nil {
		// On a whole-file write failure, every task we attempted is
		// considered failed. Caller can retry the entire batch.
		for _, t := range params.Tasks {
			failed[t.ID] = err.Error()
		}
	} else {
		for _, t := range params.Tasks {
			updated = append(updated, t.ID)
		}
	}

	return Response{
		JSONRPC: "2.0",
		Result: map[string]interface{}{
			"updated": updated,
			"failed":  failed,
		},
		ID: req.ID,
	}
}

func handleDelete(req Request) Response {
	var params SyncDeleteParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "Invalid params", nil)
	}

	path, perr := resolveFileSource(params.Source)
	if perr != nil {
		return errorResponse(req.ID, perr.code, perr.msg, nil)
	}

	deleted := []string{}
	failed := map[string]string{}

	existing, err := readTasksFromFile(path)
	if err != nil && !os.IsNotExist(err) {
		for _, t := range params.Tasks {
			failed[t.ID] = err.Error()
		}
		return Response{
			JSONRPC: "2.0",
			Result: map[string]interface{}{
				"deleted": deleted,
				"failed":  failed,
			},
			ID: req.ID,
		}
	}

	toRemove := make(map[string]struct{}, len(params.Tasks))
	for _, t := range params.Tasks {
		toRemove[t.ID] = struct{}{}
	}

	kept := make([]Task, 0, len(existing))
	for _, t := range existing {
		if _, drop := toRemove[t.ID]; drop {
			deleted = append(deleted, t.ID)
			continue
		}
		kept = append(kept, t)
	}

	// Use replaceTasksFile (snapshot) — we already computed the kept set
	// from the existing file, and we want the deletion to actually drop
	// the listed IDs. writeTasksToFile would merge them back in.
	if writeErr := replaceTasksFile(path, kept); writeErr != nil {
		// Whole-file write failed — flip every "deleted" to failed.
		for _, id := range deleted {
			failed[id] = writeErr.Error()
		}
		deleted = []string{}
	}

	return Response{
		JSONRPC: "2.0",
		Result: map[string]interface{}{
			"deleted": deleted,
			"failed":  failed,
		},
		ID: req.ID,
	}
}

// rpcErr is an internal error helper that carries a JSON-RPC error code
// and message together so source-resolution failures can short-circuit
// without losing the wire-level metadata.
type rpcErr struct {
	code int
	msg  string
}

func (e *rpcErr) Error() string { return e.msg }

// resolveFileSource validates the configured source URI. file:// is the
// only scheme supported by this build (Phase 3a). https://, http:// and
// missing/empty values return JSON-RPC -32600 (Invalid Request) with a
// pointer at the file:// migration path.
func resolveFileSource(source string) (string, *rpcErr) {
	if source == "" {
		return "", &rpcErr{code: -32602, msg: "source is required"}
	}
	u, err := url.Parse(source)
	if err != nil {
		return "", &rpcErr{
			code: -32602,
			msg:  fmt.Sprintf("invalid source URI %q: %v", source, err),
		}
	}
	switch strings.ToLower(u.Scheme) {
	case "file":
		path := u.Path
		if path == "" {
			path = u.Opaque
		}
		if path == "" {
			return "", &rpcErr{
				code: -32602,
				msg:  fmt.Sprintf("source %q has no path; expected file:///abs/path/cal.ics", source),
			}
		}
		return path, nil
	case "":
		return "", &rpcErr{
			code: -32602,
			msg:  fmt.Sprintf("source %q missing scheme; expected file:///abs/path/cal.ics", source),
		}
	default:
		return "", &rpcErr{
			code: -32600,
			msg:  "CalDAV mode not yet implemented; use file:// for now",
		}
	}
}

// readTasksFromFile reads an .ics file and returns the decoded plugin
// Tasks. A non-existent file yields an empty slice (so a first-time pull
// degrades gracefully rather than erroring).
func readTasksFromFile(path string) ([]Task, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Task{}, nil
		}
		return nil, err
	}
	defer f.Close()

	res, err := vtodo.ParseVCalendar(f)
	if err != nil {
		return nil, err
	}
	out := make([]Task, 0, len(res.Tasks))
	for _, t := range res.Tasks {
		out = append(out, *taskFromCore(t))
	}
	return out, nil
}

// writeTasksToFile serialises tasks into a fresh VCALENDAR and atomically
// replaces the target path.
// writeTasksToFile merges the incoming tasks into the existing
// calendar (if any) keyed by ID. Tasks not mentioned in the incoming
// batch are preserved verbatim — tlc's sync layer sends incremental
// batches, so a snapshot replace would silently delete unchanged work.
// Used by sync.push.
func writeTasksToFile(path string, tasks []Task) error {
	existing, err := readTasksFromFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	merged := mergeTasksByID(existing, tasks)
	return replaceTasksFile(path, merged)
}

// replaceTasksFile unconditionally replaces the calendar file's
// VTODO list with the supplied tasks. Used by sync.delete after the
// caller has already filtered the to-keep set.
func replaceTasksFile(path string, tasks []Task) error {
	coreTasks := make([]*core.Task, 0, len(tasks))
	for i := range tasks {
		coreTasks = append(coreTasks, taskToCore(&tasks[i]))
	}
	cal, err := vtodo.BuildVCalendar(coreTasks, nil, nil)
	if err != nil {
		return err
	}
	body, err := vtodo.Serialize(cal)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// mergeTasksByID returns existing entries with any matching incoming
// entry replacing it; new IDs are appended at the end. Iteration order
// of `existing` is preserved so the .ics file's component order stays
// stable across incremental pushes.
func mergeTasksByID(existing, incoming []Task) []Task {
	if len(incoming) == 0 {
		return existing
	}
	idx := make(map[string]int, len(incoming))
	for i, t := range incoming {
		idx[t.ID] = i
	}

	out := make([]Task, 0, len(existing)+len(incoming))
	used := make(map[string]bool, len(incoming))
	for _, e := range existing {
		if i, ok := idx[e.ID]; ok {
			out = append(out, incoming[i])
			used[e.ID] = true
		} else {
			out = append(out, e)
		}
	}
	for _, t := range incoming {
		if !used[t.ID] {
			out = append(out, t)
		}
	}
	return out
}

func sendResponse(resp Response) {
	data, _ := json.Marshal(resp) //nolint:errcheck // marshalling known-valid response struct
	fmt.Println(string(data))
}

func sendError(id interface{}, code int, message string, data interface{}) {
	sendResponse(errorResponse(id, code, message, data))
}

func errorResponse(id interface{}, code int, message string, data interface{}) Response {
	return Response{
		JSONRPC: "2.0",
		Error: &Error{
			Code:    code,
			Message: message,
			Data:    data,
		},
		ID: id,
	}
}
