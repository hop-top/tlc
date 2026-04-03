package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
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

var blockedByRe = regexp.MustCompile(`(?i)blocked\s+by\s+#(\d+)`)

// priorityLabelMap maps GitLab priority labels to TLC priorities.
var priorityLabelMap = map[string]string{
	"priority:critical": "P0",
	"priority:high":     "P1",
	"priority:medium":   "P2",
	"priority:low":      "P3",
}

// priorityToLabel is the reverse mapping.
var priorityToLabel = map[string]string{
	"P0": "priority:critical",
	"P1": "priority:high",
	"P2": "priority:medium",
	"P3": "priority:low",
}

// effortLabelMap maps GitLab effort labels to TLC efforts.
var effortLabelMap = map[string]string{
	"effort:xs": "XS",
	"effort:s":  "S",
	"effort:m":  "M",
	"effort:l":  "L",
	"effort:xl": "XL",
}

// effortToLabel is the reverse mapping.
var effortToLabel = map[string]string{
	"XS": "effort:xs",
	"S":  "effort:s",
	"M":  "effort:m",
	"L":  "effort:l",
	"XL": "effort:xl",
}

// MapGitLabIssueToTask maps a GitLab issue to a TLC task.
func MapGitLabIssueToTask(issue *gitlab.Issue) *Task {
	iid := issue.IID
	title := issue.Title
	state := issue.State
	body := issue.Description

	status := deriveStatus(state, issue.Labels)
	blockedReason := deriveBlockedReason(state, issue.Labels)

	task := &Task{
		ID:            fmt.Sprintf("GL-%d", iid),
		Title:         title,
		Description:   body,
		Status:        status,
		BlockedReason: blockedReason,
		CreatedAt:     safeTime(issue.CreatedAt),
		UpdatedAt:     safeTime(issue.UpdatedAt),
		Meta: map[string]interface{}{
			"origin_system": "gitlab",
			"origin_id":     fmt.Sprintf("%d", iid),
			"origin_url":    issue.WebURL,
		},
	}

	// Map assignee
	if issue.Assignee != nil {
		task.AssignedTo = issue.Assignee.Username
	}

	// Parse labels into priority, effort, and tags
	parseLabelDimensions(issue.Labels, task)

	// Map milestone
	if issue.Milestone != nil {
		task.Meta["milestone"] = issue.Milestone.Title
		task.Meta["milestone_id"] = fmt.Sprintf("%d", issue.Milestone.ID)
	}

	// Parse body for "blocked by #N"
	if matches := blockedByRe.FindAllStringSubmatch(body, -1); len(matches) > 0 {
		blocked := make([]string, 0, len(matches))
		for _, m := range matches {
			blocked = append(blocked, m[1])
		}
		task.Meta["blocked_by"] = strings.Join(blocked, ",")
	}

	return task
}

func deriveStatus(state string, labels []string) string {
	hasLabel := labelSet(labels)

	if state == "closed" {
		if hasLabel["wontfix"] {
			return "SKIPPED"
		}
		return "DONE"
	}

	// state == "opened"
	if hasLabel["status:in-progress"] {
		return "IN_PROGRESS"
	}
	return "TODO"
}

func deriveBlockedReason(state string, labels []string) string {
	if state != "opened" {
		return ""
	}
	hasLabel := labelSet(labels)
	if hasLabel["status:blocked"] {
		return "blocked (see linked issues)"
	}
	return ""
}

func labelSet(labels []string) map[string]bool {
	m := make(map[string]bool, len(labels))
	for _, l := range labels {
		m[strings.ToLower(l)] = true
	}
	return m
}

func parseLabelDimensions(labels []string, task *Task) {
	tags := make([]string, 0)
	for _, label := range labels {
		lower := strings.ToLower(label)

		// Priority dimension
		if p, ok := priorityLabelMap[lower]; ok {
			task.Priority = p
			continue
		}

		// Effort dimension
		if e, ok := effortLabelMap[lower]; ok {
			task.Effort = e
			continue
		}

		// Status labels are consumed by deriveStatus, skip them
		if strings.HasPrefix(lower, "status:") {
			continue
		}

		// Scoped labels (dimension:scope) become tags
		if strings.Contains(lower, ":") {
			tags = append(tags, label)
			continue
		}

		// Flat labels are ignored per spec
	}
	if len(tags) > 0 {
		task.Tags = tags
	}
}

// MapTaskToGitLabIssue maps a TLC task to GitLab issue create/update options.
type GitLabIssueData struct {
	Title       string
	Description string
	State       string
	Labels      gitlab.LabelOptions
	AssigneeID  int
}

// MapTaskToGitLabIssueData converts a TLC task to GitLab issue fields.
func MapTaskToGitLabIssueData(task *Task) *GitLabIssueData {
	state := "reopen"
	if task.Status == "DONE" || task.Status == "SKIPPED" {
		state = "close"
	}

	// Build labels from priority, effort, tags, and status
	labels := buildLabels(task)

	// Inject "Blocked by #N" into description if present
	description := strings.ReplaceAll(task.Description, "\\`", "`")
	if blockedBy, ok := task.Meta["blocked_by"].(string); ok && blockedBy != "" {
		parts := strings.Split(blockedBy, ",")
		var lines []string
		for _, p := range parts {
			lines = append(lines, fmt.Sprintf("Blocked by #%s", strings.TrimSpace(p)))
		}
		suffix := "\n\n" + strings.Join(lines, "\n")
		if !strings.Contains(description, "Blocked by #") {
			description += suffix
		}
	}

	return &GitLabIssueData{
		Title:       task.Title,
		Description: description,
		State:       state,
		Labels:      labels,
	}
}

func buildLabels(task *Task) gitlab.LabelOptions {
	var labels gitlab.LabelOptions

	// Priority
	if l, ok := priorityToLabel[task.Priority]; ok {
		labels = append(labels, l)
	}

	// Effort
	if l, ok := effortToLabel[task.Effort]; ok {
		labels = append(labels, l)
	}

	// Status label
	switch task.Status {
	case "IN_PROGRESS":
		labels = append(labels, "status:in-progress")
	case "TODO":
		if task.BlockedReason != "" {
			labels = append(labels, "status:blocked")
		}
	}

	// Wontfix for SKIPPED
	if task.Status == "SKIPPED" {
		labels = append(labels, "wontfix")
	}

	// Tags are scoped labels
	labels = append(labels, task.Tags...)

	return labels
}

func safeTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
