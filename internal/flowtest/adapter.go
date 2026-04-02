package flowtest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"hop.top/tlc/internal/core"
)

// AgentAdapter abstracts a CLI agent binary: binary path, argument construction,
// optional env injection, and output parsing. The runner owns exec and env setup;
// the adapter owns the args shape and output format.
type AgentAdapter interface {
	// Name returns the canonical adapter name (e.g. "claude", "gemini").
	Name() string
	// Binary returns the executable name looked up in the sandbox BinDir.
	Binary() string
	// BuildArgs constructs the argv slice for the given prompt, config, and operation.
	// config is the resolved config map; operation is from Operation(step).
	BuildArgs(prompt string, config map[string]any, operation string) []string
	// BuildEnv optionally injects auth or config env vars into the base env.
	// config is the resolved config map for this adapter (key "dir" = config path).
	// When config is nil or config["dir"] is empty, the adapter auto-detects.
	// Implementations with no config to inject should return env unchanged.
	BuildEnv(env []string, config map[string]any) []string
	// ParseOutput converts raw combined output into a structured result map.
	// On JSON parse failure, implementations must fall back to {"output": raw}.
	ParseOutput(raw []byte) (map[string]any, error)
	// Probe discovers available tools, models, flags by running the CLI.
	// binPath is the resolved binary (from sandbox BinDir or PATH).
	// Called once per session; result cached by AdapterResolver.
	Probe(ctx context.Context, binPath string) (*AdapterCapabilities, error)
	// Operation returns the logical operation name for a step (e.g. "prompt", "embed").
	// Used to extract the right sub-map from config before BuildArgs.
	// Return "" for single-mode adapters.
	Operation(step core.Step) string
}

// buildFlagsFromConfig converts a config map to a --key val slice.
//
// Logic:
//  1. Collect flat keys from config (skip sub-maps and keys in skip list)
//  2. If operation != "" and config[operation] is map[string]any: merge operation keys,
//     overriding flat values
//  3. For each key/val: append "--key", "val" (bool true → flag only, no value)
//  4. Sub-map keys are always skipped (they are operation namespaces, not flags)
func buildFlagsFromConfig(config map[string]any, operation string, skip []string) []string {
	if len(config) == 0 {
		return nil
	}

	skipSet := make(map[string]bool, len(skip))
	for _, s := range skip {
		skipSet[s] = true
	}

	// Step 1: collect flat keys (skip sub-maps and skip-list).
	flat := make(map[string]any)
	for k, v := range config {
		if skipSet[k] {
			continue
		}
		if _, isMap := v.(map[string]any); isMap {
			continue // sub-map = operation namespace; skip
		}
		flat[k] = v
	}

	// Step 2: merge operation sub-map keys (override flat).
	if operation != "" {
		if opVal, ok := config[operation]; ok {
			if opMap, ok := opVal.(map[string]any); ok {
				for k, v := range opMap {
					if _, isMap := v.(map[string]any); isMap {
						continue
					}
					flat[k] = v
				}
			}
		}
	}

	if len(flat) == 0 {
		return nil
	}

	// Sort keys for determinism.
	keys := make([]string, 0, len(flat))
	for k := range flat {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}

	// Step 3: emit --key val pairs.
	var out []string
	for _, k := range keys {
		v := flat[k]
		switch tv := v.(type) {
		case bool:
			if tv {
				out = append(out, "--"+k)
			}
		default:
			out = append(out, "--"+k, fmt.Sprintf("%v", tv))
		}
	}
	return out
}

// parseJSONOrWrap tries to unmarshal raw as JSON; on failure returns a raw wrap.
func parseJSONOrWrap(raw []byte) (map[string]any, error) {
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return map[string]any{"output": string(raw)}, nil
	}
	return result, nil
}

// injectEnvVar returns env with key=val set, replacing any existing value.
func injectEnvVar(env []string, key, val string) []string {
	if val == "" {
		return env
	}
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	found := false
	for _, kv := range env {
		if len(kv) >= len(prefix) && kv[:len(prefix)] == prefix {
			out = append(out, prefix+val)
			found = true
		} else {
			out = append(out, kv)
		}
	}
	if !found {
		out = append(out, prefix+val)
	}
	return out
}

