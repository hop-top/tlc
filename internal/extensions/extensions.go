// Package extensions provides the tlc-specific ext.Manager bootstrap layer.
//
// It initialises the kit/ext Manager, wires capability callbacks, and
// exposes registration + lifecycle methods for CLI startup.
package extensions

import (
	"context"
	"fmt"

	"charm.land/log/v2"
	"hop.top/kit/ext"
)

// Manager wraps ext.Manager with tlc-specific configuration and
// built-in extension registration.
type Manager struct {
	core *ext.Manager
}

// New creates a Manager and wires capability callbacks.
// Call RegisterBuiltins to add built-in extensions after creation.
// Pass nil logger to disable debug output.
func New(logger *log.Logger) *Manager {
	m := &Manager{core: ext.NewManager(logger)}

	// Wire capability callbacks before adding extensions.
	m.core.SetOnRegistry(func(e ext.Extension) {
		if logger != nil {
			logger.Debug("ext/registry: registered", "name", e.Meta().Name)
		}
	})
	m.core.SetOnHook(func(e ext.Extension) {
		// Stub — bus not available until Track C (kit-bus).
		if logger != nil {
			logger.Debug("ext/hook: stub registered", "name", e.Meta().Name)
		}
	})
	m.core.SetOnConfig(func(e ext.Extension) {
		if logger != nil {
			logger.Debug("ext/config: registered", "name", e.Meta().Name)
		}
	})

	return m
}

// Add registers an extension with the underlying ext.Manager.
func (m *Manager) Add(e ext.Extension) { m.core.Add(e) }

// InitAll initialises every registered extension.
func (m *Manager) InitAll(ctx context.Context) error {
	if err := m.core.InitAll(ctx); err != nil {
		return fmt.Errorf("extensions init: %w", err)
	}
	return nil
}

// CloseAll shuts down every extension in reverse order.
func (m *Manager) CloseAll() []error { return m.core.CloseAll() }

// Extensions returns the registered extensions.
func (m *Manager) Extensions() []ext.Extension { return m.core.Extensions() }
