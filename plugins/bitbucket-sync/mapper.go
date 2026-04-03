package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Task represents the TLC task structure as used by plugins.
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

// BitbucketIssue represents a Bitbucket issue from the REST API v2.0.
type BitbucketIssue struct {
	ID        int                    `json:"id"`
	Title     string                 `json:"title"`
	Content   *BitbucketContent      `json:"content"`
	State     string                 `json:"state"`
	Priority  string                 `json:"priority"`
	Kind      string                 `json:"kind"`
	Assignee  *BitbucketUser         `json:"assignee"`
	Reporter  *BitbucketUser         `json:"reporter"`
	CreatedOn string                 `json:"created_on"`
	UpdatedOn string                 `json:"updated_on"`
	Links     map[string]interface{} `json:"links"`
	Component *BitbucketComponent    `json:"component"`
}

// BitbucketContent holds the rendered and raw markup of an issue body.
type BitbucketContent struct {
	Raw    string `json:"raw"`
	Markup string `json:"markup"`
	HTML   string `json:"html"`
}

// BitbucketUser represents a Bitbucket user reference.
type BitbucketUser struct {
	DisplayName string `json:"display_name"`
	UUID        string `json:"uuid"`
	Nickname    string `json:"nickname"`
}

// BitbucketComponent represents an issue component.
type BitbucketComponent struct {
	Name string `json:"name"`
}

// BitbucketIssueRequest is the payload for creating/updating a Bitbucket issue.
type BitbucketIssueRequest struct {
	Title    string            `json:"title"`
	Content  *BitbucketContent `json:"content,omitempty"`
	State    string            `json:"state,omitempty"`
	Priority string            `json:"priority,omitempty"`
	Kind     string            `json:"kind,omitempty"`
	Assignee *BitbucketUser    `json:"assignee,omitempty"`
}

var blockedByRegex = regexp.MustCompile(`(?i)blocked\s+by\s+#(\d+)`)

// MapBitbucketIssueToTask maps a Bitbucket issue to a TLC task.
func MapBitbucketIssueToTask(issue *BitbucketIssue, components []string) *Task {
	body := ""
	if issue.Content != nil {
		body = issue.Content.Raw
	}

	status := mapBitbucketStateToStatus(issue.State, components)
	blockedReason := ""
	if status == "TODO" && containsComponent(components, "status:blocked") {
		blockedReason = "blocked (from Bitbucket status:blocked label)"
	}

	task := &Task{
		ID:            fmt.Sprintf("BB-%d", issue.ID),
		Title:         issue.Title,
		Description:   body,
		Status:        status,
		BlockedReason: blockedReason,
		Meta: map[string]interface{}{
			"origin_system": "bitbucket",
			"origin_id":     fmt.Sprintf("%d", issue.ID),
		},
	}

	// Parse timestamps
	if t, err := time.Parse(time.RFC3339, issue.CreatedOn); err == nil {
		task.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, issue.UpdatedOn); err == nil {
		task.UpdatedAt = t
	}

	// Map assignee
	if issue.Assignee != nil {
		if issue.Assignee.Nickname != "" {
			task.AssignedTo = issue.Assignee.Nickname
		} else {
			task.AssignedTo = issue.Assignee.DisplayName
		}
	}

	// Map Bitbucket native priority
	task.Priority = mapBitbucketPriority(issue.Priority)

	// Map components as structured labels
	mapComponentsToTask(task, components)

	// Parse body for "blocked by #N"
	if matches := blockedByRegex.FindStringSubmatch(body); len(matches) > 1 {
		task.Meta["blocked_by"] = matches[1]
	}

	// Build origin URL from links
	if issue.Links != nil {
		if htmlLink, ok := issue.Links["html"].(map[string]interface{}); ok {
			if href, ok := htmlLink["href"].(string); ok {
				task.Meta["origin_url"] = href
			}
		}
	}

	return task
}

// mapBitbucketStateToStatus converts Bitbucket issue state + component labels
// to a TLC status.
func mapBitbucketStateToStatus(state string, components []string) string {
	switch state {
	case "new":
		return "TODO"
	case "open":
		if containsComponent(components, "status:in-progress") {
			return "IN_PROGRESS"
		}
		// status:blocked stays TODO but sets BlockedReason
		return "TODO"
	case "resolved", "closed":
		return "DONE"
	case "wontfix", "invalid":
		return "SKIPPED"
	default:
		return "TODO"
	}
}

