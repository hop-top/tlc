package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// configDomain maps a cobra parent command name to its config namespace.
// Special mappings:
//   - "track" -> "tracks"
//   - "task" -> "task"
//   - anything else -> unchanged
func configDomain(parent string) string {
	if parent == "track" {
		return "tracks"
	}
	return parent
}

// resolveFlagDefaultKey returns the first viper key in the resolution
// ladder for which viper.IsSet(key) is true, or ("", false) if none match.
// It does NOT read the value — callers read it so type handling stays with
// the flag.
//
// The resolution ladder (first hit wins):
//
// 1. <domain>.<verb>.<flag>   e.g. "task.list.columns"
// 2. defaults.<verb>.<flag>   e.g. "defaults.list.columns"
// 3. defaults.<flag>          e.g. "defaults.columns"
//
// Where:
//   - verb = cmd.Name() (leaf command name)
//   - domain = configDomain(cmd.Parent().Name())
//   - flag = the flag name verbatim
//
// If cmd has no parent (root), skip step 1 and probe only steps 2 and 3.
func resolveFlagDefaultKey(cmd *cobra.Command, flag string) (key string, ok bool) {
	verb := cmd.Name()

	// Step 1: Try <domain>.<verb>.<flag> if cmd has a parent.
	if parent := cmd.Parent(); parent != nil {
		domain := configDomain(parent.Name())
		candidateKey := domain + "." + verb + "." + flag
		if viper.IsSet(candidateKey) {
			return candidateKey, true
		}
	}

	// Step 2: Try defaults.<verb>.<flag>.
	candidateKey := "defaults." + verb + "." + flag
	if viper.IsSet(candidateKey) {
		return candidateKey, true
	}

	// Step 3: Try defaults.<flag>.
	candidateKey = "defaults." + flag
	if viper.IsSet(candidateKey) {
		return candidateKey, true
	}

	return "", false
}
