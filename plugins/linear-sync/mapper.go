package main

import (
	"regexp"
	"strings"
	"time"

	"hop.top/tlc/internal/core"
)

// tlcUIDRe matches the tlc TypeID footer embedded in remote issue bodies.
// Format: <!-- tlc-uid: task_<26 char crockford base32> -->
var tlcUIDRe = regexp.MustCompile(`<!-- tlc-uid: (task_[0-9a-z]{26}) -->`)

// Task represents the TLC task structure as used by plugins.
type Task struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description,omitempty"`
	Status      string                 `json:"status"`
	AssignedTo  string                 `json:"assigned_to,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Meta        map[string]interface{} `json:"meta,omitempty"`
	DueAt       *time.Time             `json:"due_at,omitempty"`
}

// MapLinearIssueToTask maps a Linear issue to a TLC task.
func MapLinearIssueToTask(issue map[string]interface{}) *Task {
	id := issue["id"].(string)                 //nolint:errcheck // required field from API
	identifier := issue["identifier"].(string) //nolint:errcheck // required field from API
	title := issue["title"].(string)           //nolint:errcheck // required field from API
	description := ""
	if d, ok := issue["description"].(string); ok {
		description = d
	}

	createdAt, _ := time.Parse(time.RFC3339, issue["createdAt"].(string)) //nolint:errcheck // best-effort parse from API
	updatedAt, _ := time.Parse(time.RFC3339, issue["updatedAt"].(string)) //nolint:errcheck // best-effort parse from API

	state := issue["state"].(map[string]interface{}) //nolint:errcheck // required field from API
	stateName := state["name"].(string)              //nolint:errcheck // required field from API

	status := "TODO"
	switch stateName {
	case "Done", "Canceled":
		status = "DONE"
	case "In Progress":
		status = "IN_PROGRESS"
	}

	task := &Task{
		ID:          recoverOrMintTaskID(description),
		Title:       title,
		Description: description,
		Status:      status,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		Meta: map[string]interface{}{
			"origin_system": "linear",
			"origin_id":     id,
			"origin_key":    identifier,
			"origin_url":    issue["url"].(string), //nolint:errcheck // required field from API
		},
	}

	if assignee, ok := issue["assignee"].(map[string]interface{}); ok {
		task.AssignedTo = assignee["email"].(string) //nolint:errcheck // required field from API
	}

	if labels, ok := issue["labels"].(map[string]interface{}); ok {
		if nodes, ok := labels["nodes"].([]interface{}); ok {
			tags := []string{}
			for _, n := range nodes {
				if label, ok := n.(map[string]interface{}); ok {
					tags = append(tags, label["name"].(string)) //nolint:errcheck // required field from API
				}
			}
			task.Tags = tags
		}
	}

	if dueDateStr, ok := issue["dueDate"].(string); ok && dueDateStr != "" {
		if t, err := time.Parse("2006-01-02", dueDateStr); err == nil {
			task.DueAt = &t
		}
	}

	return task
}

// MapTaskToLinearDescription returns the Linear issue description body for a
// TLC task, embedding the tlc-uid footer when the task carries a typeid so
// a later sync.pull can recover the original tlc identity.
func MapTaskToLinearDescription(task *Task) string {
	body := task.Description
	body = tlcUIDRe.ReplaceAllString(body, "")
	body = strings.TrimRight(body, "\n ")

	if core.IsTaskID(task.ID) {
		if body != "" {
			body += "\n\n"
		}
		body += "<!-- tlc-uid: " + task.ID + " -->"
	}
	return body
}

// recoverOrMintTaskID returns the tlc TypeID embedded in body's tlc-uid
// footer, or mints a fresh one when no footer is present (or invalid).
func recoverOrMintTaskID(body string) string {
	if m := tlcUIDRe.FindStringSubmatch(body); m != nil && core.IsTaskID(m[1]) {
		return m[1]
	}
	return core.NewTaskID()
}
