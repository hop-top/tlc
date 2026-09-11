package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"

	"hop.top/kit/go/ai/llm"
)

// systemPromptTemplate is the instruction set sent to the LLM for command
// routing. The {schema} placeholder is replaced with the task schema JSON.
const systemPromptTemplate = `You are a CLI command router for ` + "`tlc`" + `. Given a user's natural language
request, return a JSON object with resolved commands.

Available commands (JSON schema):
{schema}

Return ONLY valid JSON in this format:
{"commands": [{"cmd": "task", "args": ["subcommand", "arg1", ...]}], "confidence": 0.0-1.0}
where "cmd" is any top-level tlc command (e.g. "task", "prompt", "track")

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
// routerTemperature keeps command routing near-deterministic. kit's
// llm.Request takes a *float64 so an explicit value is distinguishable
// from unset, hence the addressable var rather than a literal.
var routerTemperature = 0.1

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
		Temperature: &routerTemperature,
		MaxTokens:   1024,
	}

	resp, err := client.Complete(ctx, req)
	if err != nil {
		return nil, "", fmt.Errorf("llm completion failed: %w", err)
	}

	return parseRouterResponse(resp.Content)
}

// resolvePromptLLM resolves the LLM provider using the following precedence:
//  1. Config key prompt.llm_provider via viper
//  2. Env TLC_PROMPT_LLM override
//  3. llm.LoadConfig("") default (LLM_PROVIDER env / config file)
//  4. All unavailable: return specific error
//
// All paths go through llm.LoadConfig to merge env var API keys
// (e.g. OPENAI_API_KEY, ANTHROPIC_API_KEY) with the URI.
func resolvePromptLLM() (llm.Provider, error) {
	// 1. Viper config key.
	if uri := viper.GetString("prompt.llm_provider"); uri != "" {
		return resolveViaLoadConfig(uri)
	}

	// 2. Env override.
	if uri := os.Getenv("TLC_PROMPT_LLM"); uri != "" {
		return resolveViaLoadConfig(uri)
	}

	// 3. Kit default config (reads LLM_PROVIDER env / config file).
	cfg, err := llm.LoadConfig("")
	if err == nil {
		return resolveFromConfig(cfg)
	}

	// 4. Nothing configured.
	return nil, fmt.Errorf(
		"no LLM provider configured; set TLC_PROMPT_LLM (e.g. ollama://llama3.2)",
	)
}

// normalizeURI ensures a bare scheme (e.g. "openai") becomes a
// valid URI ("openai://"). Preserves already-valid URIs.
func normalizeURI(uri string) string {
	if !strings.Contains(uri, "://") {
		return uri + "://"
	}
	return uri
}

// providerEnvVars maps URI schemes to provider-specific API key env vars.
// LoadConfig only reads LLM_API_KEY; this fills the gap until kit adds
// per-provider env var support.
var providerEnvVars = map[string]string{
	"openai":     "OPENAI_API_KEY",
	"anthropic":  "ANTHROPIC_API_KEY",
	"openrouter": "OPENROUTER_API_KEY",
	"xai":        "XAI_API_KEY",
}

// resolveViaLoadConfig normalizes the URI, loads the full config
// (merging env vars), and creates the provider.
func resolveViaLoadConfig(uri string) (llm.Provider, error) {
	uri = normalizeURI(uri)
	cfg, err := llm.LoadConfig(uri)
	if err != nil {
		return nil, fmt.Errorf("llm config failed for %q: %w", uri, err)
	}

	// Fill API key from provider-specific env var if LoadConfig
	// didn't find one (LoadConfig only reads LLM_API_KEY).
	if cfg.Provider.APIKey == "" {
		if envVar, ok := providerEnvVars[cfg.URI.Scheme]; ok {
			if v := os.Getenv(envVar); v != "" {
				cfg.Provider.APIKey = v
			}
		}
	}

	return resolveFromConfig(cfg)
}

// resolveFromConfig creates a provider from a fully resolved config.
func resolveFromConfig(cfg llm.ResolvedConfig) (llm.Provider, error) {
	uri := cfg.URI.Scheme + "://"
	if cfg.URI.Host != "" {
		uri += cfg.URI.Host + "/"
	}
	uri += cfg.Provider.Model
	if cfg.Provider.APIKey != "" {
		uri += "?api_key=" + cfg.Provider.APIKey
	}
	client, err := llm.Resolve(uri)
	if err != nil {
		return nil, fmt.Errorf("resolve llm provider: %w", err)
	}
	return client, nil
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
