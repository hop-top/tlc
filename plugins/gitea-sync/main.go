package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// --- JSONRPC types ---

// Request is a JSONRPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      interface{}     `json:"id"`
}

// SyncPullParams for sync.pull.
type SyncPullParams struct {
	Repo       string      `json:"repo"`
	LastSyncAt string      `json:"last_sync_at,omitempty"`
	Vocabulary *Vocabulary `json:"vocabulary,omitempty"`
}

// SyncPushParams for sync.push.
type SyncPushParams struct {
	Repo       string      `json:"repo"`
	Tasks      []Task      `json:"tasks"`
	Vocabulary *Vocabulary `json:"vocabulary,omitempty"`
}

// SyncDeleteParams for sync.delete.
type SyncDeleteParams struct {
	Repo  string `json:"repo"`
	Tasks []Task `json:"tasks"`
}

// Response is a JSONRPC 2.0 response.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// RPCError is a JSONRPC 2.0 error.
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// --- Gitea HTTP client helpers ---

func giteaBaseURL() string {
	u := os.Getenv("GITEA_URL")
	if u == "" {
		u = "https://gitea.com"
	}
	return strings.TrimRight(u, "/")
}

func giteaToken() string {
	return os.Getenv("GITEA_TOKEN")
}

func giteaRequest(method, path string, body interface{}) (*http.Response, error) {
	token := giteaToken()
	if token == "" {
		return nil, fmt.Errorf("GITEA_TOKEN not set; export GITEA_TOKEN=<token>")
	}

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := giteaBaseURL() + "/api/v1" + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return http.DefaultClient.Do(req)
}

func decodeResponse(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gitea API %d: %s", resp.StatusCode, string(data))
	}
	if v != nil {
		return json.NewDecoder(resp.Body).Decode(v)
	}
	return nil
}

// --- Main loop ---

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer for large payloads.
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			sendError(nil, -32700, "Parse error", nil)
			continue
		}
		resp := handleRequest(req)
		sendResponse(resp)
	}
}

func handleRequest(req Request) Response {
	switch req.Method {
	case "sync.pull":
		return handleSyncPull(req)
	case "sync.push":
		return handleSyncPush(req)
	case "sync.delete":
		return handleSyncDelete(req)
	case "auth.status":
		return handleAuthStatus(req)
	default:
		return errResponse(req.ID, -32601, "Method not found")
	}
}

// --- sync.pull ---

func handleSyncPull(req Request) Response {
	var params SyncPullParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errResponse(req.ID, -32602, "Invalid params")
	}

	tasks, err := fetchGiteaIssues(params.Repo, params.LastSyncAt, params.Vocabulary)
	if err != nil {
		return errResponse(req.ID, -32603, fmt.Sprintf("fetch issues: %v", err))
	}

	return Response{
		JSONRPC: "2.0",
		Result: map[string]interface{}{
			"tasks":     tasks,
			"conflicts": []interface{}{},
			"sync_at":   time.Now().Format(time.RFC3339),
		},
		ID: req.ID,
	}
}

func fetchGiteaIssues(repoFull, lastSyncAt string, vocab *Vocabulary) ([]*Task, error) {
	owner, repo, err := splitRepo(repoFull)
	if err != nil {
		return nil, err
	}

	page := 1
	var tasks []*Task
	for {
		path := fmt.Sprintf("/repos/%s/%s/issues?state=all&type=issues&limit=50&page=%d",
			owner, repo, page)
		if lastSyncAt != "" {
			path += "&since=" + lastSyncAt
		}

		resp, err := giteaRequest("GET", path, nil)
		if err != nil {
			return nil, err
		}

		var issues []GiteaIssue
		if err := decodeResponse(resp, &issues); err != nil {
			return nil, fmt.Errorf("decode issues: %w", err)
		}

		if len(issues) == 0 {
			break
		}

		for i := range issues {
			deps := fetchDependencies(owner, repo, issues[i].Index)
			tasks = append(tasks, MapGiteaIssueToTaskWith(&issues[i], deps, vocab))
		}

		page++
	}
	return tasks, nil
}

func fetchDependencies(owner, repo string, index int64) []GiteaDependency {
	const limit = 50
	page := 1
	var all []GiteaDependency
	for {
		path := fmt.Sprintf("/repos/%s/%s/issues/%d/dependencies?limit=%d&page=%d",
			owner, repo, index, limit, page)
		resp, err := giteaRequest("GET", path, nil)
		if err != nil {
			return all
		}
		var deps []GiteaDependency
		if err := decodeResponse(resp, &deps); err != nil {
			return all
		}
		all = append(all, deps...)
		if len(deps) < limit {
			break
		}
		page++
	}
	return all
}

// --- sync.push ---

