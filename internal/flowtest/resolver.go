package flowtest

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
	"hop.top/tlc/internal/core"
)

// GlobalAdapterConfig holds the user-level adapter defaults loaded from
// ~/.tlc/adapters.yaml. Missing file is not an error — results in empty config.
type GlobalAdapterConfig struct {
	Adapters core.FlowAdapters `json:"adapters" yaml:"adapters"`
}

// LoadGlobalAdapterConfig reads ~/.tlc/adapters.yaml.
// Returns an empty config (not an error) if the file does not exist.
func LoadGlobalAdapterConfig() (*GlobalAdapterConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return &GlobalAdapterConfig{}, nil
	}
	path := filepath.Join(home, ".tlc", "adapters.yaml")
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

// AdapterResolver resolves the AgentAdapter and config for a step via a chain:
//
// Adapter resolution (first non-zero wins):
//  1. step.Agent.Name
//  2. flow.Agent.Name
//  3. flow.Adapters.Mappings — first capability intersection match
//  4. global.Adapters.Mappings — first capability intersection match
//  5. flow.Adapters.Default.Name
//  6. global.Adapters.Default.Name
//  7. fatal error
//
// Config resolution (first non-nil wins):
//  1. step.Agent.Config (if step agent was explicit)
//  2. matched mapping's Agent.Config (if resolved via mapping)
//  3. flow.Adapters.Configs[adapterName]
//  4. global.Adapters.Configs[adapterName]
//  5. nil → adapter auto-detects
type AdapterResolver struct {
	adapters map[string]AgentAdapter
	flow     *core.Flow
	global   *GlobalAdapterConfig
}

// NewAdapterResolver creates a resolver bound to a flow and optional global config.
func NewAdapterResolver(adapters map[string]AgentAdapter, flow *core.Flow, global *GlobalAdapterConfig) *AdapterResolver {
	return &AdapterResolver{adapters: adapters, flow: flow, global: global}
}

// resolvedRef carries both the resolved adapter name and any inline config.
type resolvedRef struct {
	name   string
	config map[string]any // may be nil
}

// Resolve returns the AgentAdapter and config map for the given step.
// config is nil when no explicit config was found — adapter should auto-detect.
func (r *AdapterResolver) Resolve(step core.Step) (AgentAdapter, map[string]any, error) {
	ref, err := r.resolveRef(step)
	if err != nil {
		return nil, nil, err
	}

	a, ok := r.adapters[ref.name]
	if !ok {
		return nil, nil, fmt.Errorf(
			"adapter resolver: step %q: adapter %q not registered; available: %s",
			step.ID, ref.name, adapterNames(r.adapters),
		)
	}

	cfg := r.resolveConfig(ref)
	return a, cfg, nil
}

// resolveRef walks the adapter name chain and returns the first match with its
// inline config (from the AgentRef that matched).
func (r *AdapterResolver) resolveRef(step core.Step) (resolvedRef, error) {
	// 1. step.Agent
	if !step.Agent.IsZero() {
		return resolvedRef{name: step.Agent.Name, config: step.Agent.Config}, nil
	}

	// 2. flow.Agent
	if r.flow != nil && !r.flow.Agent.IsZero() {
		return resolvedRef{name: r.flow.Agent.Name, config: r.flow.Agent.Config}, nil
	}

	stepCaps := stepCapabilities(step)

	// 3. flow.Adapters.Mappings
	if r.flow != nil && r.flow.Adapters != nil {
		if ref, ok := firstMatchRef(r.flow.Adapters.Mappings, stepCaps); ok {
			return ref, nil
		}
	}

	// 4. global.Adapters.Mappings
	if r.global != nil {
		if ref, ok := firstMatchRef(r.global.Adapters.Mappings, stepCaps); ok {
			return ref, nil
		}
	}

	// 5. flow.Adapters.Default
	if r.flow != nil && r.flow.Adapters != nil && !r.flow.Adapters.Default.IsZero() {
		d := r.flow.Adapters.Default
		return resolvedRef{name: d.Name, config: d.Config}, nil
	}

	// 6. global.Adapters.Default
	if r.global != nil && !r.global.Adapters.Default.IsZero() {
		d := r.global.Adapters.Default
		return resolvedRef{name: d.Name, config: d.Config}, nil
	}

	// 7. fatal
	return resolvedRef{}, fmt.Errorf(
		"adapter resolver: step %q: no adapter resolved; "+
			"set agent: on the step or flow, or configure adapters.default",
		step.ID,
	)
}

// resolveConfig walks the config chain for the resolved adapter name.
// Returns nil if no config found — adapter should auto-detect.
func (r *AdapterResolver) resolveConfig(ref resolvedRef) map[string]any {
	// 1+2. inline config from step/mapping AgentRef
	if len(ref.config) > 0 {
		return ref.config
	}

	// 3. flow.Adapters.Configs[name]
	if r.flow != nil && r.flow.Adapters != nil {
		if cfg, ok := r.flow.Adapters.Configs[ref.name]; ok && len(cfg) > 0 {
			return cfg
		}
	}

	// 4. global.Adapters.Configs[name]
	if r.global != nil {
		if cfg, ok := r.global.Adapters.Configs[ref.name]; ok && len(cfg) > 0 {
			return cfg
		}
	}

	return nil
}

// firstMatchRef returns the resolvedRef from the first mapping whose capabilities
// or tools intersect with stepSet (caps ∪ tools from the step).
func firstMatchRef(mappings []core.AdapterMapping, stepSet map[string]bool) (resolvedRef, bool) {
	for _, m := range mappings {
		for _, cap := range m.Capabilities {
			if stepSet[cap] {
				return resolvedRef{name: m.Agent.Name, config: m.Agent.Config}, true
			}
		}
		for _, tool := range m.Tools {
			if stepSet[tool] {
				return resolvedRef{name: m.Agent.Name, config: m.Agent.Config}, true
			}
		}
	}
	return resolvedRef{}, false
}

// stepCapabilities returns a unified set of the step's task_template
// capabilities and tools (caps ∪ tools), used for adapter mapping lookups.
func stepCapabilities(step core.Step) map[string]bool {
	set := make(map[string]bool)
	if step.TaskTemplate == nil || step.TaskTemplate.Requirements == nil {
		return set
	}
	for _, c := range step.TaskTemplate.Requirements.Capabilities {
		set[c] = true
	}
	for _, t := range step.TaskTemplate.Requirements.Tools {
		set[t] = true
	}
	return set
}

// adapterNames returns a comma-separated sorted list of registered adapter names.
func adapterNames(m map[string]AgentAdapter) string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}
