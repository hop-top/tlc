package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
)

// keyHint describes a single config key for the interactive wizard.
type keyHint struct {
	Key         string
	Description string
	Enum        []string // nil = free text
	IsBool      bool
	IsDuration  bool
	IsMap       bool // skipped in wizard
}

// groupEntry describes a named group of config keys.
type groupEntry struct {
	Name        string
	Description string
	Patterns    []string // exact keys or globs (e.g. "git.branch.*")
}

// defaultGroups returns the named group registry.
func defaultGroups() []groupEntry {
	return []groupEntry{
		{
			Name:        "ai",
			Description: "AI/LLM provider settings",
			Patterns:    []string{"prompt.llm_provider"},
		},
		{
			Name:        "llm",
			Description: "LLM provider settings",
			Patterns:    []string{"prompt.llm_provider"},
		},
		{
			Name:        "core",
			Description: "Project identity and storage",
			Patterns: []string{
				"project.id", "storage.backend", "storage.db_path",
			},
		},
		{
			Name:        "ui",
			Description: "Editor, pager, theme, table style",
			Patterns: []string{
				"ui.editor", "ui.pager", "ui.theme",
				"ui.table_style", "ui.date_format", "ui.timezone",
				"ui.log_sort_direction",
			},
		},
		{
			Name:        "git",
			Description: "Git tracking and branch/commit config",
			Patterns: []string{
				"git.track", "git.branch.*", "git.commit.*",
			},
		},
		{
			Name:        "sync",
			Description: "Sync and GitHub integration",
			Patterns:    []string{"sync.enabled", "sync.github.*"},
		},
	}
}