// configDir returns config["dir"] if non-empty, otherwise calls fallback().
// The "dir" value must be a string; non-string values are ignored.
func configDir(config map[string]any, fallback func() string) string {
	if config != nil {
		if v, ok := config["dir"]; ok {
			if d, ok := v.(string); ok && d != "" {
				return d
			}
		}
	}
	return fallback()
}

// AdapterCapabilities describes the known capabilities of an adapter binary,
// discovered at runtime from help/list output or static registration.
type AdapterCapabilities struct {
	Tools  []string
	Models []string
	Flags  []string // known --flag names discovered from help/list
}

// userHome returns the current user's home directory, or "" on error.
func userHome() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}

// runProbeCmd runs binPath with args and returns combined output.
// Returns empty bytes (not an error) when the binary is not found.
func runProbeCmd(ctx context.Context, binPath string, args ...string) []byte {
	cmd := exec.CommandContext(ctx, binPath, args...)
	out, _ := cmd.CombinedOutput()
	return out
}

// ---- ClaudeAdapter ----

type claudeAdapter struct{}

// NewClaudeAdapter returns an adapter for the claude CLI.
// Config dir is resolved at BuildEnv time from config["dir"] or auto-detected.
func NewClaudeAdapter() AgentAdapter { return &claudeAdapter{} }

func (a *claudeAdapter) Name() string   { return "claude" }
func (a *claudeAdapter) Binary() string { return "claude" }

func (a *claudeAdapter) BuildArgs(prompt string, config map[string]any, operation string) []string {
	fixed := []string{
		"-p", prompt,
		"--print", "--output-format", "json",
		"--dangerously-skip-permissions",
		"--max-budget-usd", "0.10",
	}
	return append(fixed, buildFlagsFromConfig(config, operation, []string{"dir"})...)
}

func (a *claudeAdapter) BuildEnv(env []string, config map[string]any) []string {
	dir := configDir(config, func() string {
		return filepath.Join(userHome(), ".claude")
	})
	return injectEnvVar(env, "CLAUDE_CONFIG_DIR", dir)
}

func (a *claudeAdapter) ParseOutput(raw []byte) (map[string]any, error) {
	return parseJSONOrWrap(raw)
}

// Probe runs `binPath --help` and parses built-in tool names from the output.
// Models: if --model flag present in help, populates known aliases (sonnet, opus, haiku).
// Flags: extracted from help lines starting with "--".
func (a *claudeAdapter) Probe(ctx context.Context, binPath string) (*AdapterCapabilities, error) {
	out := runProbeCmd(ctx, binPath, "--help")
	caps := &AdapterCapabilities{}

	// Parse known tool names from help text.
	knownTools := []string{
		"Bash", "Read", "Edit", "Write", "Glob", "Grep",
		"WebFetch", "WebSearch", "mcp", "TodoWrite", "Agent",
	}
	helpText := string(out)
	for _, tool := range knownTools {
		if strings.Contains(helpText, tool) {
			caps.Tools = append(caps.Tools, tool)
		}
	}

	// Parse flag names: lines containing "--flag" patterns.
	hasModelFlag := false
	for _, line := range strings.Split(helpText, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			parts := strings.Fields(trimmed)
			if len(parts) > 0 {
				flagName := strings.TrimRight(parts[0], ",")
				caps.Flags = append(caps.Flags, flagName)
				if flagName == "--model" {
					hasModelFlag = true
				}
			}
		}
	}

	// Populate known model aliases when --model flag is present.
	if hasModelFlag {
		caps.Models = []string{"sonnet", "opus", "haiku"}
	}

	return caps, nil
}

// Operation returns "" — claude is single-mode.
func (a *claudeAdapter) Operation(_ core.Step) string { return "" }

// ---- GeminiAdapter ----

// GeminiAdapter wraps the gemini CLI. Config dir is ~/.gemini; no env var
// override is currently supported (upstream issue #2815). BuildEnv is a no-op.
type geminiAdapter struct{}

