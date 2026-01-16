package auth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// JiraOAuthConfig returns the OAuth2 configuration for Jira
func JiraOAuthConfig(clientID, clientSecret string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       []string{"read:jira-work", "write:jira-work", "read:jira-user"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://auth.atlassian.com/authorize",
			TokenURL: "https://auth.atlassian.com/oauth/token",
		},
		RedirectURL: "http://localhost:8081/callback",
	}
}

// LoginWithJiraOAuth performs the interactive OAuth flow for Jira
func (a *JiraAuthenticator) LoginWithJiraOAuth(ctx context.Context, config *oauth2.Config) (*Credential, error) {
	state := "random-jira-state"
	// Jira OAuth requires audience parameter in AuthCodeURL for API access
	authURL := config.AuthCodeURL(state, oauth2.SetAuthURLParam("audience", "api.atlassian.com"), oauth2.AccessTypeOffline)

	fmt.Printf("Opening browser for Jira OAuth...\n")
	fmt.Printf("Authorize TLC at: %s\n", authURL)

	codeChan := make(chan string)
	errChan := make(chan error)

	server := &http.Server{Addr: ":8081"}

	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errChan <- fmt.Errorf("invalid state token")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("no code received")
			return
		}
		fmt.Fprintf(w, "Authentication successful! You can close this window.")
		codeChan <- code
	})

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	select {
	case code := <-codeChan:
		token, err := config.Exchange(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("failed to exchange code for token: %w", err)
		}

		server.Shutdown(ctx)

		cred := &Credential{
			Service: "jira",
			Account: "default",
			Payload: CredentialPayload{
				Type:         TypeOAuth,
				AccessToken:  token.AccessToken,
				RefreshToken: token.RefreshToken,
				ExpiresAt:    &token.Expiry,
			},
		}

		if err := a.store.Upsert(cred); err != nil {
			return nil, err
		}

		return cred, nil

	case err := <-errChan:
		server.Shutdown(ctx)
		return nil, err

	case <-time.After(5 * time.Minute):
		server.Shutdown(ctx)
		return nil, fmt.Errorf("authentication timed out")
	}
}
