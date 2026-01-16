package auth

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/zalando/go-keyring"
)

// Store defines the interface for credential storage
type Store interface {
	Get(service, account string) (*Credential, error)
	Upsert(cred *Credential) error
	Delete(service, account string) error
	List(service string) ([]string, error)
}

// KeychainStore implements Store using OS keychain
type KeychainStore struct {
	appName string
}

// NewKeychainStore creates a new KeychainStore
func NewKeychainStore(appName string) *KeychainStore {
	return &KeychainStore{
		appName: appName,
	}
}

func (s *KeychainStore) getFullService(service string) string {
	return fmt.Sprintf("%s-%s", s.appName, service)
}

// Get retrieves a credential from keychain
func (s *KeychainStore) Get(service, account string) (*Credential, error) {
	fullService := s.getFullService(service)
	data, err := keyring.Get(fullService, account)
	if err != nil {
		if err == keyring.ErrNotFound {
			return nil, fmt.Errorf("credential not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get credential from keychain: %w", err)
	}

	var cred Credential
	if err := json.Unmarshal([]byte(data), &cred); err != nil {
		return nil, fmt.Errorf("failed to unmarshal credential: %w", err)
	}

	return &cred, nil
}

// Upsert saves or updates a credential in keychain
func (s *KeychainStore) Upsert(cred *Credential) error {
	if cred.CreatedAt.IsZero() {
		cred.CreatedAt = time.Now()
	}
	cred.UpdatedAt = time.Now()

	data, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("failed to marshal credential: %w", err)
	}

	fullService := s.getFullService(cred.Service)
	if err := keyring.Set(fullService, cred.Account, string(data)); err != nil {
		return fmt.Errorf("failed to set credential in keychain: %w", err)
	}

	return nil
}

// Delete removes a credential from keychain
func (s *KeychainStore) Delete(service, account string) error {
	fullService := s.getFullService(service)
	if err := keyring.Delete(fullService, account); err != nil {
		if err == keyring.ErrNotFound {
			return nil
		}
		return fmt.Errorf("failed to delete credential from keychain: %w", err)
	}
	return nil
}

// List is not natively supported by go-keyring in a cross-platform way for all services.
// Usually, we might need to store a list of accounts separately or just use it as is.
// For now, we'll return an error or implement a simple registry if needed.
func (s *KeychainStore) List(service string) ([]string, error) {
	return nil, fmt.Errorf("list operation not supported by keychain store")
}