func NewGeminiAdapter() AgentAdapter { return &geminiAdapter{} }

func (a *geminiAdapter) Name() string   { return "gemini" }
func (a *geminiAdapter) Binary() string { return "gemini" }

func (a *geminiAdapter) BuildArgs(prompt string, config map[string]any, operation string) []string {
	fixed := []string{"-p", prompt, "--output-format", "json", "--yolo"}
	skip := []string{"dir"}
	return append(fixed, buildFlagsFromConfig(config, "", skip)...)
}

// BuildEnv is a no-op: gemini has no config-dir env var override (upstream issue #2815).
func (a *geminiAdapter) BuildEnv(env []string, _ map[string]any) []string { return env }

func (a *geminiAdapter) ParseOutput(raw []byte) (map[string]any, error) {
	return parseJSONOrWrap(raw)
}

// Probe runs `binPath --help` and parses known flag names from the output.
// gemini has no `models list` subcommand; Models list is always empty.
func (a *geminiAdapter) Probe(ctx context.Context, binPath string) (*AdapterCapabilities, error) {
	out := runProbeCmd(ctx, binPath, "--help")
	caps := &AdapterCapabilities{}

	helpText := string(out)

	// Known flags to detect from help output.
	knownFlags := []string{"model", "output-format", "approval-mode", "allowed-tools"}
	for _, flag := range knownFlags {
		if strings.Contains(helpText, "--"+flag) || strings.Contains(helpText, flag) {
			caps.Flags = append(caps.Flags, flag)
		}
	}

	// Parse any additional --flag lines not already covered above.
	for _, line := range strings.Split(helpText, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			parts := strings.Fields(trimmed)
			if len(parts) > 0 {
				flagName := strings.TrimLeft(strings.TrimRight(parts[0], ","), "--")
				already := false
				for _, f := range caps.Flags {
					if f == flagName {
						already = true
						break
					}
				}
				if !already && flagName != "" {
					caps.Flags = append(caps.Flags, flagName)
				}
			}
		}
	}

	return caps, nil
}

func (a *geminiAdapter) Operation(_ core.Step) string { return "" }

// ---- FabricAdapter ----

// FabricAdapter wraps the fabric CLI which produces markdown/text output.
// Config dir is ~/.config/fabric; no env var override. BuildEnv is a no-op.
type fabricAdapter struct{}

func NewFabricAdapter() AgentAdapter { return &fabricAdapter{} }

func (a *fabricAdapter) Name() string   { return "fabric" }
func (a *fabricAdapter) Binary() string { return "fabric" }

// normaliseBoolFlags removes the value token for boolean flags that take no value.
// For each name in boolFlags, if args contains "--name" immediately followed by "true",
// the "true" token is dropped.
func normaliseBoolFlags(args []string, boolFlags []string) []string {
	flagSet := make(map[string]bool, len(boolFlags))
	for _, f := range boolFlags {
		flagSet["--"+f] = true
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		out = append(out, args[i])
		if flagSet[args[i]] && i+1 < len(args) && args[i+1] == "true" {
			i++ // skip the "true" value token
		}
	}
	return out
}

func (a *fabricAdapter) BuildArgs(prompt string, config map[string]any, operation string) []string {
	fixed := []string{"-p", prompt}
	args := append(fixed, buildFlagsFromConfig(config, "", []string{"dir"})...)
	return normaliseBoolFlags(args, []string{"stream"})
}

func (a *fabricAdapter) BuildEnv(env []string, _ map[string]any) []string { return env }

func (a *fabricAdapter) ParseOutput(raw []byte) (map[string]any, error) {
	return map[string]any{"output": string(raw)}, nil
}

