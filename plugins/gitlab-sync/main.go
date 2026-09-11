package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// Request is a JSONRPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      interface{}     `json:"id"`
}

// SyncPullParams holds parameters for the sync.pull method.
type SyncPullParams struct {
	Repo       string      `json:"repo"`
	LastSyncAt string      `json:"last_sync_at,omitempty"`
	Vocabulary *Vocabulary `json:"vocabulary,omitempty"`
}

// SyncPushParams holds parameters for the sync.push method.
type SyncPushParams struct {
	Repo       string      `json:"repo"`
	Tasks      []Task      `json:"tasks"`
	Vocabulary *Vocabulary `json:"vocabulary,omitempty"`
}

// SyncDeleteParams holds parameters for the sync.delete method.
type SyncDeleteParams struct {
	Repo  string `json:"repo"`
	Tasks []Task `json:"tasks"`
}

// Response is a JSONRPC 2.0 response.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// Error is a JSONRPC 2.0 error object.
type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
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
		return errResp(req.ID, -32601, "Method not found")
	}
}

func handleSyncPull(req Request) Response {
	var params SyncPullParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errResp(req.ID, -32602, "Invalid params")
	}

	tasks, err := fetchGitLabIssues(params.Repo, params.LastSyncAt, params.Vocabulary)
	if err != nil {
		return errResp(req.ID, -32603, fmt.Sprintf("Failed to fetch issues: %v", err))
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
		return errResp(req.ID, -32602, "Invalid params")
	}

	client, err := newGitLabClient()
	if err != nil {
		return errResp(req.ID, -32603, fmt.Sprintf("GitLab client: %v", err))
	}

	updated := []string{}
	failed := map[string]string{}
	for _, task := range params.Tasks {
		var opErr error
		originID, ok := task.Meta["origin_id"].(string)
		if !ok || originID == "" {
			opErr = createGitLabIssue(client, params.Repo, &task, params.Vocabulary)
		} else {
			opErr = updateGitLabIssue(client, params.Repo, &task, params.Vocabulary)
		}

		if opErr != nil {
			failed[task.ID] = opErr.Error()
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
		return errResp(req.ID, -32602, "Invalid params")
	}

	client, err := newGitLabClient()
	if err != nil {
		return errResp(req.ID, -32603, fmt.Sprintf("GitLab client: %v", err))
	}

	deleted := []string{}
	failed := map[string]string{}
	for _, task := range params.Tasks {
		if err := closeGitLabIssue(client, params.Repo, &task); err != nil {
			failed[task.ID] = err.Error()
		} else {
			deleted = append(deleted, task.ID)
		}
	}

	// "deleted" is the sync-protocol key shared by all sync plugins
	// (github-sync, gitlab-sync, jira-sync, linear-sync). In GitLab
	// context the issues are closed, not removed — the key name is
	// kept for protocol compatibility.
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
	return Response{
		JSONRPC: "2.0",
		Result: map[string]interface{}{
			"authenticated": os.Getenv("GITLAB_TOKEN") != "",
		},
		ID: req.ID,
	}
}

// newGitLabClient creates a GitLab API client from environment variables.
func newGitLabClient() (*gitlab.Client, error) {
	token := os.Getenv("GITLAB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITLAB_TOKEN not set; export it before running")
	}

	baseURL := os.Getenv("GITLAB_URL")
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}

	client, err := gitlab.NewClient(token, gitlab.WithBaseURL(baseURL+"/api/v4"))
	if err != nil {
		return nil, fmt.Errorf("failed to create gitlab client: %w", err)
	}
	return client, nil
}

// parseRepo splits "owner/repo" into namespace and project name.
func parseRepo(repoFull string) (string, string, error) {
	parts := strings.SplitN(repoFull, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf(
			"invalid repo format %q; expected owner/repo", repoFull,
		)
	}
	return parts[0], parts[1], nil
}