// defaultKeyHints returns the manual hint registry for known config keys.
func defaultKeyHints() map[string]keyHint {
	return map[string]keyHint{
		"storage.backend": {
			Key: "storage.backend", Description: "Storage engine",
			Enum: []string{"sqlite", "local", "postgres"},
		},
		"storage.db_path": {
			Key: "storage.db_path", Description: "Path to SQLite database file",
		},
		"output.format": {
			Key: "output.format", Description: "Default output format",
			Enum: []string{"table", "json", "yaml", "tls", "summary"},
		},
		"output.color": {
			Key: "output.color", Description: "Enable colored output",
			IsBool: true,
		},
		"output.verbose": {
			Key: "output.verbose", Description: "Enable verbose logging",
			IsBool: true,
		},
		"output.quiet": {
			Key: "output.quiet", Description: "Suppress non-essential output",
			IsBool: true,
		},
		"output.log_file": {
			Key: "output.log_file", Description: "Path to log file",
		},
		"project.id": {
			Key: "project.id", Description: "Unique project identifier",
		},
		"project.fallback_mode": {
			Key: "project.fallback_mode", Description: "Fallback detection mode",
			Enum: []string{"auto", "detected", "prompt"},
		},
		"project.duplicate_id_strategy": {
			Key:  "project.duplicate_id_strategy",
			Description: "How to handle duplicate project IDs",
			Enum: []string{"share", "unique", "prompt"},
		},
		"git.track": {
			Key: "git.track", Description: "Enable git integration",
			IsBool: true,
		},
		"git.branch.prefix_from_type": {
			Key: "git.branch.prefix_from_type",
			Description: "Derive branch prefix from task type",
			IsBool: true,
		},
		"git.branch.zero_pad_issue": {
			Key:         "git.branch.zero_pad_issue",
			Description: "Zero-pad width for issue numbers in branch names",
		},
		"git.branch.separator": {
			Key:         "git.branch.separator",
			Description: "Separator between branch name components",
		},
		"git.commit.auto_generate": {
			Key: "git.commit.auto_generate",
			Description: "Auto-generate commit messages",
			IsBool: true,
		},
		"git.commit.template": {
			Key:         "git.commit.template",
			Description: "Commit message template",
		},
		"git.commit.co_author": {
			Key:         "git.commit.co_author",
			Description: "Co-author line for generated commits",
		},
		"task.default_status": {
			Key:         "task.default_status",
			Description: "Default status for new tasks",
		},
		"task.id_format": {
			Key:         "task.id_format",
			Description: "Task ID format template (e.g. T-{seq:04d})",
		},
		"task.auto_assign": {
			Key: "task.auto_assign", Description: "Auto-assign tasks on create",
			IsBool: true,
		},
		"task.require_reference": {
			Key: "task.require_reference",
			Description: "Require external reference on task create",
			IsBool: true,
		},
		"task.archive_threshold": {
			Key: "task.archive_threshold",
			Description: "Auto-archive completed tasks after this duration",
			IsDuration: true,
		},
		"sync.enabled": {
			Key: "sync.enabled", Description: "Enable sync",
			IsBool: true,
		},
		"sync.auto_push": {
			Key: "sync.auto_push", Description: "Auto-push after sync",
			IsBool: true,
		},
		"sync.interval": {
			Key: "sync.interval", Description: "Sync interval",
			IsDuration: true,
		},
		"sync.conflict_strategy": {
			Key:         "sync.conflict_strategy",
			Description: "How to resolve sync conflicts",
		},
		"sync.github.enabled": {
			Key: "sync.github.enabled", Description: "Enable GitHub sync",
			IsBool: true,
		},
		"sync.github.repo": {
			Key:         "sync.github.repo",
			Description: "GitHub repo (owner/name) to sync with",
		},
		"sync.github.sync_direction": {
			Key:         "sync.github.sync_direction",
			Description: "Sync direction for GitHub",
		},
		"sync.github.import_labels": {
			Key: "sync.github.import_labels", Description: "Import labels from GitHub",
			IsBool: true,
		},
		"sync.github.import_milestones": {
			Key: "sync.github.import_milestones",
			Description: "Import milestones from GitHub",
			IsBool: true,
		},
		"ui.pager": {
			Key:         "ui.pager",
			Description: "Pager command (auto, less, more, none)",
		},
		"ui.editor": {
			Key:         "ui.editor",
			Description: "Editor command for task editing",
		},
		"ui.date_format": {
			Key:         "ui.date_format",
			Description: "Date format (Go time layout)",
		},
		"ui.timezone": {
			Key:         "ui.timezone",
			Description: "Timezone for date display (local, UTC, etc.)",
		},
		"ui.table_style": {
			Key:         "ui.table_style",
			Description: "Table rendering style",
		},
		"ui.tag_colors": {
			Key: "ui.tag_colors", Description: "Tag color map",
			IsMap: true,
		},
		"ui.log_sort_direction": {
			Key:         "ui.log_sort_direction",
			Description: "Sort direction for log output (asc/desc)",
		},
		"ui.theme": {
			Key:         "ui.theme",
			Description: "UI color theme",
		},
		"prompt.llm_provider": {
			Key:         "prompt.llm_provider",
			Description: "LLM provider for NL prompt (e.g. openai, anthropic)",
		},
		"tracks.stale_threshold": {
			Key: "tracks.stale_threshold",
			Description: "Mark tracks stale after this duration",
			IsDuration: true,
		},
		"tracks.health.max_active": {
			Key:         "tracks.health.max_active",
			Description: "Maximum concurrent active tracks",
		},
		"tracks.health.min_progress_to_start": {
			Key:         "tracks.health.min_progress_to_start",
			Description: "Min progress % before starting new track",
		},
		"tracks.plan_extractor": {
			Key:         "tracks.plan_extractor",
			Description: "Plan extraction method",
		},
		"tracks.default_type": {
			Key:         "tracks.default_type",
			Description: "Default track type",
		},
	}
}

// expandGlob expands a pattern like "git.branch.*" against viper's key set.
func expandGlob(pattern string, allKeys []string) []string {
	if !strings.Contains(pattern, "*") {
		return []string{pattern}
	}
	prefix := strings.TrimSuffix(pattern, "*")
	var matched []string
	for _, k := range allKeys {
		if strings.HasPrefix(k, prefix) {
			matched = append(matched, k)
		}
	}
	return matched
}

