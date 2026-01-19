package core

import (
	"fmt"
	"sort"
)

// AssignmentEngine matches tasks to assignees based on capability requirements.
type AssignmentEngine struct {
	assignees []*Assignee
}

// NewAssignmentEngine creates a new assignment engine with the given assignees.
func NewAssignmentEngine(assignees []*Assignee) *AssignmentEngine {
	return &AssignmentEngine{
		assignees: assignees,
	}
}

// FindBestAssignee finds the best-matching assignee for a task based on requirements.
func (e *AssignmentEngine) FindBestAssignee(task *Task) (*Assignee, float64, error) {
	if len(e.assignees) == 0 {
		return nil, 0, fmt.Errorf("no assignees available")
	}

	requirements := extractTaskRequirements(task)
	if requirements == nil {
		// No specific requirements, return first available assignee
		return e.assignees[0], 0, nil
	}

	type scoredAssignee struct {
		assignee *Assignee
		score    float64
	}

	scores := []scoredAssignee{}

	for _, assignee := range e.assignees {
		score := e.calculateMatchScore(requirements, assignee)
		scores = append(scores, scoredAssignee{assignee: assignee, score: score})
	}

	// Sort by score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	if scores[0].score == 0 {
		return nil, 0, fmt.Errorf("no assignee matches task requirements")
	}

	return scores[0].assignee, scores[0].score, nil
}

// calculateMatchScore computes a weighted score for how well an assignee matches task requirements.
func (e *AssignmentEngine) calculateMatchScore(req *TaskRequirements, assignee *Assignee) float64 {
	score := 0.0

	// Capability matching (highest weight: 10 points per match)
	for _, reqCap := range req.Capabilities {
		if contains(assignee.Capabilities.TaskTypes, reqCap) {
			score += 10.0
		}
	}

	// Domain matching (medium weight: 5 points per match)
	for _, reqDomain := range req.Domains {
		if contains(assignee.Capabilities.Domains, reqDomain) {
			score += 5.0
		}
	}

	// Tool matching (lowest weight: 2 points per match)
	for _, reqTool := range req.Tools {
		if contains(assignee.Capabilities.Tools, reqTool) {
			score += 2.0
		}
	}

	return score
}

// extractTaskRequirements extracts task requirements from task metadata.
func extractTaskRequirements(task *Task) *TaskRequirements {
	if task.Meta == nil {
		return nil
	}

	reqData, ok := task.Meta["requirements"]
	if !ok {
		return nil
	}

	// Type assertion - in production, handle map[string]interface{} properly
	switch v := reqData.(type) {
	case *TaskRequirements:
		return v
	case TaskRequirements:
		return &v
	case map[string]interface{}:
		// Convert from map to TaskRequirements
		req := &TaskRequirements{}

		if caps, ok := v["capabilities"].([]interface{}); ok {
			for _, cap := range caps {
				if capStr, ok := cap.(string); ok {
					req.Capabilities = append(req.Capabilities, capStr)
				}
			}
		}

		if domains, ok := v["domains"].([]interface{}); ok {
			for _, domain := range domains {
				if domainStr, ok := domain.(string); ok {
					req.Domains = append(req.Domains, domainStr)
				}
			}
		}

		if tools, ok := v["tools"].([]interface{}); ok {
			for _, tool := range tools {
				if toolStr, ok := tool.(string); ok {
					req.Tools = append(req.Tools, toolStr)
				}
			}
		}

		return req
	default:
		return nil
	}
}

// contains checks if a string exists in a slice.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
