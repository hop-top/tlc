# Plugin Infrastructure Phase 1 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement core plugin discovery and loading infrastructure for workflow bundle plugins

**Architecture:** Extend existing PluginManifest to support `provides` section (flows, assignees, executors). Create lightweight plugin registry for lazy loading. Support local plugin installation.

**Tech Stack:** Go 1.21+, YAML parsing (gopkg.in/yaml.v3), existing TLC plugin infrastructure

---

## Phase 1 Scope

This implements basic plugin infrastructure WITHOUT execution capabilities:
- ✅ Extended PluginManifest with `provides` section
- ✅ Plugin registry (lightweight index)
- ✅ Lazy loading mechanism
- ✅ Local installation: `tlc plugin install ./path`
- ✅ Updated `tlc plugin list` to show workflow bundles
- ❌ NOT included: Context system, assignee execution, Git installation (future phases)

---

## Task 1: Extend PluginManifest

**Goal:** Add `provides` section to PluginManifest for workflow bundles

**Files:**
- Modify: `internal/core/plugin.go`
- Test: `internal/core/plugin_test.go`

### Step 1: Write failing test for PluginManifest with provides section

**File:** `internal/core/plugin_test.go`

```go
package core

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPluginManifest_WorkflowBundle(t *testing.T) {
	yamlData := `
name: test-workflow
version: 1.0.0
description: Test workflow bundle

provides:
  flows:
    - id: "flow:test:example:1.0"
      file: "flows/example.yaml"
      description: "Example flow"

  assignees:
    - id: "assignee:test:executor:1.0"
      file: "assignees/executor.yaml"
      execution_type: executor
      description: "Test executor"
      patterns: []

  executors:
    - id: "executor:test:1.0"
      entry_point: "./bin/test-executor"
      protocol: jsonrpc

requires:
  tlc_version: ">=0.2.0"

permissions:
  - process.spawn

author: "Test Author"
license: "MIT"
`

	var manifest PluginManifest
	err := yaml.Unmarshal([]byte(yamlData), &manifest)
	if err != nil {
		t.Fatalf("failed to parse manifest: %v", err)
	}

	// Verify provides section parsed
	if manifest.Provides == nil {
		t.Fatal("expected Provides to be non-nil")
	}

	// Verify flows
	if len(manifest.Provides.Flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(manifest.Provides.Flows))
	}
	flow := manifest.Provides.Flows[0]
	if flow.ID != "flow:test:example:1.0" {
		t.Errorf("expected flow ID 'flow:test:example:1.0', got '%s'", flow.ID)
	}
	if flow.File != "flows/example.yaml" {
		t.Errorf("expected flow file 'flows/example.yaml', got '%s'", flow.File)
	}

	// Verify assignees
	if len(manifest.Provides.Assignees) != 1 {
		t.Fatalf("expected 1 assignee, got %d", len(manifest.Provides.Assignees))
	}
	assignee := manifest.Provides.Assignees[0]
	if assignee.ID != "assignee:test:executor:1.0" {
		t.Errorf("expected assignee ID 'assignee:test:executor:1.0', got '%s'", assignee.ID)
	}
	if assignee.ExecutionType != "executor" {
		t.Errorf("expected execution_type 'executor', got '%s'", assignee.ExecutionType)
	}

	// Verify executors
	if len(manifest.Provides.Executors) != 1 {
		t.Fatalf("expected 1 executor, got %d", len(manifest.Provides.Executors))
	}
	executor := manifest.Provides.Executors[0]
	if executor.ID != "executor:test:1.0" {
		t.Errorf("expected executor ID 'executor:test:1.0', got '%s'", executor.ID)
	}
	if executor.Protocol != "jsonrpc" {
		t.Errorf("expected protocol 'jsonrpc', got '%s'", executor.Protocol)
	}
}
```

### Step 2: Run test to verify it fails

```bash
go test ./internal/core -run TestPluginManifest_WorkflowBundle -v
```

