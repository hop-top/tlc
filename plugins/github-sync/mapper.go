package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
)

const (
	githubStateClosed = "closed"
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
	BlockedReason *string                `json:"blocked_reason,omitempty"`
	DueAt         *time.Time             `json:"due_at,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	Meta          map[string]interface{} `json:"meta,omitempty"`
}

// Label-prefix to Priority mapping (pull direction).
var labelToPriority = map[string]string{
	"priority:critical": "P0",
	"priority:high":     "P1",
	"priority:medium":   "P2",
	"priority:low":      "P3",
}

// Priority to label mapping (push direction).
var priorityToLabel = map[string]string{
	"P0": "priority:critical",
	"P1": "priority:high",
	"P2": "priority:medium",
	"P3": "priority:low",
}

// Label to Effort mapping (pull direction).
var labelToEffort = map[string]string{
	"effort:xs": "XS",
	"effort:s":  "S",
	"effort:m":  "M",
	"effort:l":  "L",
	"effort:xl": "XL",
}

// Effort to label mapping (push direction).
var effortToLabel = map[string]string{
	"XS": "effort:xs",
	"S":  "effort:s",
	"M":  "effort:m",
	"L":  "effort:l",
	"XL": "effort:xl",
}

// dueBodyRe matches the due date footer convention in issue bodies.
var dueBodyRe = regexp.MustCompile(`<!-- tlc:due (\d{4}-\d{2}-\d{2}) -->`)

// blockedByRe matches "blocked by #N" or "depends on #N" patterns.
var blockedByRe = regexp.MustCompile(`(?i)(?:blocked by|depends on)\s+#(\d+)`)

// blockedByLineRe matches full lines containing only "Blocked by #N" or
// "Depends on #N" references (used to strip stale header lines on push).
var blockedByLineRe = regexp.MustCompile(`(?im)^(?:blocked by|depends on)\s+#\d+\s*$`)

// MapGitHubIssueToTask maps a GitHub issue to a TLC task.
func MapGitHubIssueToTask(issue *github.Issue) *Task {
	number := issue.GetNumber()
	title := issue.GetTitle()
	state := issue.GetState()
	body := issue.GetBody()

	task := &Task{
		ID:          fmt.Sprintf("GH-%d", number),
		Title:       title,
		Description: body,
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

	// Map milestone to meta
	if issue.Milestone != nil {
		task.Meta["milestone"] = issue.Milestone.GetTitle()
		task.Meta["milestone_id"] = fmt.Sprintf("%d", issue.Milestone.GetNumber())
	}

	// Classify labels (also detects status labels in single pass)
	hasInProgress, hasBlocked := mapLabelsToTask(issue.Labels, task)

	// Determine status from state + labels + state_reason
	mapStatusFromIssue(issue, state, hasInProgress, hasBlocked, task)

	// Parse body for blocked-by references
	parseBlockedBy(body, task)

	// Parse due date from body footer: <!-- tlc:due YYYY-MM-DD -->
	if dueAt := parseDueFromBody(body); dueAt != nil {
		task.DueAt = dueAt
	}

	// Fallback: milestone due date
	if task.DueAt == nil && issue.Milestone != nil && issue.Milestone.DueOn != nil {
		d := issue.Milestone.DueOn.Time
		task.DueAt = &d
	}

	// Derive BlockedReason from actual refs when available
	if task.BlockedReason != nil {
		if refs, ok := task.Meta["blocked_by"].([]string); ok && len(refs) > 0 {
			parts := make([]string, len(refs))
			for i, r := range refs {
				parts[i] = "#" + r
			}
			reason := "blocked by " + strings.Join(parts, ", ")
			task.BlockedReason = &reason
		}
	}

	return task
}

// mapLabelsToTask classifies issue labels into priority, effort, tags, and
// detects status labels. Returns status label flags so callers avoid a
// second iteration over labels.
func mapLabelsToTask(labels []*github.Label, task *Task) (hasInProgress, hasBlocked bool) {
	var tags []string
	for _, label := range labels {
		name := label.GetName()
		nameLower := strings.ToLower(name)

		// Priority labels
		if p, ok := labelToPriority[nameLower]; ok {
			task.Priority = p
			continue
		}

		// Effort labels
		if e, ok := labelToEffort[nameLower]; ok {
			task.Effort = e
			continue
		}

		// Status labels — capture flags, don't add to tags
		if nameLower == "status:blocked" {
			hasBlocked = true
			continue
		}
		if nameLower == "status:in-progress" {
			hasInProgress = true
			continue
		}
		if strings.HasPrefix(nameLower, "status:") {
			continue
		}

		// Flat labels (no colon) are ignored
		if !strings.Contains(name, ":") {
			continue
		}

		// Other dimension:scope labels become tags
		tags = append(tags, name)
	}
	if len(tags) > 0 {
		task.Tags = tags
	}
	return hasInProgress, hasBlocked
}

// mapStatusFromIssue determines the task status from issue state, label flags,
// and state_reason. The hasInProgress/hasBlocked flags are provided by
// mapLabelsToTask to avoid a second label iteration.
func mapStatusFromIssue(issue *github.Issue, state string, hasInProgress, hasBlocked bool, task *Task) {
	if state == githubStateClosed {
		reason := issue.GetStateReason()
		if reason == "not_planned" {
			task.Status = "SKIPPED"
		} else {
			task.Status = "DONE"
		}
		return
	}

	switch {
	case hasBlocked:
		task.Status = "TODO"
		reason := "blocked"
		task.BlockedReason = &reason
	case hasInProgress:
		task.Status = "IN_PROGRESS"
	default:
		task.Status = "TODO"
	}
}

// parseBlockedBy extracts "blocked by #N" / "depends on #N" references from
// the issue body and stores them in Meta["blocked_by"].
func parseBlockedBy(body string, task *Task) {
	matches := blockedByRe.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return
	}
	refs := make([]string, 0, len(matches))
	for _, m := range matches {
		refs = append(refs, m[1])
	}
	task.Meta["blocked_by"] = refs
}

// parseDueFromBody extracts a due date from the <!-- tlc:due YYYY-MM-DD --> footer.
func parseDueFromBody(body string) *time.Time {
	m := dueBodyRe.FindStringSubmatch(body)
	if m == nil {
		return nil
	}
	t, err := time.Parse("2006-01-02", m[1])
	if err != nil {
		return nil
	}
	return &t
}

// MapTaskToGitHubIssueRequest maps a TLC task to a GitHub issue request.
func MapTaskToGitHubIssueRequest(task *Task) *github.IssueRequest {
	state, stateReason := mapTaskStatusToGitHub(task)

	// Build labels list
	labels := buildPushLabels(task)

	// Build body: prepend blocked-by lines if present, then original description
	body := buildPushBody(task)

	req := &github.IssueRequest{
		Title: &task.Title,
		Body:  &body,
		State: &state,
	}

	if len(labels) > 0 {
		req.Labels = &labels
	}

	if stateReason != "" {
		req.StateReason = &stateReason
	}

	if task.AssignedTo != "" {
		req.Assignee = &task.AssignedTo
	}

	return req
}

// mapTaskStatusToGitHub returns the GitHub state and state_reason for a task.
func mapTaskStatusToGitHub(task *Task) (state, stateReason string) {
	switch task.Status {
	case "DONE":
		return "closed", "completed"
	case "SKIPPED":
		return "closed", "not_planned"
	default:
		return "open", ""
	}
}

// buildPushLabels constructs the label list for a GitHub issue from a task.
func buildPushLabels(task *Task) []string {
	var labels []string

	// Status label
	if task.BlockedReason != nil && *task.BlockedReason != "" {
		labels = append(labels, "status:blocked")
	} else if task.Status == "IN_PROGRESS" {
		labels = append(labels, "status:in-progress")
	}
	// TODO, DONE, SKIPPED: no status:* label

	// Priority label
	if l, ok := priorityToLabel[task.Priority]; ok {
		labels = append(labels, l)
	}

	// Effort label
	if l, ok := effortToLabel[task.Effort]; ok {
		labels = append(labels, l)
	}

	// Pass through tags
	labels = append(labels, task.Tags...)

	if len(labels) == 0 {
		return nil
	}
	return labels
}

// buildPushBody prepends blocked-by references to the task description and
// appends a due-date footer. Existing blocked-by header lines and due-date
// footers are stripped first to prevent duplication across sync cycles.
func buildPushBody(task *Task) string {
	// Unescape backticks that were shell-escaped during task creation.
	body := strings.ReplaceAll(task.Description, "\\`", "`")

	// Strip existing blocked-by/depends-on header lines to avoid duplication.
	body = blockedByLineRe.ReplaceAllString(body, "")

	// Strip existing due-date footer to avoid duplication.
	body = dueBodyRe.ReplaceAllString(body, "")

	body = strings.TrimLeft(body, "\n")
	body = strings.TrimRight(body, "\n ")

	refs := extractBlockedByRefs(task)
	if len(refs) > 0 {
		var sb strings.Builder
		for _, ref := range refs {
			sb.WriteString(fmt.Sprintf("Blocked by #%s\n", ref))
		}
		sb.WriteString("\n")
		sb.WriteString(body)
		body = sb.String()
	}

	// Append due-date footer
	if task.DueAt != nil {
		body += "\n\n<!-- tlc:due " + task.DueAt.Format("2006-01-02") + " -->"
	}

	return body
}

// extractBlockedByRefs returns the blocked_by refs from task Meta, if any.
func extractBlockedByRefs(task *Task) []string {
	if task.Meta == nil {
		return nil
	}
	raw, ok := task.Meta["blocked_by"]
	if !ok {
		return nil
	}

	// Handle []string (typed)
	if refs, ok := raw.([]string); ok {
		return refs
	}

	// Handle []interface{} (from JSON unmarshalling)
	if arr, ok := raw.([]interface{}); ok {
		refs := make([]string, 0, len(arr))
		for _, v := range arr {
			if s, ok := v.(string); ok {
				refs = append(refs, s)
			}
		}
		return refs
	}

	return nil
}
