// Package auth_test provides tests for the auth package.
package auth

import (
	"testing"
)

const testToken = "test-token"

func TestGitHubAuthenticator(t *testing.T) {
	store := NewMockStore()
	auth := NewGitHubAuthenticator(store)

	// Test GetCredential (no network)
	cred := &Credential{
		Service: "github",
		Account: "test-user",
		Payload: CredentialPayload{
			Type:  TypeAPIToken,
			Token: testToken,
		},
	}
	store.Upsert(cred)

	got, err := auth.GetCredential("github", "test-user")
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}
	if got.Payload.Token != testToken {
		t.Errorf("expected %s, got %s", testToken, got.Payload.Token)
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
