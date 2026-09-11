package core

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/viper"
)

// apsProfileIDEnv is the profile identifier exported by `aps run`.
// aps also exports APS_PROFILE_DIR/_YAML/_SECRETS/_DOCS_DIR; only the
// ID names the actor.
const apsProfileIDEnv = "APS_PROFILE_ID"

// GetCurrentUser resolves the actor attributed to state-mutating
// operations. Precedence, first non-empty wins:
//
//  1. runtime.profile — explicit --profile flag or config
//  2. APS_PROFILE_ID — ambient aps profile (`aps run <id> -- tlc ...`)
//  3. TLC_USER — explicit override
//  4. git config user.name
//  5. $USER
func GetCurrentUser() string {
	if profile := viper.GetString("runtime.profile"); profile != "" {
		return profile
	}
	if profile := os.Getenv(apsProfileIDEnv); profile != "" {
		return profile
	}
	if user := os.Getenv("TLC_USER"); user != "" {
		return user
	}
	cmd := exec.CommandContext(context.Background(), "git", "config", "user.name")
	if out, err := cmd.Output(); err == nil {
		name := strings.TrimSpace(string(out))
		if name != "" {
			return name
		}
	}
	return os.Getenv("USER")
}

// ResolveAssignee normalizes an assignee string using the global
// ProfileResolver. Returns the canonical aps profile ID if matched,
// or the input unchanged otherwise.
func ResolveAssignee(input string) string {
	return GetGlobalResolver().Resolve(input)
}
