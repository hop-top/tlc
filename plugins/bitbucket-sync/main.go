package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// --- JSONRPC types ---

// Request represents an incoming JSONRPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      interface{}     `json:"id"`
}

// SyncPullParams holds parameters for the sync.pull method.
type SyncPullParams struct {
	Repo       string `json:"repo"`
	LastSyncAt string `json:"last_sync_at,omitempty"`
}

// SyncPushParams holds parameters for the sync.push method.
type SyncPushParams struct {
	Repo  string `json:"repo"`
	Tasks []Task `json:"tasks"`
}

// SyncDeleteParams holds parameters for the sync.delete method.
type SyncDeleteParams struct {
	Repo  string `json:"repo"`
	Tasks []Task `json:"tasks"`
}

// Response represents an outgoing JSONRPC 2.0 response.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// Error represents a JSONRPC error payload.
type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// --- Bitbucket API response types ---

// BitbucketPaginated wraps a paginated response from the Bitbucket API.
type BitbucketPaginated struct {
	Values []json.RawMessage `json:"values"`
	Next   string            `json:"next"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer for large payloads
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

func handleSyncPull(req Request) Response {
	var params SyncPullParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errResponse(req.ID, -32602, "Invalid params")
	}

	tasks, err := fetchBitbucketIssues(params.Repo, params.LastSyncAt)
	if err != nil {
		return errResponse(req.ID, -32603,
			fmt.Sprintf("failed to fetch issues: %v", err))
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

func handleSyncPush(req Request) Response {
	var params SyncPushParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errResponse(req.ID, -32602, "Invalid params")
	}

	updated := []string{}
	failed := map[string]string{}
	for _, task := range params.Tasks {
		var err error
		originID, ok := task.Meta["origin_id"].(string)
		if !ok || originID == "" {
			err = createBitbucketIssue(params.Repo, &task)
		} else {
			err = updateBitbucketIssue(params.Repo, &task)
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

func handleSyncDelete(req Request) Response {
	var params SyncDeleteParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errResponse(req.ID, -32602, "Invalid params")
	}

	deleted := []string{}
	failed := map[string]string{}
	for _, task := range params.Tasks {
		err := deleteBitbucketIssue(params.Repo, &task)
		if err != nil {
			failed[task.ID] = err.Error()
		} else {
			deleted = append(deleted, task.ID)
		}
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

func handleAuthStatus(req Request) Response {
	username := os.Getenv("BITBUCKET_USERNAME")
	password := os.Getenv("BITBUCKET_APP_PASSWORD")
	return Response{
		JSONRPC: "2.0",
		Result: map[string]interface{}{
			"authenticated": username != "" && password != "",
		},
		ID: req.ID,
	}
}

// --- Bitbucket REST API v2.0 helpers ---

const bitbucketAPIBase = "https://api.bitbucket.org/2.0"

func bbCredentials() (string, string, error) {
	username := os.Getenv("BITBUCKET_USERNAME")
	password := os.Getenv("BITBUCKET_APP_PASSWORD")
	if username == "" || password == "" {
		return "", "", fmt.Errorf(
			"BITBUCKET_USERNAME and BITBUCKET_APP_PASSWORD must be set")
	}
	return username, password, nil
}

func bbRequest(method, url string, body interface{}) (*http.Response, error) {
	username, password, err := bbCredentials()
	if err != nil {
		return nil, err
	}

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return http.DefaultClient.Do(req)
}

func parseRepo(repoFull string) (string, string, error) {
	parts := strings.Split(repoFull, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf(
			"invalid repo format %q: expected owner/repo", repoFull)
	}
	return parts[0], parts[1], nil
}

func fetchBitbucketIssues(repoFull, lastSyncAt string) ([]*Task, error) {
	owner, repo, err := parseRepo(repoFull)
	if err != nil {
		return nil, err
	}

	pageURL := fmt.Sprintf(
		"%s/repositories/%s/%s/issues", bitbucketAPIBase, owner, repo)
	if lastSyncAt != "" {
		if _, err := time.Parse(time.RFC3339, lastSyncAt); err != nil {
			return nil, fmt.Errorf(
				"invalid last_sync_at %q: must be RFC3339", lastSyncAt)
		}
		pageURL += "?q=" + url.QueryEscape(
			fmt.Sprintf(`updated_on>"%s"`, lastSyncAt))
	}

	var tasks []*Task
	for pageURL != "" {
		resp, err := bbRequest("GET", pageURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list bitbucket issues: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf(
				"bitbucket API returned status %d", resp.StatusCode)
		}

		var page BitbucketPaginated
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		resp.Body.Close()

		for _, raw := range page.Values {
			var issue BitbucketIssue
			if err := json.Unmarshal(raw, &issue); err != nil {
				continue
			}

			// Component is already in the list response
			var components []string
			if issue.Component != nil && issue.Component.Name != "" {
				components = strings.Split(issue.Component.Name, ",")
			}
			tasks = append(tasks, MapBitbucketIssueToTask(&issue, components))
		}

		pageURL = page.Next
	}

	return tasks, nil
}

func createBitbucketIssue(repoFull string, task *Task) error {
	owner, repo, err := parseRepo(repoFull)
	if err != nil {
		return err
	}

	issueReq := MapTaskToBitbucketIssue(task)
	url := fmt.Sprintf("%s/repositories/%s/%s/issues",
		bitbucketAPIBase, owner, repo)

	resp, err := bbRequest("POST", url, issueReq)
	if err != nil {
		return fmt.Errorf("failed to create bitbucket issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf(
			"failed to create issue, status %d: %s",
			resp.StatusCode, string(body))
	}

	var created BitbucketIssue
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return fmt.Errorf("failed to decode created issue: %w", err)
	}

	if task.Meta == nil {
		task.Meta = make(map[string]interface{})
	}
	task.Meta["origin_system"] = "bitbucket"
	task.Meta["origin_id"] = fmt.Sprintf("%d", created.ID)

	return nil
}

func updateBitbucketIssue(repoFull string, task *Task) error {
	owner, repo, err := parseRepo(repoFull)
	if err != nil {
		return err
	}

	originIDStr, ok := task.Meta["origin_id"].(string)
	if !ok {
		return fmt.Errorf("origin_id missing or not a string")
	}

	issueReq := MapTaskToBitbucketIssue(task)
	url := fmt.Sprintf("%s/repositories/%s/%s/issues/%s",
		bitbucketAPIBase, owner, repo, originIDStr)

	resp, err := bbRequest("PUT", url, issueReq)
	if err != nil {
		return fmt.Errorf("failed to update bitbucket issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf(
			"failed to update issue, status %d: %s",
			resp.StatusCode, string(body))
	}

	return nil
}

func deleteBitbucketIssue(repoFull string, task *Task) error {
	owner, repo, err := parseRepo(repoFull)
	if err != nil {
		return err
	}

	originIDStr, ok := task.Meta["origin_id"].(string)
	if !ok {
		return fmt.Errorf("origin_id missing or not a string")
	}

	// Bitbucket doesn't support true deletion; resolve/close the issue.
	// BB v2 PUT requires title; fetch current issue to get it.
	issueURL := fmt.Sprintf("%s/repositories/%s/%s/issues/%s",
		bitbucketAPIBase, owner, repo, originIDStr)

	getResp, err := bbRequest("GET", issueURL, nil)
	if err != nil {
		return fmt.Errorf("failed to fetch issue for close: %w", err)
	}

	var existing BitbucketIssue
	decodeErr := json.NewDecoder(getResp.Body).Decode(&existing)
	getResp.Body.Close()
	if decodeErr != nil {
		return fmt.Errorf("failed to decode existing issue: %w", decodeErr)
	}

	closeReq := &BitbucketIssueRequest{
		Title: existing.Title,
		State: "resolved",
	}

	resp, err := bbRequest("PUT", issueURL, closeReq)
	if err != nil {
		return fmt.Errorf("failed to close bitbucket issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf(
			"failed to close issue, status %d: %s",
			resp.StatusCode, string(body))
	}

	return nil
}

// --- JSONRPC response helpers ---

func sendResponse(resp Response) {
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

func sendError(id interface{}, code int, message string, data interface{}) {
	resp := Response{
		JSONRPC: "2.0",
		Error: &Error{
			Code:    code,
			Message: message,
			Data:    data,
		},
		ID: id,
	}
	sendResponse(resp)
}

func errResponse(id interface{}, code int, message string) Response {
	return Response{
		JSONRPC: "2.0",
		Error: &Error{
			Code:    code,
			Message: message,
		},
		ID: id,
	}
}
