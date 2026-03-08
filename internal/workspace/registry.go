package workspace

import (
	"fmt"

	"hop.top/tlc/internal/config"
)

// Registry maps adapter names and URI schemes to implementations.
type Registry struct {
	adapters map[string]SpaceAdapter // keyed by adapter name
	schemes  map[string]string       // scheme -> adapter name
}

// NewRegistry creates an empty adapter registry.
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]SpaceAdapter),
		schemes:  make(map[string]string),
	}
}

// Register adds an adapter to the registry, indexed by name and schemes.
func (r *Registry) Register(adapter SpaceAdapter) {
	name := adapter.Name()
	r.adapters[name] = adapter
	for _, scheme := range adapter.Schemes() {
		r.schemes[scheme] = name
	}
}

// Resolve returns the adapter for a space config.
// Resolution order:
//  1. Explicit space.Adapter field
//  2. InferAdapterFromURI(space.URI)
//  3. URI scheme lookup
func (r *Registry) Resolve(space config.SpaceConfig) (SpaceAdapter, error) {
	// 1. Explicit adapter name.
	if space.Adapter != "" {
		if a, ok := r.adapters[space.Adapter]; ok {
			return a, nil
		}
		return nil, fmt.Errorf("unknown adapter %q", space.Adapter)
	}

	// 2. Infer adapter name from URI.
	if inferred := config.InferAdapterFromURI(space.URI); inferred != "" {
		if a, ok := r.adapters[inferred]; ok {
			return a, nil
		}
	}

	// 3. Scheme lookup.
	scheme := uriScheme(space.URI)
	if name, ok := r.schemes[scheme]; ok {
		if a, ok2 := r.adapters[name]; ok2 {
			return a, nil
		}
	}

	return nil, fmt.Errorf("no adapter for space URI %q", space.URI)
}

// uriScheme extracts the scheme portion of a URI, returning "" for bare paths.
func uriScheme(uri string) string {
	for i, c := range uri {
		if c == ':' {
			return uri[:i]
		}
		if c == '/' || c == '?' || c == '#' {
			break
		}
	}
	return ""
}
