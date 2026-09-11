package main

import (
	"fmt"
	"regexp"
	"strconv"
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

// GiteaIssue represents a Gitea issue from the REST API.
type GiteaIssue struct {
	ID        int64        `json:"id"`
	Index     int64        `json:"number"`
	Title     string       `json:"title"`
	Body      string       `json:"body"`
	State     string       `json:"state"`
	HTMLURL   string       `json:"html_url"`
	Labels    []GiteaLabel `json:"labels"`
	Assignee  *GiteaUser   `json:"assignee"`
	Milestone *struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	} `json:"milestone"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GiteaLabel represents a Gitea label.
type GiteaLabel struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// GiteaUser represents a Gitea user.
type GiteaUser struct {
	Login string `json:"login"`
}

// GiteaDependency represents an issue dependency from the Gitea API.
type GiteaDependency struct {
	ID    int64 `json:"id"`
	Index int64 `json:"number"`
}

var blockedByRe = regexp.MustCompile(`(?i)(?:blocked by|depends on)\s+#(\d+)`)

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

// builtinLabelToPriority maps priority label names to TLC priority values.
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

// effortToLabel maps effort values to labels.
// builtinLabelToEffort is the pull-direction table. Gitea previously
// parsed the effort:* prefix and accepted the five built-in sizes; the
// table states that same set explicitly so both directions read from
// one place.
var builtinLabelToEffort = map[string]string{
	"effort:xs": "XS",
	"effort:s":  "S",
	"effort:m":  "M",
	"effort:l":  "L",
	"effort:xl": "XL",
}

var builtinEffortToLabel = map[string]string{
	"XS": "effort:xs",
	"S":  "effort:s",
	"M":  "effort:m",
	"L":  "effort:l",
	"XL": "effort:xl",
}

// MapGiteaIssueToTask maps a Gitea issue (with optional dependencies) to a TLC task.
func MapGiteaIssueToTask(issue *GiteaIssue, deps []GiteaDependency) *Task {
	return MapGiteaIssueToTaskWith(issue, deps, nil)
}

// MapGiteaIssueToTaskWith is MapGiteaIssueToTask against the vocabulary
// the host sent.
func MapGiteaIssueToTaskWith(issue *GiteaIssue, deps []GiteaDependency, vocab *Vocabulary) *Task {
	task := &Task{
		ID:          recoverOrMintTaskID(issue.Body),
		Title:       issue.Title,
		Description: issue.Body,
		CreatedAt:   issue.CreatedAt,
		UpdatedAt:   issue.UpdatedAt,
		Meta: map[string]interface{}{
			"origin_system": "gitea",
			"origin_id":     fmt.Sprintf("%d", issue.Index),
			"origin_url":    issue.HTMLURL,
		},
	}

	// Map assignee.
	if issue.Assignee != nil {
		task.AssignedTo = issue.Assignee.Login
	}

	// Map milestone.
	if issue.Milestone != nil {
		task.Meta["milestone"] = issue.Milestone.Title
		task.Meta["milestone_id"] = fmt.Sprintf("%d", issue.Milestone.ID)
	}

	// Process labels for status, priority, effort, and tags.
	activeStatus := ""
	hasStatusBlocked := false
	hasWontfix := false
	var blockedReasonFromLabel string

	for _, label := range issue.Labels {
		name := strings.ToLower(label.Name)

		// Priority labels.
		if p, ok := vocab.priorityFor(name); ok {
			task.Priority = p
			continue
		}

		// Effort labels.
		if e, ok := vocab.effortFor(name); ok {
			task.Effort = e
			continue
		}

		// Dimension/scope tags.
		if strings.HasPrefix(name, "dimension:") {
			tag := strings.TrimPrefix(name, "dimension:")
			task.Tags = append(task.Tags, tag)
			continue
		}

		// Status labels.
		if st, ok := vocab.activeStatusFor(name); ok {
			activeStatus = st
			continue
		}
		if name == blockedLabel {
			hasStatusBlocked = true
			continue
		}
		if name == "wontfix" {
			hasWontfix = true
			continue
		}

		// Check for blocked reason in label like "blocked:reason text".
		if strings.HasPrefix(name, "blocked:") {
			blockedReasonFromLabel = strings.TrimPrefix(name, "blocked:")
			hasStatusBlocked = true
			continue
		}
	}

	// Determine status from state + labels.
	initial := vocab.initialStatus()
	switch {
	case issue.State == "closed" && hasWontfix:
		task.Status = vocab.terminalFor("not_planned")
	case issue.State == "closed":
		task.Status = vocab.terminalFor("completed")
	case issue.State == "open" && activeStatus != "":
		task.Status = activeStatus
	case issue.State == "open" && hasStatusBlocked:
		task.Status = initial
		if blockedReasonFromLabel != "" {
			task.BlockedReason = blockedReasonFromLabel
		} else {
			task.BlockedReason = "blocked"
		}
	default:
		task.Status = initial
	}

	// Parse body for "blocked by #N" / "depends on #N".
	blockedBy := parseBlockedByFromBody(issue.Body)

	// Add dependencies from API.
	for _, dep := range deps {
		ref := fmt.Sprintf("#%d", dep.Index)
		if !containsString(blockedBy, ref) {
			blockedBy = append(blockedBy, ref)
		}
	}

	if len(blockedBy) > 0 {
		task.Meta["blocked_by"] = blockedBy
	}

	return task
}

// parseBlockedByFromBody extracts issue references from body text.
func parseBlockedByFromBody(body string) []string {
	matches := blockedByRe.FindAllStringSubmatch(body, -1)
	var refs []string
	for _, m := range matches {
		ref := "#" + m[1]
		if !containsString(refs, ref) {
			refs = append(refs, ref)
		}
	}
	return refs
}

// MapTaskToGiteaIssue converts a TLC task to Gitea issue create/edit fields.
func MapTaskToGiteaIssue(task *Task) map[string]interface{} {
	return MapTaskToGiteaIssueWith(task, nil)
}

// MapTaskToGiteaIssueWith is MapTaskToGiteaIssue against the vocabulary
// the host sent. A nil vocab falls back to the built-in tables and emits
// byte-identical fields to the version this replaced.
func MapTaskToGiteaIssueWith(task *Task, vocab *Vocabulary) map[string]interface{} {
	state := "open"
	closeReason, terminal := vocab.isTerminal(task.Status)
	if terminal {
		state = "closed"
	}

	body := strings.ReplaceAll(task.Description, "\\`", "`")

	// Build labels from priority, effort, tags, and status.
	var labels []string

	if l, ok := vocab.priorityLabel(task.Priority); ok {
		labels = append(labels, l)
	}
	if l, ok := vocab.effortLabel(task.Effort); ok {
		labels = append(labels, l)
	}
	for _, tag := range task.Tags {
		labels = append(labels, "dimension:"+tag)
	}

	// Status label. Blocked is an orthogonal flag rather than a member
	// of the vocabulary, so it is checked independently of the status.
	if l, ok := vocab.statusLabel(task.Status); ok {
		labels = append(labels, l)
	}
	if !terminal && task.BlockedReason != "" {
		labels = append(labels, blockedLabel)
	}
	if terminal && closeReason == "not_planned" {
		labels = append(labels, "wontfix")
	}

	// Append blocked-by references to body if present.
	if task.Meta != nil {
		if blockedBy, ok := task.Meta["blocked_by"]; ok {
			if refs, ok := blockedBy.([]interface{}); ok && len(refs) > 0 {
				var lines []string
				for _, r := range refs {
					if s, ok := r.(string); ok {
						lines = append(lines, fmt.Sprintf("Blocked by %s", s))
					}
				}
				if len(lines) > 0 {
					body = body + "\n\n" + strings.Join(lines, "\n")
				}
			}
		}
	}

	// Embed tlc-uid footer so a later sync.pull can recover identity.
	body = embedTLCUIDFooter(body, task.ID)

	result := map[string]interface{}{
		"title":  task.Title,
		"body":   body,
		"state":  state,
		"labels": labels,
	}

	if task.AssignedTo != "" {
		result["assignee"] = task.AssignedTo
	}

	return result
}

// BlockedByIssueIndices extracts issue indices from the blocked_by meta field.
func BlockedByIssueIndices(task *Task) []int64 {
	if task.Meta == nil {
		return nil
	}
	blockedBy, ok := task.Meta["blocked_by"]
	if !ok {
		return nil
	}
	refs, ok := blockedBy.([]interface{})
	if !ok {
		return nil
	}
	var indices []int64
	for _, r := range refs {
		s, ok := r.(string)
		if !ok {
			continue
		}
		s = strings.TrimPrefix(s, "#")
		if idx, err := strconv.ParseInt(s, 10, 64); err == nil {
			indices = append(indices, idx)
		}
	}
	return indices
}

func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
