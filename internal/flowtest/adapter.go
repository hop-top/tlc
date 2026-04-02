package flowtest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// AgentAdapter abstracts a CLI agent binary: binary path, argument construction,
// optional env injection, and output parsing. The runner owns exec and env setup;
// the adapter owns the args shape and output format.
type AgentAdapter interface {
	// Name returns the canonical adapter name (e.g. "claude", "gemini").
	Name() string
	// Binary returns the executable name looked up in the sandbox BinDir.
	Binary() string
	// BuildArgs constructs the argv slice for the given prompt.
	BuildArgs(prompt string) []string
	// BuildEnv optionally injects auth or config env vars into the base env.
	// config is the resolved config map for this adapter (key "dir" = config path).
	// When config is nil or config["dir"] is empty, the adapter auto-detects.
	// Implementations with no config to inject should return env unchanged.
	BuildEnv(env []string, config map[string]string) []string
	// ParseOutput converts raw combined output into a structured result map.
	// On JSON parse failure, implementations must fall back to {"output": raw}.
	ParseOutput(raw []byte) (map[string]any, error)
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
func configDir(config map[string]string, fallback func() string) string {
	if config != nil {
		if d := config["dir"]; d != "" {
			return d
		}
	}
	return fallback()
}

// userHome returns the current user's home directory, or "" on error.
func userHome() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}

// ---- ClaudeAdapter ----

type claudeAdapter struct{}

// NewClaudeAdapter returns an adapter for the claude CLI.
// Config dir is resolved at BuildEnv time from config["dir"] or auto-detected.
func NewClaudeAdapter() AgentAdapter { return &claudeAdapter{} }

func (a *claudeAdapter) Name() string   { return "claude" }
func (a *claudeAdapter) Binary() string { return "claude" }

func (a *claudeAdapter) BuildArgs(prompt string) []string {
	return []string{
		"-p", prompt,
		"--print", "--output-format", "json",
		"--dangerously-skip-permissions",
		"--max-budget-usd", "0.10",
	}
}

func (a *claudeAdapter) BuildEnv(env []string, config map[string]string) []string {
	dir := configDir(config, func() string {
		return filepath.Join(userHome(), ".claude")
	})
	return injectEnvVar(env, "CLAUDE_CONFIG_DIR", dir)
}

func (a *claudeAdapter) ParseOutput(raw []byte) (map[string]any, error) {
	return parseJSONOrWrap(raw)
}

// ---- GeminiAdapter ----

// GeminiAdapter wraps the gemini CLI. Config dir is ~/.gemini; no env var
// override is currently supported (upstream issue #2815). BuildEnv is a no-op.
type geminiAdapter struct{}

func NewGeminiAdapter() AgentAdapter { return &geminiAdapter{} }

func (a *geminiAdapter) Name() string   { return "gemini" }
func (a *geminiAdapter) Binary() string { return "gemini" }

func (a *geminiAdapter) BuildArgs(prompt string) []string {
	return []string{"-p", prompt, "--output-format", "json"}
}

func (a *geminiAdapter) BuildEnv(env []string, _ map[string]string) []string { return env }

func (a *geminiAdapter) ParseOutput(raw []byte) (map[string]any, error) {
	return parseJSONOrWrap(raw)
}

// ---- FabricAdapter ----

// FabricAdapter wraps the fabric CLI which produces markdown/text output.
// Config dir is ~/.config/fabric; no env var override. BuildEnv is a no-op.
type fabricAdapter struct{}

func NewFabricAdapter() AgentAdapter { return &fabricAdapter{} }

func (a *fabricAdapter) Name() string   { return "fabric" }
func (a *fabricAdapter) Binary() string { return "fabric" }

func (a *fabricAdapter) BuildArgs(prompt string) []string {
	return []string{"-p", prompt}
}

func (a *fabricAdapter) BuildEnv(env []string, _ map[string]string) []string { return env }

func (a *fabricAdapter) ParseOutput(raw []byte) (map[string]any, error) {
	return map[string]any{"output": string(raw)}, nil
}

// ---- LLMAdapter ----

// LLMAdapter wraps the llm CLI (simonw/llm).
// Default config: ~/Library/Application Support/io.datasette.llm (macOS),
//
//	~/.config/io.datasette.llm (Linux). Override: LLM_USER_PATH.
type llmAdapter struct{}

func NewLLMAdapter() AgentAdapter { return &llmAdapter{} }

func (a *llmAdapter) Name() string   { return "llm" }
func (a *llmAdapter) Binary() string { return "llm" }

func (a *llmAdapter) BuildArgs(prompt string) []string {
	return []string{"-s", prompt}
}

func (a *llmAdapter) BuildEnv(env []string, config map[string]string) []string {
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

// ---- CodexAdapter ----

// CodexAdapter wraps the codex CLI (openai/codex).
// Default config: ~/.codex. Override: CODEX_HOME.
type codexAdapter struct{}

func NewCodexAdapter() AgentAdapter { return &codexAdapter{} }

func (a *codexAdapter) Name() string   { return "codex" }
func (a *codexAdapter) Binary() string { return "codex" }

func (a *codexAdapter) BuildArgs(prompt string) []string {
	return []string{"-p", prompt}
}

func (a *codexAdapter) BuildEnv(env []string, config map[string]string) []string {
	dir := configDir(config, func() string {
		return filepath.Join(userHome(), ".codex")
	})
	return injectEnvVar(env, "CODEX_HOME", dir)
}

func (a *codexAdapter) ParseOutput(raw []byte) (map[string]any, error) {
	return parseJSONOrWrap(raw)
}

// ---- OpenCodeAdapter ----

// OpenCodeAdapter wraps the opencode CLI (sst/opencode).
// Default config: $XDG_CONFIG_HOME/opencode or ~/.config/opencode.
// BuildEnv sets XDG_CONFIG_HOME to the parent of the resolved opencode dir.
type openCodeAdapter struct{}

func NewOpenCodeAdapter() AgentAdapter { return &openCodeAdapter{} }

func (a *openCodeAdapter) Name() string   { return "opencode" }
func (a *openCodeAdapter) Binary() string { return "opencode" }

func (a *openCodeAdapter) BuildArgs(prompt string) []string {
	return []string{"-p", prompt}
}

func (a *openCodeAdapter) BuildEnv(env []string, config map[string]string) []string {
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

// ---- RouteLLMAdapter ----

// RouteLLMAdapter wraps routellm (BerriAI/routellm), an OpenAI-compatible
// routing server launched via `python -m routellm.openai_server`.
// config["dir"] points to the routellm config YAML file (not a directory).
// Default: ~/.config/routellm/config.yaml. Injected as ROUTELLM_CONFIG.
type routeLLMAdapter struct{}

func NewRouteLLMAdapter() AgentAdapter { return &routeLLMAdapter{} }

func (a *routeLLMAdapter) Name() string   { return "routellm" }
func (a *routeLLMAdapter) Binary() string { return "python" }

func (a *routeLLMAdapter) BuildArgs(prompt string) []string {
	return []string{"-m", "routellm.openai_server", "-p", prompt}
}

func (a *routeLLMAdapter) BuildEnv(env []string, config map[string]string) []string {
	cfgFile := configDir(config, func() string {
		return filepath.Join(userHome(), ".config", "routellm", "config.yaml")
	})
	return injectEnvVar(env, "ROUTELLM_CONFIG", cfgFile)
}

func (a *routeLLMAdapter) ParseOutput(raw []byte) (map[string]any, error) {
	return parseJSONOrWrap(raw)
}
