package flowtest

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

// allAdapters returns one instance of every built-in adapter.
func allAdapters() []AgentAdapter {
	return []AgentAdapter{
		NewClaudeAdapter(),
		NewGeminiAdapter(),
		NewFabricAdapter(),
		NewLLMAdapter(),
		NewCodexAdapter(),
		NewOpenCodeAdapter(),
		NewRouteLLMAdapter(),
	}
}

// TestAdapterInterface verifies each built-in adapter satisfies the interface
// and returns consistent metadata.
func TestAdapterInterface(t *testing.T) {
	for _, a := range allAdapters() {
		a := a
		t.Run(a.Name(), func(t *testing.T) {
			if a.Name() == "" {
				t.Fatal("Name() must not be empty")
			}
			if a.Binary() == "" {
				t.Fatal("Binary() must not be empty")
			}
			args := a.BuildArgs("hello")
			if len(args) == 0 {
				t.Fatal("BuildArgs() must return at least one arg")
			}
			found := false
			for _, arg := range args {
				if arg == "hello" {
					found = true
				}
			}
			if !found {
				t.Errorf("BuildArgs() args %v do not contain prompt", args)
			}
		})
	}
}

// --- BuildEnv with explicit config ---

func TestClaudeAdapterBuildEnvExplicitDir(t *testing.T) {
	a := NewClaudeAdapter()
	env := a.BuildEnv([]string{"HOME=/tmp"}, map[string]string{"dir": "/custom/.claude"})
	if !hasEnv(env, "CLAUDE_CONFIG_DIR=/custom/.claude") {
		t.Errorf("expected CLAUDE_CONFIG_DIR=/custom/.claude; got %v", env)
	}
}

func TestLLMAdapterBuildEnvExplicitDir(t *testing.T) {
	a := NewLLMAdapter()
	env := a.BuildEnv([]string{"HOME=/tmp"}, map[string]string{"dir": "/custom/llm"})
	if !hasEnv(env, "LLM_USER_PATH=/custom/llm") {
		t.Errorf("expected LLM_USER_PATH=/custom/llm; got %v", env)
	}
}

func TestCodexAdapterBuildEnvExplicitDir(t *testing.T) {
	a := NewCodexAdapter()
	env := a.BuildEnv([]string{"HOME=/tmp"}, map[string]string{"dir": "/custom/.codex"})
	if !hasEnv(env, "CODEX_HOME=/custom/.codex") {
		t.Errorf("expected CODEX_HOME=/custom/.codex; got %v", env)
	}
}

func TestOpenCodeAdapterBuildEnvExplicitDir(t *testing.T) {
	a := NewOpenCodeAdapter()
	env := a.BuildEnv([]string{"HOME=/tmp"}, map[string]string{"dir": "/custom/opencode"})
	// opencode uses XDG_CONFIG_HOME set to the parent of the opencode dir
	if !hasEnv(env, "XDG_CONFIG_HOME=/custom") {
		t.Errorf("expected XDG_CONFIG_HOME=/custom; got %v", env)
	}
}

func TestRouteLLMAdapterBuildEnvExplicitDir(t *testing.T) {
	a := NewRouteLLMAdapter()
	env := a.BuildEnv([]string{"HOME=/tmp"}, map[string]string{"dir": "/custom/routellm/config.yaml"})
	if !hasEnv(env, "ROUTELLM_CONFIG=/custom/routellm/config.yaml") {
		t.Errorf("expected ROUTELLM_CONFIG=/custom/routellm/config.yaml; got %v", env)
	}
}

// --- BuildEnv with nil config → auto-detect ---

func TestClaudeAdapterBuildEnvAutoDetect(t *testing.T) {
	a := NewClaudeAdapter()
	env := a.BuildEnv([]string{"HOME=/tmp"}, nil)
	// should inject CLAUDE_CONFIG_DIR pointing somewhere under real HOME
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "CLAUDE_CONFIG_DIR=") && kv != "CLAUDE_CONFIG_DIR=" {
			found = true
		}
	}
	if !found {
		t.Errorf("auto-detect: expected CLAUDE_CONFIG_DIR to be set; got %v", env)
	}
}

func TestLLMAdapterBuildEnvAutoDetect(t *testing.T) {
	a := NewLLMAdapter()
	env := a.BuildEnv([]string{"HOME=/tmp"}, nil)
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "LLM_USER_PATH=") && kv != "LLM_USER_PATH=" {
			found = true
		}
	}
	if !found {
		t.Errorf("auto-detect: expected LLM_USER_PATH to be set; got %v", env)
	}
}

