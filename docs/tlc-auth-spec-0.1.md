# TLC Authentication Specification v0.1

## Overview

This document defines authentication mechanisms for TLC (Task Line CLI), including credential storage, external system authentication (GitHub, Jira, Linear), and security best practices.

## Design Principles

1. **Secure by default**: Never store credentials in plaintext
2. **OS-native storage**: Use system keychain/credential manager
3. **Minimal permissions**: Request only required OAuth scopes
4. **Multi-account support**: Handle multiple accounts per system
5. **Token refresh**: Automatic OAuth token renewal
6. **Audit trail**: Log authentication events

---

## Authentication Modes

### Local-Only Mode

No authentication required for local TLC usage:

```bash
tlc init
tlc task create "Add feature"
tlc task list

# No credentials needed
```

### External System Authentication

Required for syncing with external systems:

```bash
tlc auth login github
tlc sync pull github
```

---

## Credential Storage

### Storage Backends

TLC uses OS-native credential storage:

| OS | Backend | Location |
|----|---------|----------|
| macOS | Keychain | Keychain Access.app |
| Linux | Secret Service | libsecret / gnome-keyring |
| Windows | Credential Manager | Windows Credential Manager |

### Fallback Storage

If OS keychain unavailable, encrypted file storage:

**Location**: `~/.config/tlc/credentials.enc`

**Encryption**:
- Algorithm: AES-256-GCM
- Key derivation: PBKDF2 with user password
- Salt: Random 32 bytes per credential

**Warning**: User prompted for password on first use:
```bash
tlc auth login github

# If keychain unavailable:
Keychain unavailable. Using encrypted file storage.
Enter password to encrypt credentials: ********
```

### Credential Structure

```json
{
  "service": "tlc-github",
  "account": "codex",
  "credential": {
    "type": "oauth",
    "access_token": "gho_xxxxxxxxxxxxx",
    "refresh_token": "ghr_xxxxxxxxxxxxx",
    "expires_at": "2025-07-16T00:00:00Z",
    "scopes": ["repo", "read:user"]
  },
  "created_at": "2025-01-16T10:00:00Z",
  "updated_at": "2025-01-16T10:00:00Z"
}
```

---

## GitHub Authentication

### Authentication Methods

#### 1. Personal Access Token (PAT)

**Best for**: Individual users, CLI usage

**Setup**:
```bash
# Generate token at: https://github.com/settings/tokens
# Required scopes: repo, read:user

tlc auth login github --token ghp_xxxxxxxxxxxxx
```

**Storage**:
```
Service: tlc-github
Account: <username>
Token: ghp_xxxxxxxxxxxxx
```

#### 2. OAuth App

**Best for**: Interactive login, automatic token refresh

**Flow**:
```bash
tlc auth login github

# Output:
Opening browser for GitHub OAuth...
Authorize TLC at: https://github.com/login/oauth/authorize?...
Waiting for authorization...
✓ Authenticated as @codex
✓ Token saved to keychain
```

**OAuth Configuration**:
```yaml
# Built-in OAuth app (public client)
client_id: tlc_oauth_client_public
scopes:
  - repo          # Access repositories
  - read:user     # Read user profile
  - read:org      # Read organization membership (optional)
```

**Token Refresh**:
```bash
# Automatic refresh when token expires
tlc sync pull github

# If refresh fails:
GitHub token expired. Please re-authenticate:
  tlc auth login github
```

#### 3. GitHub App

**Best for**: Organization-wide deployments, fine-grained permissions

**Setup**:
```bash
# Install GitHub App at: https://github.com/apps/tlc
# Note installation ID

tlc auth login github --app-id 123456 --installation-id 789012 --private-key /path/to/key.pem
```

**Token Generation**:
- TLC generates JWT from private key
- Exchanges JWT for installation access token
- Tokens auto-refresh (valid 1 hour)

### Required Permissions

| Permission | Scope | Reason |
|------------|-------|--------|
| `repo` (read) | Repository contents | Read issue data |
| `repo` (write) | Repository contents | Create/update issues |
| `read:user` | User profile | Get authenticated user info |
| `read:org` | Organization | Support org repositories (optional) |

### Multi-Account Support

