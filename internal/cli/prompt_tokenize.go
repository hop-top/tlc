package cli

import (
	"sort"
	"strings"
)

// PromptTokens holds the structured result of tokenizing a prompt.
type PromptTokens struct {
	Verb      string       // raw verb word matched (e.g. "count")
	VerbClass VerbClass    // semantic class (e.g. VerbQuery)
	Noun      string       // raw noun word matched (e.g. "tracks")
	Domain    NounDomain   // domain class (e.g. DomainTrack)
	Modifiers []string     // canonical modifier flags (e.g. ["status:active"])
	Remaining string       // leftover tokens not matched to any category
}

// TokenizePrompt splits prompt into structured tokens.
// Word order is flexible — verb, noun, and modifiers are detected positionally.
// Returns nil if no noun is found (noun is required for a valid result).
func TokenizePrompt(prompt string) *PromptTokens {
	text := strings.TrimSpace(prompt)
	if text == "" {
		return nil
	}

	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		return nil
	}

	var verbWord string
	var verbClass VerbClass
	var nounWord string
	var domain NounDomain
	var modifiers []string
	var remaining []string
	used := make([]bool, len(words))

	// First pass: find verb and noun (take first match for each)
	for i, w := range words {
		if verbWord == "" {
			if vc, ok := LookupVerb(w); ok {
				verbWord = w
				verbClass = vc
				used[i] = true
				continue
			}
		}
		if nounWord == "" {
			if nd, ok := LookupNoun(w); ok {
				nounWord = w
				domain = nd
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
			remaining = append(remaining, w)
		}
	}

	// Require at least a noun to return a result
	if nounWord == "" {
		return nil
	}

	// Sort modifiers for deterministic output
	sort.Strings(modifiers)

	var rem string
	if len(remaining) > 0 {
		rem = strings.Join(remaining, " ")
	}

	return &PromptTokens{
		Verb:      verbWord,
		VerbClass: verbClass,
		Noun:      nounWord,
		Domain:    domain,
		Modifiers: modifiers,
		Remaining: rem,
	}
}
