package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
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
