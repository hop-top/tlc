package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/machinebox/graphql"
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      interface{}     `json:"id"`
}

type SyncPullParams struct {
	Repo       string `json:"repo"`
	LastSyncAt string `json:"last_sync_at,omitempty"`
}

type SyncPushParams struct {
	Repo  string `json:"repo"` // Team name
	Tasks []Task `json:"tasks"`
}

type SyncDeleteParams struct {
	Repo  string `json:"repo"` // Team name
	Tasks []Task `json:"tasks"`
}

type SyncDeleteResult struct {
	Deleted []string          `json:"deleted"`
	Failed  map[string]string `json:"failed"`
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

		tasks, err := fetchLinearIssues(params.Repo, params.LastSyncAt)
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

		teamName := params.Repo
		result, err := pushToLinear(teamName, params.Tasks)
		if err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
		}

		return Response{
			JSONRPC: "2.0",
			Result:  result,
			ID:      req.ID,
		}
	case "sync.delete":
		var params SyncDeleteParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "Invalid params"}, ID: req.ID}
		}

		teamName := params.Repo
		result, err := deleteFromLinear(teamName, params.Tasks)
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
				"authenticated": os.Getenv("LINEAR_API_KEY") != "",
			},
			ID: req.ID,
		}
	default:
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32601, Message: "Method not found"}, ID: req.ID}
	}
}

func fetchLinearIssues(teamName, lastSyncAt string) ([]interface{}, error) {
	apiKey := os.Getenv("LINEAR_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("LINEAR_API_KEY not set")
	}

	client := graphql.NewClient("https://api.linear.app/graphql")
	ctx := context.Background()

	// First find team ID by name
	teamReq := graphql.NewRequest(`
		query($name: String!) {
			teams(filter: { name: { eq: $name } }) {
				nodes {
					id
				}
			}
		}
	`)
	teamReq.Var("name", teamName)
	teamReq.Header.Set("Authorization", apiKey)

	var teamResp struct {
		Teams struct {
			Nodes []struct {
				ID string `json:"id"`
			} `json:"nodes"`
		} `json:"teams"`
	}

	if err := client.Run(ctx, teamReq, &teamResp); err != nil {
		return nil, fmt.Errorf("failed to query linear teams: %w", err)
	}

	if len(teamResp.Teams.Nodes) == 0 {
		return nil, fmt.Errorf("team not found: %s", teamName)
	}
	teamID := teamResp.Teams.Nodes[0].ID

	// Fetch issues
	filter := fmt.Sprintf(`{ team: { id: { eq: "%s" } } }`, teamID)
	if lastSyncAt != "" {
		t, _ := time.Parse(time.RFC3339, lastSyncAt) //nolint:errcheck // best-effort parse; zero time is safe fallback
		filter = fmt.Sprintf(`{ team: { id: { eq: "%s" } }, updatedAt: { gt: "%s" } }`, teamID, t.Format(time.RFC3339))
	}

	issueReq := graphql.NewRequest(fmt.Sprintf(`
		query {
			issues(filter: %s) {
				nodes {
					id
					identifier
					title
					description
					url
					state {
						name
					}
					assignee {
						email
					}
					labels {
						nodes {
							name
						}
					}
				}
			}
		}
	`, filter))
	issueReq.Header.Set("Authorization", apiKey)

	var issueResp struct {
		Issues struct {
			Nodes []map[string]interface{} `json:"nodes"`
		} `json:"issues"`
	}

	if err := client.Run(ctx, issueReq, &issueResp); err != nil {
		return nil, fmt.Errorf("failed to query linear issues: %w", err)
	}

	tasks := []interface{}{}
	for _, node := range issueResp.Issues.Nodes {
		tasks = append(tasks, MapLinearIssueToTask(node))
	}

	return tasks, nil
}

