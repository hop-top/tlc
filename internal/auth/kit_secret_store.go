package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"hop.top/kit/go/storage/secret"
	// Register backends so callers can select any of them via Config.Backend.
	_ "hop.top/kit/go/storage/secret/agefile"
	_ "hop.top/kit/go/storage/secret/ghsecrets"
	_ "hop.top/kit/go/storage/secret/infisical"
	_ "hop.top/kit/go/storage/secret/keyring"
	_ "hop.top/kit/go/storage/secret/memory"
	_ "hop.top/kit/go/storage/secret/onepassword"
	_ "hop.top/kit/go/storage/secret/openbao"
)

// validKitBackends lists the backend names accepted by NewStore beyond the
// pass-through "keyring" / empty options handled by KeychainStore.
var validKitBackends = []string{
	"agefile",
	"env",
	"ghsecrets",
	"infisical",
	"keyring",
	"memory",
	"onepassword",
	"openbao",
}

// KitSecretStore wraps a kit secret.MutableStore so the auth package can
// continue to use the (service, account) addressing the rest of tlc expects.
//
// Keys are encoded as "<service>/<account>" so all credentials live in a flat
// namespace inside the underlying backend. Values are JSON-encoded Credential
// records, matching the wire format already used by KeychainStore.
type KitSecretStore struct {
	appName string
	backend string
	store   secret.MutableStore
}

// NewKitSecretStore builds a Store backed by the kit secret package.
//
// appName is used as the keyring service identifier (when relevant) and is
// prefixed onto every key so multiple apps can share a backend without
// colliding. backend selects the kit backend (memory, agefile, openbao, ...).
func NewKitSecretStore(appName, backend string) (*KitSecretStore, error) {
	if backend == "" {
		return nil, errors.New("kit secret store: backend is required")
	}
	cfg := secret.Config{
		Backend: backend,
		// kit's keyring backend uses Service; tlc's KeychainStore uses
		// "tlc-<service>" per-credential. Keep the service stable here and
		// encode the per-credential service in the key instead.
		Service: appName,
	}
	store, err := secret.Open(cfg)
	if err != nil {
		return nil, fmt.Errorf(
			"kit secret store: open backend %q: %w; valid backends: %s",
			backend, err, strings.Join(validKitBackends, ", "),
		)
	}
	return &KitSecretStore{
		appName: appName,
		backend: backend,
		store:   store,
	}, nil
}

func (s *KitSecretStore) key(service, account string) string {
	return fmt.Sprintf("%s/%s/%s", s.appName, service, account)
}

func (s *KitSecretStore) servicePrefix(service string) string {
	return fmt.Sprintf("%s/%s/", s.appName, service)
}

// Get retrieves a credential from the kit-backed store.
func (s *KitSecretStore) Get(service, account string) (*Credential, error) {
	ctx := context.Background()
	sec, err := s.store.Get(ctx, s.key(service, account))
	if err != nil {
		if errors.Is(err, secret.ErrNotFound) {
			return nil, fmt.Errorf("credential not found for %s/%s; run 'tlc auth login %s' to authenticate: %w", service, account, service, err)
		}
		return nil, fmt.Errorf("failed to get credential from %s backend: %w", s.backend, err)
	}
	var cred Credential
	if err := json.Unmarshal(sec.Value, &cred); err != nil {
		return nil, fmt.Errorf("failed to unmarshal credential: %w", err)
	}
	return &cred, nil
}

// Upsert saves or updates a credential.
func (s *KitSecretStore) Upsert(cred *Credential) error {
	if cred == nil {
		return errors.New("credential is nil")
	}
	if cred.CreatedAt.IsZero() {
		cred.CreatedAt = time.Now()
	}
	cred.UpdatedAt = time.Now()

	data, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("failed to marshal credential: %w", err)
	}

	ctx := context.Background()
	if err := s.store.Set(ctx, s.key(cred.Service, cred.Account), data); err != nil {
		return fmt.Errorf("failed to set credential in %s backend: %w", s.backend, err)
	}
	return nil
}

// Delete removes a credential. Missing keys are treated as success, matching
// the behaviour of KeychainStore.
func (s *KitSecretStore) Delete(service, account string) error {
	ctx := context.Background()
	if err := s.store.Delete(ctx, s.key(service, account)); err != nil {
		if errors.Is(err, secret.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("failed to delete credential from %s backend: %w", s.backend, err)
	}
	return nil
}

// List returns the accounts stored for the given service.
//
// Backends that return ErrNotSupported (notably the keyring backend) surface
// the same error contract as KeychainStore.
func (s *KitSecretStore) List(service string) ([]string, error) {
	ctx := context.Background()
	prefix := s.servicePrefix(service)
	keys, err := s.store.List(ctx, prefix)
	if err != nil {
		if errors.Is(err, secret.ErrNotSupported) {
			return nil, fmt.Errorf("list operation not supported by %s backend", s.backend)
		}
		return nil, fmt.Errorf("failed to list credentials from %s backend: %w", s.backend, err)
	}
	accounts := make([]string, 0, len(keys))
	for _, k := range keys {
		accounts = append(accounts, strings.TrimPrefix(k, prefix))
	}
	sort.Strings(accounts)
	return accounts, nil
}

// BackendResolver returns the configured auth backend name. Implemented
// by *viper.Viper (GetString) and any other config source that exposes
// a string-keyed lookup. Defining the seam here lets auth stay free of a
// hard viper import while letting CLI / extensions / future plugins
// share a single backend selector.
type BackendResolver interface {
	GetString(key string) string
}

// AuthBackendKey is the viper config key consulted by NewDefaultStore.
// Centralising the literal lets adopters reuse the same key in tests
// and config docs.
const AuthBackendKey = "auth.backend"

// NewDefaultStore returns a Store using the backend selected by the
// supplied resolver under AuthBackendKey. Empty / unset routes to the
// legacy keychain backend so existing users see no behaviour change.
//
// Use this from any tlc subsystem that needs a credential store (cli,
// extensions, future plugins) so the resolver is the single source of
// truth — never construct KeychainStore / KitSecretStore directly.
//
// A nil resolver behaves like an empty resolver and therefore selects
// the keyring backend, keeping callers tolerant of partial wiring (e.g.
// boot-time tests that haven't initialised viper yet).
func NewDefaultStore(appName string, r BackendResolver) (Store, error) {
	var backend string
	if r != nil {
		backend = r.GetString(AuthBackendKey)
	}
	return NewStore(appName, backend)
}

// NewStore returns a Store implementation selected by backend name.
//
// "" or "keyring" returns the existing KeychainStore so behaviour stays bit-
// identical for users who have not opted in to the kit-backed path. Any other
// known backend is routed through KitSecretStore. Unknown backends return an
// error listing valid options.
func NewStore(appName, backend string) (Store, error) {
	switch backend {
	case "", "keyring":
		return NewKeychainStore(appName), nil
	}
	for _, b := range validKitBackends {
		if backend == b {
			return NewKitSecretStore(appName, backend)
		}
	}
	return nil, fmt.Errorf(
		"auth: unknown backend %q; valid backends: %s",
		backend, strings.Join(validKitBackends, ", "),
	)
}
