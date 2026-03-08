package workspace

import (
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// ProjectSource provides task access for a discovered project.
type ProjectSource interface {
	// OpenReadOnly returns a read-only storage handle.
	OpenReadOnly() (core.TaskReader, error)
	// Close releases any resources held by this source.
	Close() error
}

// SpaceAdapter discovers projects within a space.
type SpaceAdapter interface {
	// Name returns the adapter name (e.g. "filesystem").
	Name() string
	// Schemes returns URI schemes this adapter handles.
	Schemes() []string
	// Discover returns projects belonging to this space.
	Discover(space config.SpaceConfig) ([]core.RegisteredProject, error)
	// Contains reports whether a local path falls within this space.
	// Returns the matching project ID or empty string.
	Contains(space config.SpaceConfig, localPath string) string
}
