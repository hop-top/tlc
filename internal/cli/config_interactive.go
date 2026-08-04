package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
)

const customInputSentinel = "__custom__"

// runWizard runs the interactive prompt loop over the given keys using huh.
// Returns changed key->value pairs.
func runWizard(
	keys []keyHint,
	_ io.Reader,
	w io.Writer,
	_ bool,
) (map[string]string, error) {
	changes := map[string]string{}

	for _, kh := range keys {
		if kh.IsMap {
			fmt.Fprintf(w, "\n  %s: use 'tlc config set %s.<key> <value>'\n",
				kh.Key, kh.Key)
			continue
		}

		current := fmt.Sprintf("%v", viper.Get(kh.Key))

		val, err := promptKey(kh, current, w)
		if err != nil {
			return nil, err
		}
		if val != "" && kh.PostValidate != nil {
			val, err = kh.PostValidate(val, w)
			if err != nil {
				return nil, err
			}
		}
		if val != "" && val != current {
			changes[kh.Key] = val
		}
	}

	return changes, nil
}

// promptKey dispatches to the right huh field for a keyHint.
func promptKey(kh keyHint, current string, w io.Writer) (string, error) {
	switch {
	case len(kh.Suggestions) > 0:
		return promptSuggestions(kh, current, w)
	case len(kh.Enum) > 0:
		return promptEnum(kh, current, w)
	case kh.IsBool:
		return promptBool(kh, current, w)
	case kh.IsDuration:
		return promptDuration(kh, current, w)
	default:
		return promptInput(kh, current, w)
	}
}

// promptEnum presents a huh.Select with enum options + keep current.
func promptEnum(kh keyHint, current string, w io.Writer) (string, error) {
	opts := make([]huh.Option[string], 0, len(kh.Enum)+1)
	opts = append(opts, huh.NewOption[string](
		fmt.Sprintf("Keep current (%s)", current), "",
	))
	for _, e := range kh.Enum {
		label := e
		if e == current {
			label = e + " (current)"
		}
		opts = append(opts, huh.NewOption[string](label, e))
	}

	var selected string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(kh.Key).
				Description(kh.Description).
				Options(opts...).
				Value(&selected),
		),
	).WithOutput(w).Run()
	if err != nil {
		return "", fmt.Errorf("aborted")
	}
	return selected, nil
}

// promptBool presents a huh.Confirm.
func promptBool(kh keyHint, current string, w io.Writer) (string, error) {
	val := strings.EqualFold(current, "true")
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(kh.Key).
				Description(kh.Description).
				Value(&val),
		),
	).WithOutput(w).Run()
	if err != nil {
		return "", fmt.Errorf("aborted")
	}
	result := "false"
	if val {
		result = "true"
	}
	if result == current {
		return "", nil
	}
	return result, nil
}

// promptDuration presents a huh.NewInput with duration validation.
func promptDuration(kh keyHint, current string, w io.Writer) (string, error) {
	val := current
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(kh.Key).
				Description(kh.Description).
				Placeholder("e.g. 72h, 24h30m").
				Value(&val).
				Validate(func(s string) error {
					if s == "" {
						return nil
					}
					_, err := time.ParseDuration(s)
					return err //nolint:wrapcheck // huh validator; message renders inline under the field
				}),
		),
	).WithOutput(w).Run()
	if err != nil {
		return "", fmt.Errorf("aborted")
	}
	if val == "" || val == current {
		return "", nil
	}
	return val, nil
}

// promptInput presents a huh.NewInput for free text.
func promptInput(kh keyHint, current string, w io.Writer) (string, error) {
	val := current
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(kh.Key).
				Description(kh.Description).
				Value(&val),
		),
	).WithOutput(w).Run()
	if err != nil {
		return "", fmt.Errorf("aborted")
	}
	if val == current {
		return "", nil
	}
	return val, nil
}

