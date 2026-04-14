package auth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// awaitOAuthCallback waits for the OAuth callback and exchanges the code for a token.
func awaitOAuthCallback(ctx context.Context, cfg *oauth2.Config, service string, store Store, server *http.Server, codeChan <-chan string, errChan <-chan error) (*Credential, error) {
	select {
	case code := <-codeChan:
		token, err := cfg.Exchange(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("failed to exchange code for token: %w", err)
		}

		_ = server.Shutdown(ctx) //nolint:errcheck // best-effort cleanup

		cred := &Credential{
			Service: service,
			Account: "default",
			Payload: CredentialPayload{
				Type:         TypeOAuth,
				AccessToken:  token.AccessToken,
				RefreshToken: token.RefreshToken,
				ExpiresAt:    &token.Expiry,
			},
		}

		if err := store.Upsert(cred); err != nil {
			return nil, fmt.Errorf("failed to upsert credential: %w", err)
		}

		return cred, nil

	case err := <-errChan:
		_ = server.Shutdown(ctx) //nolint:errcheck // best-effort cleanup
		return nil, err

	case <-time.After(5 * time.Minute):
		_ = server.Shutdown(ctx) //nolint:errcheck // best-effort cleanup
		return nil, fmt.Errorf("authentication timed out")
	}
}
