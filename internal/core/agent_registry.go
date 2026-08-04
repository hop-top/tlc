package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// AgentConfig describes how to invoke a particular agent.
type AgentConfig struct {
	Image          string            `yaml:"image"`
	Binary         string            `yaml:"binary"`
	Env            map[string]string `yaml:"env"`
	DefaultTimeout time.Duration     `yaml:"default_timeout"`
}

// agentConfigFile is the on-disk YAML shape.
type agentConfigFile struct {
	Agents map[string]*AgentConfig `yaml:"agents"`
}

// ErrTrustRequired signals that a project-local agent config was found
// but has not been approved by the user yet.
type ErrTrustRequired struct {
	Path string
}

func (e *ErrTrustRequired) Error() string {
	return fmt.Sprintf(
		"project-local agent config %s requires trust; "+
			"review before proceeding (y/N)",
		e.Path,
	)
}

// AgentRegistry maps agent names to their configuration. Global config
// supports ${VAR} env-var expansion; project-local values are literal
// (security: prevents secret exfiltration via committed config).
type AgentRegistry struct {
	mu             sync.RWMutex
	global         map[string]*AgentConfig
	project        map[string]*AgentConfig
	trusted        map[string]bool // path -> user approved
	projectCfgPath string          // actual loaded project config path
}

// NewAgentRegistry returns an empty registry.
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		global:  make(map[string]*AgentConfig),
		project: make(map[string]*AgentConfig),
		trusted: make(map[string]bool),
	}
}

// LoadDefaults loads from ~/.config/tlc/agents.yaml (global) then
// .tlc/agents.yaml (project-local). Missing files are silently skipped.
func (r *AgentRegistry) LoadDefaults() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("agent registry: cannot determine home dir: %w", err)
	}
	globalPath := filepath.Join(home, ".config", "tlc", "agents.yaml")
	if err := r.loadGlobal(globalPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	projectPath := filepath.Join(".tlc", "agents.yaml")
	if err := r.loadProject(projectPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// LoadFromFile loads agent configs from a YAML file. If global is true,
// env-var expansion is applied to env values.
func (r *AgentRegistry) LoadFromFile(path string, global bool) error {
	if global {
		return r.loadGlobal(path)
	}
	return r.loadProject(path)
}

func (r *AgentRegistry) loadGlobal(path string) error {
	configs, err := parseAgentFile(path)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, cfg := range configs {
		expandEnvMap(cfg.Env)
		r.global[name] = cfg
	}
	return nil
}

func (r *AgentRegistry) loadProject(path string) error {
	configs, err := parseAgentFile(path)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.projectCfgPath = path
	for name, cfg := range configs {
		// Project-local env values are literal — no expansion.
		r.project[name] = cfg
	}
	return nil
}

// ProjectConfigPath returns the path of the loaded project-local config,
// or the default ".tlc/agents.yaml" if none was loaded.
func (r *AgentRegistry) ProjectConfigPath() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.projectCfgPath != "" {
		return r.projectCfgPath
	}
	return filepath.Join(".tlc", "agents.yaml")
}

// TrustProject marks a project-local config path as user-approved.
func (r *AgentRegistry) TrustProject(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.trusted[path] = true
}

// Get returns the merged config for the named agent. Project-local
// overrides global for image, binary, and default_timeout. Env maps
// are merged (project wins on key conflict). Returns ErrTrustRequired
// if a project-local config is used and has not been trusted.
func (r *AgentRegistry) Get(name string) (*AgentConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	g := r.global[name]
	p := r.project[name]

	if g == nil && p == nil {
		return nil, fmt.Errorf(
			"agent %q not found; available: %s",
			name, strings.Join(r.listLocked(), ", "),
		)
	}

	// Check trust for project-local against actual loaded path.
	if p != nil && !r.trusted[r.projectCfgPath] {
		return nil, &ErrTrustRequired{Path: r.projectCfgPath}
	}

	merged := mergeConfigs(g, p)
	return merged, nil
}

// List returns sorted agent names from both global and project configs.
func (r *AgentRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.listLocked()
}

func (r *AgentRegistry) listLocked() []string {
	seen := make(map[string]struct{})
	for k := range r.global {
		seen[k] = struct{}{}
	}
	for k := range r.project {
		seen[k] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for k := range seen {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// mergeConfigs merges global (g) and project (p) configs. Either may
// be nil. Project overrides global for scalar fields; env maps merge.
func mergeConfigs(g, p *AgentConfig) *AgentConfig {
	if g == nil && p == nil {
		return nil
	}
	if g == nil {
		return copyConfig(p)
	}
	if p == nil {
		return copyConfig(g)
	}

	out := copyConfig(g)
	if p.Image != "" {
		out.Image = p.Image
	}
	if p.Binary != "" {
		out.Binary = p.Binary
	}
	if p.DefaultTimeout != 0 {
		out.DefaultTimeout = p.DefaultTimeout
	}
	// Merge env: project wins on conflict.
	for k, v := range p.Env {
		out.Env[k] = v
	}
	return out
}

func copyConfig(c *AgentConfig) *AgentConfig {
	out := *c
	out.Env = make(map[string]string, len(c.Env))
	for k, v := range c.Env {
		out.Env[k] = v
	}
	return &out
}

func parseAgentFile(path string) (map[string]*AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("agent registry: read %s: %w", path, err)
	}
	var f agentConfigFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("agent registry: parse %s: %w", path, err)
	}
	if f.Agents == nil {
		return make(map[string]*AgentConfig), nil
	}
	return f.Agents, nil
}

// expandEnvMap replaces ${VAR} patterns in map values with os.Getenv.
func expandEnvMap(m map[string]string) {
	for k, v := range m {
		m[k] = os.Expand(v, os.Getenv)
	}
}
