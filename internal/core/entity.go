package core

import "hop.top/kit/domain"

// Compile-time assertions: Task, Track, and FlowRun implement domain.Entity.
var (
	_ domain.Entity = (*Task)(nil)
	_ domain.Entity = (*Track)(nil)
	_ domain.Entity = (*FlowRun)(nil)
)

// CompoundKey builds a composite key from projectID and entityID.
// Returns entityID alone when projectID is empty.
func CompoundKey(projectID, entityID string) string {
	if projectID == "" {
		return entityID
	}
	return projectID + ":" + entityID
}

// GetID returns a compound key encoding project_id:id for composite PK support.
func (t *Task) GetID() string {
	var pid string
	if t.ProjectID != nil {
		pid = *t.ProjectID
	}
	return CompoundKey(pid, t.ID)
}

// GetID returns a compound key encoding project_id:id for composite PK support.
func (t *Track) GetID() string {
	var pid string
	if t.ProjectID != nil {
		pid = *t.ProjectID
	}
	return CompoundKey(pid, t.ID)
}

// GetID returns the flow run ID. FlowRun has a simple (non-composite) PK.
func (r *FlowRun) GetID() string {
	return r.ID
}
