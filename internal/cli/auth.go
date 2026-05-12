package cli

import (
	"context"
	"fmt"

	"charm.land/log/v2"
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
// rules.
func authStore() auth.Store {
	store, err := auth.NewDefaultStore("tlc", viper.GetViper())
	if err != nil {
		log.Fatal("Failed to open credential store",
			"backend", viper.GetString(auth.AuthBackendKey), "error", err)
	}
	return store
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
	Run: func(_ *cobra.Command, args []string) {
		system := args[0]
		store := authStore()

		ctx := context.Background()

		switch system {
		case "github":
			authenticator := auth.NewGitHubAuthenticator(store)
			if token != "" {
				cred, err := authenticator.LoginWithPAT(ctx, token, account)
				if err != nil {
					log.Fatal("Failed to login with GitHub PAT", "error", err)
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
					log.Fatal("Failed to login with GitHub OAuth", "error", err)
				}
				fmt.Printf("✓ Authenticated as %s\n", cred.Account)
			}

		case "jira":
			authenticator := auth.NewJiraAuthenticator(store)
			if jiraURL == "" || jiraEmail == "" || token == "" {
				log.Fatal("Missing required flags for Jira login: --url, --email, --token")
			}
			cred, err := authenticator.LoginWithAPIToken(jiraURL, jiraEmail, token, account)
			if err != nil {
				log.Fatal("Failed to login with Jira API token", "error", err)
			}
			fmt.Printf("✓ Authenticated as %s\n", cred.Account)

		case "linear":
			authenticator := auth.NewLinearAuthenticator(store)
			if linearKey == "" {
				log.Fatal("Missing required flag for Linear login: --api-key")
			}
			cred, err := authenticator.LoginWithAPIKey(ctx, linearKey, account)
			if err != nil {
				log.Fatal("Failed to login with Linear API key", "error", err)
			}
			fmt.Printf("✓ Authenticated as %s\n", cred.Account)

		default:
			log.Fatal("Unsupported system", "system", system)
		}
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
		store := authStore()
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
	Run: func(_ *cobra.Command, args []string) {
		store := authStore()
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