// Probe discovers fabric patterns (tools) and models by calling --listpatterns
// and --listmodels. Lines matching "Vendor|model" are parsed for models;
// bare pattern names populate tools.
func (a *fabricAdapter) Probe(ctx context.Context, binPath string) (*AdapterCapabilities, error) {
	caps := &AdapterCapabilities{
		Flags: []string{"pattern", "model", "temperature", "stream"},
	}

	// Parse patterns → tools; each line is a bare pattern name.
	patternsOut := runProbeCmd(ctx, binPath, "--listpatterns")
	for _, line := range strings.Split(string(patternsOut), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			caps.Tools = append(caps.Tools, name)
		}
	}

	// Parse models; lines are formatted as "Vendor|model-name".
	modelsOut := runProbeCmd(ctx, binPath, "--listmodels")
	for _, line := range strings.Split(string(modelsOut), "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, "|"); idx >= 0 {
			model := strings.TrimSpace(line[idx+1:])
			if model != "" {
				caps.Models = append(caps.Models, model)
			}
		}
	}

	return caps, nil
}

func (a *fabricAdapter) Operation(_ core.Step) string { return "" }

// ---- LLMAdapter ----

// LLMAdapter wraps the llm CLI (simonw/llm).
// Default config: ~/Library/Application Support/io.datasette.llm (macOS),
//
//	~/.config/io.datasette.llm (Linux). Override: LLM_USER_PATH.
type llmAdapter struct{}

func NewLLMAdapter() AgentAdapter { return &llmAdapter{} }

func (a *llmAdapter) Name() string   { return "llm" }
func (a *llmAdapter) Binary() string { return "llm" }

func (a *llmAdapter) BuildArgs(prompt string, config map[string]any, operation string) []string {
	op := operation
	if op == "" {
		op = "prompt"
	}
	fixed := []string{op, "-s", prompt}
	return append(fixed, buildFlagsFromConfig(config, op, []string{"dir", "prompt", "embed"})...)
}

func (a *llmAdapter) BuildEnv(env []string, config map[string]any) []string {
	dir := configDir(config, func() string {
		if runtime.GOOS == "darwin" {
			return filepath.Join(userHome(), "Library", "Application Support", "io.datasette.llm")
		}
		return filepath.Join(userHome(), ".config", "io.datasette.llm")
	})
	return injectEnvVar(env, "LLM_USER_PATH", dir)
}

func (a *llmAdapter) ParseOutput(raw []byte) (map[string]any, error) {
	return parseJSONOrWrap(raw)
}

// Probe runs `binPath tools list` and `binPath models list` to discover capabilities.
func (a *llmAdapter) Probe(ctx context.Context, binPath string) (*AdapterCapabilities, error) {
	caps := &AdapterCapabilities{}

	// Parse tools: lines containing "() -> "
	toolsOut := runProbeCmd(ctx, binPath, "tools", "list")
	for _, line := range strings.Split(string(toolsOut), "\n") {
		if strings.Contains(line, "() -> ") {
			// Extract function name before "()"
			idx := strings.Index(line, "()")
			if idx > 0 {
				name := strings.TrimSpace(line[:idx])
				if name != "" {
					caps.Tools = append(caps.Tools, name)
				}
			}
		}
	}

	// Parse models: lines containing "<Provider> Chat: " (any provider prefix).
	modelsOut := runProbeCmd(ctx, binPath, "models", "list")
	const chatSuffix = " Chat: "
	for _, line := range strings.Split(string(modelsOut), "\n") {
		if idx := strings.Index(line, chatSuffix); idx >= 0 {
			model := strings.TrimSpace(line[idx+len(chatSuffix):])
			if model != "" {
				caps.Models = append(caps.Models, model)
			}
		}
	}

	caps.Flags = []string{"model", "system", "tool", "schema"}
	return caps, nil
}

// Operation returns "embed" if the step requires embed (via Capabilities or Tools), else "prompt".
func (a *llmAdapter) Operation(step core.Step) string {
	if step.TaskTemplate != nil && step.TaskTemplate.Requirements != nil {
		req := step.TaskTemplate.Requirements
		for _, cap := range req.Capabilities {
			if cap == "embed" {
				return "embed"
			}
		}
		for _, tool := range req.Tools {
			if tool == "embed" {
				return "embed"
			}
		}
	}
	return "prompt"
}

// ---- CodexAdapter ----

// CodexAdapter wraps the codex CLI (openai/codex).
// Default config: ~/.codex. Override: CODEX_HOME.
type codexAdapter struct{}

