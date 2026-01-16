package plugin

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/oss-tlc-cli/internal/core"
	"gopkg.in/yaml.v3"
)

// Manager handles plugin discovery and lifecycle
type Manager struct {
	pluginsDir string
	plugins    map[string]*core.PluginManifest
}

func NewManager(pluginsDir string) *Manager {
	return &Manager{
		pluginsDir: pluginsDir,
		plugins:    make(map[string]*core.PluginManifest),
	}
}

// Discover scans the plugins directory for manifest files.
func (m *Manager) Discover() error {
	if _, err := os.Stat(m.pluginsDir); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(m.pluginsDir)
	if err != nil {
		return fmt.Errorf("failed to read plugins directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(m.pluginsDir, entry.Name(), "manifest.yaml")
		if _, err := os.Stat(manifestPath); err != nil {
			continue
		}

		manifest, err := m.loadManifest(manifestPath)
		if err != nil {
			fmt.Printf("Warning: failed to load plugin manifest at %s: %v\n", manifestPath, err)
			continue
		}

		// Ensure name matches directory name as per spec
		if manifest.Name != entry.Name() {
			fmt.Printf("Warning: plugin name %s does not match directory name %s\n", manifest.Name, entry.Name())
		}

		m.plugins[manifest.Name] = manifest
	}

	return nil
}

func (m *Manager) loadManifest(path string) (*core.PluginManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var manifest core.PluginManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

func (m *Manager) List() []*core.PluginManifest {
	var list []*core.PluginManifest
	for _, p := range m.plugins {
		list = append(list, p)
	}
	return list
}
