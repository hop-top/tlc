package main

import "time"

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

// MapLinearIssueToTask maps a Linear issue to a TLC task.
func MapLinearIssueToTask(issue map[string]interface{}) *Task {
	id := issue["id"].(string)
	identifier := issue["identifier"].(string)
	title := issue["title"].(string)
	description := ""
	if d, ok := issue["description"].(string); ok {
		description = d
	}

	createdAt, _ := time.Parse(time.RFC3339, issue["createdAt"].(string))
	updatedAt, _ := time.Parse(time.RFC3339, issue["updatedAt"].(string))

	state := issue["state"].(map[string]interface{})
	stateName := state["name"].(string)

	status := "TODO"
	switch stateName {
	case "Done", "Canceled":
		status = "DONE"
	case "In Progress":
		status = "IN_PROGRESS"
	}

	task := &Task{
		ID:          identifier,
		Title:       title,
		Description: description,
		Status:      status,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		Meta: map[string]interface{}{
			"origin_system": "linear",
			"origin_id":     id,
			"origin_key":    identifier,
			"origin_url":    issue["url"].(string),
		},
	}

	if assignee, ok := issue["assignee"].(map[string]interface{}); ok {
		task.AssignedTo = assignee["email"].(string)
	}

	if labels, ok := issue["labels"].(map[string]interface{}); ok {
		if nodes, ok := labels["nodes"].([]interface{}); ok {
			tags := []string{}
			for _, n := range nodes {
				if label, ok := n.(map[string]interface{}); ok {
					tags = append(tags, label["name"].(string))
				}
			}
			task.Tags = tags
		}
	}

	return task
}
