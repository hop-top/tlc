package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
	"hop.top/tlc/internal/core"
)

const (
	githubStateClosed = "closed"
	githubStateOpen   = "open"
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

// tlcUIDRe matches the tlc TypeID footer embedded in remote issue bodies.
// Format: <!-- tlc-uid: task_<26 char crockford base32> -->
var tlcUIDRe = regexp.MustCompile(`<!-- tlc-uid: (task_[0-9a-z]{26}) -->`)

// blockedByRe matches "blocked by #N" or "depends on #N" patterns.
var blockedByRe = regexp.MustCompile(`(?i)(?:blocked by|depends on)\s+#(\d+)`)

// blockedByLineRe matches full lines containing only "Blocked by #N" or
// "Depends on #N" references (used to strip stale header lines on push).
var blockedByLineRe = regexp.MustCompile(`(?im)^(?:blocked by|depends on)\s+#\d+\s*$`)

// MapGitHubIssueToTask maps a GitHub issue to a TLC task using the
// built-in vocabulary.
func MapGitHubIssueToTask(issue *github.Issue) *Task {
	return MapGitHubIssueToTaskWith(issue, nil)
}

// MapGitHubIssueToTaskWith is MapGitHubIssueToTask against the vocabulary
// the host sent.
func MapGitHubIssueToTaskWith(issue *github.Issue, vocab *Vocabulary) *Task {
	number := issue.GetNumber()
	title := issue.GetTitle()
	state := issue.GetState()
	body := issue.GetBody()

	task := &Task{
		ID:          recoverOrMintTaskID(body),
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
	activeStatus, hasBlocked := classifyLabels(issue.Labels, task, vocab)

	// Determine status from state + labels + state_reason
	mapStatusFromIssueWith(issue, state, activeStatus, hasBlocked, task, vocab)

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
//
// Retained as the built-in-vocabulary entry point so existing callers and
// tests keep working; the vocabulary-aware form is mapLabelsToTaskWith.
func mapLabelsToTask(labels []*github.Label, task *Task) (hasInProgress, hasBlocked bool) {
	return mapLabelsToTaskWith(labels, task, nil)
}

// mapLabelsToTaskWith is mapLabelsToTask against an explicit vocabulary.
// A nil vocab falls back to the built-in tables, so this is a superset of
// the previous behavior rather than a change to it.
func mapLabelsToTaskWith(labels []*github.Label, task *Task, vocab *Vocabulary) (hasInProgress, hasBlocked bool) {
	active, blocked := classifyLabels(labels, task, vocab)
	return active != "", blocked
}

// classifyLabels is mapLabelsToTaskWith returning the active status NAME
// instead of a flag, for callers that must write the status through.
func classifyLabels(labels []*github.Label, task *Task, vocab *Vocabulary) (activeStatus string, hasBlocked bool) {
	var tags []string
	for _, label := range labels {
		name := label.GetName()
		nameLower := strings.ToLower(name)

		// Priority labels
		if p, ok := vocab.priorityFor(nameLower); ok {
			task.Priority = p
			continue
		}

		// Effort labels
		if e, ok := vocab.effortFor(nameLower); ok {
			task.Effort = e
			continue
		}

		// Status labels — capture flags, don't add to tags
		if nameLower == blockedLabel {
			hasBlocked = true
			continue
		}
		if s, ok := vocab.activeStatusFor(nameLower); ok {
			activeStatus = s
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
	return activeStatus, hasBlocked
}

// mapStatusFromIssue determines the task status from issue state, label flags,
// and state_reason. The hasInProgress/hasBlocked flags are provided by
// mapLabelsToTask to avoid a second label iteration.
func mapStatusFromIssue(issue *github.Issue, state string, hasInProgress, hasBlocked bool, task *Task) {
	active := ""
	if hasInProgress {
		active = builtinActiveStatus
	}
	mapStatusFromIssueWith(issue, state, active, hasBlocked, task, nil)
}

// mapStatusFromIssueWith is mapStatusFromIssue carrying the active status
// NAME rather than a bare flag.
//
// The name matters: an open issue labeled status:doing under a DOING
// vocabulary must pull back as DOING. Collapsing that to a boolean and
// then hardcoding IN_PROGRESS would round-trip the label correctly and
// still write the wrong status into the store.
func mapStatusFromIssueWith(issue *github.Issue, state, activeStatus string, hasBlocked bool, task *Task, vocab *Vocabulary) {
	if state == githubStateClosed {
		reason := issue.GetStateReason()
		if reason == "not_planned" {
			task.Status = "SKIPPED"
		} else {
			task.Status = "DONE"
		}
		return
	}

	initial := builtinInitialStatus
	if vocab != nil && vocab.InitialStatus != "" {
		initial = vocab.InitialStatus
	}

	switch {
	case hasBlocked:
		task.Status = initial
		reason := "blocked"
		task.BlockedReason = &reason
	case activeStatus != "":
		task.Status = activeStatus
	default:
		task.Status = initial
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

// recoverOrMintTaskID returns the tlc TypeID embedded in body's tlc-uid
// footer, or mints a fresh one when no footer is present (or invalid).
// This is what enables sync.pull to update the existing tlc row instead of
// creating a duplicate on every cycle.
func recoverOrMintTaskID(body string) string {
	if m := tlcUIDRe.FindStringSubmatch(body); m != nil && core.IsTaskID(m[1]) {
		return m[1]
	}
	return core.NewTaskID()
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

// MapTaskToGitHubIssueRequest maps a TLC task to a GitHub issue request
// using the built-in vocabulary.
func MapTaskToGitHubIssueRequest(task *Task) *github.IssueRequest {
	return MapTaskToGitHubIssueRequestWith(task, nil)
}

// MapTaskToGitHubIssueRequestWith is MapTaskToGitHubIssueRequest against
// the vocabulary the host sent.
func MapTaskToGitHubIssueRequestWith(task *Task, vocab *Vocabulary) *github.IssueRequest {
	state, stateReason := mapTaskStatusToGitHubWith(task, vocab)

	// Build labels list
	labels := buildPushLabelsWith(task, vocab)

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
	return mapTaskStatusToGitHubWith(task, nil)
}

// mapTaskStatusToGitHubWith closes an issue for any status the vocabulary
// calls terminal, not only the two built-in names.
//
// Without this a DELIVERED/ABANDONED vocabulary would push every task as
// an open issue, and the open/closed bit — which the status axis relies
// on to carry terminality without a label — would stop meaning anything.
func mapTaskStatusToGitHubWith(task *Task, vocab *Vocabulary) (state, stateReason string) {
	if vocab != nil && len(vocab.TerminalStatuses) > 0 {
		if reason, ok := vocab.TerminalStatuses[task.Status]; ok {
			return githubStateClosed, reason
		}
		return githubStateOpen, ""
	}

	switch task.Status {
	case "DONE":
		return githubStateClosed, "completed"
	case "SKIPPED":
		return githubStateClosed, "not_planned"
	default:
		return githubStateOpen, ""
	}
}

// buildPushLabels constructs the label list for a GitHub issue from a task,
// using the built-in vocabulary.
func buildPushLabels(task *Task) []string {
	return buildPushLabelsWith(task, nil)
}

// buildPushLabelsWith is buildPushLabels against an explicit vocabulary.
// A nil vocab falls back to the built-in tables and emits byte-identical
// labels to the version this replaced.
func buildPushLabelsWith(task *Task, vocab *Vocabulary) []string {
	var labels []string

	// Status label. Blocked wins over the status axis because it is an
	// orthogonal flag, not a member of the vocabulary.
	if task.BlockedReason != nil && *task.BlockedReason != "" {
		labels = append(labels, blockedLabel)
	} else if l, ok := vocab.statusLabel(task.Status); ok {
		labels = append(labels, l)
	}
	// Terminal and initial statuses get no status:* label: open/closed
	// already says it.

	// Priority label
	if l, ok := vocab.priorityLabel(task.Priority); ok {
		labels = append(labels, l)
	}

	// Effort label
	if l, ok := vocab.effortLabel(task.Effort); ok {
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
// appends footers (due date + tlc-uid). Existing blocked-by header lines and
// existing footers are stripped first to prevent duplication across cycles.
func buildPushBody(task *Task) string {
	// Unescape backticks that were shell-escaped during task creation.
	body := strings.ReplaceAll(task.Description, "\\`", "`")

	// Strip existing blocked-by/depends-on header lines to avoid duplication.
	body = blockedByLineRe.ReplaceAllString(body, "")

	// Strip existing due-date and tlc-uid footers to avoid duplication.
	body = dueBodyRe.ReplaceAllString(body, "")
	body = tlcUIDRe.ReplaceAllString(body, "")

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

	// Append tlc-uid footer so a later sync.pull can recover this task's
	// tlc identity. Only emit when the task already carries a typeid.
	if core.IsTaskID(task.ID) {
		body += "\n\n<!-- tlc-uid: " + task.ID + " -->"
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
