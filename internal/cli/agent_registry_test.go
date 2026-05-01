// Tests for `tlc agent register|show|registered` CLI subcommands.
// Tests exercise the RunE handlers directly (bypassing cobra root lifecycle
// to avoid spurious TUI initialization in test runs).
//
// Author: jadb
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withHome temporarily redirects HOME so the registry uses a tmp config path.
// Returns the resolved global config path.
func withHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	return filepath.Join(tmp, ".config", "tlc", "agents.yaml")
}

func resetAgentRegisterFlags() {
	agentRegisterBinary = ""
	agentRegisterImage = ""
	agentRegisterEnv = nil
	agentRegisterTimeout = 0
	agentRegisterProject = false
}

func TestAgentRegister_CreatesGlobalConfig(t *testing.T) {
	cfgPath := withHome(t)
	defer resetAgentRegisterFlags()

	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatalf("pre-test: expected no config file, got %v", err)
	}

	resetAgentRegisterFlags()
	agentRegisterBinary = "/usr/local/bin/claude"
	agentRegisterImage = "ghcr.io/hop-top/pod-claude:latest"

	var out bytes.Buffer
	AgentRegisterCmd.SetOut(&out)
	AgentRegisterCmd.SetErr(&out)
	if err := AgentRegisterCmd.RunE(AgentRegisterCmd, []string{"claude"}); err != nil {
		t.Fatalf("register failed: %v\noutput: %s", err, out.String())
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "claude:") {
		t.Errorf("config missing agent key 'claude':\n%s", body)
	}
	if !strings.Contains(body, "/usr/local/bin/claude") {
		t.Errorf("config missing binary path:\n%s", body)
	}
}

func TestAgentRegister_RejectsMissingBinary(t *testing.T) {
	withHome(t)
	defer resetAgentRegisterFlags()

	resetAgentRegisterFlags()
	// No --binary, no --image: must fail.

	var out bytes.Buffer
	AgentRegisterCmd.SetOut(&out)
	AgentRegisterCmd.SetErr(&out)
	err := AgentRegisterCmd.RunE(AgentRegisterCmd, []string{"claude"})
	if err == nil {
		t.Fatal("expected error when neither --binary nor --image set")
	}
	if !strings.Contains(err.Error(), "binary") && !strings.Contains(err.Error(), "image") {
		t.Errorf("error should mention binary or image: %v", err)
	}
}

func TestAgentRegister_AppendsToExisting(t *testing.T) {
	cfgPath := withHome(t)
	defer resetAgentRegisterFlags()

	// Seed first agent.
	resetAgentRegisterFlags()
	agentRegisterBinary = "/usr/local/bin/claude"
	var out bytes.Buffer
	AgentRegisterCmd.SetOut(&out)
	if err := AgentRegisterCmd.RunE(AgentRegisterCmd, []string{"claude"}); err != nil {
		t.Fatal(err)
	}

	// Add second agent.
	resetAgentRegisterFlags()
	agentRegisterBinary = "/usr/local/bin/codex"
	out.Reset()
	if err := AgentRegisterCmd.RunE(AgentRegisterCmd, []string{"codex"}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(cfgPath)
	body := string(data)
	if !strings.Contains(body, "claude:") || !strings.Contains(body, "codex:") {
		t.Errorf("config should contain both agents:\n%s", body)
	}
}

func TestAgentShow_ReadsRegistered(t *testing.T) {
	cfgPath := withHome(t)

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `agents:
  gemini:
    binary: /opt/gemini
    image: ghcr.io/hop-top/pod-gemini:latest
    default_timeout: 1h
`
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	AgentShowCmd.SetOut(&out)
	AgentShowCmd.SetErr(&out)
	if err := AgentShowCmd.RunE(AgentShowCmd, []string{"gemini"}); err != nil {
		t.Fatalf("show failed: %v\noutput: %s", err, out.String())
	}

	output := out.String()
	for _, want := range []string{"gemini", "/opt/gemini", "ghcr.io/hop-top/pod-gemini"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, output)
		}
	}
}

func TestAgentShow_MissingAgent(t *testing.T) {
	withHome(t)

	var out bytes.Buffer
	AgentShowCmd.SetOut(&out)
	AgentShowCmd.SetErr(&out)
	err := AgentShowCmd.RunE(AgentShowCmd, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should say 'not found': %v", err)
	}
}

func TestAgentRegistered_ListsAll(t *testing.T) {
	cfgPath := withHome(t)
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `agents:
  claude:
    binary: /a/b/claude
  codex:
    binary: /a/b/codex
`
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	AgentRegisteredCmd.SetOut(&out)
	AgentRegisteredCmd.SetErr(&out)
	if err := AgentRegisteredCmd.RunE(AgentRegisteredCmd, nil); err != nil {
		t.Fatalf("registered failed: %v", err)
	}

	output := out.String()
	for _, want := range []string{"claude", "codex"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected list to mention %q, got:\n%s", want, output)
		}
	}
}

func TestAgentRegistered_NoConfigEmpty(t *testing.T) {
	withHome(t)

	var out bytes.Buffer
	AgentRegisteredCmd.SetOut(&out)
	AgentRegisteredCmd.SetErr(&out)
	if err := AgentRegisteredCmd.RunE(AgentRegisteredCmd, nil); err != nil {
		t.Fatalf("registered failed on empty: %v", err)
	}

	if !strings.Contains(out.String(), "No agents registered") {
		t.Errorf("expected 'No agents registered' message, got:\n%s", out.String())
	}
}

func TestParseEnvFlags(t *testing.T) {
	cases := []struct {
		name    string
		in      []string
		want    map[string]string
		wantErr bool
	}{
		{"empty", nil, nil, false},
		{"single", []string{"KEY=VAL"}, map[string]string{"KEY": "VAL"}, false},
		{"multi", []string{"A=1", "B=2"}, map[string]string{"A": "1", "B": "2"}, false},
		{"value with =", []string{"X=a=b"}, map[string]string{"X": "a=b"}, false},
		{"missing =", []string{"BARE"}, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseEnvFlags(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("[%s] got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}
