package core

// PluginType defines the category of a plugin.
type PluginType string

const (
	PluginTypeExternalSync PluginType = "external-sync"
	PluginTypeTaskExecutor PluginType = "task-executor"
	PluginTypeFormatter    PluginType = "formatter"
	PluginTypeNotification PluginType = "notification"
	PluginTypeStorage      PluginType = "storage"
)

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
}

// PluginRequirements defines the system requirements for a plugin.
type PluginRequirements struct {
	TLCVersion string `json:"tlc_version" yaml:"tlc_version"`
}