// --- BuildEnv no-op adapters (gemini, fabric) ---

func TestGeminiAdapterBuildEnvIsNoop(t *testing.T) {
	a := NewGeminiAdapter()
	base := []string{"HOME=/tmp", "PATH=/usr/bin"}
	got := a.BuildEnv(base, map[string]string{"dir": "/custom"})
	if len(got) != len(base) {
		t.Errorf("gemini BuildEnv should be no-op; got %v", got)
	}
}

func TestFabricAdapterBuildEnvIsNoop(t *testing.T) {
	a := NewFabricAdapter()
	base := []string{"HOME=/tmp", "PATH=/usr/bin"}
	got := a.BuildEnv(base, nil)
	if len(got) != len(base) {
		t.Errorf("fabric BuildEnv should be no-op; got %v", got)
	}
}

// --- Auto-detect paths match expected OS defaults ---

func TestClaudeAdapterAutoDetectPath(t *testing.T) {
	a := NewClaudeAdapter()
	env := a.BuildEnv(nil, nil)
	dir := envVal(env, "CLAUDE_CONFIG_DIR")
	if !strings.HasSuffix(dir, ".claude") {
		t.Errorf("CLAUDE_CONFIG_DIR=%q, want suffix .claude", dir)
	}
}

func TestCodexAdapterAutoDetectPath(t *testing.T) {
	a := NewCodexAdapter()
	env := a.BuildEnv(nil, nil)
	dir := envVal(env, "CODEX_HOME")
	if !strings.HasSuffix(dir, ".codex") {
		t.Errorf("CODEX_HOME=%q, want suffix .codex", dir)
	}
}

func TestLLMAdapterAutoDetectPath(t *testing.T) {
	a := NewLLMAdapter()
	env := a.BuildEnv(nil, nil)
	dir := envVal(env, "LLM_USER_PATH")
	if runtime.GOOS == "darwin" {
		if !strings.Contains(dir, "Application Support") {
			t.Errorf("LLM_USER_PATH=%q, want Application Support on macOS", dir)
		}
	} else {
		if !strings.Contains(dir, "io.datasette.llm") {
			t.Errorf("LLM_USER_PATH=%q, want io.datasette.llm on Linux", dir)
		}
	}
}

// --- ParseOutput ---

func TestClaudeAdapterParseOutputJSON(t *testing.T) {
	a := NewClaudeAdapter()
	raw := []byte(`{"type":"result","subtype":"success","result":"done"}`)
	out, err := a.ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error: %v", err)
	}
	if out["type"] != "result" {
		t.Errorf("expected type=result, got %v", out["type"])
	}
}

func TestFabricAdapterParseOutputText(t *testing.T) {
	a := NewFabricAdapter()
	raw := []byte("# Analysis\n\nSome text output.")
	out, err := a.ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error: %v", err)
	}
	if out["output"] != string(raw) {
		t.Errorf("expected output=%q, got %v", string(raw), out["output"])
	}
}

func TestParseOutputFallbackOnInvalidJSON(t *testing.T) {
	for _, a := range allAdapters() {
		a := a
		if a.Name() == "fabric" {
			continue // fabric always wraps as text
		}
		t.Run(a.Name(), func(t *testing.T) {
			raw := []byte("not json at all")
			out, err := a.ParseOutput(raw)
			if err != nil {
				t.Fatalf("ParseOutput() should not error on bad JSON; got: %v", err)
			}
			if out["output"] != string(raw) {
				t.Errorf("expected fallback output=%q, got %v", string(raw), out["output"])
			}
		})
	}
}

func TestAdapterParseOutputValidJSON(t *testing.T) {
	payload := map[string]any{"type": "result", "result": "ok"}
	raw, _ := json.Marshal(payload)
	for _, a := range allAdapters() {
		a := a
		if a.Name() == "fabric" {
			continue
		}
		t.Run(a.Name(), func(t *testing.T) {
			out, err := a.ParseOutput(raw)
			if err != nil {
				t.Fatalf("ParseOutput() error: %v", err)
			}
			if out["type"] != "result" {
				t.Errorf("expected type=result, got %v", out["type"])
			}
		})
	}
}

// --- helpers ---

func hasEnv(env []string, kv string) bool {
	for _, e := range env {
		if e == kv {
			return true
		}
	}
	return false
}

func envVal(env []string, key string) string {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return e[len(prefix):]
		}
	}
	return ""
}