// mapBitbucketPriority maps Bitbucket native priority to TLC priority.
func mapBitbucketPriority(bbPriority string) string {
	switch strings.ToLower(bbPriority) {
	case "blocker", "critical":
		return "P0"
	case "major":
		return "P1"
	case "minor":
		return "P2"
	case "trivial":
		return "P3"
	default:
		return ""
	}
}

// mapComponentsToTask extracts structured labels from Bitbucket components.
// priority:* -> Priority, effort:* -> Effort, dimension:scope -> Tags.
// Flat labels (no colon) are ignored.
func mapComponentsToTask(task *Task, components []string) {
	for _, comp := range components {
		parts := strings.SplitN(comp, ":", 2)
		if len(parts) != 2 {
			continue // ignore flat labels
		}
		prefix, value := parts[0], parts[1]

		switch prefix {
		case "priority":
			mapped := mapLabelPriority(value)
			if mapped != "" {
				task.Priority = mapped
			}
		case "effort":
			task.Effort = strings.ToUpper(value)
		case "dimension":
			if value == "scope" {
				task.Tags = append(task.Tags, "dimension:scope")
			}
		case "status":
			// handled in status mapping, skip
		}
	}
}

// mapLabelPriority maps a priority label value to TLC priority.
func mapLabelPriority(value string) string {
	switch strings.ToLower(value) {
	case "p0", "critical":
		return "P0"
	case "p1", "high":
		return "P1"
	case "p2", "medium":
		return "P2"
	case "p3", "low":
		return "P3"
	default:
		return ""
	}
}

// MapTaskToBitbucketIssue maps a TLC task to a Bitbucket issue request.
func MapTaskToBitbucketIssue(task *Task) (*BitbucketIssueRequest, []string) {
	state := mapStatusToBitbucketState(task.Status)
	bbPriority := mapTLCPriorityToBitbucket(task.Priority)

	body := task.Description
	// Append blocked-by reference in body (BB has no native linking API)
	if task.Meta != nil {
		if blockedBy, ok := task.Meta["blocked_by"].(string); ok && blockedBy != "" {
			if !blockedByRegex.MatchString(body) {
				if body != "" {
					body += "\n\n"
				}
				body += fmt.Sprintf("Blocked by #%s", blockedBy)
			}
		}
	}

	req := &BitbucketIssueRequest{
		Title:    task.Title,
		Priority: bbPriority,
		State:    state,
		Kind:     "bug", // default kind
	}

	if body != "" {
		req.Content = &BitbucketContent{
			Raw:    body,
			Markup: "markdown",
		}
	}

	if task.AssignedTo != "" {
		req.Assignee = &BitbucketUser{
			Nickname: task.AssignedTo,
		}
	}

	// Build component labels from task fields
	var labels []string
	if task.Priority != "" {
		labels = append(labels, "priority:"+strings.ToLower(task.Priority))
	}
	if task.Effort != "" {
		labels = append(labels, "effort:"+strings.ToLower(task.Effort))
	}
	for _, tag := range task.Tags {
		labels = append(labels, tag)
	}
	// Add status label for in-progress or blocked
	switch task.Status {
	case "IN_PROGRESS":
		labels = append(labels, "status:in-progress")
	}
	if task.BlockedReason != "" {
		labels = append(labels, "status:blocked")
	}

	return req, labels
}

// mapStatusToBitbucketState converts TLC status to Bitbucket issue state.
func mapStatusToBitbucketState(status string) string {
	switch status {
	case "TODO":
		return "open"
	case "IN_PROGRESS":
		return "open"
	case "DONE":
		return "resolved"
	case "SKIPPED":
		return "invalid"
	default:
		return "new"
	}
}

// mapTLCPriorityToBitbucket converts TLC priority to Bitbucket priority.
func mapTLCPriorityToBitbucket(priority string) string {
	switch priority {
	case "P0":
		return "critical"
	case "P1":
		return "major"
	case "P2":
		return "minor"
	case "P3":
		return "trivial"
	default:
		return "major" // sensible default
	}
}

// containsComponent checks if a component name is in the list.
func containsComponent(components []string, name string) bool {
	for _, c := range components {
		if c == name {
			return true
		}
	}
	return false
}
