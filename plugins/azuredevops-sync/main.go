package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// --- JSONRPC types ---

// Request is an inbound JSONRPC 2.0 request on stdin.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      interface{}     `json:"id"`
}

// SyncPullParams are the parameters for sync.pull.
type SyncPullParams struct {
	Repo       string `json:"repo"` // ADO project name
	LastSyncAt string `json:"last_sync_at,omitempty"`
}

// SyncPushParams are the parameters for sync.push.
type SyncPushParams struct {
	Repo  string `json:"repo"`
	Tasks []Task `json:"tasks"`
}

// SyncDeleteParams are the parameters for sync.delete.
type SyncDeleteParams struct {
	Repo  string `json:"repo"`
	Tasks []Task `json:"tasks"`
}

// Response is an outbound JSONRPC 2.0 response on stdout.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// RPCError represents a JSONRPC error object.
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// --- entrypoint ---

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	// Allow up to 1 MB per line for large payloads.
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

// --- sync.pull ---

func handleSyncPull(req Request) Response {
	var params SyncPullParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errResp(req.ID, -32602, "Invalid params")
	}

	org, pat, err := adoCredentials()
	if err != nil {
		return errResp(req.ID, -32603, err.Error())
	}

	ids, err := queryWorkItems(org, params.Repo, params.LastSyncAt, pat)
	if err != nil {
		return errResp(req.ID, -32603, fmt.Sprintf("WIQL query failed: %v", err))
	}

	if len(ids) == 0 {
		return Response{
			JSONRPC: "2.0",
			Result: map[string]interface{}{
				"tasks":     []interface{}{},
				"conflicts": []interface{}{},
				"sync_at":   time.Now().Format(time.RFC3339),
			},
			ID: req.ID,
		}
	}

	tasks, err := fetchWorkItems(org, params.Repo, ids, pat)
	if err != nil {
		return errResp(req.ID, -32603, fmt.Sprintf("Failed to fetch work items: %v", err))
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

// --- sync.push ---

func handleSyncPush(req Request) Response {
	var params SyncPushParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errResp(req.ID, -32602, "Invalid params")
	}

	org, pat, err := adoCredentials()
	if err != nil {
		return errResp(req.ID, -32603, err.Error())
	}

	updated := []string{}
	failed := map[string]string{}

	for _, task := range params.Tasks {
		originID, ok := task.Meta["origin_id"].(string)
		if !ok || originID == "" {
			if err := createWorkItem(org, params.Repo, &task, pat); err != nil {
				failed[task.ID] = err.Error()
			} else {
				updated = append(updated, task.ID)
			}
		} else {
			if err := updateWorkItem(org, params.Repo, originID, &task, pat); err != nil {
				failed[task.ID] = err.Error()
			} else {
				updated = append(updated, task.ID)
			}
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

// --- sync.delete ---

func handleSyncDelete(req Request) Response {
	var params SyncDeleteParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errResp(req.ID, -32602, "Invalid params")
	}

	org, pat, err := adoCredentials()
	if err != nil {
		return errResp(req.ID, -32603, err.Error())
	}

	deleted := []string{}
	failed := map[string]string{}

	for _, task := range params.Tasks {
		originID, ok := task.Meta["origin_id"].(string)
		if !ok || originID == "" {
			failed[task.ID] = "origin_id missing; cannot delete work item without origin_id"
			continue
		}
		if err := deleteWorkItem(org, params.Repo, originID, pat); err != nil {
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

// --- auth.status ---

func handleAuthStatus(req Request) Response {
	pat := os.Getenv("AZURE_DEVOPS_PAT")
	org := os.Getenv("AZURE_DEVOPS_ORG")
	return Response{
		JSONRPC: "2.0",
		Result: map[string]interface{}{
			"authenticated": pat != "" && org != "",
		},
		ID: req.ID,
	}
}

// --- output helpers ---

func sendResponse(resp Response) {
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

func sendError(id interface{}, code int, message string, data interface{}) {
	sendResponse(Response{
		JSONRPC: "2.0",
		Error:   &RPCError{Code: code, Message: message, Data: data},
		ID:      id,
	})
}

func errResp(id interface{}, code int, message string) Response {
	return Response{
		JSONRPC: "2.0",
		Error:   &RPCError{Code: code, Message: message},
		ID:      id,
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
