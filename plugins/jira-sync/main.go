package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/andygrunwald/go-jira"
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      interface{}     `json:"id"`
}

type SyncPullParams struct {
	URL         string `json:"url"`
	Project     string `json:"project"`
	LastSyncAt  string `json:"last_sync_at,omitempty"`
}

type SyncPushParams struct {
	URL   string `json:"url"`
	Repo  string `json:"repo"` // This is usually the project key in Jira
	Tasks []Task `json:"tasks"`
}

type SyncPushResult struct {
	Updated []string          `json:"updated"`
	Failed  map[string]string `json:"failed"`
}

type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			sendError(nil, -32700, "Parse error")
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
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "Invalid params"}, ID: req.ID}
		}

		tasks, err := fetchJiraIssues(params.URL, params.Project, params.LastSyncAt)
		if err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
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
	case "sync.push":
		var params SyncPushParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "Invalid params"}, ID: req.ID}
		}

		result, err := pushToJira(params.URL, params.Repo, params.Tasks)
		if err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
		}

		return Response{
			JSONRPC: "2.0",
			Result:  result,
			ID:      req.ID,
		}
	case "auth.status":
		return Response{
			JSONRPC: "2.0",
			Result: map[string]interface{}{
				"authenticated": os.Getenv("JIRA_TOKEN") != "",
			},
			ID: req.ID,
		}
	default:
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32601, Message: "Method not found"}, ID: req.ID}
	}
}

func fetchJiraIssues(url, project, lastSyncAt string) ([]interface{}, error) {
	email := os.Getenv("JIRA_EMAIL")
	token := os.Getenv("JIRA_TOKEN")
	if email == "" || token == "" {
		return nil, fmt.Errorf("JIRA_EMAIL or JIRA_TOKEN not set")
	}

	tp := jira.BasicAuthTransport{
		Username: email,
		Password: token,
	}

	client, err := jira.NewClient(tp.Client(), url)
	if err != nil {
		return nil, err
	}

	jql := fmt.Sprintf("project = %s", project)
	if lastSyncAt != "" {
		// Convert ISO to Jira JQL format if needed
		// For now, simple updated check
		t, _ := time.Parse(time.RFC3339, lastSyncAt)
		jql = fmt.Sprintf("%s AND updated >= \"%s\"", jql, t.Format("2006-01-02 15:04"))
	}

	issues, _, err := client.Issue.Search(jql, nil)
	if err != nil {
		return nil, err
	}

	tasks := []interface{}{}
	for _, issue := range issues {
		tasks = append(tasks, MapJiraIssueToTask(&issue))
	}

	return tasks, nil
}

func pushToJira(url, projectKey string, tasks []Task) (*SyncPushResult, error) {
	email := os.Getenv("JIRA_EMAIL")
	token := os.Getenv("JIRA_TOKEN")
	if email == "" || token == "" {
		return nil, fmt.Errorf("JIRA_EMAIL or JIRA_TOKEN not set")
	}

	tp := jira.BasicAuthTransport{
		Username: email,
		Password: token,
	}

	client, err := jira.NewClient(tp.Client(), url)
	if err != nil {
		return nil, err
	}

	result := &SyncPushResult{
		Updated: []string{},
		Failed:  make(map[string]string),
	}

	for _, task := range tasks {
		originID, ok := task.Meta["origin_id"].(string)
		var err error
		if !ok || originID == "" {
			err = createJiraIssue(client, projectKey, &task)
		} else {
			err = updateJiraIssue(client, originID, &task)
		}

		if err != nil {
			result.Failed[task.ID] = err.Error()
		} else {
			result.Updated = append(result.Updated, task.ID)
		}
	}

	return result, nil
}

func createJiraIssue(client *jira.Client, projectKey string, task *Task) error {
	issue := &jira.Issue{
		Fields: &jira.IssueFields{
			Project: jira.Project{
				Key: projectKey,
			},
			Summary:     task.Title,
			Description: task.Description,
			Type: jira.IssueType{
				Name: "Task", // Default to Task, could be configurable
			},
		},
	}

	newIssue, _, err := client.Issue.Create(issue)
	if err != nil {
		return err
	}

	// Update task metadata with origin info
	if task.Meta == nil {
		task.Meta = make(map[string]interface{})
	}
	task.Meta["origin_system"] = "jira"
	task.Meta["origin_id"] = newIssue.ID
	task.Meta["origin_key"] = newIssue.Key
	// origin_url update is handled by the caller/CLI if needed, 
	// or we can set it here if we have the base URL.
	
	return nil
}

func updateJiraIssue(client *jira.Client, issueID string, task *Task) error {
	issue := &jira.Issue{
		ID: issueID,
		Fields: &jira.IssueFields{
			Summary:     task.Title,
			Description: task.Description,
		},
	}

	_, _, err := client.Issue.Update(issue)
	if err != nil {
		return err
	}

	// Handle status transition
	return transitionJiraIssue(client, issueID, task.Status)
}

func transitionJiraIssue(client *jira.Client, issueID string, status string) error {
	// 1. Get available transitions
	transitions, _, err := client.Issue.GetTransitions(issueID)
	if err != nil {
		return err
	}

	targetStatus := ""
	switch status {
	case "TODO":
		targetStatus = "To Do"
	case "IN_PROGRESS":
		targetStatus = "In Progress"
	case "DONE":
		targetStatus = "Done"
	default:
		return nil // No mapping for other statuses
	}

	for _, t := range transitions {
		if strings.EqualFold(t.To.Name, targetStatus) || strings.EqualFold(t.Name, targetStatus) {
			_, err := client.Issue.DoTransition(issueID, t.ID)
			return err
		}
	}

	return nil // Transition not found or not allowed
}

func sendResponse(resp Response) {
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

func sendError(id interface{}, code int, message string) {
	resp := Response{
		JSONRPC: "2.0",
		Error: &Error{
			Code:    code,
			Message: message,
		},
		ID: id,
	}
	sendResponse(resp)
}
