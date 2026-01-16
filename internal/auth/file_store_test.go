package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-auth-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	path := filepath.Join(tmpDir, "credentials.enc")
	password := "super-secret"
	store := NewFileStore(path, password)

	service := "github"
	account := "test-user"
	cred := &Credential{
		Service: service,
		Account: account,
		Payload: CredentialPayload{
			Type:        TypeOAuth,
			AccessToken: "test-token",
		},
	}

	// Test Upsert
	err = store.Upsert(cred)
	if err != nil {
		t.Fatalf("failed to upsert: %v", err)
	}

	// Test Get
	got, err := store.Get(service, account)
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}

	if got.Payload.AccessToken != "test-token" {
		t.Errorf("expected access token test-token, got %s", got.Payload.AccessToken)
	}

	// Test with wrong password
	wrongStore := NewFileStore(path, "wrong-password")
	_, err = wrongStore.Get(service, account)
	if err == nil {
		t.Errorf("expected error with wrong password, got nil")
	}

	// Test Delete
	err = store.Delete(service, account)
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	// Test List
	accounts, err := store.List(service)
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(accounts) != 0 {
		t.Errorf("expected 0 accounts, got %d", len(accounts))
	}
}