func handleSyncPush(req Request) Response {
	var params SyncPushParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errResponse(req.ID, -32602, "Invalid params")
	}

	owner, repo, err := splitRepo(params.Repo)
	if err != nil {
		return errResponse(req.ID, -32602, err.Error())
	}

	updated := []string{}
	failed := map[string]string{}

	for i := range params.Tasks {
		task := &params.Tasks[i]
		originID, _ := task.Meta["origin_id"].(string)

		if originID == "" {
			err = createGiteaIssue(owner, repo, task, params.Vocabulary)
		} else {
			err = updateGiteaIssue(owner, repo, task, params.Vocabulary)
		}

		if err != nil {
			failed[task.ID] = err.Error()
		} else {
			updated = append(updated, task.ID)
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

func createGiteaIssue(owner, repo string, task *Task, vocab *Vocabulary) error {
	issueData := MapTaskToGiteaIssueWith(task, vocab)
	path := fmt.Sprintf("/repos/%s/%s/issues", owner, repo)

	resp, err := giteaRequest("POST", path, issueData)
	if err != nil {
		return fmt.Errorf("create issue: %w", err)
	}

	var created GiteaIssue
	if err := decodeResponse(resp, &created); err != nil {
		return fmt.Errorf("decode created issue: %w", err)
	}

	if task.Meta == nil {
		task.Meta = make(map[string]interface{})
	}
	task.Meta["origin_system"] = "gitea"
	task.Meta["origin_id"] = fmt.Sprintf("%d", created.Index)
	task.Meta["origin_url"] = created.HTMLURL

	// Create dependencies.
	createDependencies(owner, repo, created.ID, task)

	return nil
}

func updateGiteaIssue(owner, repo string, task *Task, vocab *Vocabulary) error {
	originIDStr, _ := task.Meta["origin_id"].(string)
	index, err := strconv.ParseInt(originIDStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid origin_id %q: %w", originIDStr, err)
	}

	issueData := MapTaskToGiteaIssueWith(task, vocab)
	path := fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, index)

	resp, err := giteaRequest("PATCH", path, issueData)
	if err != nil {
		return fmt.Errorf("update issue: %w", err)
	}
	if err := decodeResponse(resp, nil); err != nil {
		return fmt.Errorf("update issue response: %w", err)
	}

	// Manage dependencies.
	createDependencies(owner, repo, 0, task)

	return nil
}

func createDependencies(owner, repo string, issueID int64, task *Task) {
	indices := BlockedByIssueIndices(task)
	if len(indices) == 0 {
		return
	}

	// Look up issue ID from index if needed.
	originIDStr, _ := task.Meta["origin_id"].(string)
	index, _ := strconv.ParseInt(originIDStr, 10, 64)
	if index == 0 {
		return
	}

	for _, depIndex := range indices {
		// Resolve the dependency issue's internal ID.
		depID := resolveIssueID(owner, repo, depIndex)
		if depID == 0 {
			continue
		}

		path := fmt.Sprintf("/repos/%s/%s/issues/%d/dependencies", owner, repo, index)
		body := map[string]interface{}{"id": depID}
		resp, err := giteaRequest("POST", path, body)
		if err != nil {
			continue
		}
		resp.Body.Close()
	}
}

func resolveIssueID(owner, repo string, index int64) int64 {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, index)
	resp, err := giteaRequest("GET", path, nil)
	if err != nil {
		return 0
	}
	var issue GiteaIssue
	if err := decodeResponse(resp, &issue); err != nil {
		return 0
	}
	return issue.ID
}

// --- sync.delete ---

func handleSyncDelete(req Request) Response {
	var params SyncDeleteParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errResponse(req.ID, -32602, "Invalid params")
	}

	owner, repo, err := splitRepo(params.Repo)
	if err != nil {
		return errResponse(req.ID, -32602, err.Error())
	}

	deleted := []string{}
	failed := map[string]string{}

	for _, task := range params.Tasks {
		originIDStr, _ := task.Meta["origin_id"].(string)
		index, err := strconv.ParseInt(originIDStr, 10, 64)
		if err != nil {
			failed[task.ID] = fmt.Sprintf("invalid origin_id %q", originIDStr)
			continue
		}

		path := fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, index)
		body := map[string]string{"state": "closed"}
		resp, err := giteaRequest("PATCH", path, body)
		if err != nil {
			failed[task.ID] = err.Error()
			continue
		}
		if err := decodeResponse(resp, nil); err != nil {
			failed[task.ID] = err.Error()
			continue
		}
		deleted = append(deleted, task.ID)
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

// --- auth.status ---

func handleAuthStatus(req Request) Response {
	return Response{
		JSONRPC: "2.0",
		Result: map[string]interface{}{
			"authenticated": giteaToken() != "",
		},
		ID: req.ID,
	}
}

// --- Helpers ---

func splitRepo(repoFull string) (string, string, error) {
	parts := strings.SplitN(repoFull, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repo format %q, expected owner/repo", repoFull)
	}
	return parts[0], parts[1], nil
}

func sendResponse(resp Response) {
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

func sendError(id interface{}, code int, message string, data interface{}) {
	resp := Response{
		JSONRPC: "2.0",
		Error: &RPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
		ID: id,
	}
	sendResponse(resp)
}

func errResponse(id interface{}, code int, msg string) Response {
	return Response{
		JSONRPC: "2.0",
		Error:   &RPCError{Code: code, Message: msg},
		ID:      id,
	}
}
