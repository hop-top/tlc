package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"go.jetify.com/typeid"
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

// tlcUIDRe matches the tlc TypeID footer embedded in remote issue bodies.
// Mirrors hop.top/tlc/internal/core (kept inline because plugin sub-modules
// cannot import internal/core).
var tlcUIDRe = regexp.MustCompile(`<!-- tlc-uid: (task_[0-9a-z]{26}) -->`)

var taskTypeIDPattern = regexp.MustCompile(`^task_[0-9a-z]{26}$`)

func isTaskID(s string) bool { return taskTypeIDPattern.MatchString(s) }

func newTaskID() string {
	id, err := typeid.WithPrefix("task")
	if err != nil {
		panic("typeid: newTaskID: " + err.Error())
	}
	return id.String()
}

func recoverOrMintTaskID(body string) string {
	if m := tlcUIDRe.FindStringSubmatch(body); m != nil && isTaskID(m[1]) {
		return m[1]
	}
	return newTaskID()
}

func embedTLCUIDFooter(body, taskID string) string {
	body = tlcUIDRe.ReplaceAllString(body, "")
	body = strings.TrimRight(body, "\n ")
	if isTaskID(taskID) {
		if body != "" {
			body += "\n\n"
		}
		body += "<!-- tlc-uid: " + taskID + " -->"
	}
	return body
}

// builtinLabelToPriority maps GitLab priority labels to TLC priorities.
var builtinLabelToPriority = map[string]string{
	"priority:critical": "P0",
	"priority:high":     "P1",
	"priority:medium":   "P2",
	"priority:low":      "P3",
}

// priorityToLabel is the reverse mapping.
var builtinPriorityToLabel = map[string]string{
	"P0": "priority:critical",
	"P1": "priority:high",
	"P2": "priority:medium",
	"P3": "priority:low",
}

// builtinLabelToEffort maps GitLab effort labels to TLC efforts.
var builtinLabelToEffort = map[string]string{
	"effort:xs": "XS",
	"effort:s":  "S",
	"effort:m":  "M",
	"effort:l":  "L",
	"effort:xl": "XL",
}

// effortToLabel is the reverse mapping.
var builtinEffortToLabel = map[string]string{
	"XS": "effort:xs",
	"S":  "effort:s",
	"M":  "effort:m",
	"L":  "effort:l",
	"XL": "effort:xl",
}

// MapGitLabIssueToTask maps a GitLab issue to a TLC task.
func MapGitLabIssueToTask(issue *gitlab.Issue) *Task {
	return MapGitLabIssueToTaskWith(issue, nil)
}

// MapGitLabIssueToTaskWith is MapGitLabIssueToTask against the
// vocabulary the host sent.
func MapGitLabIssueToTaskWith(issue *gitlab.Issue, vocab *Vocabulary) *Task {
	iid := issue.IID
	title := issue.Title
	state := issue.State
	body := issue.Description

	status := deriveStatusWith(state, issue.Labels, vocab)
	blockedReason := deriveBlockedReason(state, issue.Labels)

	task := &Task{
		ID:            recoverOrMintTaskID(body),
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
	parseLabelDimensionsWith(issue.Labels, task, vocab)

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
	return deriveStatusWith(state, labels, nil)
}

// deriveStatusWith resolves the status through the vocabulary, so a
// renamed active status pulls back under its declared name instead of a
// hardcoded IN_PROGRESS.
func deriveStatusWith(state string, labels []string, vocab *Vocabulary) string {
	hasLabel := labelSet(labels)

	if state == "closed" {
		if hasLabel["wontfix"] {
			return vocab.terminalFor("not_planned")
		}
		return vocab.terminalFor("completed")
	}

	// state == "opened"
	for _, l := range labels {
		if st, ok := vocab.activeStatusFor(strings.ToLower(l)); ok {
			return st
		}
	}
	return vocab.initialStatus()
}

func deriveBlockedReason(state string, labels []string) string {
	if state != "opened" {
		return ""
	}
	hasLabel := labelSet(labels)
	if hasLabel[blockedLabel] {
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
	parseLabelDimensionsWith(labels, task, nil)
}

// parseLabelDimensionsWith is parseLabelDimensions against the
// vocabulary the host sent.
func parseLabelDimensionsWith(labels []string, task *Task, vocab *Vocabulary) {
	tags := make([]string, 0)
	for _, label := range labels {
		lower := strings.ToLower(label)

		// Priority dimension
		if p, ok := vocab.priorityFor(lower); ok {
			task.Priority = p
			continue
		}

		// Effort dimension
		if e, ok := vocab.effortFor(lower); ok {
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
	return MapTaskToGitLabIssueDataWith(task, nil)
}

// MapTaskToGitLabIssueDataWith is MapTaskToGitLabIssueData against the
// vocabulary the host sent.
func MapTaskToGitLabIssueDataWith(task *Task, vocab *Vocabulary) *GitLabIssueData {
	state := "reopen"
	if _, terminal := vocab.isTerminal(task.Status); terminal {
		state = "close"
	}

	// Build labels from priority, effort, tags, and status
	labels := buildLabelsWith(task, vocab)

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

	// Embed tlc-uid footer so a later sync.pull can recover identity.
	description = embedTLCUIDFooter(description, task.ID)

	return &GitLabIssueData{
		Title:       task.Title,
		Description: description,
		State:       state,
		Labels:      labels,
	}
}

func buildLabels(task *Task) gitlab.LabelOptions {
	return buildLabelsWith(task, nil)
}

// buildLabelsWith is buildLabels against the vocabulary the host sent.
func buildLabelsWith(task *Task, vocab *Vocabulary) gitlab.LabelOptions {
	var labels gitlab.LabelOptions

	closeReason, terminal := vocab.isTerminal(task.Status)

	// Priority
	if l, ok := vocab.priorityLabel(task.Priority); ok {
		labels = append(labels, l)
	}

	// Effort
	if l, ok := vocab.effortLabel(task.Effort); ok {
		labels = append(labels, l)
	}

	// Status label. Blocked is an orthogonal flag rather than a member
	// of the vocabulary, so it is checked independently of the status.
	if l, ok := vocab.statusLabel(task.Status); ok {
		labels = append(labels, l)
	}
	if !terminal && task.BlockedReason != "" {
		labels = append(labels, blockedLabel)
	}

	// Wontfix for a not-planned close
	if terminal && closeReason == "not_planned" {
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
