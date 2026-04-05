package uri

import (
	"context"
	"fmt"
	"sync"
)

// Completer returns a list of suggested values for a URI type.
type Completer func(ctx context.Context, prefix string) ([]string, error)

// TypeRegistration defines how a specific URI type is handled.
type TypeRegistration struct {
	Name      string
	Completer Completer
}

// Registry manages URI types and their completion logic.
type Registry struct {
	mu    sync.RWMutex
	types map[string]TypeRegistration
}

// NewRegistry creates a new URI type registry.
func NewRegistry() *Registry {
	return &Registry{
		types: make(map[string]TypeRegistration),
	}
}

// Register adds a new URI type to the registry.
func (r *Registry) Register(reg TypeRegistration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.types[reg.Name]; exists {
		return fmt.Errorf("URI type %q already registered", reg.Name)
	}

	r.types[reg.Name] = reg
	return nil
}

// Complete returns suggestions for a given URI type.
func (r *Registry) Complete(ctx context.Context, typeName, prefix string) ([]string, error) {
	r.mu.RLock()
	reg, exists := r.types[typeName]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unknown URI type %q", typeName)
	}

	if reg.Completer == nil {
		return nil, nil
	}

	return reg.Completer(ctx, prefix)
}

// Types returns a list of registered type names.
func (r *Registry) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.types))
	for name := range r.types {
		types = append(types, name)
	}
	return types
}
