package main

import (
	"fmt"
	"time"
	"github.com/google/go-github/v69/github"
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

// MapGitHubIssueToTask maps a GitHub issue to a TLC task
func MapGitHubIssueToTask(issue *github.Issue) *Task {
	number := issue.GetNumber()
	title := issue.GetTitle()
	state := issue.GetState()
	body := issue.GetBody()

	status := "TODO"
	if state == "closed" {
		status = "DONE"
	}

	task := &Task{
		ID:          fmt.Sprintf("GH-%d", number),
		Title:       title,
		Description: body,
		Status:      status,
		CreatedAt:   issue.GetCreatedAt().Time,
		UpdatedAt:   issue.GetUpdatedAt().Time,
		Meta: map[string]interface{}{
			"origin_system": "github",
			"origin_id":     fmt.Sprintf("%d", number),
			"origin_url":    issue.GetHTMLURL(),
		},
	}

	// Map assignee
	if issue.Assignee != nil {
		task.AssignedTo = issue.Assignee.GetLogin()
	}

	// Map labels to tags
	if len(issue.Labels) > 0 {
		tags := []string{}
		for _, label := range issue.Labels {
			tags = append(tags, label.GetName())
		}
		task.Tags = tags
	}

	// Map milestone to meta
	if issue.Milestone != nil {
		task.Meta["milestone"] = issue.Milestone.GetTitle()
		task.Meta["milestone_id"] = fmt.Sprintf("%d", issue.Milestone.GetNumber())
	}

	return task
}

// MapTaskToGitHubIssueRequest maps a TLC task to a GitHub issue request
func MapTaskToGitHubIssueRequest(task *Task) *github.IssueRequest {
	state := "open"
	if task.Status == "DONE" || task.Status == "SKIPPED" {
		state = "closed"
	}

	req := &github.IssueRequest{
		Title:    &task.Title,
		Body:     &task.Description,
		State:    &state,
		Labels:   &task.Tags,
	}

	if task.AssignedTo != "" {
		req.Assignee = &task.AssignedTo
	}

	return req
}