// promptSuggestions presents a huh.Select with curated options + "Custom...".
func promptSuggestions(kh keyHint, current string, w io.Writer) (string, error) {
	opts := make([]huh.Option[string], 0, len(kh.Suggestions)+2)
	opts = append(opts, huh.NewOption[string](
		fmt.Sprintf("Keep current (%s)", current), "",
	))
	for _, s := range kh.Suggestions {
		opts = append(opts, huh.NewOption[string](s.Label, s.Value))
	}
	opts = append(opts, huh.NewOption[string]("Custom URI...", customInputSentinel))

	var selected string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(kh.Key).
				Description(kh.Description).
				Options(opts...).
				Value(&selected),
		),
	).WithOutput(w).Run()
	if err != nil {
		return "", fmt.Errorf("aborted")
	}

	if selected == "" {
		return "", nil
	}
	if selected != customInputSentinel {
		return selected, nil
	}

	var custom string
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(kh.Key).
				Placeholder("scheme://model").
				Value(&custom),
		),
	).WithOutput(w).Run()
	if err != nil || custom == "" {
		return "", nil
	}
	return custom, nil
}

// showGroupMenu displays the named groups as a huh.MultiSelect.
func showGroupMenu(
	_ io.Reader,
	w io.Writer,
	hints map[string]keyHint,
	_ bool,
) ([]keyHint, error) {
	groups := defaultGroups()
	allKeys := viper.AllKeys()

	opts := make([]huh.Option[string], 0, len(groups))
	for _, g := range groups {
		expanded := resolveGroupKeys(g, allKeys)
		label := fmt.Sprintf("%-6s  %s (%d keys)",
			g.Name, g.Description, len(expanded))
		opts = append(opts, huh.NewOption[string](label, g.Name))
	}

	var selected []string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Config groups").
				Description("Select groups to configure").
				Options(opts...).
				Value(&selected),
		),
	).WithOutput(w).Run()
	if err != nil {
		return nil, fmt.Errorf("aborted")
	}
	if len(selected) == 0 {
		return nil, nil
	}

	seen := map[string]bool{}
	var result []keyHint
	for _, name := range selected {
		for _, g := range groups {
			if g.Name == name {
				expanded := resolveGroupKeys(g, allKeys)
				for _, h := range keysToHints(expanded, hints) {
					if !seen[h.Key] {
						seen[h.Key] = true
						result = append(result, h)
					}
				}
				break
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
	Annotations: map[string]string{
		"kit/side-effect": "interactive",
		"kit/idempotent":  "yes",
	},
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
	in := cmd.InOrStdin()
	out := cmd.OutOrStdout()

	if !interactiveAvailable(cmd) {
		return fmt.Errorf(
			"interactive mode requires a terminal; " +
				"use 'tlc config set <key> <value>' instead",
		)
	}

	noColor := viper.GetBool("no-color")
	hints := defaultKeyHints()

	var keys []keyHint

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
				keyFilter,
			)
		}
	} else {
		var err error
		keys, err = showGroupMenu(in, out, hints, noColor)
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return nil
		}
	}

	oldValues := map[string]string{}
	for _, k := range keys {
		oldValues[k.Key] = fmt.Sprintf("%v", viper.Get(k.Key))
	}

	changes, err := runWizard(keys, in, out, noColor)
	if err != nil {
		return err
	}

	if len(changes) == 0 {
		fmt.Fprintln(out, "\nNo changes.")
		return nil
	}

	for k, v := range changes {
		viper.Set(k, v)
	}

	if _, err := config.PrepareViperForWrite(
		viper.GetViper(),
	); err != nil {
		return fmt.Errorf("cannot write config: %w", err)
	}
	if err := viper.WriteConfig(); err != nil {
		if err := viper.SafeWriteConfig(); err != nil {
			return fmt.Errorf(
				"cannot write config: %s; check file permissions",
				viper.ConfigFileUsed(),
			)
		}
	}

	fmt.Fprintln(out)
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
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
