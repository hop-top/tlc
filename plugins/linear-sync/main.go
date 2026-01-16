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
	Team        string `json:"team"`
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

		tasks, err := fetchLinearIssues(params.Team, params.LastSyncAt)
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
		return nil, err
	}

	if len(teamResp.Teams.Nodes) == 0 {
		return nil, fmt.Errorf("team not found: %s", teamName)
	}
	teamID := teamResp.Teams.Nodes[0].ID

	// Fetch issues
	filter := fmt.Sprintf(`{ team: { id: { eq: "%s" } } }`, teamID)
	if lastSyncAt != "" {
		t, _ := time.Parse(time.RFC3339, lastSyncAt)
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
		return nil, err
	}

	tasks := []interface{}{}
	for _, node := range issueResp.Issues.Nodes {
		tasks = append(tasks, MapLinearIssueToTask(node))
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
