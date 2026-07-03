package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

// applyConfigDefaults seeds unset flags on cmd from config, using the
// resolution ladder. For every flag the user did NOT set on the CLI
// (!cmd.Flags().Changed(name)) that resolves to a set config key, it sets
// the flag's value from config. Returns the set of flag names sourced from
// config so callers can treat them like explicitly-provided flags. Does
// NOT flip pflag's Changed bit.
func applyConfigDefaults(cmd *cobra.Command) (fromConfig map[string]bool, err error) {
	fromConfig = make(map[string]bool)

	var applyErr error
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if applyErr != nil {
			return
		}
		if cmd.Flags().Changed(flag.Name) {
			return
		}
		key, ok := resolveFlagDefaultKey(cmd, flag.Name)
		if !ok {
			return
		}
		switch flag.Value.Type() {
		case "stringSlice":
			if sv, ok := flag.Value.(pflag.SliceValue); ok {
				if e := sv.Replace(viper.GetStringSlice(key)); e != nil {
					applyErr = e
					return
				}
			}
		case "bool":
			if e := flag.Value.Set(strconv.FormatBool(viper.GetBool(key))); e != nil {
				applyErr = e
				return
			}
		case "int", "int64":
			if e := flag.Value.Set(fmt.Sprint(viper.GetInt(key))); e != nil {
				applyErr = e
				return
			}
		default:
			if e := flag.Value.Set(viper.GetString(key)); e != nil {
				applyErr = e
				return
			}
		}
		fromConfig[flag.Name] = true
	})

	if applyErr != nil {
		return nil, applyErr
	}
	return fromConfig, nil
}