```bash
# Add multiple GitHub accounts
tlc auth login github --account personal --token ghp_xxx
tlc auth login github --account work --token ghp_yyy

# Use specific account
tlc sync pull github --account work

# Default account
tlc config set sync.github.default_account work
```

---

## Jira Authentication

### Authentication Methods

#### 1. API Token

**Best for**: Jira Cloud

**Setup**:
```bash
# Generate token at: https://id.atlassian.com/manage-profile/security/api-tokens

tlc auth login jira \
  --url https://company.atlassian.net \
  --email user@company.com \
  --token ATATTxxxxxxxxxxxxx
```

**Storage**:
```json
{
  "service": "tlc-jira",
  "account": "user@company.com",
  "credential": {
    "type": "api_token",
    "url": "https://company.atlassian.net",
    "email": "user@company.com",
    "token": "ATATTxxxxxxxxxxxxx"
  }
}
```

#### 2. OAuth 2.0

**Best for**: Jira Cloud with automatic refresh

**Flow**:
```bash
tlc auth login jira --oauth

# Output:
Opening browser for Jira OAuth...
Authorize TLC at: https://auth.atlassian.com/authorize?...
✓ Authenticated
✓ Token saved to keychain
```

#### 3. Basic Auth

**Best for**: Jira Server/Data Center (deprecated, not recommended)

```bash
tlc auth login jira \
  --url https://jira.company.com \
  --username user \
  --password 'password'

# Warning displayed:
⚠ Basic auth is deprecated. Use API tokens or OAuth.
```

### Required Permissions

| Permission | Reason |
|------------|--------|
| `read:jira-work` | Read issues, projects |
| `write:jira-work` | Create/update issues |
| `read:jira-user` | Read user information |

---

## Linear Authentication

### Authentication Method

#### API Key

**Best for**: All Linear usage

**Setup**:
```bash
# Generate key at: https://linear.app/settings/api

tlc auth login linear --api-key lin_api_xxxxxxxxxxxxx
```

**Storage**:
```json
{
  "service": "tlc-linear",
  "account": "default",
  "credential": {
    "type": "api_key",
    "api_key": "lin_api_xxxxxxxxxxxxx"
  }
}
```

### Required Scopes

| Scope | Reason |
|-------|--------|
| `read` | Read issues, projects |
| `write` | Create/update issues |

---

## CLI Commands

### `tlc auth login`

Authenticate with external system.

#### Synopsis

```bash
tlc auth login <system> [flags]
```

#### Systems

- `github` — GitHub authentication
- `jira` — Jira authentication
- `linear` — Linear authentication

#### Flags

**Common**:
| Flag | Type | Description |
|------|------|-------------|
| `--account` | string | Account name (for multi-account) |
| `--force` | bool | Re-authenticate even if logged in |

**GitHub**:
| Flag | Type | Description |
|------|------|-------------|
| `--token` | string | Personal access token |
| `--app-id` | int | GitHub App ID |
| `--installation-id` | int | GitHub App installation ID |
| `--private-key` | path | GitHub App private key path |

**Jira**:
| Flag | Type | Description |
|------|------|-------------|
| `--url` | string | Jira instance URL |
| `--email` | string | Jira account email |
| `--token` | string | Jira API token |
| `--oauth` | bool | Use OAuth flow |

**Linear**:
| Flag | Type | Description |
|------|------|-------------|
| `--api-key` | string | Linear API key |

#### Examples

```bash
# GitHub OAuth (interactive)
tlc auth login github

# GitHub PAT
tlc auth login github --token ghp_xxxxxxxxxxxxx

# GitHub with account name
tlc auth login github --account work --token ghp_yyy

# Jira API token
tlc auth login jira \
  --url https://company.atlassian.net \
  --email user@company.com \
  --token ATATTxxxxxxxxxxxxx

# Linear API key
tlc auth login linear --api-key lin_api_xxxxxxxxxxxxx
```

---

### `tlc auth logout`

Remove stored credentials.

#### Synopsis

```bash
tlc auth logout <system> [flags]
```

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--account` | string | Account to logout (default: all) |
| `--all` | bool | Logout from all systems |

#### Examples

```bash
# Logout from GitHub
tlc auth logout github

# Logout specific account
tlc auth logout github --account work

