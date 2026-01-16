package main

import (
	"fmt"
	"time"
	"github.com/andygrunwald/go-jira"
)

// Task represents the TLC task structure as used by plugins
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

// MapJiraIssueToTask maps a Jira issue to a TLC task
func MapJiraIssueToTask(issue *jira.Issue) *Task {
	status := "TODO"
	if issue.Fields.Status != nil {
		s := issue.Fields.Status.Name
		if s == "Done" || s == "Closed" || s == "Resolved" {
			status = "DONE"
		} else if s == "In Progress" {
			status = "IN_PROGRESS"
		}
	}

	task := &Task{
		ID:          issue.Key,
		Title:       issue.Fields.Summary,
		Description: issue.Fields.Description,
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
