package auth

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// TestNewDefaultStoreNilResolver confirms a nil resolver routes to the
// keychain backend so callers tolerant of partial wiring (tests, boot
// races) get a working store.
func TestNewDefaultStoreNilResolver(t *testing.T) {
	t.Parallel()

	store, err := NewDefaultStore("tlc-test", nil)
	if err != nil {
		t.Fatalf("NewDefaultStore: %v", err)
	}
	if _, ok := store.(*KeychainStore); !ok {
		t.Fatalf("nil resolver should select KeychainStore, got %T", store)
	}
}

// TestNewDefaultStoreViperKeyringDefault confirms an unset viper key
// behaves like the empty backend literal — keychain.
func TestNewDefaultStoreViperKeyringDefault(t *testing.T) {
	t.Parallel()

	v := viper.New()
	store, err := NewDefaultStore("tlc-test", v)
	if err != nil {
		t.Fatalf("NewDefaultStore: %v", err)
	}
	if _, ok := store.(*KeychainStore); !ok {
		t.Fatalf("unset auth.backend should select KeychainStore, got %T", store)
	}
}

// TestNewDefaultStoreViperMemoryBackend exercises the real viper path
// adopters wire from cli + extensions: setting auth.backend = "memory"
// returns a KitSecretStore configured with that backend.
func TestNewDefaultStoreViperMemoryBackend(t *testing.T) {
	t.Parallel()

	v := viper.New()
	v.Set(AuthBackendKey, "memory")

	store, err := NewDefaultStore("tlc-test", v)
	if err != nil {
		t.Fatalf("NewDefaultStore: %v", err)
	}
	kss, ok := store.(*KitSecretStore)
	if !ok {
		t.Fatalf("memory backend should select KitSecretStore, got %T", store)
	}
	if kss.backend != "memory" {
		t.Errorf("KitSecretStore.backend = %q, want %q", kss.backend, "memory")
	}
}

// TestNewDefaultStoreUnknownBackendReturnsError verifies the resolver
// surfaces the same listing error NewStore emits — adopters fail loud
// at boot rather than silently falling back to keyring.
func TestNewDefaultStoreUnknownBackendReturnsError(t *testing.T) {
	t.Parallel()

	v := viper.New()
	v.Set(AuthBackendKey, "bogus")

	if _, err := NewDefaultStore("tlc-test", v); err == nil {
		t.Fatal("expected error for unknown backend")
	} else if !strings.Contains(err.Error(), `unknown backend "bogus"`) {
		t.Errorf("error missing backend name: %q", err)
	}
}

// stubResolver lets tests assert the resolver is consulted exactly once
// per call and with the configured key.
type stubResolver struct {
	calls []string
	value string
}

func (s *stubResolver) GetString(key string) string {
	s.calls = append(s.calls, key)
	return s.value
}

// TestNewDefaultStoreCallsResolverWithCanonicalKey ensures the
// resolver is queried for AuthBackendKey, not an ad-hoc literal.
func TestNewDefaultStoreCallsResolverWithCanonicalKey(t *testing.T) {
	t.Parallel()

	r := &stubResolver{value: "memory"}
	if _, err := NewDefaultStore("tlc-test", r); err != nil {
		t.Fatalf("NewDefaultStore: %v", err)
	}
	if len(r.calls) != 1 || r.calls[0] != AuthBackendKey {
		t.Errorf("resolver calls = %v, want [%s]", r.calls, AuthBackendKey)
	}
}

// TestAuthBackendKey guards the public viper key against accidental
// rename — both cli/auth.go and extensions/builtins.go pin to it.
func TestAuthBackendKey(t *testing.T) {
	t.Parallel()
	const want = "auth.backend"
	if AuthBackendKey != want {
		t.Errorf("AuthBackendKey = %q, want %q", AuthBackendKey, want)
	}
}
