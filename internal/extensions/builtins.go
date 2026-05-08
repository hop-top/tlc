package extensions

import (
	"charm.land/log/v2"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/auth"
)

// RegisterBuiltins adds all built-in sync extensions to the manager.
// The credential store is resolved through auth.NewDefaultStore so the
// backend selection (auth.backend viper key) is applied uniformly across
// the cli and any future plugin host that calls this. Falls back to the
// keychain backend if the resolver fails so extensions stay registered
// even when an alternate backend is misconfigured at boot.
func RegisterBuiltins(m *Manager) {
	store, err := auth.NewDefaultStore("tlc", viper.GetViper())
	if err != nil {
		log.Warn("auth backend resolution failed; falling back to keychain",
			"backend", viper.GetString(auth.AuthBackendKey), "error", err)
		store = auth.NewKeychainStore("tlc")
	}
	m.Add(NewGitHubSync(store))
	m.Add(NewJiraSync(store))
	m.Add(NewLinearSync(store))
}
