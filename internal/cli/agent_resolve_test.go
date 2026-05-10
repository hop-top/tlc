package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

func TestParseAgentExpr(t *testing.T) {
	cases := []struct {
		in        string
		wantName  string
		wantPairs []string
	}{
		{"", "", nil},
		{"claude", "claude", nil},
		{"claude:env.MODEL=opus", "claude", []string{"env.MODEL=opus"}},
		// Image refs contain ':' (registry:tag); only the first colon
		// separates the agent name from the override value.
		{
			"claude:image=ghcr.io/me/agent:dev",
			"claude",
			[]string{"image=ghcr.io/me/agent:dev"},
		},
		// Multi-override is NOT supported via the inline shorthand —
		// trailing colons become part of the value verbatim.
		// applyAgentOverrides will reject this as malformed key=value.
		{
			"claude:env.MODEL=opus:default_timeout=10m",
			"claude",
			[]string{"env.MODEL=opus:default_timeout=10m"},
		},
		// Empty trailing colon: name plus an empty value;
		// applyAgentOverrides rejects malformed pairs.
		{"claude:", "claude", []string{""}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			gotName, gotPairs := parseAgentExpr(tc.in)
			if gotName != tc.wantName {
				t.Fatalf("name: got %q want %q", gotName, tc.wantName)
			}
			if len(gotPairs) != len(tc.wantPairs) {
				t.Fatalf("pairs: got %v want %v", gotPairs, tc.wantPairs)
			}
			for i := range gotPairs {
				if gotPairs[i] != tc.wantPairs[i] {
					t.Fatalf("pair[%d]: got %q want %q", i, gotPairs[i], tc.wantPairs[i])
				}
			}
		})
	}
}

func TestApplyAgentOverrides_AllKnownKeys(t *testing.T) {
	cfg := &core.AgentConfig{
		Image: "old:latest",
		Env:   map[string]string{"KEEP": "yes", "DROP": "later"},
	}
	err := applyAgentOverrides(cfg,
		[]string{"image=new:dev", "env.MODEL=opus", "default_timeout=10m"},
		[]string{"binary=/usr/local/bin/claude", "env.DROP="},
	)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if cfg.Image != "new:dev" {
		t.Errorf("image: got %q", cfg.Image)
	}
	if cfg.Binary != "/usr/local/bin/claude" {
		t.Errorf("binary: got %q", cfg.Binary)
	}
	if cfg.DefaultTimeout != 10*time.Minute {
		t.Errorf("default_timeout: got %v", cfg.DefaultTimeout)
	}
	if cfg.Env["MODEL"] != "opus" {
		t.Errorf("env.MODEL: got %q", cfg.Env["MODEL"])
	}
	if cfg.Env["KEEP"] != "yes" {
		t.Errorf("env.KEEP unexpectedly mutated: %q", cfg.Env["KEEP"])
	}
	if _, ok := cfg.Env["DROP"]; ok {
		t.Errorf("env.DROP should be cleared by empty value, still: %q", cfg.Env["DROP"])
	}
}

func TestApplyAgentOverrides_ExplicitWinsOverInline(t *testing.T) {
	cfg := &core.AgentConfig{}
	err := applyAgentOverrides(cfg,
		[]string{"image=inline"},
		[]string{"image=explicit"},
	)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if cfg.Image != "explicit" {
		t.Errorf("expected -A to win on duplicate keys; got %q", cfg.Image)
	}
}

func TestApplyAgentOverrides_Errors(t *testing.T) {
	cases := []struct {
		name string
		pair string
		want string
	}{
		{"unknown_key", "wat=1", "unknown key"},
		{"missing_eq", "noequals", "expected key=value"},
		{"bad_duration", "default_timeout=not-a-duration", "default_timeout"},
		{"empty_env_key", "env.=v", "requires a key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := applyAgentOverrides(&core.AgentConfig{}, nil, []string{tc.pair})
			if err == nil {
				t.Fatalf("expected error for %q", tc.pair)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestWithPodMode(t *testing.T) {
	cases := []struct {
		in        string
		wantLocal bool
		wantImg   string
	}{
		{"", true, ""},
		{"false", true, ""},
		{"FALSE", true, ""},
		{"true", false, ""},
		{"TRUE", false, ""},
		{"ghcr.io/me/agent:dev", false, "ghcr.io/me/agent:dev"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			local, img := withPodMode(tc.in)
			if local != tc.wantLocal || img != tc.wantImg {
				t.Fatalf("got (local=%v image=%q), want (%v %q)",
					local, img, tc.wantLocal, tc.wantImg)
			}
		})
	}
}

func TestResolveAgentName(t *testing.T) {
	registry := newTestRegistry(t, map[string]*core.AgentConfig{
		"claude": {Image: "claude:latest"},
		"codex":  {Image: "codex:latest"},
	})

	t.Run("exact_match", func(t *testing.T) {
		got, err := resolveAgentName(registry, "claude")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got != "claude" {
			t.Fatalf("got %q want claude", got)
		}
	})

	t.Run("fuzzy_match", func(t *testing.T) {
		got, err := resolveAgentName(registry, "cla")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got != "claude" {
			t.Fatalf("got %q want claude", got)
		}
	})

	t.Run("no_match", func(t *testing.T) {
		_, err := resolveAgentName(registry, "zzzzz")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Fatalf("error %q missing 'not found'", err.Error())
		}
	})
}

func TestResolveAgentName_AmbiguousFails(t *testing.T) {
	registry := newTestRegistry(t, map[string]*core.AgentConfig{
		"claude-opus":   {Image: "x"},
		"claude-sonnet": {Image: "y"},
		"claude-haiku":  {Image: "z"},
	})
	_, err := resolveAgentName(registry, "claude")
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("error %q missing 'ambiguous'", err.Error())
	}
	// Candidate list should mention the names.
	for _, name := range []string{"claude-opus", "claude-sonnet", "claude-haiku"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("expected candidate %q in error: %s", name, err.Error())
		}
	}
}

func TestResolveAgentName_EmptyRegistry(t *testing.T) {
	_, err := resolveAgentName(core.NewAgentRegistry(), "anything")
	if err == nil {
		t.Fatal("expected error from empty registry")
	}
	if !strings.Contains(err.Error(), "no agents registered") {
		t.Fatalf("error %q missing expected hint", err.Error())
	}
}

// newTestRegistry seeds an AgentRegistry from an in-memory map by writing
// a temporary YAML file and loading it. Avoids touching package-private
// fields on AgentRegistry.
func newTestRegistry(t *testing.T, agents map[string]*core.AgentConfig) *core.AgentRegistry {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "agents.yaml")
	var b strings.Builder
	b.WriteString("agents:\n")
	for name, cfg := range agents {
		b.WriteString("  ")
		b.WriteString(name)
		b.WriteString(":\n")
		if cfg.Image != "" {
			b.WriteString("    image: ")
			b.WriteString(cfg.Image)
			b.WriteString("\n")
		}
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write registry yaml: %v", err)
	}
	registry := core.NewAgentRegistry()
	if err := registry.LoadFromFile(path, true); err != nil {
		t.Fatalf("load registry: %v", err)
	}
	return registry
}
