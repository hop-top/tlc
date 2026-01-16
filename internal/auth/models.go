package auth

import "time"

// CredentialType defines the type of credential
type CredentialType string

const (
	TypeOAuth    CredentialType = "oauth"
	TypeAPIToken CredentialType = "api_token"
	TypeAPIKey   CredentialType = "api_key"
)

// CredentialPayload contains the actual sensitive data
type CredentialPayload struct {
	Type         CredentialType    `json:"type"`
	AccessToken  string            `json:"access_token,omitempty"`
	RefreshToken string            `json:"refresh_token,omitempty"`
	ExpiresAt    *time.Time        `json:"expires_at,omitempty"`
	Scopes       []string          `json:"scopes,omitempty"`
	URL          string            `json:"url,omitempty"`
	Email        string            `json:"email,omitempty"`
	Token        string            `json:"token,omitempty"`   // For API tokens
	APIKey       string            `json:"api_key,omitempty"` // For API keys
}

// Credential represents a stored credential
type Credential struct {
	Service    string            `json:"service"`
	Account    string            `json:"account"`
	Payload    CredentialPayload `json:"credential"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}