func NewCodexAdapter() AgentAdapter { return &codexAdapter{} }

func (a *codexAdapter) Name() string   { return "codex" }
func (a *codexAdapter) Binary() string { return "codex" }

// BuildArgs builds codex argv: "exec" subcommand + prompt, then -c key=value pairs.
// codex uses -c key=value form, NOT --key value; do not use buildFlagsFromConfig.
func (a *codexAdapter) BuildArgs(prompt string, config map[string]any, operation string) []string {
	fixed := []string{"exec", prompt}
	skip := map[string]bool{"dir": true, "exec": true}
	merged := codexFlattenConfig(config, operation, skip)

	// Sort keys for determinism.
	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}

	args := make([]string, 0, len(fixed)+len(keys)*2)
	args = append(args, fixed...)
	for _, k := range keys {
		args = append(args, "-c", k+"="+fmt.Sprint(merged[k]))
	}
	return args
}

// codexFlattenConfig merges flat scalar keys and operation sub-map keys,
// skipping any key in the skip set and any nested sub-maps.
func codexFlattenConfig(config map[string]any, operation string, skip map[string]bool) map[string]any {
	out := make(map[string]any)
	for k, v := range config {
		if skip[k] {
			continue
		}
		if _, isMap := v.(map[string]any); isMap {
			continue
		}
		out[k] = v
	}
	if operation != "" {
		if opVal, ok := config[operation]; ok {
			if opMap, ok := opVal.(map[string]any); ok {
				for k, v := range opMap {
					if _, isMap := v.(map[string]any); isMap {
						continue
					}
					out[k] = v
				}
			}
		}
	}
	return out
}

func (a *codexAdapter) BuildEnv(env []string, config map[string]any) []string {
	dir := configDir(config, func() string {
		return filepath.Join(userHome(), ".codex")
	})
	return injectEnvVar(env, "CODEX_HOME", dir)
}

func (a *codexAdapter) ParseOutput(raw []byte) (map[string]any, error) {
	return parseJSONOrWrap(raw)
}

// Probe runs `binPath exec --help` and parses flag names from the output.
// codex has no models list subcommand — Models is always empty.
func (a *codexAdapter) Probe(ctx context.Context, binPath string) (*AdapterCapabilities, error) {
	out := runProbeCmd(ctx, binPath, "exec", "--help")
	caps := &AdapterCapabilities{}

	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			parts := strings.Fields(trimmed)
			if len(parts) > 0 {
				flagName := strings.TrimRight(parts[0], ",")
				// Store bare name without leading "--".
				caps.Flags = append(caps.Flags, strings.TrimPrefix(flagName, "--"))
			}
		}
	}

	return caps, nil
}

// Operation returns "exec" — codex requires the exec subcommand for non-interactive use.
func (a *codexAdapter) Operation(_ core.Step) string { return "exec" }

// ---- OpenCodeAdapter ----

// OpenCodeAdapter wraps the opencode CLI (sst/opencode).
// Default config: $XDG_CONFIG_HOME/opencode or ~/.config/opencode.
// BuildEnv sets XDG_CONFIG_HOME to the parent of the resolved opencode dir.
type openCodeAdapter struct{}

func NewOpenCodeAdapter() AgentAdapter { return &openCodeAdapter{} }

func (a *openCodeAdapter) Name() string   { return "opencode" }
func (a *openCodeAdapter) Binary() string { return "opencode" }

func (a *openCodeAdapter) BuildArgs(prompt string, config map[string]any, operation string) []string {
	fixed := []string{"run", prompt}
	return append(fixed, buildFlagsFromConfig(config, "run", []string{"dir", "run"})...)
}

func (a *openCodeAdapter) BuildEnv(env []string, config map[string]any) []string {
	dir := configDir(config, func() string {
		return filepath.Join(userHome(), ".config", "opencode")
	})
	// opencode appends /opencode to XDG_CONFIG_HOME itself, so set parent.
	parent := filepath.Dir(dir)
	return injectEnvVar(env, "XDG_CONFIG_HOME", parent)
}

