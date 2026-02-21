package core

// Assignee represents a specialized task executor with specific capabilities.
// Assignees are the TLC equivalent of superpowers agents.
type Assignee struct {
	ID           string               `json:"assignee_id" yaml:"assignee_id"`
	Name         string               `json:"name" yaml:"name"`
	Version      string               `json:"version" yaml:"version"`
	Description  string               `json:"description" yaml:"description"`
	Capabilities AssigneeCapabilities `json:"capabilities" yaml:"capabilities"`
	Instructions string               `json:"instructions,omitempty" yaml:"instructions,omitempty"`
	Delegation   *DelegationRules     `json:"delegation,omitempty" yaml:"delegation,omitempty"`
}

// AssigneeCapabilities defines what an assignee can do.
type AssigneeCapabilities struct {
	TaskTypes []string `json:"task_types,omitempty" yaml:"task_types,omitempty"`
	Tools     []string `json:"tools,omitempty" yaml:"tools,omitempty"`
	Domains   []string `json:"domains,omitempty" yaml:"domains,omitempty"`
}

// DelegationRules defines when and how an assignee delegates work.
type DelegationRules struct {
	HandoffConditions []HandoffCondition `json:"handoff_conditions,omitempty" yaml:"handoff_conditions,omitempty"`
	Unblocks          []string           `json:"unblocks,omitempty" yaml:"unblocks,omitempty"`
}

// HandoffCondition specifies when to delegate to another assignee.
type HandoffCondition struct {
	When       string `json:"when" yaml:"when"`
	DelegateTo string `json:"delegate_to" yaml:"delegate_to"`
}

// CanDelegate checks if the assignee can delegate for the given reason.
func (a *Assignee) CanDelegate(reason string) (string, bool) {
	if a.Delegation == nil {
		return "", false
	}

	for _, rule := range a.Delegation.HandoffConditions {
		if rule.When == reason {
			return rule.DelegateTo, true
		}
	}

	return "", false
}

// CheckUnblocks returns the list of task types this assignee can unblock.
func (a *Assignee) CheckUnblocks(_ *Task) []string {
	if a.Delegation == nil {
		return nil
	}

	return a.Delegation.Unblocks
}
