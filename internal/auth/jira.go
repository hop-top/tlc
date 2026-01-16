package auth

import (
	"fmt"
	"github.com/andygrunwald/go-jira"
)

// JiraAuthenticator handles Jira authentication
type JiraAuthenticator struct {
	store Store
}

// NewJiraAuthenticator creates a new JiraAuthenticator
func NewJiraAuthenticator(store Store) *JiraAuthenticator {
	return &JiraAuthenticator{
		store: store,
	}
}

// LoginWithAPIToken authenticates using a Jira API Token
func (a *JiraAuthenticator) LoginWithAPIToken(url, email, token, accountName string) (*Credential, error) {
	tp := jira.BasicAuthTransport{
		Username: email,
		Password: token,
	}

	client, err := jira.NewClient(tp.Client(), url)
	if err != nil {
		return nil, fmt.Errorf("failed to create Jira client: %w", err)
	}

	// Validate by getting current user info
	user, _, err := client.User.GetSelf()
	if err != nil {
		return nil, fmt.Errorf("failed to validate Jira credentials: %w", err)
	}

	if accountName == "" {
		accountName = user.EmailAddress
	}

	cred := &Credential{
		Service: "jira",
		Account: accountName,
		Payload: CredentialPayload{
			Type:  TypeAPIToken,
			URL:   url,
			Email: email,
			Token: token,
		},
	}

	if err := a.store.Upsert(cred); err != nil {
		return nil, fmt.Errorf("failed to store Jira credential: %w", err)
	}

	return cred, nil
}
