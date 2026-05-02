package auth

import (
	"strings"
	"testing"
	"time"
)

func TestNewStoreDefaultsToKeychain(t *testing.T) {
	t.Parallel()

	for _, backend := range []string{"", "keyring"} {
		store, err := NewStore("tlc-test", backend)
		if err != nil {
			t.Fatalf("NewStore(%q): %v", backend, err)
		}
		if _, ok := store.(*KeychainStore); !ok {
			t.Fatalf("NewStore(%q) returned %T, want *KeychainStore", backend, store)
		}
	}
}

func TestNewStoreUnknownBackend(t *testing.T) {
	t.Parallel()

	_, err := NewStore("tlc-test", "bogus")
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
	msg := err.Error()
	if !strings.Contains(msg, `unknown backend "bogus"`) {
		t.Errorf("error message missing backend name: %q", msg)
	}
	if !strings.Contains(msg, "memory") || !strings.Contains(msg, "keyring") {
		t.Errorf("error message should list valid backends: %q", msg)
	}
}

func TestKitSecretStoreMemoryRoundTrip(t *testing.T) {
	t.Parallel()

	store, err := NewStore("tlc-test", "memory")
	if err != nil {
		t.Fatalf("NewStore memory: %v", err)
	}
	if _, ok := store.(*KitSecretStore); !ok {
		t.Fatalf("NewStore(memory) returned %T, want *KitSecretStore", store)
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	cred := &Credential{
		Service: "github",
		Account: "test-user",
		Payload: CredentialPayload{
			Type:        TypeOAuth,
			AccessToken: "test-token",
			ExpiresAt:   &expiresAt,
			Scopes:      []string{"repo", "read:user"},
		},
	}

	if err := store.Upsert(cred); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if cred.CreatedAt.IsZero() {
		t.Error("Upsert should set CreatedAt")
	}
	if cred.UpdatedAt.IsZero() {
		t.Error("Upsert should set UpdatedAt")
	}

	got, err := store.Get("github", "test-user")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Account != "test-user" {
		t.Errorf("Account: got %q, want %q", got.Account, "test-user")
	}
	if got.Payload.AccessToken != "test-token" {
		t.Errorf("AccessToken: got %q, want %q", got.Payload.AccessToken, "test-token")
	}
	if got.Payload.Type != TypeOAuth {
		t.Errorf("Type: got %q, want %q", got.Payload.Type, TypeOAuth)
	}
}

func TestKitSecretStoreMemoryUpdatePreservesCreatedAt(t *testing.T) {
	t.Parallel()

	store, err := NewStore("tlc-test", "memory")
	if err != nil {
		t.Fatalf("NewStore memory: %v", err)
	}

	cred := &Credential{
		Service: "github",
		Account: "user-a",
		Payload: CredentialPayload{Type: TypeAPIToken, Token: "v1"},
	}
	if err := store.Upsert(cred); err != nil {
		t.Fatalf("Upsert v1: %v", err)
	}
	created := cred.CreatedAt

	// Force a small delta so UpdatedAt advances on the second write.
	time.Sleep(time.Millisecond)

	cred.Payload.Token = "v2"
	if err := store.Upsert(cred); err != nil {
		t.Fatalf("Upsert v2: %v", err)
	}
	if !cred.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt changed on update: %v -> %v", created, cred.CreatedAt)
	}
	if !cred.UpdatedAt.After(created) {
		t.Errorf("UpdatedAt should advance on update: created=%v updated=%v", created, cred.UpdatedAt)
	}

	got, err := store.Get("github", "user-a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Payload.Token != "v2" {
		t.Errorf("Token: got %q, want v2", got.Payload.Token)
	}
}

func TestKitSecretStoreMemoryDelete(t *testing.T) {
	t.Parallel()

	store, err := NewStore("tlc-test", "memory")
	if err != nil {
		t.Fatalf("NewStore memory: %v", err)
	}

	cred := &Credential{
		Service: "linear",
		Account: "lin-user",
		Payload: CredentialPayload{Type: TypeAPIKey, APIKey: "lin_xxx"},
	}
	if err := store.Upsert(cred); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err := store.Delete("linear", "lin-user"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := store.Get("linear", "lin-user"); err == nil {
		t.Error("expected error after delete, got nil")
	}

	// Deleting a missing key is a no-op (matches KeychainStore behaviour).
	if err := store.Delete("linear", "lin-user"); err != nil {
		t.Errorf("Delete on missing key should be no-op, got: %v", err)
	}
}

func TestKitSecretStoreMemoryList(t *testing.T) {
	t.Parallel()

	store, err := NewStore("tlc-test", "memory")
	if err != nil {
		t.Fatalf("NewStore memory: %v", err)
	}

	for _, account := range []string{"alice", "bob", "carol"} {
		cred := &Credential{
			Service: "jira",
			Account: account,
			Payload: CredentialPayload{Type: TypeAPIToken, Token: account + "-tok"},
		}
		if err := store.Upsert(cred); err != nil {
			t.Fatalf("Upsert %s: %v", account, err)
		}
	}
	// Different service should not bleed into the list.
	other := &Credential{
		Service: "github",
		Account: "alice",
		Payload: CredentialPayload{Type: TypeOAuth, AccessToken: "gh"},
	}
	if err := store.Upsert(other); err != nil {
		t.Fatalf("Upsert github/alice: %v", err)
	}

	accounts, err := store.List("jira")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(accounts) != 3 {
		t.Fatalf("List: got %d accounts, want 3 (%v)", len(accounts), accounts)
	}
	want := []string{"alice", "bob", "carol"}
	for i, a := range want {
		if accounts[i] != a {
			t.Errorf("accounts[%d]: got %q, want %q", i, accounts[i], a)
		}
	}
}

func TestKitSecretStoreEmptyBackendErrors(t *testing.T) {
	t.Parallel()

	if _, err := NewKitSecretStore("tlc-test", ""); err == nil {
		t.Fatal("expected error for empty backend")
	}
}
