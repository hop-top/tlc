package cli

import (
	"strings"
)

// RouteNounToDomain returns the CLI domain for the noun token, or "" if unrecognized.
// Uses exact lookup first, then fuzzy if no exact match.
func RouteNounToDomain(tokens PromptTokens) (NounDomain, float64) {
	noun := tokens.Noun
	if noun == "" {
		return "", 0
	}
	// Exact lookup.
	if nd, ok := LookupNoun(noun); ok {
		return nd, 1.0
	}
	// Fuzzy fallback.
	nd, conf := FuzzyMatchNoun(noun)
	return nd, conf
}

// buildModifierArgs converts canonical modifier strings to CLI flag args,
// using domain-specific flag mappings.
//
// Task domain:
//   - "status:active" → ["--status", "active"]
//   - "status:done"   → ["--status", "DONE"]
//   - "blocked"       → ["--blocked"]
//   - "stale"         → ["--stale"]
//   - "mine"          → ["--mine"]
//
// Track domain:
//   - "status:active" → ["--status", "active"]
//   - "status:done"   → ["--status", "completed"]
//   - "blocked"       → ["--state", "blocked"]
//   - "stale"         → ["--state", "stale"]
//   - "mine"          → (skipped — no --mine on track)
//
// Flow/Project domains: no modifier flags (returns empty).
func buildModifierArgs(modifiers []string, domain NounDomain) []string {
	var out []string
	switch domain {
	case DomainTask:
		for _, m := range modifiers {
			switch m {
			case "status:active":
				out = append(out, "--status", "active")
			case "status:done":
				out = append(out, "--status", "DONE")
			case "blocked":
				out = append(out, "--blocked")
			case "stale":
				out = append(out, "--stale")
			case "mine":
				out = append(out, "--mine")
			}
		}
	case DomainTrack:
		for _, m := range modifiers {
			switch m {
			case "status:active":
				out = append(out, "--status", "active")
			case "status:done":
				out = append(out, "--status", "completed")
			case "blocked":
				out = append(out, "--state", "blocked")
			case "stale":
				out = append(out, "--state", "stale")
			// "mine" is not supported on track — skip
			}
		}
	// Flow and Project domains: no modifier flags
	}
	return out
}

// isCountVerb returns true when the raw verb word signals a count query.
func isCountVerb(verb string) bool {
	return verb == "count"
}

// isSummaryVerb returns true when the raw verb word signals a summary query.
func isSummaryVerb(verb string) bool {
	return verb == "summary"
}

// BuildCommand assembles the full CLI command from verb + domain + modifiers.
// domainConf is the confidence from RouteNounToDomain; verbConf is derived internally.
// Returns nil when the verb+domain combination is not supported.
func BuildCommand(tokens PromptTokens, domain NounDomain, domainConf float64) []ResolvedCommand {
	verb := tokens.Verb

	// Determine verb category and confidence.
	vc, vcConf := resolveVerbCategory(verb)
	if vc == "" && !isSummaryVerb(verb) {
		return nil
	}
	confidence := domainConf * vcConf
	if confidence > 1.0 {
		confidence = 1.0
	}

	switch domain {
	case DomainTask:
		return buildTaskCommand(tokens, vc, confidence)
	case DomainTrack:
		return buildTrackCommand(tokens, vc, confidence)
	case DomainFlow:
		return buildFlowCommand(tokens, vc, confidence)
	case DomainProject:
		return buildProjectCommand(tokens, vc, confidence)
	}
	return nil
}

// resolveVerbCategory maps a raw verb word to (VerbCategory, confidence).
// "summary" is treated as VerbQuery with conf=1.0.
// Returns ("", 0) when no match.
func resolveVerbCategory(verb string) (VerbCategory, float64) {
	if verb == "" {
		return "", 0
	}
	// Special cases not in alias map.
	if verb == "summary" {
		return VerbQuery, 1.0
	}
	if verb == "run" {
		return VerbCreate, 1.0
	}
	if vc, ok := LookupVerb(verb); ok {
		return vc, 1.0
	}
	vc, conf := FuzzyMatchVerb(verb)
	if conf == 0 {
		return "", 0
	}
	return vc, conf
}

