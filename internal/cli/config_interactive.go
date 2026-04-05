package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
)

// runWizard runs the interactive prompt loop over the given keys.
// It reads from r and writes prompts to w. Returns changed key->value pairs.
func runWizard(
	keys []keyHint,
	r io.Reader,
	w io.Writer,
	noColor bool,
) (map[string]string, error) {
	scanner := bufio.NewScanner(r)
	changes := map[string]string{}

	bold := "\033[1m"
	reset := "\033[0m"
	dim := "\033[2m"
	if noColor {
		bold, reset, dim = "", "", ""
	}

	for _, kh := range keys {
		if kh.IsMap {
			fmt.Fprintf(w, "%s%s%s: use 'tlc config set %s.<key> <value>'\n",
				dim, kh.Key, reset, kh.Key)
			continue
		}

		current := fmt.Sprintf("%v", viper.Get(kh.Key))
		prompt := formatPrompt(kh, current, bold, reset, dim)

		for {
			fmt.Fprint(w, prompt)

			if !scanner.Scan() {
				// EOF or Ctrl-C
				return nil, fmt.Errorf("aborted")
			}

			line := strings.TrimSpace(scanner.Text())

			// ? = show description
			if line == "?" {
				fmt.Fprintf(w, "  %s%s%s\n", dim, kh.Description, reset)
				continue
			}

			// Empty = keep current
			if line == "" {
				break
			}

			// Validate enum
			if len(kh.Enum) > 0 {
				valid := false
				for _, e := range kh.Enum {
					if strings.EqualFold(line, e) {
						line = e
						valid = true
						break
					}
				}
				if !valid {
					fmt.Fprintf(w, "  invalid: must be one of %s\n",
						strings.Join(kh.Enum, "/"))
					continue
				}
			}

			// Validate bool
			if kh.IsBool {
				switch strings.ToLower(line) {
				case "y", "yes", "true":
					line = "true"
				case "n", "no", "false":
					line = "false"
				default:
					fmt.Fprintf(w, "  invalid: enter y or n\n")
					continue
				}
			}

			if line != current {
				changes[kh.Key] = line
			}
			break
		}
	}

	return changes, nil
}

// formatPrompt builds the prompt string for a single key.
func formatPrompt(kh keyHint, current, bold, reset, _ string) string {
	var hint string
	switch {
	case len(kh.Enum) > 0:
		hint = " (" + strings.Join(kh.Enum, "/") + ")"
	case kh.IsBool:
		hint = " (y/n)"
	case kh.IsDuration:
		hint = " (e.g. 72h, 24h30m)"
	}
	return fmt.Sprintf("%s%s%s [%s]%s: ", bold, kh.Key, reset, current, hint)
}

// showGroupMenu displays the named groups and returns selected keyHints.
func showGroupMenu(
	r io.Reader,
	w io.Writer,
	hints map[string]keyHint,
	noColor bool,
) ([]keyHint, error) {
	groups := defaultGroups()
	allKeys := viper.AllKeys()

	bold := "\033[1m"
	reset := "\033[0m"
	if noColor {
		bold, reset = "", ""
	}

	fmt.Fprintf(w, "\n%sConfig groups:%s\n\n", bold, reset)
	for i, g := range groups {
		expanded := resolveGroupKeys(g, allKeys)
		fmt.Fprintf(w, "  %d) %-6s  %s (%d keys)\n",
			i+1, g.Name, g.Description, len(expanded))
	}
	fmt.Fprintf(w, "\nSelect groups (numbers or names, comma-separated): ")

	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return nil, fmt.Errorf("aborted")
	}

	line := strings.TrimSpace(scanner.Text())
	if line == "" {
		return nil, nil
	}

	seen := map[string]bool{}
	var result []keyHint
	parts := strings.Split(line, ",")

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		var g *groupEntry

		// Try number first
		if n, err := strconv.Atoi(p); err == nil && n >= 1 && n <= len(groups) {
			g = &groups[n-1]
		} else {
			// Try name
			for i := range groups {
				if strings.EqualFold(groups[i].Name, p) {
					g = &groups[i]
					break
				}
			}
		}

		if g == nil {
			return nil, fmt.Errorf(
				"key %q not found; "+
					"run 'tlc config interactive' to see available groups", p)
		}

		expanded := resolveGroupKeys(*g, allKeys)
		for _, h := range keysToHints(expanded, hints) {
			if !seen[h.Key] {
				seen[h.Key] = true
				result = append(result, h)
			}
		}
	}

	return result, nil
}

// ConfigInteractiveCmd is the cobra command for `config interactive`.
var ConfigInteractiveCmd = &cobra.Command{
	Use:   "interactive",
	Short: "Interactively configure settings",
	Long: `Walk through config keys one at a time, showing current values.

Use --key to filter keys by group name, section prefix, or substring.
Without --key, a group menu is shown.

Alias: tlc setup`,
	RunE: runConfigInteractive,
}

var configInteractiveKey string

func init() {
	ConfigInteractiveCmd.Flags().StringVar(
		&configInteractiveKey, "key", "",
		"filter keys (comma-separated groups, prefixes, or substrings)",
	)
	ConfigCmd.AddCommand(ConfigInteractiveCmd)
}

func runConfigInteractive(cmd *cobra.Command, args []string) error {
	// TTY check
	if !isatty.IsTerminal(os.Stdin.Fd()) &&
		!isatty.IsCygwinTerminal(os.Stdin.Fd()) {
		return fmt.Errorf(
			"interactive mode requires a terminal; " +
				"use 'tlc config set <key> <value>' instead")
	}

	noColor := viper.GetBool("no-color")
	hints := defaultKeyHints()

	var keys []keyHint

	// If --key provided via flag or trailing args from alias
	keyFilter := configInteractiveKey
	if keyFilter == "" && len(args) > 0 {
		keyFilter = strings.Join(args, ",")
	}

	if keyFilter != "" {
		keys = resolveMultipleTokens(keyFilter, hints)
		if len(keys) == 0 {
			return fmt.Errorf(
				"no config keys match %q; "+
					"run 'tlc config list' to see all keys",
				keyFilter)
		}
	} else {
		// Show group menu
		var err error
		keys, err = showGroupMenu(
			os.Stdin, cmd.OutOrStdout(), hints, noColor)
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return nil
		}
	}

	// Capture old values before wizard
	oldValues := map[string]string{}
	for _, k := range keys {
		oldValues[k.Key] = fmt.Sprintf("%v", viper.Get(k.Key))
	}

	changes, err := runWizard(
		keys, os.Stdin, cmd.OutOrStdout(), noColor)
	if err != nil {
		return err
	}

	if len(changes) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\nNo changes.")
		return nil
	}

	// Apply changes
	for k, v := range changes {
		viper.Set(k, v)
	}

	// Write config
	if _, err := config.PrepareViperForWrite(
		viper.GetViper()); err != nil {
		return fmt.Errorf("cannot write config: %w", err)
	}
	if err := viper.WriteConfig(); err != nil {
		if err := viper.SafeWriteConfig(); err != nil {
			return fmt.Errorf(
				"cannot write config: %s; check file permissions",
				viper.ConfigFileUsed())
		}
	}

	// Print summary
	fmt.Fprintln(cmd.OutOrStdout())
	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "KEY\tOLD\tNEW\n")
	for _, kh := range keys {
		if newVal, ok := changes[kh.Key]; ok {
			fmt.Fprintf(tw, "%s\t%s\t%s\n",
				kh.Key, oldValues[kh.Key], newVal)
		}
	}
	tw.Flush()

	return nil
}
