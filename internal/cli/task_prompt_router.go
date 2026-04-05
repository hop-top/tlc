package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"

	"hop.top/kit/llm"
)

// systemPromptTemplate is the instruction set sent to the LLM for command
// routing. The {schema} placeholder is replaced with the task schema JSON.
const systemPromptTemplate = `You are a CLI command router for ` + "`tlc task`" + `. Given a user's natural language
request, return a JSON object with resolved commands.

Available commands (JSON schema):
{schema}

Return ONLY valid JSON in this format:
{"commands": [{"cmd": "task", "args": ["subcommand", "arg1", ...]}], "confidence": 0.0-1.0}

Rules:
- confidence 1.0 = certain the mapping is correct
- confidence <0.7 = uncertain, include "clarification" field with a question
- For task IDs, normalize to T-NNNN format (4-digit, zero-padded)
- For destructive commands (delete, unclaim, unassign), always set confidence <= 0.9`

// destructiveSubcommands are task subcommands that should never have
// confidence above the destructive cap.
var destructiveSubcommands = map[string]bool{
	"delete":   true,
	"unclaim":  true,
	"unassign": true,
}

// destructiveConfidenceCap is the maximum confidence allowed for destructive
// commands. The LLM is instructed to respect this, but we enforce it too.
const destructiveConfidenceCap = 0.9

// routerResponse is the JSON structure returned by the LLM.
type routerResponse struct {
	Commands      []routerCommand `json:"commands"`
	Confidence    float64         `json:"confidence"`
	Clarification string          `json:"clarification,omitempty"`
}

// routerCommand is a single command inside the LLM response.
type routerCommand struct {
	Cmd  string   `json:"cmd"`
	Args []string `json:"args"`
}

// routePrompt sends the user prompt to an LLM and returns resolved commands.
// It is the escalation path when ClassifyPrompt returns nil.
func routePrompt(ctx context.Context, prompt string, schemaJSON []byte) ([]ResolvedCommand, string, error) {
	provider, err := resolvePromptLLM()
	if err != nil {
		return nil, "", err
	}
	defer provider.Close()

	client := llm.NewClient(provider)

	systemMsg := strings.ReplaceAll(systemPromptTemplate, "{schema}", string(schemaJSON))

	// Enrich system message with repository context when available.
	if xrayCtx := loadXrayContext(); xrayCtx != "" {
		systemMsg += "\n\nRepository context:\n" + xrayCtx
	}

	req := llm.Request{
		Messages: []llm.Message{
			{Role: "system", Content: systemMsg},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.1,
		MaxTokens:   1024,
	}

	resp, err := client.Complete(ctx, req)
	if err != nil {
		return nil, "", fmt.Errorf("llm completion failed: %w", err)
	}

	return parseRouterResponse(resp.Content)
}

// resolvePromptLLM resolves the LLM provider using the following precedence:
//  1. Config key task_prompt.llm_provider via viper
//  2. Env TLC_PROMPT_LLM override
//  3. llm.LoadConfig("") default (LLM_PROVIDER env / config file)
//  4. All unavailable: return specific error
func resolvePromptLLM() (llm.Provider, error) {
	// 1. Viper config key.
	if uri := viper.GetString("task_prompt.llm_provider"); uri != "" {
		return llm.Resolve(uri)
	}

	// 2. Env override.
	if uri := os.Getenv("TLC_PROMPT_LLM"); uri != "" {
		return llm.Resolve(uri)
	}

	// 3. Kit default config (reads LLM_PROVIDER env / config file).
	cfg, err := llm.LoadConfig("")
	if err == nil {
		return llm.Resolve(cfg.URI.Scheme + "://" + cfg.URI.Model)
	}

	// 4. Nothing configured.
	return nil, fmt.Errorf(
		"no LLM provider configured; set TLC_PROMPT_LLM (e.g. ollama://llama3.2)",
	)
}

// parseRouterResponse extracts []ResolvedCommand and any clarification text
// from the LLM's JSON output.
func parseRouterResponse(raw string) ([]ResolvedCommand, string, error) {
	// Strip markdown fences if present.
	body := strings.TrimSpace(raw)
	body = stripCodeFence(body)

	var rr routerResponse
	if err := json.Unmarshal([]byte(body), &rr); err != nil {
		return nil, "", fmt.Errorf(
			"failed to parse LLM response as JSON: %w; raw: %s", err, truncateStr(raw, 200),
		)
	}

	if len(rr.Commands) == 0 {
		return nil, "", fmt.Errorf("LLM returned no commands; raw: %s", truncateStr(raw, 200))
	}

	confidence := rr.Confidence
	cmds := make([]ResolvedCommand, 0, len(rr.Commands))
	for _, rc := range rr.Commands {
		c := confidence
		if isDestructiveArgs(rc.Args) {
			c = capConfidence(c, destructiveConfidenceCap)
		}
		cmds = append(cmds, ResolvedCommand{
			Cmd:        rc.Cmd,
			Args:       rc.Args,
			Confidence: c,
		})
	}

	return cmds, rr.Clarification, nil
}

// stripCodeFence removes ```json ... ``` wrapping from LLM output.
func stripCodeFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Remove opening fence line.
	if idx := strings.Index(s, "\n"); idx >= 0 {
		s = s[idx+1:]
	}
	// Remove closing fence.
	if idx := strings.LastIndex(s, "```"); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

// isDestructiveArgs checks whether the command args indicate a destructive
// operation.
func isDestructiveArgs(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return destructiveSubcommands[args[0]]
}

// capConfidence returns the lower of c and cap.
func capConfidence(c, cap float64) float64 {
	if c > cap {
		return cap
	}
	return c
}

// truncateStr returns at most n bytes of s, appending "..." if truncated.
func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
