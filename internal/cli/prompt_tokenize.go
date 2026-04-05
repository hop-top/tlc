package cli

import (
	"sort"
	"strings"
)

// PromptTokens holds the structured result of tokenizing a prompt.
type PromptTokens struct {
	Verb      string   // raw verb word matched (e.g. "count")
	Noun      string   // raw noun word matched (e.g. "tracks")
	Modifiers []string // canonical modifier flags (e.g. ["status:active"])
	Rest      []string // unrecognized words not matched to any category
}

// TokenizePrompt splits prompt into structured tokens.
// Word order is flexible — verb, noun, and modifiers are detected positionally.
// Returns zero-value PromptTokens with empty Noun if no noun is found.
func TokenizePrompt(prompt string) PromptTokens {
	text := strings.TrimSpace(prompt)
	if text == "" {
		return PromptTokens{}
	}

	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		return PromptTokens{}
	}

	var verbWord string
	var nounWord string
	var modifiers []string
	var rest []string
	used := make([]bool, len(words))

	// First pass: find verb and noun (take first match for each)
	for i, w := range words {
		if verbWord == "" {
			if _, ok := LookupVerb(w); ok {
				verbWord = w
				used[i] = true
				continue
			}
		}
		if nounWord == "" {
			if _, ok := LookupNoun(w); ok {
				nounWord = w
				used[i] = true
			}
		}
	}

	// Second pass: find modifiers from unused words (skip verb and noun)
	for i, w := range words {
		if used[i] {
			continue
		}
		if mf, ok := LookupModifier(w); ok {
			modifiers = append(modifiers, string(mf))
			used[i] = true
		}
	}

	// Collect remaining unused words
	for i, w := range words {
		if !used[i] {
			rest = append(rest, w)
		}
	}

	// Require at least a noun to return a meaningful result
	if nounWord == "" {
		return PromptTokens{}
	}

	// Sort modifiers for deterministic output
	sort.Strings(modifiers)

	return PromptTokens{
		Verb:      verbWord,
		Noun:      nounWord,
		Modifiers: modifiers,
		Rest:      rest,
	}
}
