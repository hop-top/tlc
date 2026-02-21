package plugin

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// PermissionManager handles plugin permission grants.
type PermissionManager struct {
	configPath string
	grants     map[string]PermissionGrant
}

type PermissionGrant struct {
	Granted   []string `yaml:"granted"`
	Denied    []string `yaml:"denied"`
	GrantedAt string   `yaml:"granted_at"`
}

func NewPermissionManager(configDir string) *PermissionManager {
	return &PermissionManager{
		configPath: filepath.Join(configDir, "plugin-permissions.yaml"),
		grants:     make(map[string]PermissionGrant),
	}
}

func (pm *PermissionManager) Load() error {
	if _, err := os.Stat(pm.configPath); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(pm.configPath)
	if err != nil {
		return fmt.Errorf("failed to read permissions file: %w", err)
	}

	if err := yaml.Unmarshal(data, &pm.grants); err != nil {
		return fmt.Errorf("failed to unmarshal permissions: %w", err)
	}
	return nil
}

func (pm *PermissionManager) Save() error {
	data, err := yaml.Marshal(pm.grants)
	if err != nil {
		return fmt.Errorf("failed to marshal permissions: %w", err)
	}
	if err := os.WriteFile(pm.configPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write permissions file: %w", err)
	}
	return nil
}

func (pm *PermissionManager) IsGranted(pluginName, permission string) bool {
	grant, ok := pm.grants[pluginName]
	if !ok {
		return false
	}
	for _, p := range grant.Granted {
		if p == permission {
			return true
		}
	}
	return false
}

func (pm *PermissionManager) Grant(pluginName string, permissions []string) {
	grant := pm.grants[pluginName]
	for _, p := range permissions {
		found := false
		for _, g := range grant.Granted {
			if g == p {
				found = true
				break
			}
		}
		if !found {
			grant.Granted = append(grant.Granted, p)
		}
	}
	pm.grants[pluginName] = grant
}
