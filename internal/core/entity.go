package core

import "hop.top/kit/go/runtime/domain"

// Compile-time assertions: Task, Track, and FlowRun implement domain.Entity.
var (
	_ domain.Entity = (*Task)(nil)
	_ domain.Entity = (*Track)(nil)
	_ domain.Entity = (*FlowRun)(nil)
)

// GetID returns the task ID.
func (t Task) GetID() string { return t.ID }

// GetID returns the track ID.
func (t Track) GetID() string { return t.ID }

// GetID returns the flow run ID.
func (r FlowRun) GetID() string { return r.ID }