// buildTaskCommand produces task sub-commands.
func buildTaskCommand(tokens PromptTokens, vc VerbCategory, confidence float64) []ResolvedCommand {
	// "run" belongs to flow domain only; guard against leakage.
	if tokens.Verb == "run" {
		return nil
	}
	var subCmd string
	switch vc {
	case VerbQuery:
		subCmd = "list"
	case VerbCreate:
		subCmd = "create"
	case VerbComplete:
		subCmd = "complete"
	case VerbDestroy:
		subCmd = "delete"
	default:
		return nil
	}

	args := []string{subCmd}
	args = append(args, buildModifierArgs(tokens.Modifiers, DomainTask)...)
	args = appendCountSummaryFlags(args, tokens.Verb)
	return []ResolvedCommand{{Cmd: "task", Args: args, Confidence: confidence}}
}

// buildTrackCommand produces track sub-commands.
func buildTrackCommand(tokens PromptTokens, vc VerbCategory, confidence float64) []ResolvedCommand {
	// "run" belongs to flow domain only; guard against leakage.
	if tokens.Verb == "run" {
		return nil
	}
	if vc != VerbQuery {
		return nil
	}
	args := []string{"list"}
	args = append(args, buildModifierArgs(tokens.Modifiers, DomainTrack)...)
	// Track domain: --counters is task-only; drop it by using base args only.
	args = appendCountSummaryFlags(args, tokens.Verb)
	// Filter out --counters: track list has no --counters flag.
	args = filterArg(args, "--counters")
	return []ResolvedCommand{{Cmd: "track", Args: args, Confidence: confidence}}
}

// buildFlowCommand produces flow sub-commands.
func buildFlowCommand(tokens PromptTokens, vc VerbCategory, confidence float64) []ResolvedCommand {
	switch vc {
	case VerbQuery:
		return []ResolvedCommand{{Cmd: "flow", Args: []string{"list"}, Confidence: confidence}}
	case VerbCreate: // "run"
		// flow run requires a concrete flow name — cobra.ExactArgs(1).
		// Scan Rest for a word that isn't a generic verb/noun placeholder.
		var flowRef string
		for _, w := range tokens.Rest {
			switch w {
			case "", "run", "flow", "pipeline", "workflow":
				continue
			default:
				flowRef = w
			}
			if flowRef != "" {
				break
			}
		}
		if flowRef == "" {
			return nil
		}
		return []ResolvedCommand{{Cmd: "flow", Args: []string{"run", flowRef}, Confidence: confidence}}
	}
	return nil
}

// buildProjectCommand produces project sub-commands.
// Supported subcommands: list (VerbQuery). All others return nil.
// "project switch" does not exist — return nil for any other verb.
func buildProjectCommand(tokens PromptTokens, vc VerbCategory, confidence float64) []ResolvedCommand {
	if vc != VerbQuery {
		return nil
	}
	return []ResolvedCommand{{Cmd: "project", Args: []string{"list"}, Confidence: confidence}}
}

// appendCountSummaryFlags appends --counters or --summary based on raw verb.
func appendCountSummaryFlags(args []string, verb string) []string {
	if isCountVerb(verb) {
		args = append(args, "--counters")
	} else if isSummaryVerb(verb) {
		args = append(args, "--summary")
	}
	return args
}

