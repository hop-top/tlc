package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/auth"
)

var (
	token     string
	account   string
	jiraURL   string
	jiraEmail string
	linearKey string
	oauthFlow bool
)

// authStore resolves the credential store backend from viper. Delegates
// to auth.NewDefaultStore so cli, extensions, and future plugins share
// the same backend selector — see auth.NewDefaultStore for resolution
// rules. Returns an error envelope so callers thread failures through
// cobra RunE rather than terminating via log.Fatal.
func authStore() (auth.Store, error) {
	store, err := auth.NewDefaultStore("tlc", viper.GetViper())
	if err != nil {
		return nil, fmt.Errorf("failed to open credential store (backend=%s): %w",
			viper.GetString(auth.AuthBackendKey), err)
	}
	return store, nil
}

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication for external systems",
}

var loginCmd = &cobra.Command{
	Use:   "login <system>",
	Short: "Authenticate with an external system",
	Long: `Authenticate with an external system (github, jira, or linear).

GitHub supports either a personal access token (--token) or OAuth.
Jira requires --url, --email, and --token. Linear requires --api-key.
Credentials are written to the configured auth store (keyring by default).`,
	Args: cobra.ExactArgs(1),
	Annotations: map[string]string{
		"kit/side-effect": "interactive",
		"kit/idempotent":  "yes",
	},
	RunE: func(_ *cobra.Command, args []string) error {
		system := args[0]
		store, err := authStore()
		if err != nil {
			return err
		}

		ctx := context.Background()

		switch system {
		case "github":
			authenticator := auth.NewGitHubAuthenticator(store)
			if token != "" {
				cred, err := authenticator.LoginWithPAT(ctx, token, account)
				if err != nil {
					return fmt.Errorf("failed to login with GitHub PAT: %w", err)
				}
				fmt.Printf("✓ Authenticated as %s\n", cred.Account)
			} else {
				// OAuth flow
				config := auth.GitHubOAuthConfig(
					viper.GetString("auth.github.client_id"),
					viper.GetString("auth.github.client_secret"),
				)
				cred, err := authenticator.LoginWithOAuth(ctx, config)
				if err != nil {
					return fmt.Errorf("failed to login with GitHub OAuth: %w", err)
				}
				fmt.Printf("✓ Authenticated as %s\n", cred.Account)
			}

		case "jira":
			authenticator := auth.NewJiraAuthenticator(store)
			if jiraURL == "" || jiraEmail == "" || token == "" {
				return fmt.Errorf("missing required flags for Jira login: --url, --email, --token")
			}
			cred, err := authenticator.LoginWithAPIToken(jiraURL, jiraEmail, token, account)
			if err != nil {
				return fmt.Errorf("failed to login with Jira API token: %w", err)
			}
			fmt.Printf("✓ Authenticated as %s\n", cred.Account)

		case "linear":
			authenticator := auth.NewLinearAuthenticator(store)
			if linearKey == "" {
				return fmt.Errorf("missing required flag for Linear login: --api-key")
			}
			cred, err := authenticator.LoginWithAPIKey(ctx, linearKey, account)
			if err != nil {
				return fmt.Errorf("failed to login with Linear API key: %w", err)
			}
			fmt.Printf("✓ Authenticated as %s\n", cred.Account)

		default:
			return fmt.Errorf("unsupported system: %s", system)
		}
		return nil
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout <system>",
	Short: "Remove stored credentials",
	Long: `Remove stored credentials for an external system (github, jira, or linear).

Deletes the credential entry from the configured auth store. Safe to
re-run; missing entries surface a non-fatal error.`,
	Args: cobra.ExactArgs(1),
	Annotations: map[string]string{
		"kit/side-effect": "destructive-local",
		"kit/idempotent":  "yes",
	},
	RunE: func(_ *cobra.Command, args []string) error {
		system := args[0]
		store, err := authStore()
		if err != nil {
			return err
		}
		if err := store.Delete(system, account); err != nil {
			return fmt.Errorf("failed to logout %s: %w", system, err)
		}
		fmt.Printf("✓ Logged out from %s\n", system)
		return nil
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status [system]",
	Short: "Show authentication status",
	Long: `Report whether credentials are stored for each external system.

Without arguments, checks github, jira, and linear. Pass a system name
to limit the check to that system.`,
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
	RunE: func(_ *cobra.Command, args []string) error {
		store, err := authStore()
		if err != nil {
			return err
		}
		systems := []string{"github", "jira", "linear"}
		if len(args) > 0 {
			systems = []string{args[0]}
		}

		for _, sys := range systems {
			cred, err := store.Get(sys, account)
			if err != nil {
				fmt.Printf("%s: ✗ Not authenticated\n", sys)
				continue
			}
			fmt.Printf("%s: ✓ Authenticated as %s\n", sys, cred.Account)
		}
		return nil
	},
}

func init() {
	loginCmd.Flags().StringVar(&token, "token", "", "GitHub PAT or Jira API token")
	loginCmd.Flags().StringVar(&account, "account", "", "Account name")
	loginCmd.Flags().StringVar(&jiraURL, "url", "", "Jira instance URL")
	loginCmd.Flags().StringVar(&jiraEmail, "email", "", "Jira account email")
	loginCmd.Flags().StringVar(&linearKey, "api-key", "", "Linear API key")
	loginCmd.Flags().BoolVar(&oauthFlow, "oauth", false, "Use OAuth flow")

	authCmd.AddCommand(loginCmd)
	authCmd.AddCommand(logoutCmd)
	authCmd.AddCommand(authStatusCmd)
	RootCmd.AddCommand(authCmd)
}
