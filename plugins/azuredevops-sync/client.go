package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const adoAPIVersion = "7.0"

// --- WIQL types ---

type wiqlRequest struct {
	Query string `json:"query"`
}

type wiqlResponse struct {
	WorkItems []wiqlWorkItemRef `json:"workItems"`
}

type wiqlWorkItemRef struct {
	ID int `json:"id"`
}

// --- credential helpers ---

func adoCredentials() (org, pat string, err error) {
	pat = os.Getenv("AZURE_DEVOPS_PAT")
	org = os.Getenv("AZURE_DEVOPS_ORG")
	if pat == "" {
		return "", "", fmt.Errorf("AZURE_DEVOPS_PAT not set; export it and retry")
	}
	if org == "" {
		return "", "", fmt.Errorf("AZURE_DEVOPS_ORG not set; export it and retry")
	}
	return org, pat, nil
}

func authHeader(pat string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(":"+pat))
}

// --- ADO REST operations ---

// queryWorkItems runs a WIQL query and returns matching work item IDs.
func queryWorkItems(org, project, lastSyncAt, pat string) ([]int, error) {
	wiql := fmt.Sprintf(
		"SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = '%s'",
		project,
	)
	if lastSyncAt != "" {
		if t, err := time.Parse(time.RFC3339, lastSyncAt); err == nil {
			wiql += fmt.Sprintf(
				" AND [System.ChangedDate] >= '%s'",
				t.Format("2006-01-02T15:04:05Z"),
			)
		}
	}
	wiql += " ORDER BY [System.ChangedDate] DESC"

	body, err := json.Marshal(wiqlRequest{Query: wiql})
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf(
		"https://dev.azure.com/%s/%s/_apis/wit/wiql?api-version=%s",
		org, project, adoAPIVersion,
	)

	respBody, err := adoRequest(http.MethodPost, url, pat, "application/json", body)
	if err != nil {
		return nil, err
	}

	var result wiqlResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse WIQL response: %w", err)
	}

	ids := make([]int, len(result.WorkItems))
	for i, wi := range result.WorkItems {
		ids[i] = wi.ID
	}
	return ids, nil
}

// fetchWorkItems retrieves full work items (with relations) and maps them to Tasks.
func fetchWorkItems(org, project string, ids []int, pat string) ([]*Task, error) {
	const batchSize = 200
	var tasks []*Task

	for i := 0; i < len(ids); i += batchSize {
		end := i + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[i:end]

		idStrs := make([]string, len(batch))
		for j, id := range batch {
			idStrs[j] = fmt.Sprintf("%d", id)
		}

		url := fmt.Sprintf(
			"https://dev.azure.com/%s/%s/_apis/wit/workitems?ids=%s&$expand=relations&api-version=%s",
			org, project, strings.Join(idStrs, ","), adoAPIVersion,
		)

		respBody, err := adoRequest(http.MethodGet, url, pat, "", nil)
		if err != nil {
			return nil, err
		}

		var result struct {
			Value []WorkItem `json:"value"`
		}
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("failed to parse work items response: %w", err)
		}

		for idx := range result.Value {
			tasks = append(tasks, MapWorkItemToTask(&result.Value[idx], org, project))
		}
	}

	return tasks, nil
}

// createWorkItem creates a new work item in ADO using JSON Patch.
func createWorkItem(org, project string, task *Task, pat string) error {
	ops := MapTaskToWorkItem(task)

	if blockedBy, ok := task.Meta["blocked_by"]; ok {
		if deps, ok := blockedBy.([]interface{}); ok {
			var depStrs []string
			for _, d := range deps {
				if s, ok := d.(string); ok {
					depStrs = append(depStrs, s)
				}
			}
			ops = append(ops, MapBlockedByToLinkPatches(depStrs, org, project)...)
		}
	}

	body, err := json.Marshal(ops)
	if err != nil {
		return err
	}

	url := fmt.Sprintf(
		"https://dev.azure.com/%s/%s/_apis/wit/workitems/$Task?api-version=%s",
		org, project, adoAPIVersion,
	)

	respBody, err := adoRequest(http.MethodPost, url, pat, "application/json-patch+json", body)
	if err != nil {
		return err
	}

	var created WorkItem
	if err := json.Unmarshal(respBody, &created); err != nil {
		return fmt.Errorf("failed to parse create response: %w", err)
	}

	if task.Meta == nil {
		task.Meta = make(map[string]interface{})
	}
	task.Meta["origin_system"] = "azuredevops"
	task.Meta["origin_id"] = fmt.Sprintf("%d", created.ID)
	task.Meta["origin_url"] = fmt.Sprintf(
		"https://dev.azure.com/%s/%s/_workitems/edit/%d", org, project, created.ID,
	)

	return nil
}

// updateWorkItem updates an existing ADO work item using JSON Patch.
func updateWorkItem(org, project, originID string, task *Task, pat string) error {
	ops := MapTaskToWorkItem(task)

	body, err := json.Marshal(ops)
	if err != nil {
		return err
	}

	url := fmt.Sprintf(
		"https://dev.azure.com/%s/%s/_apis/wit/workitems/%s?api-version=%s",
		org, project, originID, adoAPIVersion,
	)

	_, err = adoRequest(http.MethodPatch, url, pat, "application/json-patch+json", body)
	return err
}

// deleteWorkItem sets a work item's state to Removed.
func deleteWorkItem(org, project, originID, pat string) error {
	ops := []PatchOperation{
		{Op: "add", Path: "/fields/System.State", Value: "Removed"},
	}

	body, err := json.Marshal(ops)
	if err != nil {
		return err
	}

	url := fmt.Sprintf(
		"https://dev.azure.com/%s/%s/_apis/wit/workitems/%s?api-version=%s",
		org, project, originID, adoAPIVersion,
	)

	_, err = adoRequest(http.MethodPatch, url, pat, "application/json-patch+json", body)
	return err
}

// adoRequest performs an authenticated HTTP request against the ADO REST API.
func adoRequest(method, url, pat, contentType string, body []byte) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", authHeader(pat))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf(
			"ADO API returned %d: %s; check AZURE_DEVOPS_PAT and AZURE_DEVOPS_ORG",
			resp.StatusCode, truncate(string(respBody), 200),
		)
	}

	return respBody, nil
}
