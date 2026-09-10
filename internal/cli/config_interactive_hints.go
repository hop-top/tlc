package cli

import (
	"io"
	"sort"
	"strings"

	"github.com/spf13/viper"
)

// suggestion is a labelled option for huh.NewSelect prompts.
type suggestion struct {
	Label string
	Value string
}

// postValidateFunc validates a chosen value after selection.
// Returns (final value, error). May prompt user for remediation.
// w is the output writer for interactive remediation.
type postValidateFunc func(value string, w io.Writer) (string, error)

// keyHint describes a single config key for the interactive wizard.
type keyHint struct {
	Key          string
	Description  string
	Enum         []string         // nil = free text
	Suggestions  []suggestion     // shown as huh.Select options + "Custom..."
	PostValidate postValidateFunc // optional; runs after value selected
	IsBool       bool
	IsDuration   bool
	IsMap        bool // skipped in wizard
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
			Description: "Theme, table style, time display",
			Patterns: []string{
				"ui.theme", "ui.table_style", "ui.timezone",
				"ui.log_sort_direction",
			},
		},
		{
			Name:        "git",
			Description: "Git tracking",
			Patterns:    []string{"git.track"},
		},
		{
			Name:        "sync",
			Description: "GitHub sync integration",
			Patterns:    []string{"sync.github.*"},
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
		"output.log_file": {
			Key: "output.log_file", Description: "Path to log file",
		},
		"project.id": {
			Key: "project.id", Description: "Unique project identifier",
		},
		"project.fallback_mode": {
			Key:         "project.fallback_mode",
			Description: "Fallback detection mode",
			Enum:        []string{"auto", "detected", "prompt"},
		},
		"project.duplicate_id_strategy": {
			Key:         "project.duplicate_id_strategy",
			Description: "How to handle duplicate project IDs",
			Enum:        []string{"share", "unique", "prompt"},
		},
		"git.track": {
			Key: "git.track", Description: "Enable git integration",
			IsBool: true,
		},
		"task.default_status": {
			Key:         "task.default_status",
			Description: "Default status for new tasks",
		},
		"task.auto_assign": {
			Key: "task.auto_assign", Description: "Auto-assign tasks on create",
			IsBool: true,
		},
		"task.require_reference": {
			Key:         "task.require_reference",
			Description: "Require external reference on task create",
			IsBool:      true,
		},
		"task.archive_threshold": {
			Key:         "task.archive_threshold",
			Description: "Auto-archive completed tasks after this duration",
			IsDuration:  true,
		},
		"sync.github.repo": {
			Key:         "sync.github.repo",
			Description: "GitHub repo (owner/name) to sync with",
		},
		"sync.github.sync_direction": {
			Key:         "sync.github.sync_direction",
			Description: "Sync direction for GitHub",
			Enum:        []string{"pull", "push", "bidirectional"},
		},
		"ui.timezone": {
			Key:         "ui.timezone",
			Description: "Timezone for date display (local, UTC, etc.)",
		},
		"ui.table_style": {
			Key:         "ui.table_style",
			Description: "Table border style",
			Enum: []string{
				"unicode", "rounded", "thick", "double", "ascii", "none",
			},
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
			Enum:        []string{"neon", "dark", "bauhaus"},
		},
		"prompt.llm_provider": {
			Key:         "prompt.llm_provider",
			Description: "LLM provider URI for NL prompt routing",
			Suggestions: []suggestion{
				{"Ollama — llama3.2 (local, free)", "ollama://llama3.2"},
				{"Ollama — qwen3 (local, free)", "ollama://qwen3"},
				{"OpenRouter — DeepSeek V3 (free)", "openrouter://deepseek/deepseek-chat-v3-0324:free"},
				{"OpenRouter — Qwen3 235B (free)", "openrouter://qwen/qwen3-235b-a22b:free"},
				{"OpenAI — GPT-4o", "openai://gpt-4o"},
				{"OpenAI — o3", "openai://o3"},
				{"Anthropic — Claude Sonnet 4", "anthropic://claude-sonnet-4-20250514"},
			},
			PostValidate: validateLLMProvider,
		},
		"tracks.stale_threshold": {
			Key:         "tracks.stale_threshold",
			Description: "Mark tracks stale after this duration",
			IsDuration:  true,
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

// groupRegistry returns a map of group name to groupEntry.
func groupRegistry() map[string]groupEntry {
	reg := map[string]groupEntry{}
	for _, g := range defaultGroups() {
		reg[g.Name] = g
	}
	return reg
}

// resolveKeys resolves a single token to a list of keyHints.
// Priority: group lookup -> prefix match -> substring match.
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