// ClassifyPromptCrossDomain is the cross-domain NLP classifier.
// It tokenizes the prompt, routes the noun to a domain, and builds a command.
// Returns nil when no domain or verb can be resolved.
func ClassifyPromptCrossDomain(prompt string) []ResolvedCommand {
	text := strings.TrimSpace(prompt)
	if text == "" {
		return nil
	}

	lower := strings.ToLower(text)
	howMany := strings.HasPrefix(lower, "how many")

	// Handle "how many" prefix: inject a synthetic "count" verb by rewriting
	// the prompt before tokenizing.
	if howMany {
		// strip "how many" and re-tokenize
		text = strings.TrimSpace(lower[len("how many"):])
	}

	tokens := TokenizePrompt(text)

	// If tokenizer returned nothing (no exact noun), attempt fuzzy noun
	// recovery directly on the raw words.
	if tokens.Noun == "" {
		tokens = fuzzyRecoverFromRaw(text, tokens)
	}

	// If no noun (exact or fuzzy), we cannot route.
	if tokens.Noun == "" {
		return nil
	}

	domain, domainConf := RouteNounToDomain(tokens)
	if domain == "" {
		return nil
	}

	// Inject count verb when "how many" prefix was detected.
	if howMany && tokens.Verb == "" {
		tokens.Verb = "count"
	}

	// "switch" can appear as a Rest word (e.g., "switch to auth project" →
	// tokenizer doesn't know "switch"; it ends up in Rest).
	if tokens.Verb == "" {
		tokens.Verb = extractSpecialVerb(tokens.Rest)
	}

	// When no verb is found but there are modifiers or a noun, default to
	// an implicit list/query (e.g., "active tracks", "blocked tasks").
	if tokens.Verb == "" && len(tokens.Modifiers) > 0 {
		tokens.Verb = "list"
	}

	// "run" in Rest means flow run; promote it to Verb and strip from Rest.
	if tokens.Verb == "" {
		tokens.Verb, tokens.Rest = extractRunVerb(tokens.Rest)
	}

	if tokens.Verb == "" {
		return nil
	}

	cmds := BuildCommand(tokens, domain, domainConf)
	if len(cmds) == 0 {
		return nil
	}

	// For "how many" prompts, ensure --counters is appended if not already present.
	if howMany {
		for i := range cmds {
			if !containsString(cmds[i].Args, "--counters") {
				cmds[i].Args = append(cmds[i].Args, "--counters")
			}
		}
	}

	return cmds
}

// extractSpecialVerb returns a special verb ("switch") if found in rest, otherwise "".
func extractSpecialVerb(rest []string) string {
	for _, w := range rest {
		if w == "switch" {
			return "switch"
		}
	}
	return ""
}

// extractRunVerb returns ("run", remaining_rest) if "run" is in rest.
// Otherwise returns ("", original_rest).
func extractRunVerb(rest []string) (string, []string) {
	for i, w := range rest {
		if w == "run" {
			remaining := make([]string, 0, len(rest)-1)
			remaining = append(remaining, rest[:i]...)
			remaining = append(remaining, rest[i+1:]...)
			return "run", remaining
		}
	}
	return "", rest
}

// filterArg removes all occurrences of flag from args.
func filterArg(args []string, flag string) []string {
	out := args[:0:0]
	for _, a := range args {
		if a != flag {
			out = append(out, a)
		}
	}
	return out
}

// containsString returns true if s is in slice.
func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// fuzzyRecoverFromRaw re-tokenizes raw text by scanning all words for fuzzy
// noun matches when exact tokenization found no noun. Returns a PromptTokens
// with the best fuzzy noun promoted and verb extracted from remaining words.
func fuzzyRecoverFromRaw(text string, orig PromptTokens) PromptTokens {
	words := strings.Fields(strings.ToLower(text))

	bestDist := 3
	bestIdx := -1
	bestWord := ""

	for i, w := range words {
		// Skip words that are exact verb or modifier matches — handled by tokenizer.
		if _, ok := LookupVerb(w); ok {
			continue
		}
		if _, ok := LookupModifier(w); ok {
			continue
		}
		_, conf := FuzzyMatchNoun(w)
		if conf == 0 {
			continue
		}
		var d int
		switch conf {
		case 1.0:
			d = 0
		case 0.9:
			d = 1
		default:
			d = 2
		}
		if d < bestDist || (d == bestDist && w < bestWord) {
			bestDist = d
			bestIdx = i
			bestWord = w
		}
	}

	if bestIdx < 0 {
		return orig
	}

	// Re-tokenize with the fuzzy noun word treated as the noun.
	// Build a synthetic prompt where the fuzzy word is replaced by a known noun
	// so TokenizePrompt can route it properly. Instead, construct tokens manually.
	var verbWord string
	var modifiers []string
	var rest []string

	for i, w := range words {
		if i == bestIdx {
			continue // this is our fuzzy noun
		}
		if verbWord == "" {
			if _, ok := LookupVerb(w); ok {
				verbWord = w
				continue
			}
		}
		if mf, ok := LookupModifier(w); ok {
			modifiers = append(modifiers, string(mf))
			continue
		}
		rest = append(rest, w)
	}

	return PromptTokens{
		Verb:      verbWord,
		Noun:      bestWord,
		Modifiers: modifiers,
		Rest:      rest,
	}
}

