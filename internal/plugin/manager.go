package plugin

// Manager handles plugin discovery and lifecycle
type Manager struct {
	pluginsDir string
}

func NewManager(pluginsDir string) *Manager {
	return &Manager{
		pluginsDir: pluginsDir,
	}
}