func pushToLinear(teamName string, tasks []Task) (*SyncPushResult, error) {
	apiKey := os.Getenv("LINEAR_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("LINEAR_API_KEY not set")
	}

	client := graphql.NewClient("https://api.linear.app/graphql")
	ctx := context.Background()

	// Get team ID
	teamReq := graphql.NewRequest(`
		query($name: String!) {
			teams(filter: { name: { eq: $name } }) {
				nodes { id }
			}
		}
	`)
	teamReq.Var("name", teamName)
	teamReq.Header.Set("Authorization", apiKey)
	var teamResp struct {
		Teams struct {
			Nodes []struct{ ID string } `json:"nodes"`
		} `json:"teams"`
	}
	if err := client.Run(ctx, teamReq, &teamResp); err != nil {
		return nil, fmt.Errorf("failed to query linear teams: %w", err)
	}
	if len(teamResp.Teams.Nodes) == 0 {
		return nil, fmt.Errorf("team not found: %s", teamName)
	}
	teamID := teamResp.Teams.Nodes[0].ID

	result := &SyncPushResult{
		Updated: []string{},
		Failed:  make(map[string]string),
	}

	for _, task := range tasks {
		originID, ok := task.Meta["origin_id"].(string)
		var err error
		if !ok || originID == "" {
			err = createLinearIssue(client, ctx, apiKey, teamID, &task)
		} else {
			err = updateLinearIssue(client, ctx, apiKey, originID, &task)
		}

		if err != nil {
			result.Failed[task.ID] = err.Error()
		} else {
			result.Updated = append(result.Updated, task.ID)
		}
	}

	return result, nil
}

func createLinearIssue(client *graphql.Client, ctx context.Context, apiKey, teamID string, task *Task) error {
	req := graphql.NewRequest(`
		mutation($input: IssueCreateInput!) {
			issueCreate(input: $input) {
				success
				issue { id identifier url }
			}
		}
	`)
	req.Var("input", map[string]interface{}{
		"teamId":      teamID,
		"title":       task.Title,
		"description": task.Description,
	})
	req.Header.Set("Authorization", apiKey)

	var resp struct {
		IssueCreate struct {
			Success bool
			Issue   struct {
				ID         string
				Identifier string
				URL        string
			}
		}
	}

	if err := client.Run(ctx, req, &resp); err != nil {
		return fmt.Errorf("failed to create linear issue: %w", err)
	}
	if !resp.IssueCreate.Success {
		return fmt.Errorf("issue creation failed")
	}

	if task.Meta == nil {
		task.Meta = make(map[string]interface{})
	}
	task.Meta["origin_system"] = "linear"
	task.Meta["origin_id"] = resp.IssueCreate.Issue.ID
	task.Meta["origin_key"] = resp.IssueCreate.Issue.Identifier
	task.Meta["origin_url"] = resp.IssueCreate.Issue.URL

	return nil
}

func updateLinearIssue(client *graphql.Client, ctx context.Context, apiKey, issueID string, task *Task) error {
	req := graphql.NewRequest(`
		mutation($id: String!, $input: IssueUpdateInput!) {
			issueUpdate(id: $id, input: $input) {
				success
			}
		}
	`)
	req.Var("id", issueID)
	req.Var("input", map[string]interface{}{
		"title":       task.Title,
		"description": task.Description,
	})
	req.Header.Set("Authorization", apiKey)

	var resp struct {
		IssueUpdate struct {
			Success bool
		}
	}

	if err := client.Run(ctx, req, &resp); err != nil {
		return fmt.Errorf("failed to update linear issue: %w", err)
	}
	if !resp.IssueUpdate.Success {
		return fmt.Errorf("issue update failed")
	}
	return nil
}

func deleteFromLinear(_ string, tasks []Task) (*SyncDeleteResult, error) {
	apiKey := os.Getenv("LINEAR_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("LINEAR_API_KEY not set")
	}

	client := graphql.NewClient("https://api.linear.app/graphql")
	ctx := context.Background()

	result := &SyncDeleteResult{
		Deleted: []string{},
		Failed:  make(map[string]string),
	}

	for _, task := range tasks {
		originID, ok := task.Meta["origin_id"].(string)
		if !ok || originID == "" {
			result.Failed[task.ID] = "origin_id missing"
			continue
		}

		err := deleteLinearIssue(client, ctx, apiKey, originID)
		if err != nil {
			result.Failed[task.ID] = err.Error()
		} else {
			result.Deleted = append(result.Deleted, task.ID)
		}
	}

	return result, nil
}

func deleteLinearIssue(client *graphql.Client, ctx context.Context, apiKey, issueID string) error {
	req := graphql.NewRequest(`
		mutation($id: String!) {
			issueDelete(id: $id) {
				success
			}
		}
	`)
	req.Var("id", issueID)
	req.Header.Set("Authorization", apiKey)

	var resp struct {
		IssueDelete struct {
			Success bool
		}
	}

	if err := client.Run(ctx, req, &resp); err != nil {
		return fmt.Errorf("failed to delete linear issue: %w", err)
	}
	if !resp.IssueDelete.Success {
		return fmt.Errorf("issue deletion failed")
	}
	return nil
}

func sendResponse(resp Response) {
	data, _ := json.Marshal(resp) //nolint:errcheck // marshalling known-valid response struct
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