// resolveGroupKeys expands all patterns in a group against viper keys.
func resolveGroupKeys(g groupEntry, allKeys []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, p := range g.Patterns {
		for _, k := range expandGlob(p, allKeys) {
			if !seen[k] {
				seen[k] = true
				result = append(result, k)
			}
		}
	}
	return result
}

// groupRegistry returns a map of group name → groupEntry.
func groupRegistry() map[string]groupEntry {
	reg := map[string]groupEntry{}
	for _, g := range defaultGroups() {
		reg[g.Name] = g
	}
	return reg
}

// resolveKeys resolves a single token to a list of keyHints.
// Priority: group lookup → prefix match → substring match.
func resolveKeys(token string, hints map[string]keyHint) []keyHint {
	token = strings.TrimSpace(strings.ToLower(token))
	if token == "" {
		return nil
	}

	allKeys := viper.AllKeys()
	sort.Strings(allKeys)
	reg := groupRegistry()

	// 1. Named group
	if g, ok := reg[token]; ok {
		expanded := resolveGroupKeys(g, allKeys)
		return keysToHints(expanded, hints)
	}

	// 2. Prefix match (token + ".")
	prefix := token + "."
	var prefixMatched []string
	for _, k := range allKeys {
		if strings.HasPrefix(k, prefix) || k == token {
			prefixMatched = append(prefixMatched, k)
		}
	}
	if len(prefixMatched) > 0 {
		return keysToHints(prefixMatched, hints)
	}

	// 3. Substring match
	var subMatched []string
	for _, k := range allKeys {
		if strings.Contains(k, token) {
			subMatched = append(subMatched, k)
		}
	}
	if len(subMatched) > 0 {
		return keysToHints(subMatched, hints)
	}

	return nil
}

// resolveMultipleTokens resolves comma-separated tokens, deduplicating.
func resolveMultipleTokens(tokens string, hints map[string]keyHint) []keyHint {
	parts := strings.Split(tokens, ",")
	seen := map[string]bool{}
	var result []keyHint
	for _, p := range parts {
		for _, h := range resolveKeys(p, hints) {
			if !seen[h.Key] {
				seen[h.Key] = true
				result = append(result, h)
			}
		}
	}
	return result
}

// keysToHints converts a list of key strings to keyHints, using the hint
// registry for metadata. Unknown keys get a free-text hint with a warning.
func keysToHints(keys []string, hints map[string]keyHint) []keyHint {
	var result []keyHint
	for _, k := range keys {
		if h, ok := hints[k]; ok {
			result = append(result, h)
		} else {
			result = append(result, keyHint{
				Key:         k,
				Description: "(unrecognized key; no validation available)",
			})
		}
	}
	return result
}

// runWizard runs the interactive prompt loop over the given keys.
// It reads from r and writes prompts to w. Returns changed key→value pairs.
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
func formatPrompt(kh keyHint, current, bold, reset, dim string) string {
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
				"key %q not found; run 'tlc config interactive' to see available groups", p)
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
	if !isatty.IsTerminal(os.Stdin.Fd()) && !isatty.IsCygwinTerminal(os.Stdin.Fd()) {
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
				"no config keys match %q; run 'tlc config list' to see all keys",
				keyFilter)
		}
	} else {
		// Show group menu
		var err error
		keys, err = showGroupMenu(os.Stdin, cmd.OutOrStdout(), hints, noColor)
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

	changes, err := runWizard(keys, os.Stdin, cmd.OutOrStdout(), noColor)
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
	if _, err := config.PrepareViperForWrite(viper.GetViper()); err != nil {
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
			fmt.Fprintf(tw, "%s\t%s\t%s\n", kh.Key, oldValues[kh.Key], newVal)
		}
	}
	tw.Flush()

	return nil
}
