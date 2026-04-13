// Package extensions provides the tlc-specific ext.Manager bootstrap layer.
//
// It initialises the kit/ext Manager, wires capability callbacks, and
// exposes registration + lifecycle methods for CLI startup.
package extensions

import (
	"context"
	"fmt"

	"charm.land/log/v2"
	"hop.top/kit/bus"
	"hop.top/kit/ext"
)

// Manager wraps ext.Manager with tlc-specific configuration and
// built-in extension registration.
type Manager struct {
	core *ext.Manager
	bus  bus.Bus
}

// New creates a Manager and wires capability callbacks.
// Call RegisterBuiltins to add built-in extensions after creation.
// Pass nil logger to disable debug output. Pass nil bus to disable
// hook subscriptions (extensions with CapHook will be logged but
// not wired).
func New(logger *log.Logger, b bus.Bus) *Manager {
	m := &Manager{core: ext.NewManager(logger), bus: b}

	// Wire capability callbacks before adding extensions.
	m.core.SetOnRegistry(func(e ext.Extension) {
		if logger != nil {
			logger.Debug("ext/registry: registered", "name", e.Meta().Name)
		}
	})
	m.core.SetOnHook(func(e ext.Extension) {
		if b == nil {
			if logger != nil {
				logger.Debug("ext/hook: no bus; skipping", "name", e.Meta().Name)
			}
			return
		}
		// Subscribe the extension to all tlc lifecycle events.
		b.SubscribeAsync("tlc.#", func(_ context.Context, ev bus.Event) {
			if logger != nil {
				logger.Debug("ext/hook: event dispatched",
					"ext", e.Meta().Name,
					"topic", string(ev.Topic),
				)
			}
		})
		if logger != nil {
			logger.Debug("ext/hook: subscribed to bus", "name", e.Meta().Name)
		}
	})
	m.core.SetOnConfig(func(e ext.Extension) {
		if logger != nil {
			logger.Debug("ext/config: registered", "name", e.Meta().Name)
		}
	})

	return m
}

// Bus returns the event bus, or nil if not configured.
func (m *Manager) Bus() bus.Bus { return m.bus }

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
