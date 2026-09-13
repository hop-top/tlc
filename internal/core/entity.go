package core

import "hop.top/kit/go/runtime/domain"

// Compile-time assertions: Task, Track and RecipeRun implement domain.Entity.
var (
	_ domain.Entity = (*Task)(nil)
	_ domain.Entity = (*Track)(nil)
	_ domain.Entity = (*RecipeRun)(nil)
)

// GetID returns the recipe run ID.
func (r RecipeRun) GetID() string { return r.ID }

// GetID returns the task ID.
func (t Task) GetID() string { return t.ID }

// GetID returns the track ID.
func (t Track) GetID() string { return t.ID }
