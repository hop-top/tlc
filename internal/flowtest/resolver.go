package flowtest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	"hop.top/kit/go/core/xdg"
	"hop.top/tlc/internal/core"
)

// GlobalAdapterConfig holds the user-level adapter defaults loaded from
// <config-dir>/adapters.yaml. A missing file is not an error: it reads as
// an empty config.
type GlobalAdapterConfig struct {
	Adapters AdapterDefaults `json:"adapters" yaml:"adapters"`
}

// AdapterDefaults is the adapter half of adapters.yaml: which adapter to
// use when a step names none, and the per-adapter config keyed by adapter
// name (the key "dir" is that adapter's config directory).
type AdapterDefaults struct {
	Default string                    `json:"default,omitempty" yaml:"default,omitempty"`
	Configs map[string]map[string]any `json:"configs,omitempty" yaml:"configs,omitempty"`
}

// LoadGlobalAdapterConfig reads <config-dir>/adapters.yaml.
// Returns an empty config (not an error) if the file does not exist.
func LoadGlobalAdapterConfig() (*GlobalAdapterConfig, error) {
	cfgDir, err := xdg.ConfigDir("tlc")
	if err != nil {
		return &GlobalAdapterConfig{}, nil
	}
	path := filepath.Join(cfgDir, "adapters.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &GlobalAdapterConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("adapter config: read %s: %w", path, err)
	}
	var cfg GlobalAdapterConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("adapter config: parse %s: %w", path, err)
	}
	return &cfg, nil
}

// AdapterResolver resolves the AgentAdapter and config for a recipe step.
//
// Adapter resolution (first non-empty wins):
//  1. the task's agent (step.Agent, from the recipe step)
//  2. recipe.Agent
//  3. global.Adapters.Default
//  4. fatal error
//
// Config resolution (first non-nil wins):
//  1. global.Adapters.Configs[adapterName]
//  3. nil → adapter auto-detects
type AdapterResolver struct {
	adapters   map[string]AgentAdapter
	recipe     *core.Recipe
	global     *GlobalAdapterConfig
	sandbox    *Sandbox                        // optional; enables Probe in ResolveCtx
	probeCache map[string]*AdapterCapabilities // keyed by adapter name
}

// NewAdapterResolver creates a resolver bound to a recipe and optional
// global config; both may be nil.
func NewAdapterResolver(adapters map[string]AgentAdapter, recipe *core.Recipe, global *GlobalAdapterConfig) *AdapterResolver {
	return &AdapterResolver{
		adapters:   adapters,
		recipe:     recipe,
		global:     global,
		probeCache: make(map[string]*AdapterCapabilities),
	}
}

// WithSandbox attaches a sandbox to the resolver enabling Probe on first resolution.
// Returns the receiver for chaining.
func (r *AdapterResolver) WithSandbox(sb *Sandbox) *AdapterResolver {
	r.sandbox = sb
	return r
}

// resolvedRef carries both the resolved adapter name and any inline config.
type resolvedRef struct {
	name   string
	config map[string]any // may be nil
}

// Resolve returns the AgentAdapter and config map for the given step.
// config is nil when no explicit config was found — adapter should auto-detect.
func (r *AdapterResolver) Resolve(step StepRef) (AgentAdapter, map[string]any, error) {
	a, cfg, _, err := r.ResolveCtx(context.Background(), step)
	return a, cfg, err
}

// ResolveCtx resolves the adapter, scoped config, and operation for the step.
// On first resolution for each adapter, Probe is called when a sandbox is set
// (errors are non-fatal: logged to stderr, empty caps stored).
func (r *AdapterResolver) ResolveCtx(ctx context.Context, step StepRef) (AgentAdapter, map[string]any, string, error) {
	ref, err := r.resolveRef(step)
	if err != nil {
		return nil, nil, "", err
	}

	a, ok := r.adapters[ref.name]
	if !ok {
		return nil, nil, "", fmt.Errorf(
			"adapter resolver: step %q: adapter %q not registered; available: %s",
			step.ID, ref.name, adapterNames(r.adapters),
		)
	}

	r.probe(ctx, ref.name, a)
	return a, r.resolveConfig(ref), a.Operation(step), nil
}

// probe runs the adapter's Probe once per adapter name; errors are non-fatal.
func (r *AdapterResolver) probe(ctx context.Context, name string, a AgentAdapter) {
	if r.sandbox == nil {
		return
	}
	if _, probed := r.probeCache[name]; probed {
		return
	}
	caps, err := a.Probe(ctx, filepath.Join(r.sandbox.BinDir, a.Binary()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "adapter resolver: probe %q: %v (continuing)\n", name, err)
		caps = &AdapterCapabilities{}
	}
	r.probeCache[name] = caps
}

// resolveRef walks the adapter name chain and returns the first match with
// the inline config of the reference that matched.
func (r *AdapterResolver) resolveRef(step StepRef) (resolvedRef, error) {
	if step.Agent != "" {
		return resolvedRef{name: step.Agent}, nil
	}
	if r.recipe != nil && r.recipe.Agent != "" {
		return resolvedRef{name: r.recipe.Agent}, nil
	}
	if r.global != nil && r.global.Adapters.Default != "" {
		return resolvedRef{name: r.global.Adapters.Default}, nil
	}
	return resolvedRef{}, fmt.Errorf(
		"adapter resolver: step %q: no adapter resolved; "+
			"set agent: on the step or the recipe, or configure adapters.default",
		step.ID,
	)
}

// resolveConfig walks the config chain for the resolved adapter name.
// Returns nil if no config found — adapter should auto-detect.
func (r *AdapterResolver) resolveConfig(ref resolvedRef) map[string]any {
	if len(ref.config) > 0 {
		return ref.config
	}
	if r.global != nil {
		if cfg, ok := r.global.Adapters.Configs[ref.name]; ok && len(cfg) > 0 {
			return cfg
		}
	}
	return nil
}

// adapterNames returns a comma-separated sorted list of registered adapter names.
func adapterNames(m map[string]AgentAdapter) string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
