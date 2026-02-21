package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/v69/github"
	"golang.org/x/oauth2"
)

// GitHubAuthenticator handles GitHub authentication.
type GitHubAuthenticator struct {
	store Store
}

// NewGitHubAuthenticator creates a new GitHubAuthenticator.
func NewGitHubAuthenticator(store Store) *GitHubAuthenticator {
	return &GitHubAuthenticator{
		store: store,
	}
}

// LoginWithPAT authenticates using a Personal Access Token.
func (a *GitHubAuthenticator) LoginWithPAT(ctx context.Context, token string, accountName string) (*Credential, error) {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	// Validate token by getting user info
	user, _, err := client.Users.Get(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to validate GitHub token: %w", err)
	}

	if accountName == "" {
		accountName = user.GetLogin()
	}

	cred := &Credential{
		Service: "github",
		Account: accountName,
		Payload: CredentialPayload{
			Type:  TypeAPIToken,
			Token: token,
		},
	}

	if err := a.store.Upsert(cred); err != nil {
		return nil, fmt.Errorf("failed to store GitHub credential: %w", err)
	}

	return cred, nil
}

// GetCredential retrieves the GitHub credential for an account.
func (a *GitHubAuthenticator) GetCredential(service, account string) (*Credential, error) {
	cred, err := a.store.Get(service, account)
	if err != nil {
		return nil, fmt.Errorf("failed to get credential: %w", err)
	}
	return cred, nil
}

// RefreshToken refreshes the OAuth token if needed.
func (a *GitHubAuthenticator) RefreshToken(ctx context.Context, config *oauth2.Config, account string) (*Credential, error) {
	cred, err := a.GetCredential("github", account)
	if err != nil {
		return nil, err
	}

	if cred.Payload.Type != TypeOAuth || cred.Payload.RefreshToken == "" {
		return cred, nil // Nothing to refresh
	}

	// Check if token is expired or expires soon (e.g., within 5 minutes)
	if cred.Payload.ExpiresAt != nil && time.Until(*cred.Payload.ExpiresAt) > 5*time.Minute {
		return cred, nil
	}

	token := &oauth2.Token{
		AccessToken:  cred.Payload.AccessToken,
		RefreshToken: cred.Payload.RefreshToken,
		Expiry:       *cred.Payload.ExpiresAt,
	}

	ts := config.TokenSource(ctx, token)
	newToken, err := ts.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	cred.Payload.AccessToken = newToken.AccessToken
	cred.Payload.RefreshToken = newToken.RefreshToken
	cred.Payload.ExpiresAt = &newToken.Expiry
	cred.UpdatedAt = time.Now()

	if err := a.store.Upsert(cred); err != nil {
		return nil, fmt.Errorf("failed to upsert credential: %w", err)
	}

	return cred, nil
}
