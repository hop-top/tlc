package auth

import (
	"testing"
)

func TestGitHubAuthenticator(t *testing.T) {
	store := NewMockStore()
	auth := NewGitHubAuthenticator(store)

	// Test GetCredential (no network)
	cred := &Credential{
		Service: "github",
		Account: "test-user",
		Payload: CredentialPayload{
			Type:  TypeAPIToken,
			Token: "test-token",
		},
	}
	store.Upsert(cred)

	got, err := auth.GetCredential("github", "test-user")
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}
	if got.Payload.Token != "test-token" {
		t.Errorf("expected test-token, got %s", got.Payload.Token)
	}
}

func TestJiraAuthenticator(t *testing.T) {
	store := NewMockStore()
	auth := NewJiraAuthenticator(store)

	if auth == nil {
		t.Fatal("failed to create JiraAuthenticator")
	}
}

func TestLinearAuthenticator(t *testing.T) {
	store := NewMockStore()
	auth := NewLinearAuthenticator(store)

	if auth == nil {
		t.Fatal("failed to create LinearAuthenticator")
	}
}
