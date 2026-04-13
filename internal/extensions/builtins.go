package extensions

import "hop.top/tlc/internal/auth"

// RegisterBuiltins adds all built-in sync extensions to the manager.
// The credential store is shared across providers.
func RegisterBuiltins(m *Manager) {
	store := auth.NewKeychainStore("tlc")
	m.Add(NewGitHubSync(store))
	m.Add(NewJiraSync(store))
	m.Add(NewLinearSync(store))
}