**Expected:** FAIL with compilation errors (Provides field doesn't exist)

### Step 3: Add Provides structs to plugin.go

**File:** `internal/core/plugin.go`

Add after PluginRequirements struct:

```go
// PluginProvides defines what a workflow bundle plugin provides.
type PluginProvides struct {
	Flows     []ProvidedFlow     `json:"flows,omitempty" yaml:"flows,omitempty"`
	Assignees []ProvidedAssignee `json:"assignees,omitempty" yaml:"assignees,omitempty"`
	Executors []ProvidedExecutor `json:"executors,omitempty" yaml:"executors,omitempty"`
}

// ProvidedFlow describes a flow provided by a plugin.
type ProvidedFlow struct {
	ID          string `json:"id" yaml:"id"`
	File        string `json:"file" yaml:"file"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// ProvidedAssignee describes an assignee provided by a plugin.
type ProvidedAssignee struct {
	ID            string   `json:"id" yaml:"id"`
	File          string   `json:"file" yaml:"file"`
	ExecutionType string   `json:"execution_type,omitempty" yaml:"execution_type,omitempty"`
	Description   string   `json:"description,omitempty" yaml:"description,omitempty"`
	Patterns      []string `json:"patterns,omitempty" yaml:"patterns,omitempty"`
}

// ProvidedExecutor describes an executor provided by a plugin.
type ProvidedExecutor struct {
	ID           string                 `json:"id" yaml:"id"`
	EntryPoint   string                 `json:"entry_point" yaml:"entry_point"`
	Protocol     string                 `json:"protocol" yaml:"protocol"`
	Capabilities map[string]interface{} `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
}
```

### Step 4: Add Provides field to PluginManifest

**File:** `internal/core/plugin.go`

Modify PluginManifest struct:

```go
// PluginManifest defines the metadata and requirements for a TLC plugin.
type PluginManifest struct {
	Name         string             `json:"name" yaml:"name"`
	Version      string             `json:"version" yaml:"version"`
	Type         PluginType         `json:"type" yaml:"type"`
	Description  string             `json:"description" yaml:"description"`
	Capabilities []string           `json:"capabilities" yaml:"capabilities"`
	EntryPoint   string             `json:"entry_point" yaml:"entry_point"`
	Requires     PluginRequirements `json:"requires" yaml:"requires"`
	ConfigSchema any                `json:"config_schema,omitempty" yaml:"config_schema,omitempty"`
	Permissions  []string           `json:"permissions" yaml:"permissions"`

	// NEW: Workflow bundle support
	Provides *PluginProvides `json:"provides,omitempty" yaml:"provides,omitempty"`

	// NEW: Additional metadata
	Author     string `json:"author,omitempty" yaml:"author,omitempty"`
	License    string `json:"license,omitempty" yaml:"license,omitempty"`
	Repository string `json:"repository,omitempty" yaml:"repository,omitempty"`
}
```

### Step 5: Run test to verify it passes

```bash
go test ./internal/core -run TestPluginManifest_WorkflowBundle -v
```

**Expected:** PASS

### Step 6: Commit

```bash
git add internal/core/plugin.go internal/core/plugin_test.go
git commit -m "feat(plugin): add Provides section to PluginManifest for workflow bundles"
```

---

## Task 2: Create Plugin Registry

**Goal:** Create lightweight index for tracking installed plugins and lazy loading

**Files:**
- Create: `internal/core/plugin_registry.go`
- Create: `internal/core/plugin_registry_test.go`

### Step 1: Write failing test for plugin registry

**File:** `internal/core/plugin_registry_test.go`

```go
package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPluginRegistry_RegisterPlugin(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "plugin-registry.yaml")

	registry := NewPluginRegistry(registryPath)

	// Register a workflow bundle plugin
	manifest := &PluginManifest{
		Name:        "test-plugin",
		Version:     "1.0.0",
		Description: "Test plugin",
		Provides: &PluginProvides{
			Flows: []ProvidedFlow{
				{ID: "flow:test:example:1.0", File: "flows/example.yaml"},
			},
			Assignees: []ProvidedAssignee{
				{ID: "assignee:test:executor:1.0", File: "assignees/executor.yaml"},
			},
		},
	}

	pluginPath := filepath.Join(tmpDir, "plugins", "test-plugin")

	err := registry.Register(manifest, pluginPath)
	if err != nil {
		t.Fatalf("failed to register plugin: %v", err)
	}

	// Verify plugin is in registry
	entry, exists := registry.Get("test-plugin")
	if !exists {
		t.Fatal("expected plugin to exist in registry")
	}

	if entry.Name != "test-plugin" {
		t.Errorf("expected name 'test-plugin', got '%s'", entry.Name)
	}

	if entry.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", entry.Version)
	}

	if entry.Path != pluginPath {
		t.Errorf("expected path '%s', got '%s'", pluginPath, entry.Path)
	}

	if entry.Loaded {
		t.Error("expected Loaded to be false initially")
	}

	// Verify flows are indexed
	if len(entry.Flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(entry.Flows))
	}

	if entry.Flows[0] != "flow:test:example:1.0" {
		t.Errorf("expected flow ID 'flow:test:example:1.0', got '%s'", entry.Flows[0])
	}
}

func TestPluginRegistry_FindPluginByFlowID(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "plugin-registry.yaml")

	registry := NewPluginRegistry(registryPath)

	// Register plugin with flow
	manifest := &PluginManifest{
		Name:    "test-plugin",
		Version: "1.0.0",
		Provides: &PluginProvides{
			Flows: []ProvidedFlow{
				{ID: "flow:test:example:1.0", File: "flows/example.yaml"},
			},
		},
	}

	pluginPath := filepath.Join(tmpDir, "plugins", "test-plugin")
	registry.Register(manifest, pluginPath)

	// Find plugin by flow ID
	pluginName, found := registry.FindPluginByFlowID("flow:test:example:1.0")
	if !found {
		t.Fatal("expected to find plugin by flow ID")
	}

	if pluginName != "test-plugin" {
		t.Errorf("expected plugin name 'test-plugin', got '%s'", pluginName)
	}

	// Try non-existent flow
	_, found = registry.FindPluginByFlowID("flow:does:not:exist:1.0")
	if found {
		t.Error("expected not to find non-existent flow")
	}
}

func TestPluginRegistry_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "plugin-registry.yaml")

	// Create and register plugin
	registry1 := NewPluginRegistry(registryPath)
	manifest := &PluginManifest{
		Name:    "test-plugin",
		Version: "1.0.0",
		Provides: &PluginProvides{
			Flows: []ProvidedFlow{
				{ID: "flow:test:example:1.0", File: "flows/example.yaml"},
			},
		},
	}
	pluginPath := filepath.Join(tmpDir, "plugins", "test-plugin")
	registry1.Register(manifest, pluginPath)

	// Save registry
	err := registry1.Save()
	if err != nil {
		t.Fatalf("failed to save registry: %v", err)
	}

	// Load registry in new instance
	registry2 := NewPluginRegistry(registryPath)
	err = registry2.Load()
	if err != nil {
		t.Fatalf("failed to load registry: %v", err)
	}

	// Verify plugin persisted
	entry, exists := registry2.Get("test-plugin")
	if !exists {
		t.Fatal("expected plugin to exist after loading")
	}

	if entry.Name != "test-plugin" {
		t.Errorf("expected name 'test-plugin', got '%s'", entry.Name)
	}

	// Verify flow index persisted
	pluginName, found := registry2.FindPluginByFlowID("flow:test:example:1.0")
	if !found {
		t.Fatal("expected to find plugin by flow ID after loading")
	}

	if pluginName != "test-plugin" {
		t.Errorf("expected plugin name 'test-plugin', got '%s'", pluginName)
	}
}
```

### Step 2: Run test to verify it fails

```bash
go test ./internal/core -run TestPluginRegistry -v
```

**Expected:** FAIL with compilation errors (PluginRegistry doesn't exist)

### Step 3: Implement plugin registry

**File:** `internal/core/plugin_registry.go`

```go
package core

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// PluginRegistry maintains an index of installed plugins for lazy loading.
type PluginRegistry struct {
	path    string
	mu      sync.RWMutex
	plugins map[string]*PluginRegistryEntry

	// Indexes for fast lookup
	flowIndex     map[string]string // flow_id -> plugin_name
	assigneeIndex map[string]string // assignee_id -> plugin_name
}

// PluginRegistryEntry represents a plugin in the registry.
type PluginRegistryEntry struct {
	Name        string   `yaml:"name"`
	Version     string   `yaml:"version"`
	Path        string   `yaml:"path"`
	Flows       []string `yaml:"flows,omitempty"`
	Assignees   []string `yaml:"assignees,omitempty"`
	Executors   []string `yaml:"executors,omitempty"`
	Loaded      bool     `yaml:"loaded"`
	Status      string   `yaml:"status"` // installed, enabled, disabled
}

// registryFile represents the YAML structure of the registry file.
type registryFile struct {
	Plugins map[string]*PluginRegistryEntry `yaml:"plugins"`
}

// NewPluginRegistry creates a new plugin registry.
func NewPluginRegistry(path string) *PluginRegistry {
	return &PluginRegistry{
		path:          path,
		plugins:       make(map[string]*PluginRegistryEntry),
		flowIndex:     make(map[string]string),
		assigneeIndex: make(map[string]string),
	}
}

// Register adds a plugin to the registry.
func (r *PluginRegistry) Register(manifest *PluginManifest, pluginPath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry := &PluginRegistryEntry{
		Name:    manifest.Name,
		Version: manifest.Version,
		Path:    pluginPath,
		Loaded:  false,
		Status:  "installed",
	}

	// Index flows
	if manifest.Provides != nil {
		for _, flow := range manifest.Provides.Flows {
			entry.Flows = append(entry.Flows, flow.ID)
			r.flowIndex[flow.ID] = manifest.Name
		}

		// Index assignees
		for _, assignee := range manifest.Provides.Assignees {
			entry.Assignees = append(entry.Assignees, assignee.ID)
			r.assigneeIndex[assignee.ID] = manifest.Name
		}

		// Index executors
		for _, executor := range manifest.Provides.Executors {
			entry.Executors = append(entry.Executors, executor.ID)
		}
	}

	r.plugins[manifest.Name] = entry
	return nil
}

// Get retrieves a plugin entry by name.
func (r *PluginRegistry) Get(name string) (*PluginRegistryEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.plugins[name]
	return entry, exists
}

// FindPluginByFlowID finds the plugin that provides a given flow ID.
func (r *PluginRegistry) FindPluginByFlowID(flowID string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pluginName, found := r.flowIndex[flowID]
	return pluginName, found
}

// FindPluginByAssigneeID finds the plugin that provides a given assignee ID.
func (r *PluginRegistry) FindPluginByAssigneeID(assigneeID string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pluginName, found := r.assigneeIndex[assigneeID]
	return pluginName, found
}

// List returns all registered plugins.
func (r *PluginRegistry) List() []*PluginRegistryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := make([]*PluginRegistryEntry, 0, len(r.plugins))
	for _, entry := range r.plugins {
		entries = append(entries, entry)
	}
	return entries
}

// MarkLoaded marks a plugin as loaded (cached in memory).
func (r *PluginRegistry) MarkLoaded(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.plugins[name]
	if !exists {
		return fmt.Errorf("plugin not found: %s", name)
	}

	entry.Loaded = true
	return nil
}

// Save persists the registry to disk.
func (r *PluginRegistry) Save() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	file := registryFile{
		Plugins: r.plugins,
	}

	data, err := yaml.Marshal(&file)
	if err != nil {
		return fmt.Errorf("failed to marshal registry: %w", err)
	}

	err = os.WriteFile(r.path, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write registry file: %w", err)
	}

	return nil
}

// Load reads the registry from disk.
func (r *PluginRegistry) Load() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			// Registry doesn't exist yet, start fresh
			return nil
		}
		return fmt.Errorf("failed to read registry file: %w", err)
	}

	var file registryFile
	err = yaml.Unmarshal(data, &file)
	if err != nil {
		return fmt.Errorf("failed to parse registry file: %w", err)
	}

	r.plugins = file.Plugins

	// Rebuild indexes
	r.flowIndex = make(map[string]string)
	r.assigneeIndex = make(map[string]string)

	for name, entry := range r.plugins {
		for _, flowID := range entry.Flows {
			r.flowIndex[flowID] = name
		}
		for _, assigneeID := range entry.Assignees {
			r.assigneeIndex[assigneeID] = name
		}
	}

	return nil
}
```

### Step 4: Run tests to verify they pass

```bash
go test ./internal/core -run TestPluginRegistry -v
```

**Expected:** PASS (all 3 tests)

### Step 5: Commit

```bash
git add internal/core/plugin_registry.go internal/core/plugin_registry_test.go
git commit -m "feat(plugin): add plugin registry for lazy loading"
```

---

## Task 3: Plugin Loader

**Goal:** Implement lazy loading of plugin flows and assignees

**Files:**
- Create: `internal/core/plugin_loader.go`
- Create: `internal/core/plugin_loader_test.go`

### Step 1: Write failing test for plugin loader

**File:** `internal/core/plugin_loader_test.go`

```go
package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPluginLoader_LoadFlow(t *testing.T) {
	// Setup test plugin directory
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	flowsDir := filepath.Join(pluginDir, "flows")

	err := os.MkdirAll(flowsDir, 0755)
	if err != nil {
		t.Fatalf("failed to create flows directory: %v", err)
	}

	// Create test flow file
	flowContent := `
flow_id: "flow:test:example:1.0"
name: "Test Flow"
version: "1.0"
description: "Test flow for plugin loader"
entry_step: "test-step"

steps:
  test-step:
    step_id: "test-step"
    type: "task"
    title: "Test task"
`
	flowPath := filepath.Join(flowsDir, "example.yaml")
	err = os.WriteFile(flowPath, []byte(flowContent), 0644)
	if err != nil {
		t.Fatalf("failed to write flow file: %v", err)
	}

	// Create loader
	loader := NewPluginLoader()

	// Load flow
	flow, err := loader.LoadFlow(pluginDir, "flows/example.yaml")
	if err != nil {
		t.Fatalf("failed to load flow: %v", err)
	}

	if flow.ID != "flow:test:example:1.0" {
		t.Errorf("expected flow ID 'flow:test:example:1.0', got '%s'", flow.ID)
	}

	if flow.Name != "Test Flow" {
		t.Errorf("expected flow name 'Test Flow', got '%s'", flow.Name)
	}
}

func TestPluginLoader_LoadAssignee(t *testing.T) {
	// Setup test plugin directory
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	assigneesDir := filepath.Join(pluginDir, "assignees")

	err := os.MkdirAll(assigneesDir, 0755)
	if err != nil {
		t.Fatalf("failed to create assignees directory: %v", err)
	}

	// Create test assignee file
	assigneeContent := `
assignee_id: "assignee:test:executor:1.0"
name: "Test Executor"
version: "1.0"
description: "Test executor for plugin loader"

capabilities:
  task_types:
    - "test-execution"
  tools:
    - "test-tool"
  domains:
    - "testing"

execution_type: executor
`
	assigneePath := filepath.Join(assigneesDir, "executor.yaml")
	err = os.WriteFile(assigneePath, []byte(assigneeContent), 0644)
	if err != nil {
		t.Fatalf("failed to write assignee file: %v", err)
	}

	// Create loader
	loader := NewPluginLoader()

	// Load assignee
	assignee, err := loader.LoadAssignee(pluginDir, "assignees/executor.yaml")
	if err != nil {
		t.Fatalf("failed to load assignee: %v", err)
	}

	if assignee.ID != "assignee:test:executor:1.0" {
		t.Errorf("expected assignee ID 'assignee:test:executor:1.0', got '%s'", assignee.ID)
	}

	if assignee.Name != "Test Executor" {
		t.Errorf("expected assignee name 'Test Executor', got '%s'", assignee.Name)
	}
}
```

### Step 2: Run test to verify it fails

```bash
go test ./internal/core -run TestPluginLoader -v
```

**Expected:** FAIL with compilation errors (PluginLoader doesn't exist)

### Step 3: Implement plugin loader

**File:** `internal/core/plugin_loader.go`

```go
package core

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// PluginLoader loads plugin components (flows, assignees) from disk.
type PluginLoader struct{}

// NewPluginLoader creates a new plugin loader.
func NewPluginLoader() *PluginLoader {
	return &PluginLoader{}
}

// LoadFlow loads a flow from a plugin directory.
func (l *PluginLoader) LoadFlow(pluginDir string, flowFile string) (*Flow, error) {
	flowPath := filepath.Join(pluginDir, flowFile)

	data, err := os.ReadFile(flowPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read flow file %s: %w", flowPath, err)
	}

	var flow Flow
	err = yaml.Unmarshal(data, &flow)
	if err != nil {
		return nil, fmt.Errorf("failed to parse flow file %s: %w", flowPath, err)
	}

	// Validate flow
	err = ValidateFlow(&flow)
	if err != nil {
		return nil, fmt.Errorf("invalid flow in %s: %w", flowPath, err)
	}

	return &flow, nil
}

// LoadAssignee loads an assignee from a plugin directory.
func (l *PluginLoader) LoadAssignee(pluginDir string, assigneeFile string) (*Assignee, error) {
	assigneePath := filepath.Join(pluginDir, assigneeFile)

	data, err := os.ReadFile(assigneePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read assignee file %s: %w", assigneePath, err)
	}

	var assignee Assignee
	err = yaml.Unmarshal(data, &assignee)
	if err != nil {
		return nil, fmt.Errorf("failed to parse assignee file %s: %w", assigneePath, err)
	}

	// Basic validation
	if assignee.ID == "" {
		return nil, fmt.Errorf("assignee in %s has no ID", assigneePath)
	}

	return &assignee, nil
}

// LoadManifest loads a plugin manifest from a plugin directory.
func (l *PluginLoader) LoadManifest(pluginDir string) (*PluginManifest, error) {
	manifestPath := filepath.Join(pluginDir, "manifest.yaml")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file %s: %w", manifestPath, err)
	}

	var manifest PluginManifest
	err = yaml.Unmarshal(data, &manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest file %s: %w", manifestPath, err)
	}

	// Basic validation
	if manifest.Name == "" {
		return nil, fmt.Errorf("manifest in %s has no name", pluginDir)
	}
	if manifest.Version == "" {
		return nil, fmt.Errorf("manifest in %s has no version", pluginDir)
	}

	return &manifest, nil
}
```

### Step 4: Run tests to verify they pass

```bash
go test ./internal/core -run TestPluginLoader -v
```

**Expected:** PASS (both tests)

### Step 5: Commit

```bash
git add internal/core/plugin_loader.go internal/core/plugin_loader_test.go
git commit -m "feat(plugin): add plugin loader for lazy loading flows and assignees"
```

---

## Task 4: Local Plugin Installation

**Goal:** Implement `tlc plugin install ./path` command

**Files:**
- Modify: `internal/cli/plugin.go` (if exists) or create it
- Test: Manual testing (integration-level)

### Step 1: Check if plugin CLI exists

```bash
ls -la internal/cli/plugin.go
```

If doesn't exist, create it. If exists, we'll extend it.

### Step 2: Create or extend plugin CLI

**File:** `internal/cli/plugin.go`

```go
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"your-module-path/internal/core" // Replace with actual module path
)

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage TLC plugins",
}

var pluginInstallCmd = &cobra.Command{
	Use:   "install <path>",
	Short: "Install a plugin from local directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sourcePath := args[0]

		// Get plugin directory from config (or use default)
		pluginDir := os.ExpandEnv("$HOME/.config/tlc/plugins")
		registryPath := filepath.Join(pluginDir, ".registry.yaml")

		// Ensure plugin directory exists
		err := os.MkdirAll(pluginDir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create plugin directory: %w", err)
		}

		// Load manifest from source
		loader := core.NewPluginLoader()
		manifest, err := loader.LoadManifest(sourcePath)
		if err != nil {
			return fmt.Errorf("failed to load plugin manifest: %w", err)
		}

		// Determine destination path
		destPath := filepath.Join(pluginDir, manifest.Name)

		// Check if plugin already installed
		if _, err := os.Stat(destPath); err == nil {
			return fmt.Errorf("plugin %s is already installed at %s", manifest.Name, destPath)
		}

		// Copy plugin directory
		fmt.Printf("Installing %s v%s...\n", manifest.Name, manifest.Version)
		err = copyDir(sourcePath, destPath)
		if err != nil {
			return fmt.Errorf("failed to copy plugin: %w", err)
		}

		// Register in registry
		registry := core.NewPluginRegistry(registryPath)
		err = registry.Load()
		if err != nil {
			return fmt.Errorf("failed to load registry: %w", err)
		}

		err = registry.Register(manifest, destPath)
		if err != nil {
			return fmt.Errorf("failed to register plugin: %w", err)
		}

		err = registry.Save()
		if err != nil {
			return fmt.Errorf("failed to save registry: %w", err)
		}

		fmt.Printf("✓ Installed %s to %s\n", manifest.Name, destPath)

		// Show what was installed
		if manifest.Provides != nil {
			if len(manifest.Provides.Flows) > 0 {
				fmt.Printf("  Flows:\n")
				for _, flow := range manifest.Provides.Flows {
					fmt.Printf("    - %s\n", flow.ID)
				}
			}
			if len(manifest.Provides.Assignees) > 0 {
				fmt.Printf("  Assignees:\n")
				for _, assignee := range manifest.Provides.Assignees {
					fmt.Printf("    - %s (%s)\n", assignee.ID, assignee.ExecutionType)
				}
			}
		}

		return nil
	},
}

// copyDir recursively copies a directory.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate destination path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Copy file
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(dstPath, data, info.Mode())
	})
}

func init() {
	pluginCmd.AddCommand(pluginInstallCmd)
	rootCmd.AddCommand(pluginCmd)
}
```

### Step 3: Test installation manually

```bash
# Build TLC
go build -o bin/tlc cmd/tlc/main.go

# Install example plugin
./bin/tlc plugin install ./examples/plugins/docker-runner
```

**Expected output:**
```
Installing docker-runner v1.0.0...
✓ Installed docker-runner to /Users/you/.config/tlc/plugins/docker-runner
  Flows:
    - flow:docker:run-container:1.0
  Assignees:
    - assignee:docker-executor:1.0 (executor)
```

### Step 4: Verify installation

```bash
# Check registry file
cat ~/.config/tlc/plugins/.registry.yaml

# Check plugin files copied
ls -la ~/.config/tlc/plugins/docker-runner/
```

**Expected:** Registry file contains entry, plugin files copied

### Step 5: Commit

```bash
git add internal/cli/plugin.go
git commit -m "feat(plugin): add local plugin installation command"
```

---

## Task 5: Update Plugin List Command

**Goal:** Show workflow bundle plugins in `tlc plugin list`

**Files:**
- Modify: `internal/cli/plugin.go`

### Step 1: Add plugin list command

**File:** `internal/cli/plugin.go`

Add after pluginInstallCmd:

```go
var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed plugins",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get plugin directory from config
		pluginDir := os.ExpandEnv("$HOME/.config/tlc/plugins")
		registryPath := filepath.Join(pluginDir, ".registry.yaml")

		// Load registry
		registry := core.NewPluginRegistry(registryPath)
		err := registry.Load()
		if err != nil {
			return fmt.Errorf("failed to load registry: %w", err)
		}

		// List plugins
		entries := registry.List()

		if len(entries) == 0 {
			fmt.Println("No plugins installed")
			return nil
		}

		fmt.Printf("Installed Plugins (%d):\n\n", len(entries))

		for _, entry := range entries {
			fmt.Printf("  %s (v%s)\n", entry.Name, entry.Version)
			fmt.Printf("    Status: %s\n", entry.Status)

			if len(entry.Flows) > 0 {
				fmt.Printf("    Flows: %d\n", len(entry.Flows))
				for _, flowID := range entry.Flows {
					fmt.Printf("      - %s\n", flowID)
				}
			}

			if len(entry.Assignees) > 0 {
				fmt.Printf("    Assignees: %d\n", len(entry.Assignees))
				for _, assigneeID := range entry.Assignees {
					fmt.Printf("      - %s\n", assigneeID)
				}
			}

			if len(entry.Executors) > 0 {
				fmt.Printf("    Executors: %d\n", len(entry.Executors))
			}

			fmt.Println()
		}

		return nil
	},
}
```

Update init function:

```go
func init() {
	pluginCmd.AddCommand(pluginInstallCmd)
	pluginCmd.AddCommand(pluginListCmd) // ADD THIS LINE
	rootCmd.AddCommand(pluginCmd)
}
```

### Step 2: Test list command

```bash
# Build
go build -o bin/tlc cmd/tlc/main.go

# List plugins
./bin/tlc plugin list
```

**Expected output:**
```
Installed Plugins (1):

  docker-runner (v1.0.0)
    Status: installed
    Flows: 1
      - flow:docker:run-container:1.0
    Assignees: 1
      - assignee:docker-executor:1.0
```

### Step 3: Commit

```bash
git add internal/cli/plugin.go
git commit -m "feat(plugin): add plugin list command for workflow bundles"
```

---

## Task 6: Integration Test

**Goal:** End-to-end test of Phase 1 functionality

**Files:**
- Create: `tests/integration/plugin_test.go`

### Step 1: Write integration test

**File:** `tests/integration/plugin_test.go`

```go
//go:build integration
// +build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginInstallAndList(t *testing.T) {
	// Setup: Build tlc binary
	cmd := exec.Command("go", "build", "-o", "../../bin/tlc-test", "../../cmd/tlc/main.go")
	err := cmd.Run()
	if err != nil {
		t.Fatalf("failed to build tlc: %v", err)
	}
	defer os.Remove("../../bin/tlc-test")

	// Setup: Create temp config directory
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Unsetenv("HOME")

	tlcBin := "../../bin/tlc-test"

	// Test: Install plugin
	cmd = exec.Command(tlcBin, "plugin", "install", "../../examples/plugins/docker-runner")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plugin install failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "Installed docker-runner") {
		t.Errorf("expected 'Installed docker-runner' in output, got: %s", outputStr)
	}

	// Test: List plugins
	cmd = exec.Command(tlcBin, "plugin", "list")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plugin list failed: %v\nOutput: %s", err, output)
	}

	outputStr = string(output)
	if !strings.Contains(outputStr, "docker-runner") {
		t.Errorf("expected 'docker-runner' in list output, got: %s", outputStr)
	}

	if !strings.Contains(outputStr, "flow:docker:run-container:1.0") {
		t.Errorf("expected flow ID in list output, got: %s", outputStr)
	}

	// Verify: Check registry file
	registryPath := filepath.Join(tmpDir, ".config", "tlc", "plugins", ".registry.yaml")
	if _, err := os.Stat(registryPath); os.IsNotExist(err) {
		t.Errorf("registry file not created at %s", registryPath)
	}

	// Verify: Check plugin directory
	pluginPath := filepath.Join(tmpDir, ".config", "tlc", "plugins", "docker-runner")
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		t.Errorf("plugin directory not created at %s", pluginPath)
	}

	// Verify: Check manifest copied
	manifestPath := filepath.Join(pluginPath, "manifest.yaml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Errorf("manifest file not copied to %s", manifestPath)
	}
}
```

### Step 2: Run integration test

```bash
go test ./tests/integration -tags=integration -v
```

**Expected:** PASS

### Step 3: Commit

```bash
git add tests/integration/plugin_test.go
git commit -m "test(plugin): add integration test for Phase 1 functionality"
```

---

## Task 7: Documentation

**Goal:** Update documentation with Phase 1 features

**Files:**
- Modify: `docs/flows-and-assignees.md`
- Modify: `examples/plugins/README.md`

### Step 1: Update flows-and-assignees.md

**File:** `docs/flows-and-assignees.md`

Add section after "Implementation Status":

```markdown
## Plugin System (Phase 1 Complete)

TLC now supports installing workflow bundle plugins that extend flows and assignees.

### Installing Plugins

**Local installation:**
```bash
tlc plugin install ./path/to/plugin
```

**List installed plugins:**
```bash
tlc plugin list
```

### Plugin Structure

Plugins use a `manifest.yaml` to declare what they provide:

```yaml
name: my-plugin
version: 1.0.0

provides:
  flows:
    - id: "flow:myplugin:example:1.0"
      file: "flows/example.yaml"

  assignees:
    - id: "assignee:myplugin:executor:1.0"
      file: "assignees/executor.yaml"
      execution_type: executor

requires:
  tlc_version: ">=0.2.0"

permissions:
  - process.spawn
```

See [Plugin Examples](../examples/plugins/README.md) for complete examples.

### Phase 1 Capabilities

✅ **Available Now:**
- Install plugins from local directories
- Plugin registry with lazy loading
- Flow and assignee discovery
- Plugin listing

❌ **Coming Soon (Future Phases):**
- Git-based installation
- Context system and task hierarchy
- Assignee execution
- Plugin registry (central)
```

### Step 2: Update examples/plugins/README.md

**File:** `examples/plugins/README.md`

Add section after "Testing These Examples":

```markdown
## Phase 1 Implementation Complete ✅

The following functionality is now available:

### Working Features

```bash
# Install a plugin locally
tlc plugin install ./examples/plugins/docker-runner

# List installed plugins
tlc plugin list

# Output shows:
# Installed Plugins (1):
#
#   docker-runner (v1.0.0)
#     Status: installed
#     Flows: 1
#       - flow:docker:run-container:1.0
#     Assignees: 1
#       - assignee:docker-executor:1.0
```

### Plugin Installation Path

Plugins are installed to:
```
~/.config/tlc/plugins/
├── .registry.yaml          # Plugin registry index
├── docker-runner/          # Installed plugin
│   ├── manifest.yaml
│   ├── flows/
│   ├── assignees/
│   └── bin/
```

### What's Not Implemented Yet

The following require future phases:
- ❌ Git-based installation (`tlc plugin install github.com/user/plugin`)
- ❌ Flow invocation with plugins
- ❌ Assignee execution
- ❌ Context passing between tasks
- ❌ Delegation and collection mechanisms
```

### Step 3: Commit documentation

```bash
git add docs/flows-and-assignees.md examples/plugins/README.md
git commit -m "docs(plugin): document Phase 1 plugin installation features"
```

---

## Verification

After completing all tasks, verify Phase 1 works end-to-end:

```bash
# 1. Build TLC
go build -o bin/tlc cmd/tlc/main.go

# 2. Install all example plugins
./bin/tlc plugin install ./examples/plugins/docker-runner
./bin/tlc plugin install ./examples/plugins/ci-pipeline
./bin/tlc plugin install ./examples/plugins/test-aggregator

# 3. List plugins
./bin/tlc plugin list

# Expected: Shows all 3 plugins with their flows and assignees

# 4. Check registry
cat ~/.config/tlc/plugins/.registry.yaml

# Expected: YAML with all 3 plugins indexed

# 5. Run all tests
go test ./... -v

# Expected: All tests pass

# 6. Run integration tests
go test ./tests/integration -tags=integration -v

# Expected: Integration test passes
```

---

## Success Criteria

- [x] PluginManifest extended with `provides` section
- [x] Plugin registry implemented with indexing
- [x] Lazy loading infrastructure in place
- [x] `tlc plugin install ./path` works
- [x] `tlc plugin list` shows workflow bundles
- [x] All unit tests pass
- [x] Integration test passes
- [x] Documentation updated
- [x] Example plugins installable

---

## What's Next (Future Phases)

**Phase 2: Context System**
- Parent-child task relationships
- Context policies (reads/delegates/collects/creates)
- Context inheritance and filtering

**Phase 3: Assignee Execution**
- JSON-RPC execution interface
- Delegation mechanism
- Collection/aggregation

**Phase 4: Git Installation**
- Clone from Git repositories
- Version resolution
- Plugin caching

**Phase 5: Registry**
- Central plugin registry
- Search and discovery
- Versioning and updates

---

## Notes

- All file paths are relative to repository root
- Tests use TDD approach: write failing test first
- Commit after each task completion
- Integration test requires `integration` build tag
- Module path placeholder (`your-module-path`) must be replaced with actual path from go.mod
