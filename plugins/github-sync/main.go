package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
	"golang.org/x/oauth2"
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      interface{}     `json:"id"`
}

type SyncPullParams struct {
	Repo       string      `json:"repo"`
	LastSyncAt string      `json:"last_sync_at,omitempty"`
	Vocabulary *Vocabulary `json:"vocabulary,omitempty"`
}

type SyncPushParams struct {
	Repo       string      `json:"repo"`
	Tasks      []Task      `json:"tasks"`
	Vocabulary *Vocabulary `json:"vocabulary,omitempty"`
}

type SyncDeleteParams struct {
	Repo  string `json:"repo"`
	Tasks []Task `json:"tasks"`
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

func main() {
	scanner := bufio.NewScanner(os.Stdin)
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
		var params SyncPullParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{
				JSONRPC: "2.0",
				Error: &Error{
					Code:    -32602,
					Message: "Invalid params",
				},
				ID: req.ID,
			}
		}

		tasks, err := fetchGitHubIssues(params.Repo, params.LastSyncAt, params.Vocabulary)
		if err != nil {
			return Response{
				JSONRPC: "2.0",
				Error: &Error{
					Code:    -32603,
					Message: fmt.Sprintf("Failed to fetch issues: %v", err),
				},
				ID: req.ID,
			}
		}

		return Response{
			JSONRPC: "2.0",
			Result: map[string]interface{}{
				"tasks":     tasks,
				"conflicts": []interface{}{}, // Conflict detection T-0084
				"sync_at":   time.Now().Format(time.RFC3339),
			},
			ID: req.ID,
		}

	case "sync.push":
		var params SyncPushParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{
				JSONRPC: "2.0",
				Error: &Error{
					Code:    -32602,
					Message: "Invalid params",
				},
				ID: req.ID,
			}
		}

		updated := []string{}
		failed := map[string]string{}
		for _, task := range params.Tasks {
			var err error
			originID, ok := task.Meta["origin_id"].(string)
			if !ok || originID == "" {
				err = createGitHubIssue(params.Repo, &task, params.Vocabulary)
			} else {
				err = updateGitHubIssue(params.Repo, &task, params.Vocabulary)
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
	case "sync.delete":
		var params SyncDeleteParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{
				JSONRPC: "2.0",
				Error: &Error{
					Code:    -32602,
					Message: "Invalid params",
				},
				ID: req.ID,
			}
		}

		deleted := []string{}
		failed := map[string]string{}
		for _, task := range params.Tasks {
			err := deleteGitHubIssue(params.Repo, &task)
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
	case "auth.status":
		return Response{
			JSONRPC: "2.0",
			Result: map[string]interface{}{
				"authenticated": os.Getenv("GITHUB_TOKEN") != "",
			},
			ID: req.ID,
		}

	default:
		return Response{
			JSONRPC: "2.0",
			Error: &Error{
				Code:    -32601,
				Message: "Method not found",
			},
			ID: req.ID,
		}
	}
}

// parseRepo splits "owner/repo" into its two components. Both halves
// must be non-empty; the input may not contain a leading or trailing
// slash. GitHub permits hyphens in owner/repo names so the only
// structural rule enforced here is "exactly one slash, both halves
// non-empty".
func parseRepo(repoFull string) (string, string, error) {
	parts := strings.SplitN(repoFull, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.Contains(parts[1], "/") {
		return "", "", fmt.Errorf("invalid repo format %q: expected owner/repo", repoFull)
	}
	return parts[0], parts[1], nil
}

func fetchGitHubIssues(repoFull string, lastSyncAt string, vocab *Vocabulary) ([]interface{}, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN environment variable not set")
	}

	owner, repo, err := parseRepo(repoFull)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	opts := &github.IssueListByRepoOptions{
		State: "all",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	if lastSyncAt != "" {
		if t, err := time.Parse(time.RFC3339, lastSyncAt); err == nil {
			opts.Since = t
		}
	}

	tasks := []interface{}{}
	for {
		issues, resp, err := client.Issues.ListByRepo(ctx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list github issues: %w", err)
		}

		for _, issue := range issues {
			if issue.IsPullRequest() {
				continue
			}
			tasks = append(tasks, MapGitHubIssueToTaskWith(issue, vocab))
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return tasks, nil
}

func createGitHubIssue(repoFull string, task *Task, vocab *Vocabulary) error {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable not set")
	}

	owner, repo, err := parseRepo(repoFull)
	if err != nil {
		return err
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	req := MapTaskToGitHubIssueRequestWith(task, vocab)
	issue, _, err := client.Issues.Create(ctx, owner, repo, req)
	if err != nil {
		return fmt.Errorf("failed to create github issue: %w", err)
	}

	// Update task with origin info
	if task.Meta == nil {
		task.Meta = make(map[string]interface{})
	}
	task.Meta["origin_system"] = "github"
	task.Meta["origin_id"] = fmt.Sprintf("%d", issue.GetNumber())
	task.Meta["origin_url"] = issue.GetHTMLURL()

	return nil
}

func updateGitHubIssue(repoFull string, task *Task, vocab *Vocabulary) error {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable not set")
	}

	owner, repo, err := parseRepo(repoFull)
	if err != nil {
		return err
	}

	originIDStr, ok := task.Meta["origin_id"].(string)
	if !ok {
		return fmt.Errorf("origin_id missing or not a string")
	}

	var issueNumber int
	if _, err := fmt.Sscanf(originIDStr, "%d", &issueNumber); err != nil {
		return fmt.Errorf("invalid origin_id: %w", err)
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	req := MapTaskToGitHubIssueRequestWith(task, vocab)
	if _, _, err := client.Issues.Edit(ctx, owner, repo, issueNumber, req); err != nil {
		return fmt.Errorf("failed to edit github issue: %w", err)
	}
	return nil
}

func deleteGitHubIssue(repoFull string, task *Task) error {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable not set")
	}

	owner, repo, err := parseRepo(repoFull)
	if err != nil {
		return err
	}

	originIDStr, ok := task.Meta["origin_id"].(string)
	if !ok {
		return fmt.Errorf("origin_id missing or not a string")
	}

	var issueNumber int
	if _, err := fmt.Sscanf(originIDStr, "%d", &issueNumber); err != nil {
		return fmt.Errorf("invalid origin_id: %w", err)
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	closed := "closed"
	req := &github.IssueRequest{
		State: &closed,
	}

	if _, _, err := client.Issues.Edit(ctx, owner, repo, issueNumber, req); err != nil {
		return fmt.Errorf("failed to close github issue: %w", err)
	}
	return nil
}

func sendResponse(resp Response) {
	data, _ := json.Marshal(resp) //nolint:errcheck // marshalling known-valid response struct
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