func (a *openCodeAdapter) ParseOutput(raw []byte) (map[string]any, error) {
	return parseJSONOrWrap(raw)
}

// Probe runs `binPath models` to discover available models and `binPath run --help`
// to discover supported flags.
func (a *openCodeAdapter) Probe(ctx context.Context, binPath string) (*AdapterCapabilities, error) {
	caps := &AdapterCapabilities{}

	// Parse models: lines matching "provider/model-name" pattern.
	modelsOut := runProbeCmd(ctx, binPath, "models")
	for _, line := range strings.Split(string(modelsOut), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "/") && !strings.HasPrefix(trimmed, "-") && trimmed != "" {
			// Take first word (model identifier before any whitespace/description).
			model := strings.Fields(trimmed)[0]
			caps.Models = append(caps.Models, model)
		}
	}

	// Parse flags from `run --help`: lines containing "--flag" patterns.
	helpOut := runProbeCmd(ctx, binPath, "run", "--help")
	for _, line := range strings.Split(string(helpOut), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			parts := strings.Fields(trimmed)
			if len(parts) > 0 {
				flagName := strings.TrimRight(parts[0], ",")
				caps.Flags = append(caps.Flags, flagName)
			}
		}
	}

	return caps, nil
}

// Operation returns "run" — opencode requires the run subcommand for non-interactive use.
func (a *openCodeAdapter) Operation(_ core.Step) string { return "run" }

// ---- RouteLLMAdapter ----

// RouteLLMAdapter wraps routellm (BerriAI/routellm), an OpenAI-compatible
// routing server launched via `python -m routellm.openai_server`.
//
// NOTE: config["dir"] is a config *file* path (e.g. ~/.config/routellm/config.yaml),
// NOT a directory. This is an exception to the convention used by other adapters
// (claude, codex, opencode) where "dir" refers to a directory. It is injected as
// ROUTELLM_CONFIG. The "dir" key is excluded from --flag expansion in BuildArgs.
type routeLLMAdapter struct{}

func NewRouteLLMAdapter() AgentAdapter { return &routeLLMAdapter{} }

func (a *routeLLMAdapter) Name() string   { return "routellm" }
func (a *routeLLMAdapter) Binary() string { return "python" }

// BuildArgs constructs argv for python -m routellm.openai_server.
// The operation parameter is ignored — routeLLM is single-mode; routing config
// comes from ROUTELLM_CONFIG, not from subcommands. "dir" is excluded from
// --flag expansion because it is a file path handled by BuildEnv.
func (a *routeLLMAdapter) BuildArgs(prompt string, config map[string]any, _ string) []string {
	fixed := []string{"-m", "routellm.openai_server", "-p", prompt}
	skip := []string{"dir"}
	return append(fixed, buildFlagsFromConfig(config, "", skip)...)
}

func (a *routeLLMAdapter) BuildEnv(env []string, config map[string]any) []string {
	// config["dir"] is a file path to the routellm YAML config, not a directory.
	cfgFile := configDir(config, func() string {
		return filepath.Join(userHome(), ".config", "routellm", "config.yaml")
	})
	return injectEnvVar(env, "ROUTELLM_CONFIG", cfgFile)
}

func (a *routeLLMAdapter) ParseOutput(raw []byte) (map[string]any, error) {
	return parseJSONOrWrap(raw)
}

// Probe attempts `python -m routellm.openai_server --help`.
// routellm has no models/tools list subcommand; routing config comes from
// ROUTELLM_CONFIG. On any error (module not found, python absent, etc.)
// returns empty capabilities non-fatally.
func (a *routeLLMAdapter) Probe(ctx context.Context, binPath string) (*AdapterCapabilities, error) {
	cmd := exec.CommandContext(ctx, binPath, "-m", "routellm.openai_server", "--help")
	if err := cmd.Run(); err != nil {
		return &AdapterCapabilities{}, nil
	}
	return &AdapterCapabilities{}, nil
}

// Operation returns "" — routeLLM is single-mode.
func (a *routeLLMAdapter) Operation(_ core.Step) string { return "" }
