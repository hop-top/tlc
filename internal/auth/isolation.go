package auth

import "fmt"

// GetScopedService returns a service name scoped to a specific plugin.
// This ensures that plugins cannot access credentials belonging to other plugins.
func GetScopedService(pluginName, service string) string {
	return fmt.Sprintf("tlc-plugin:%s:%s", pluginName, service)
}

// IsServiceAllowed checks if a plugin is allowed to access a particular service name.
// For v0.1, we strictly enforce that plugins only access services prefixed with their name.
func IsServiceAllowed(pluginName, requestedService string) bool {
	expectedPrefix := fmt.Sprintf("tlc-plugin:%s:", pluginName)
	// Internal tlc services are never allowed for plugins
	if requestedService == "tlc-github" || requestedService == "tlc-jira" || requestedService == "tlc-linear" {
		return false
	}
	// Simplified check for now: must start with the expected prefix
	return requestedService != "" && len(requestedService) >= len(expectedPrefix) && requestedService[:len(expectedPrefix)] == expectedPrefix
}