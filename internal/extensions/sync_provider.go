package extensions

import (
	"context"

	"hop.top/kit/go/ai/ext"
	"hop.top/tlc/internal/auth"
)

// SyncProvider is the interface that sync extensions register via CapRegistry.
// It exposes the authenticator and service name so the sync subsystem can
// resolve credentials without reaching into internal/auth directly.
type SyncProvider interface {
	// ServiceName returns the credential service key (e.g. "github").
	ServiceName() string
	// Store returns the credential store for this provider.
	Store() auth.Store
}

// syncExtBase is the shared scaffold for GitHub/Jira/Linear sync extensions.
type syncExtBase struct {
	meta  ext.Metadata
	caps  ext.Capability
	store auth.Store
	svc   string
}

func (s *syncExtBase) Meta() ext.Metadata        { return s.meta }
func (s *syncExtBase) Capabilities() ext.Capability { return s.caps }
func (s *syncExtBase) Init(_ context.Context) error  { return nil }
func (s *syncExtBase) Close() error                  { return nil }
func (s *syncExtBase) ServiceName() string           { return s.svc }
func (s *syncExtBase) Store() auth.Store             { return s.store }

// defaultCaps is the capability set shared by all built-in sync extensions.
// CapHook is included as a stub — the bus is not wired until Track C.
var defaultCaps = ext.CapRegistry | ext.CapHook | ext.CapConfig
