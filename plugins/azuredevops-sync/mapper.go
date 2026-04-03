package main

import (
	"fmt"
	"strings"
	"time"
)

// Task represents the TLC task structure as used by sync plugins.
type Task struct {
	ID            string                 `json:"id"`
	Title         string                 `json:"title"`
	Description   string                 `json:"description,omitempty"`
	Status        string                 `json:"status"`
	AssignedTo    string                 `json:"assigned_to,omitempty"`
	Tags          []string               `json:"tags,omitempty"`
	Priority      string                 `json:"priority,omitempty"`
	Effort        string                 `json:"effort,omitempty"`
	BlockedReason string                 `json:"blocked_reason,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	Meta          map[string]interface{} `json:"meta,omitempty"`
}

// WorkItemFields holds the ADO work item field values returned by the REST API.
type WorkItemFields map[string]interface{}

// WorkItemRelation represents a single link relation on an ADO work item.
type WorkItemRelation struct {
	Rel        string                 `json:"rel"`
	URL        string                 `json:"url"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// WorkItem represents an Azure DevOps work item from the REST API.
type WorkItem struct {
	ID        int                  `json:"id"`
	Rev       int                  `json:"rev"`
	Fields    WorkItemFields       `json:"fields"`
	Relations []WorkItemRelation   `json:"relations,omitempty"`
	URL       string               `json:"url"`
}

// adoPriorityToTLC maps ADO's native priority (1-4) to TLC priority strings.
var adoPriorityToTLC = map[int]string{
	1: "P0",
	2: "P1",
	3: "P2",
	4: "P3",
}

// tlcPriorityToADO maps TLC priority back to ADO native priority.
var tlcPriorityToADO = map[string]int{
	"P0": 1,
	"P1": 2,
	"P2": 3,
	"P3": 4,
}

// MapWorkItemToTask converts an Azure DevOps work item to a TLC Task.
func MapWorkItemToTask(wi *WorkItem, org, project string) *Task {
	fields := wi.Fields

	title := fieldString(fields, "System.Title")
	description := fieldString(fields, "System.Description")
	state := fieldString(fields, "System.State")
	assignedTo := extractAssignee(fields)
	iterationPath := fieldString(fields, "System.IterationPath")
	areaPath := fieldString(fields, "System.AreaPath")
	createdAt := fieldTime(fields, "System.CreatedDate")
	updatedAt := fieldTime(fields, "System.ChangedDate")

	// Collect raw tags from ADO tag field.
	rawTags := fieldString(fields, "System.Tags")
	var adoTags []string
	if rawTags != "" {
		for _, t := range strings.Split(rawTags, ";") {
			adoTags = append(adoTags, strings.TrimSpace(t))
		}
	}

	// Derive status from ADO state + tags.
	status, blockedReason := mapStateToStatus(state, adoTags)

	// Derive priority: ADO native field first, then tag override.
	priority := mapADOPriority(fields)

	// Derive effort and dimension tags from ADO tags.
	var effort string
	var tlcTags []string
	for _, tag := range adoTags {
		lower := strings.ToLower(tag)
		switch {
		case strings.HasPrefix(lower, "priority:"):
			// Tag-based priority overrides native ADO priority.
			priority = strings.ToUpper(strings.TrimPrefix(lower, "priority:"))
		case strings.HasPrefix(lower, "effort:"):
			effort = strings.ToUpper(strings.TrimPrefix(lower, "effort:"))
		case strings.HasPrefix(lower, "dimension:"):
			tlcTags = append(tlcTags, tag)
		}
		// Flat tags (no colon prefix) are intentionally ignored.
	}

	// Extract blocked_by from dependency-reverse relations.
	var blockedBy []string
	for _, rel := range wi.Relations {
		if rel.Rel == "System.LinkTypes.Dependency-Reverse" {
			// URL format: https://dev.azure.com/{org}/{project}/_apis/wit/workItems/{id}
			parts := strings.Split(rel.URL, "/")
			if len(parts) > 0 {
				blockedBy = append(blockedBy, "AZ-"+parts[len(parts)-1])
			}
		}
	}

	meta := map[string]interface{}{
		"origin_system":  "azuredevops",
		"origin_id":      fmt.Sprintf("%d", wi.ID),
		"origin_url":     fmt.Sprintf("https://dev.azure.com/%s/%s/_workitems/edit/%d", org, project, wi.ID),
		"iteration_path": iterationPath,
		"area_path":      areaPath,
	}
	if len(blockedBy) > 0 {
		meta["blocked_by"] = blockedBy
	}

	return &Task{
		ID:            fmt.Sprintf("AZ-%d", wi.ID),
		Title:         title,
		Description:   description,
		Status:        status,
		AssignedTo:    assignedTo,
		Tags:          tlcTags,
		Priority:      priority,
		Effort:        effort,
		BlockedReason: blockedReason,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
		Meta:          meta,
	}
}

// mapStateToStatus converts ADO state + status tags to TLC status and blocked reason.
func mapStateToStatus(state string, tags []string) (status string, blockedReason string) {
	hasTag := func(prefix string) bool {
		for _, t := range tags {
			if strings.EqualFold(t, prefix) || strings.HasPrefix(strings.ToLower(t), strings.ToLower(prefix)) {
				return true
			}
		}
		return false
	}

	switch state {
	case "New":
		return "TODO", ""
	case "Active":
		if hasTag("status:blocked") || hasTag("Blocked") {
			return "TODO", "blocked via ADO tag"
		}
		if hasTag("status:in-progress") {
			return "IN_PROGRESS", ""
		}
		return "IN_PROGRESS", ""
	case "Resolved", "Closed":
		return "DONE", ""
	case "Removed":
		return "SKIPPED", ""
	default:
		return "TODO", ""
	}
}

// mapADOPriority extracts TLC priority from the ADO native Priority field.
func mapADOPriority(fields WorkItemFields) string {
	raw, ok := fields["Microsoft.VSTS.Common.Priority"]
	if !ok {
		return ""
	}
	// JSON numbers unmarshal as float64.
	switch v := raw.(type) {
	case float64:
		if p, ok := adoPriorityToTLC[int(v)]; ok {
			return p
		}
	case int:
		if p, ok := adoPriorityToTLC[v]; ok {
			return p
		}
	}
	return ""
}

// MapTaskToWorkItem builds JSON Patch operations for creating/updating an ADO work item.
func MapTaskToWorkItem(task *Task) []PatchOperation {
	ops := []PatchOperation{
		{Op: "add", Path: "/fields/System.Title", Value: task.Title},
		{Op: "add", Path: "/fields/System.Description", Value: task.Description},
		{Op: "add", Path: "/fields/System.State", Value: mapStatusToState(task.Status, task.BlockedReason)},
	}

	if task.AssignedTo != "" {
		ops = append(ops, PatchOperation{
			Op: "add", Path: "/fields/System.AssignedTo", Value: task.AssignedTo,
		})
	}

	if p, ok := tlcPriorityToADO[task.Priority]; ok {
		ops = append(ops, PatchOperation{
			Op: "add", Path: "/fields/Microsoft.VSTS.Common.Priority", Value: p,
		})
	}

	// Build ADO tags from priority, effort, and dimension tags.
	var tagParts []string
	if task.Priority != "" {
		tagParts = append(tagParts, "priority:"+strings.ToLower(task.Priority))
	}
	if task.Effort != "" {
		tagParts = append(tagParts, "effort:"+strings.ToLower(task.Effort))
	}
	tagParts = append(tagParts, task.Tags...)

	// Add status tag for blocked/in-progress states.
	if task.Status == "IN_PROGRESS" {
		tagParts = append(tagParts, "status:in-progress")
	}
	if task.BlockedReason != "" {
		tagParts = append(tagParts, "status:blocked")
	}

	if len(tagParts) > 0 {
		ops = append(ops, PatchOperation{
			Op: "add", Path: "/fields/System.Tags", Value: strings.Join(tagParts, "; "),
		})
	}

	return ops
}

// MapBlockedByToLinkPatches creates JSON Patch ops for adding dependency-reverse links.
func MapBlockedByToLinkPatches(blockedBy []string, org, project string) []PatchOperation {
	var ops []PatchOperation
	for _, dep := range blockedBy {
		// Strip AZ- prefix to get the raw work item ID.
		id := strings.TrimPrefix(dep, "AZ-")
		url := fmt.Sprintf(
			"https://dev.azure.com/%s/%s/_apis/wit/workItems/%s",
			org, project, id,
		)
		ops = append(ops, PatchOperation{
			Op:   "add",
			Path: "/relations/-",
			Value: map[string]interface{}{
				"rel": "System.LinkTypes.Dependency-Reverse",
				"url": url,
			},
		})
	}
	return ops
}

// mapStatusToState converts a TLC status to an ADO work item state.
func mapStatusToState(status, blockedReason string) string {
	switch status {
	case "TODO":
		return "New"
	case "IN_PROGRESS":
		return "Active"
	case "DONE":
		return "Closed"
	case "SKIPPED":
		return "Removed"
	default:
		return "New"
	}
}

// PatchOperation represents a single JSON Patch operation for the ADO REST API.
type PatchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

// --- field extraction helpers ---

func fieldString(fields WorkItemFields, key string) string {
	v, ok := fields[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func fieldTime(fields WorkItemFields, key string) time.Time {
	v, ok := fields[key]
	if !ok || v == nil {
		return time.Time{}
	}
	s, ok := v.(string)
	if !ok {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func extractAssignee(fields WorkItemFields) string {
	raw, ok := fields["System.AssignedTo"]
	if !ok || raw == nil {
		return ""
	}
	// ADO returns AssignedTo as an object with displayName and uniqueName.
	m, ok := raw.(map[string]interface{})
	if !ok {
		// Fallback: plain string.
		s, _ := raw.(string)
		return s
	}
	if name, ok := m["uniqueName"].(string); ok {
		return name
	}
	if name, ok := m["displayName"].(string); ok {
		return name
	}
	return ""
}