# Logout from all systems
tlc auth logout --all
```

---

### `tlc auth status`

Show authentication status.

#### Synopsis

```bash
tlc auth status [system] [flags]
```

#### Examples

```bash
# All systems
tlc auth status

# Output:
GitHub:
  Account: codex (default)
  ✓ Authenticated
  Token expires: 2025-07-16
  Scopes: repo, read:user

  Account: work
  ✓ Authenticated
  Token expires: 2025-07-16

Jira:
  Account: user@company.com
  ✓ Authenticated
  URL: https://company.atlassian.net

Linear:
  ✗ Not authenticated

# Specific system
tlc auth status github

# JSON output
tlc auth status --format json
{
  "github": {
    "accounts": [
      {
        "name": "codex",
        "default": true,
        "authenticated": true,
        "expires_at": "2025-07-16T00:00:00Z",
        "scopes": ["repo", "read:user"]
      }
    ]
  }
}
```

---

### `tlc auth refresh`

Manually refresh tokens.

#### Synopsis

```bash
tlc auth refresh <system> [flags]
```

#### Examples

```bash
# Refresh GitHub token
tlc auth refresh github

# Output:
✓ Token refreshed
New expiration: 2025-07-16T00:00:00Z
```

---

### `tlc auth test`

Test authentication by making API call.

#### Synopsis

```bash
tlc auth test <system>
```

#### Examples

```bash
tlc auth test github

# Output:
Testing GitHub authentication...
✓ Connection successful
✓ Authenticated as: @codex
✓ API rate limit: 4999/5000 remaining
```

---

## Environment Variables

Override credentials via environment variables (for CI/CD):

| Variable | Description |
|----------|-------------|
| `GITHUB_TOKEN` | GitHub personal access token |
| `JIRA_URL` | Jira instance URL |
| `JIRA_EMAIL` | Jira account email |
| `JIRA_TOKEN` | Jira API token |
| `LINEAR_API_KEY` | Linear API key |

**Example**:
```bash
export GITHUB_TOKEN=ghp_xxxxxxxxxxxxx
tlc sync pull github
# Uses token from environment, no keychain access needed
```

**Priority**: Environment variables > Keychain

---

## OAuth Flow Details

### GitHub OAuth Flow

1. **User initiates login**:
   ```bash
   tlc auth login github
   ```

2. **TLC starts local server** (http://localhost:8080):
   ```
   Starting OAuth server on http://localhost:8080...
   ```

3. **TLC opens browser** with authorization URL:
   ```
   https://github.com/login/oauth/authorize?
     client_id=tlc_oauth_client_public&
     redirect_uri=http://localhost:8080/callback&
     scope=repo,read:user&
     state=random_state_token
   ```

4. **User authorizes** on GitHub

5. **GitHub redirects** to localhost:
   ```
   http://localhost:8080/callback?code=auth_code&state=random_state_token
   ```

6. **TLC exchanges code for token**:
   ```bash
   POST https://github.com/login/oauth/access_token
   {
     "client_id": "tlc_oauth_client_public",
     "client_secret": "...",
     "code": "auth_code",
     "redirect_uri": "http://localhost:8080/callback"
   }
   ```

7. **TLC receives token**:
   ```json
   {
     "access_token": "gho_xxxxxxxxxxxxx",
     "refresh_token": "ghr_xxxxxxxxxxxxx",
     "expires_in": 15552000,
     "scope": "repo,read:user"
   }
   ```

8. **TLC stores token** in keychain

9. **Server shuts down**, browser shows success page

### Headless OAuth Flow

For SSH/remote systems without browser:

```bash
tlc auth login github --headless

# Output:
Visit this URL to authorize:
https://github.com/login/oauth/authorize?...

Waiting for authorization (timeout: 5 minutes)...
✓ Authorized
✓ Token saved
```

User opens URL on another device, authorizes, TLC polls for completion.

---

## Token Refresh

### Automatic Refresh

TLC automatically refreshes OAuth tokens before expiration:

```bash
tlc sync pull github

# If token expires in < 24 hours:
Refreshing GitHub token...
✓ Token refreshed

# Then proceeds with sync
```

### Refresh Failure

If refresh fails, user must re-authenticate:

```bash
tlc sync pull github

