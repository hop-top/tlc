package auth

import (
	"context"
	"fmt"

	"github.com/machinebox/graphql"
)

// LinearAuthenticator handles Linear authentication.
type LinearAuthenticator struct {
	store Store
}

// NewLinearAuthenticator creates a new LinearAuthenticator.
func NewLinearAuthenticator(store Store) *LinearAuthenticator {
	return &LinearAuthenticator{
		store: store,
	}
}

// LoginWithAPIKey authenticates using a Linear API Key.
func (a *LinearAuthenticator) LoginWithAPIKey(ctx context.Context, apiKey, accountName string) (*Credential, error) {
	client := graphql.NewClient("https://api.linear.app/graphql")

	// Validate by getting current user info
	req := graphql.NewRequest(`
		query {
			viewer {
				id
				name
				email
			}
		}
	`)
	req.Header.Set("Authorization", apiKey)

	var resp struct {
		Viewer struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"viewer"`
	}

	if err := client.Run(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to validate Linear API key: %w", err)
	}

	if accountName == "" {
		accountName = resp.Viewer.Email
	}

	cred := &Credential{
		Service: "linear",
		Account: accountName,
		Payload: CredentialPayload{
			Type:   TypeAPIKey,
			APIKey: apiKey,
		},
	}

	if err := a.store.Upsert(cred); err != nil {
		return nil, fmt.Errorf("failed to store Linear credential: %w", err)
	}

	return cred, nil
}