func fetchGitLabIssues(repoFull, lastSyncAt string, vocab *Vocabulary) ([]interface{}, error) {
	client, err := newGitLabClient()
	if err != nil {
		return nil, err
	}

	pid := repoFull // GitLab accepts "namespace/project" as project ID

	opts := &gitlab.ListProjectIssuesOptions{
		State: gitlab.Ptr("all"),
		ListOptions: gitlab.ListOptions{
			PerPage: 100,
		},
	}

	if lastSyncAt != "" {
		if t, parseErr := time.Parse(time.RFC3339, lastSyncAt); parseErr == nil {
			opts.UpdatedAfter = &t
		}
	}

	tasks := []interface{}{}
	for {
		issues, resp, listErr := client.Issues.ListProjectIssues(pid, opts)
		if listErr != nil {
			return nil, fmt.Errorf("failed to list gitlab issues: %w", listErr)
		}

		for _, issue := range issues {
			task := MapGitLabIssueToTaskWith(issue, vocab)
			fetchBlockingLinks(client, pid, issue.IID, task)
			tasks = append(tasks, task)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return tasks, nil
}

// fetchBlockingLinks retrieves issue links and populates blocked_by metadata.
func fetchBlockingLinks(
	client *gitlab.Client, pid string, iid int, task *Task,
) {
	links, _, err := client.IssueLinks.ListIssueRelations(pid, iid, nil)
	if err != nil {
		return // non-fatal: skip link enrichment on error
	}

	var blocked []string
	for _, link := range links {
		if link.LinkType == "is_blocked_by" {
			blocked = append(blocked, fmt.Sprintf("%d", link.IID))
		}
	}
	if len(blocked) > 0 {
		task.Meta["blocked_by"] = strings.Join(blocked, ",")
	}
}

func createGitLabIssue(client *gitlab.Client, repoFull string, task *Task, vocab *Vocabulary) error {
	pid := repoFull
	data := MapTaskToGitLabIssueDataWith(task, vocab)

	opts := &gitlab.CreateIssueOptions{
		Title:       &data.Title,
		Description: &data.Description,
		Labels:      &data.Labels,
	}

	if task.AssignedTo != "" {
		if uid, lookupErr := lookupUserID(client, task.AssignedTo); lookupErr == nil {
			opts.AssigneeIDs = &[]int{uid}
		}
	}

	issue, _, createErr := client.Issues.CreateIssue(pid, opts)
	if createErr != nil {
		return fmt.Errorf("failed to create gitlab issue: %w", createErr)
	}

	if task.Meta == nil {
		task.Meta = make(map[string]interface{})
	}
	task.Meta["origin_system"] = "gitlab"
	task.Meta["origin_id"] = fmt.Sprintf("%d", issue.IID)
	task.Meta["origin_url"] = issue.WebURL

	// Create issue links for blocked_by
	createBlockedByLinks(client, pid, issue.IID, task)

	return nil
}

func updateGitLabIssue(client *gitlab.Client, repoFull string, task *Task, vocab *Vocabulary) error {
	pid := repoFull
	iid, err := extractIID(task)
	if err != nil {
		return err
	}

	data := MapTaskToGitLabIssueDataWith(task, vocab)
	opts := &gitlab.UpdateIssueOptions{
		Title:       &data.Title,
		Description: &data.Description,
		Labels:      &data.Labels,
		StateEvent:  &data.State,
	}

	if task.AssignedTo != "" {
		if uid, lookupErr := lookupUserID(client, task.AssignedTo); lookupErr == nil {
			opts.AssigneeIDs = &[]int{uid}
		}
	}

	_, _, updateErr := client.Issues.UpdateIssue(pid, iid, opts)
	if updateErr != nil {
		return fmt.Errorf("failed to update gitlab issue: %w", updateErr)
	}

	createBlockedByLinks(client, pid, iid, task)

	return nil
}

func closeGitLabIssue(client *gitlab.Client, repoFull string, task *Task) error {
	pid := repoFull
	iid, err := extractIID(task)
	if err != nil {
		return err
	}

	stateEvent := "close"
	opts := &gitlab.UpdateIssueOptions{
		StateEvent: &stateEvent,
	}

	_, _, closeErr := client.Issues.UpdateIssue(pid, iid, opts)
	if closeErr != nil {
		return fmt.Errorf("failed to close gitlab issue: %w", closeErr)
	}
	return nil
}

// createBlockedByLinks creates "is_blocked_by" issue links for blocked_by meta.
func createBlockedByLinks(
	client *gitlab.Client, pid string, iid int, task *Task,
) {
	blockedBy, ok := task.Meta["blocked_by"].(string)
	if !ok || blockedBy == "" {
		return
	}

	for _, ref := range strings.Split(blockedBy, ",") {
		targetRef := strings.TrimSpace(ref)
		if targetRef == "" {
			continue
		}
		// Validate ref is numeric before sending to API.
		if _, convErr := strconv.Atoi(targetRef); convErr != nil {
			continue
		}
		_, _, _ = client.IssueLinks.CreateIssueLink(
			pid, iid,
			&gitlab.CreateIssueLinkOptions{
				TargetProjectID: &pid,
				TargetIssueIID:  &targetRef,
				LinkType:        gitlab.Ptr("is_blocked_by"),
			},
		)
	}
}

func extractIID(task *Task) (int, error) {
	originIDStr, ok := task.Meta["origin_id"].(string)
	if !ok {
		return 0, fmt.Errorf("origin_id missing or not a string for task %s", task.ID)
	}

	iid, err := strconv.Atoi(originIDStr)
	if err != nil {
		return 0, fmt.Errorf("invalid origin_id %q for task %s: %w", originIDStr, task.ID, err)
	}
	return iid, nil
}

// lookupUserID resolves a GitLab username to a user ID.
func lookupUserID(client *gitlab.Client, username string) (int, error) {
	users, _, err := client.Users.ListUsers(&gitlab.ListUsersOptions{
		Username: &username,
	})
	if err != nil || len(users) == 0 {
		return 0, fmt.Errorf("user %q not found", username)
	}
	return users[0].ID, nil
}

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

func errResp(id interface{}, code int, msg string) Response {
	return Response{
		JSONRPC: "2.0",
		Error:   &Error{Code: code, Message: msg},
		ID:      id,
	}
}
