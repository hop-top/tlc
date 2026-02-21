package auth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

// GitHubOAuthConfig returns the OAuth2 configuration for GitHub.
func GitHubOAuthConfig(clientID, clientSecret string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       []string{"repo", "read:user"},
		Endpoint:     github.Endpoint,
		RedirectURL:  "http://localhost:8080/callback",
	}
}

// LoginWithOAuth performs the interactive OAuth flow.
func (a *GitHubAuthenticator) LoginWithOAuth(ctx context.Context, config *oauth2.Config) (*Credential, error) {
	state := "random-state-token" // In production, this should be random
	authURL := config.AuthCodeURL(state, oauth2.AccessTypeOffline)

	fmt.Printf("Opening browser for GitHub OAuth...\n")
	fmt.Printf("Authorize TLC at: %s\n", authURL)

	// Start local server to receive callback
	codeChan := make(chan string)
	errChan := make(chan error)

	server := &http.Server{
		Addr:              ":8080",
		ReadHeaderTimeout: 10 * time.Second,
	}

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
		_, _ = fmt.Fprintf(w, "Authentication successful! You can close this window.")
		codeChan <- code
	})

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	return awaitOAuthCallback(ctx, config, "github", a.store, server, codeChan, errChan)
}