# Output:
Error: GitHub token expired and refresh failed
Reason: Refresh token invalid

Please re-authenticate:
  tlc auth login github
```

---

## Security Best Practices

### For Users

1. **Never share tokens**: Treat tokens like passwords
2. **Use minimal scopes**: Only request needed permissions
3. **Rotate tokens regularly**: Generate new tokens periodically
4. **Use OAuth over PAT**: OAuth tokens are revocable and scoped
5. **Secure your system**: Keychain is only as secure as OS login
6. **Review authorizations**: Periodically check GitHub/Jira authorized apps
7. **Use separate accounts**: Personal vs work accounts

### For Developers

1. **Never log tokens**: Don't log credentials to files/console
2. **Clear memory**: Zero out token strings after use
3. **Use HTTPS only**: Never send tokens over HTTP
4. **Validate certificates**: Prevent MITM attacks
5. **Handle expiration**: Gracefully handle token expiration
6. **Audit access**: Log authentication events (not credentials)

---

## Credential Migration

### Export Credentials (for backup)

```bash
tlc auth export --output /secure/backup.json

# Prompts for encryption password
Enter password to encrypt backup: ********

✓ Credentials exported to /secure/backup.json
⚠ Store this file securely. It contains sensitive data.
```

**Backup format** (encrypted):
```json
{
  "version": "0.1",
  "exported_at": "2025-01-16T10:00:00Z",
  "credentials": [
    {
      "service": "tlc-github",
      "account": "codex",
      "credential": "<encrypted>"
    }
  ]
}
```

### Import Credentials

```bash
tlc auth import /secure/backup.json

# Prompts for decryption password
Enter password to decrypt backup: ********

✓ Imported 3 credentials
  - GitHub (codex)
  - Jira (user@company.com)
  - Linear (default)
```

---

## Troubleshooting

### Token Not Found

```bash
tlc sync pull github

Error: GitHub credentials not found

To authenticate:
  tlc auth login github
```

### Invalid Token

```bash
tlc sync pull github

Error: GitHub API returned 401 Unauthorized
Reason: Bad credentials

Your token may be expired or revoked.
Re-authenticate:
  tlc auth login github --force
```

### Keychain Access Denied

```bash
tlc auth login github --token ghp_xxx

Error: Keychain access denied

Grant TLC access to keychain in System Preferences > Security & Privacy
Or use encrypted file storage:
  tlc config set auth.storage_backend file
```

### Permission Denied

```bash
tlc sync pull github

Error: GitHub API returned 403 Forbidden
Reason: Resource not accessible by personal access token

Your token is missing required scopes.
Generate new token with scopes: repo, read:user
  https://github.com/settings/tokens/new
```

---

## Audit Logging

### Authentication Events

TLC logs authentication events (not credentials):

```bash
tlc log --action AUTH_LOGIN --action AUTH_LOGOUT

# Output
2025-01-16T10:00:00Z  AUTH_LOGIN    system=github account=codex
2025-01-16T10:30:00Z  AUTH_REFRESH  system=github account=codex
2025-01-16T15:00:00Z  AUTH_LOGOUT   system=github account=work
```

### Audit Log Location

```
~/.config/tlc/audit.log
```

**Format**:
```json
{
  "timestamp": "2025-01-16T10:00:00Z",
  "action": "AUTH_LOGIN",
  "system": "github",
  "account": "codex",
  "ip_address": "127.0.0.1",
  "user_agent": "TLC/0.1.0"
}
```

---

## References

- [tlc-config-spec-0.1.md](tlc-config-spec-0.1.md) — Configuration
- [tlc-plugin-spec-0.1.md](tlc-plugin-spec-0.1.md) — Plugin system
- [sync-architecture-0.1.md](sync-architecture-0.1.md) — Sync architecture
- [GitHub OAuth Apps](https://docs.github.com/en/apps/oauth-apps)
- [Jira API Tokens](https://support.atlassian.com/atlassian-account/docs/manage-api-tokens-for-your-atlassian-account/)
- [Linear API](https://developers.linear.app/docs/graphql/working-with-the-graphql-api#authentication)

---

**Version**: 0.1
**Last Updated**: 2025-01-16
**Status**: Normative
