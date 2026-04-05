package cli

import (
	"regexp"
	"strings"

	"hop.top/tlc/internal/uri"
)

// ResolvedCommand represents a CLI command resolved from natural language input.
type ResolvedCommand struct {
	Cmd        string   `json:"cmd"`
	Args       []string `json:"args"`
	Confidence float64  `json:"confidence"`
}

// Compiled patterns for ClassifyPrompt. Each entry pairs a regex with a
// builder function that converts submatches into ResolvedCommand(s).
type classifyRule struct {
	re    *regexp.Regexp
	build func(matches []string) []ResolvedCommand
}

var classifyRules = []classifyRule{
	// "complete T-42", "mark T-42 done", "mark T-42 as done", "finish T-42"
	{
		re: regexp.MustCompile(`(?i)(?:complete|finish|mark)\s+(?:task\s+)?` +
			`((?:T-)?\d+)(?:\s+(?:as\s+)?done)?`),
		build: func(m []string) []ResolvedCommand {
			return []ResolvedCommand{{
				Cmd:        "task",
				Args:       []string{"complete", normalizeID(m[1])},
				Confidence: 1.0,
			}}
		},
	},
	// "mark T-42 done" (inverted order: "mark ... done")
	{
		re: regexp.MustCompile(`(?i)mark\s+(?:task\s+)?((?:T-)?\d+)\s+(?:as\s+)?done`),
		build: func(m []string) []ResolvedCommand {
			return []ResolvedCommand{{
				Cmd:        "task",
				Args:       []string{"complete", normalizeID(m[1])},
				Confidence: 1.0,
			}}
		},
	},
	// "show T-42", "view T-42", "details T-42", "describe T-42"
	{
		re: regexp.MustCompile(`(?i)(?:show|view|details?(?:\s+(?:of|for))?|describe)\s+(?:task\s+)?((?:T-)?\d+)`),
		build: func(m []string) []ResolvedCommand {
			return []ResolvedCommand{{
				Cmd:        "task",
				Args:       []string{"show", normalizeID(m[1])},
				Confidence: 1.0,
			}}
		},
	},
	// "delete T-42", "remove T-42"
	{
		re: regexp.MustCompile(`(?i)(?:delete|remove)\s+(?:task\s+)?((?:T-)?\d+)`),
		build: func(m []string) []ResolvedCommand {
			return []ResolvedCommand{{
				Cmd:        "task",
				Args:       []string{"delete", normalizeID(m[1])},
				Confidence: 1.0,
			}}
		},
	},
	// "unclaim T-42", "release T-42", "drop T-42" — must precede claim rule
	{
		re: regexp.MustCompile(`(?i)(?:unclaim|release|drop)\s+(?:task\s+)?((?:T-)?\d+)`),
		build: func(m []string) []ResolvedCommand {
			return []ResolvedCommand{{
				Cmd:        "task",
				Args:       []string{"unclaim", normalizeID(m[1])},
				Confidence: 1.0,
			}}
		},
	},
	// "claim T-42", "take T-42", "grab T-42"
	{
		re: regexp.MustCompile(`(?i)\b(?:claim|take|grab)\s+(?:task\s+)?((?:T-)?\d+)`),
		build: func(m []string) []ResolvedCommand {
			return []ResolvedCommand{{
				Cmd:        "task",
				Args:       []string{"claim", normalizeID(m[1])},
				Confidence: 1.0,
			}}
		},
	},
	// "assign T-42 to jadb", "assign jadb to T-42"
	{
		re: regexp.MustCompile(`(?i)assign\s+(?:task\s+)?((?:T-)?\d+)\s+to\s+(\S+)`),
		build: func(m []string) []ResolvedCommand {
			return []ResolvedCommand{{
				Cmd:        "task",
				Args:       []string{"assign", m[2], normalizeID(m[1])},
				Confidence: 1.0,
			}}
		},
	},
	// "assign jadb to T-42" (assignee first)
	{
		re: regexp.MustCompile(`(?i)assign\s+([a-zA-Z][\w-]*)\s+to\s+(?:task\s+)?((?:T-)?\d+)`),
		build: func(m []string) []ResolvedCommand {
			return []ResolvedCommand{{
				Cmd:        "task",
				Args:       []string{"assign", m[1], normalizeID(m[2])},
				Confidence: 1.0,
			}}
		},
	},
	// "create a task called Login flow", "create task Login flow", "new task Login flow"
	{
		re: regexp.MustCompile(`(?i)(?:create|new)\s+(?:a\s+)?task\s+(?:called\s+|named\s+|titled\s+)?(.+)`),
		build: func(m []string) []ResolvedCommand {
			title := strings.TrimSpace(m[1])
			if title == "" {
				return nil
			}
			return []ResolvedCommand{{
				Cmd:        "task",
				Args:       []string{"create", title},
				Confidence: 1.0,
			}}
		},
	},
	// "what tasks are blocked?", "blocked tasks", "show blocked tasks", "list blocked"
	{
		re: regexp.MustCompile(`(?i)(?:what\s+tasks?\s+(?:are\s+)?blocked|(?:show|list)\s+blocked(?:\s+tasks?)?|blocked\s+tasks?)`),
		build: func(_ []string) []ResolvedCommand {
			return []ResolvedCommand{{
				Cmd:        "task",
				Args:       []string{"list", "--blocked"},
				Confidence: 1.0,
			}}
		},
	},
	// "list my tasks", "my tasks", "show my tasks"
	{
		re: regexp.MustCompile(`(?i)(?:(?:list|show)\s+)?my\s+tasks?`),
		build: func(_ []string) []ResolvedCommand {
			return []ResolvedCommand{{
				Cmd:        "task",
				Args:       []string{"list", "--mine"},
				Confidence: 1.0,
			}}
		},
	},
	// "list tasks", "show tasks", "all tasks"
	{
		re: regexp.MustCompile(`(?i)(?:list|show|all)\s+tasks?`),
		build: func(_ []string) []ResolvedCommand {
			return []ResolvedCommand{{
				Cmd:        "task",
				Args:       []string{"list"},
				Confidence: 1.0,
			}}
		},
	},
}

// ClassifyPrompt parses freeform text and attempts to map it to concrete
// CLI commands. Returns nil when no pattern matches (caller should escalate
// to the router).
func ClassifyPrompt(prompt string) []ResolvedCommand {
	text := strings.TrimSpace(prompt)
	if text == "" {
		return nil
	}

	for _, rule := range classifyRules {
		if m := rule.re.FindStringSubmatch(text); m != nil {
			if cmds := rule.build(m); len(cmds) > 0 {
				return cmds
			}
		}
	}
	return nil
}

// normalizeID uppercases and delegates to uri.NormalizeTaskID for canonical T-NNNN form.
func normalizeID(raw string) string {
	return uri.NormalizeTaskID(strings.ToUpper(raw))
}
