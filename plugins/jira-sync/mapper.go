package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/andygrunwald/go-jira"
	"hop.top/tlc/internal/core"
)

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
}

// tlcUIDRe matches the tlc TypeID footer embedded in remote issue bodies.
// Format: <!-- tlc-uid: task_<26 char crockford base32> -->
var tlcUIDRe = regexp.MustCompile(`<!-- tlc-uid: (task_[0-9a-z]{26}) -->`)

// MapJiraIssueToTask maps a Jira issue to a TLC task.
func MapJiraIssueToTask(issue *jira.Issue) *Task {
	status := "TODO"
	if issue.Fields.Status != nil {
		s := issue.Fields.Status.Name
		switch s {
		case "Done", "Closed", "Resolved":
			status = "DONE"
		case "In Progress":
			status = "IN_PROGRESS"
		}
	}

	body := issue.Fields.Description

	task := &Task{
		ID:          recoverOrMintTaskID(body),
		Title:       issue.Fields.Summary,
		Description: body,
		Status:      status,
		CreatedAt:   time.Time(issue.Fields.Created),
		UpdatedAt:   time.Time(issue.Fields.Updated),
		Meta: map[string]interface{}{
			"origin_system": "jira",
			"origin_id":     issue.ID,
			"origin_key":    issue.Key,
			"origin_url":    fmt.Sprintf("%s/browse/%s", "https://jira.placeholder.com", issue.Key), // Real URL should come from config
		},
	}

	if issue.Fields.Assignee != nil {
		task.AssignedTo = issue.Fields.Assignee.EmailAddress
	}

	if len(issue.Fields.Labels) > 0 {
		task.Tags = issue.Fields.Labels
	}

	return task
}

// MapTaskToJiraDescription returns the Jira issue description body for a TLC
// task, embedding the tlc-uid footer when the task carries a typeid so a
// later sync.pull can recover the original tlc identity.
func MapTaskToJiraDescription(task *Task) string {
	body := task.Description
	// Strip any pre-existing footer to avoid duplication across cycles.
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
