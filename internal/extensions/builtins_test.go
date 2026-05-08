package extensions

import (
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/auth"
)

// TestRegisterBuiltins_HonoursMemoryBackend asserts the global viper
// key auth.backend = "memory" routes both built-in registrations
// through KitSecretStore — proves cli/auth.go and extensions/builtins.go
// share a single backend selector via auth.NewDefaultStore.
//
// Resets viper at the end so subsequent tests in the package see a
// clean baseline (viper.GetViper returns the package-global instance).
func TestRegisterBuiltins_HonoursMemoryBackend(t *testing.T) {
	prev := viper.Get(auth.AuthBackendKey)
	viper.Set(auth.AuthBackendKey, "memory")
	t.Cleanup(func() { viper.Set(auth.AuthBackendKey, prev) })

	m := New(nil, nil)
	RegisterBuiltins(m)

	exts := m.Extensions()
	if got := len(exts); got != 3 {
		t.Fatalf("expected 3 extensions, got %d", got)
	}

	for _, e := range exts {
		sp, ok := e.(SyncProvider)
		if !ok {
			t.Fatalf("extension %s does not implement SyncProvider", e.Meta().Name)
		}
		store := sp.Store()
		if _, ok := store.(*auth.KitSecretStore); !ok {
			t.Errorf("%s store = %T, want *auth.KitSecretStore", sp.ServiceName(), store)
		}
	}
}

// TestRegisterBuiltins_DefaultsToKeychain confirms the legacy path is
// preserved when no backend is configured — adopters that never opt in
// see no behaviour change.
func TestRegisterBuiltins_DefaultsToKeychain(t *testing.T) {
	prev := viper.Get(auth.AuthBackendKey)
	viper.Set(auth.AuthBackendKey, "")
	t.Cleanup(func() { viper.Set(auth.AuthBackendKey, prev) })

	m := New(nil, nil)
	RegisterBuiltins(m)

	exts := m.Extensions()
	if got := len(exts); got != 3 {
		t.Fatalf("expected 3 extensions, got %d", got)
	}

	for _, e := range exts {
		sp, ok := e.(SyncProvider)
		if !ok {
			t.Fatalf("extension %s does not implement SyncProvider", e.Meta().Name)
		}
		if _, ok := sp.Store().(*auth.KeychainStore); !ok {
			t.Errorf("%s store = %T, want *auth.KeychainStore", sp.ServiceName(), sp.Store())
		}
	}
}
