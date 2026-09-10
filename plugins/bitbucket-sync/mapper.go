package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"

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
	Title     string              `json:"title"`
	Content   *BitbucketContent   `json:"content,omitempty"`
	State     string              `json:"state,omitempty"`
	Priority  string              `json:"priority,omitempty"`
	Kind      string              `json:"kind,omitempty"`
	Assignee  *BitbucketUser      `json:"assignee,omitempty"`
	Component *BitbucketComponent `json:"component,omitempty"`
}

var blockedByRegex = regexp.MustCompile(`(?i)blocked\s+by\s+#(\d+)`)

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

// MapBitbucketIssueToTask maps a Bitbucket issue to a TLC task.
func MapBitbucketIssueToTask(issue *BitbucketIssue, components []string) *Task {
	return MapBitbucketIssueToTaskWith(issue, components, nil)
}

// MapBitbucketIssueToTaskWith is MapBitbucketIssueToTask against the
// vocabulary the host sent.
func MapBitbucketIssueToTaskWith(issue *BitbucketIssue, components []string, vocab *Vocabulary) *Task {
	body := ""
	if issue.Content != nil {
		body = issue.Content.Raw
	}

	status := mapBitbucketStateToStatusWith(issue.State, components, vocab)
	blockedReason := ""
	if status == vocab.initialStatus() && containsComponent(components, blockedLabel) {
		blockedReason = "blocked (from Bitbucket status:blocked label)"
	}

	task := &Task{
		ID:            recoverOrMintTaskID(body),
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
	mapComponentsToTaskWith(task, components, vocab)

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
	return mapBitbucketStateToStatusWith(state, components, nil)
}

// mapBitbucketStateToStatusWith resolves the status through the
// vocabulary, so a renamed active or terminal status survives the trip.
func mapBitbucketStateToStatusWith(state string, components []string, vocab *Vocabulary) string {
	initial := vocab.initialStatus()
	switch state {
	case "new":
		return initial
	case "open":
		for _, c := range components {
			if st, ok := vocab.activeStatusFor(strings.ToLower(c)); ok {
				return st
			}
		}
		// blocked stays in the initial status but sets BlockedReason
		return initial
	case "resolved", "closed":
		return vocab.terminalFor("completed")
	case "wontfix", "invalid":
		return vocab.terminalFor("not_planned")
	default:
		return initial
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
	mapComponentsToTaskWith(task, components, nil)
}

// mapComponentsToTaskWith is mapComponentsToTask against the vocabulary
// the host sent.
func mapComponentsToTaskWith(task *Task, components []string, vocab *Vocabulary) {
	for _, comp := range components {
		parts := strings.SplitN(comp, ":", 2)
		if len(parts) != 2 {
			continue // ignore flat labels
		}
		prefix, value := parts[0], parts[1]

		switch prefix {
		case "priority":
			if mapped, ok := vocab.priorityFor(strings.ToLower(comp)); ok {
				task.Priority = mapped
			} else if mapped := mapLabelPriority(value); mapped != "" {
				task.Priority = mapped
			}
		case "effort":
			if mapped, ok := vocab.effortFor(strings.ToLower(comp)); ok {
				task.Effort = mapped
			} else {
				task.Effort = strings.ToUpper(value)
			}
		case "status":
			// handled in status mapping, skip
		default:
			// Any remaining dim:value label goes into Tags
			task.Tags = append(task.Tags, comp)
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
func MapTaskToBitbucketIssue(task *Task) *BitbucketIssueRequest {
	return MapTaskToBitbucketIssueWith(task, nil)
}

// MapTaskToBitbucketIssueWith is MapTaskToBitbucketIssue against the
// vocabulary the host sent.
func MapTaskToBitbucketIssueWith(task *Task, vocab *Vocabulary) *BitbucketIssueRequest {
	state := mapStatusToBitbucketStateWith(task.Status, vocab)
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

	// Embed tlc-uid footer so a later sync.pull can recover this task's
	// tlc identity. Only emits when task.ID is a valid TypeID.
	body = embedTLCUIDFooter(body, task.ID)

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

	// Build component label from task fields for BB component field
	var labels []string
	if l, ok := vocab.priorityLabel(task.Priority); ok {
		labels = append(labels, l)
	}
	if l, ok := vocab.effortLabel(task.Effort); ok {
		labels = append(labels, l)
	}
	labels = append(labels, task.Tags...)
	if l, ok := vocab.statusLabel(task.Status); ok {
		labels = append(labels, l)
	}
	if task.BlockedReason != "" {
		labels = append(labels, blockedLabel)
	}

	// Store labels as comma-separated BB component (BB's only label-like field)
	if len(labels) > 0 {
		req.Component = &BitbucketComponent{
			Name: strings.Join(labels, ","),
		}
	}

	return req
}

// mapStatusToBitbucketState converts TLC status to Bitbucket issue state.
func mapStatusToBitbucketState(status string) string {
	return mapStatusToBitbucketStateWith(status, nil)
}

// mapStatusToBitbucketStateWith closes an issue for any status the
// vocabulary calls terminal, not only the two built-in names.
func mapStatusToBitbucketStateWith(status string, vocab *Vocabulary) string {
	if reason, terminal := vocab.isTerminal(status); terminal {
		if reason == "not_planned" {
			return "invalid"
		}
		return "resolved"
	}
	if status == vocab.initialStatus() {
		return "open"
	}
	if _, ok := vocab.statusLabel(status); ok {
		return "open"
	}
	return "new"
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
		return "" // let BB use its own default
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
