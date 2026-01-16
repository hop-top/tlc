package auth

import (
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

func TestKeychainStore(t *testing.T) {
	// Use mock keyring for tests
	keyring.MockInit()

	store := NewKeychainStore("tlc-test")
	service := "github"
	account := "test-user"

	expiresAt := time.Now().Add(24 * time.Hour)
	cred := &Credential{
		Service: service,
		Account: account,
		Payload: CredentialPayload{
			Type:        TypeOAuth,
			AccessToken: "test-token",
			ExpiresAt:   &expiresAt,
			Scopes:      []string{"repo", "read:user"},
		},
	}

	// Test Upsert
	err := store.Upsert(cred)
	if err != nil {
		t.Fatalf("failed to upsert: %v", err)
	}

	// Test Get
	got, err := store.Get(service, account)
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}

	if got.Account != account {
		t.Errorf("expected account %s, got %s", account, got.Account)
	}
	if got.Payload.AccessToken != "test-token" {
		t.Errorf("expected access token test-token, got %s", got.Payload.AccessToken)
	}

	// Test Delete
	err = store.Delete(service, account)
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	// Test Get after Delete
	_, err = store.Get(service, account)
	if err == nil {
		t.Errorf("expected error getting deleted credential, got nil")
	}
}
