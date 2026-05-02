// Package cli: `tlc agent register|show|registered` subcommands.
//
// These commands manage the agent registry config file
// (~/.config/tlc/agents.yaml or .tlc/agents.yaml). Distinct from
// `tlc agent list`, which lists agent _runs_ (audit records).
//
// Author: jadb
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"hop.top/tlc/internal/core"
)

var (
	agentRegisterBinary  string
	agentRegisterImage   string
	agentRegisterTimeout time.Duration
	agentRegisterEnv     []string
	agentRegisterProject bool
)

// AgentRegisterCmd writes a new entry to ~/.config/tlc/agents.yaml
// (or .tlc/agents.yaml when --project is set).
var AgentRegisterCmd = &cobra.Command{
	Use:   "register <name>",
	Short: "Register an agent in the agents.yaml config",
	Long: `Register or update an entry in the agents.yaml config file.

By default writes to ~/.config/tlc/agents.yaml (global). Use --project
to write to .tlc/agents.yaml (project-local) instead.

Examples:
  tlc agent register claude --binary /usr/local/bin/claude
  tlc agent register codex --image ghcr.io/hop-top/pod-codex:v1
  tlc agent register gemini --binary /opt/gemini --env API_KEY=xyz`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if agentRegisterBinary == "" && agentRegisterImage == "" {
			return fmt.Errorf("agent register: --binary or --image is required")
		}

		path, err := agentsConfigPath(agentRegisterProject)
		if err != nil {
			return err
		}

		envMap, err := parseEnvFlags(agentRegisterEnv)
		if err != nil {
			return err
		}

		entry := &core.AgentConfig{
			Binary:         agentRegisterBinary,
			Image:          agentRegisterImage,
			Env:            envMap,
			DefaultTimeout: agentRegisterTimeout,
		}

		if err := writeAgentEntry(path, name, entry); err != nil {
			return fmt.Errorf("write agents.yaml: %w", err)
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"Registered agent %q in %s\n", name, path)
		return nil
	},
}

// AgentShowCmd prints the merged config for a single named agent.
var AgentShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show the registered config for an agent",
	Long: `Show the merged (global + project) config for an agent registered
in the agents.yaml config file.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		reg := core.NewAgentRegistry()
		if err := reg.LoadDefaults(); err != nil {
			return fmt.Errorf("load agents: %w", err)
		}
		// Auto-trust project config in show context: read-only inspection.
		reg.TrustProject(reg.ProjectConfigPath())

		cfg, err := reg.Get(name)
		if err != nil {
			return fmt.Errorf("agent %q not found: %w", name, err)
		}
		printAgentConfig(cmd.OutOrStdout(), name, cfg)
		return nil
	},
}

// AgentRegisteredCmd lists agent names registered in agents.yaml.
//
// Deprecated: this verb/adjective sibling pair (register / registered)
// violates the kit CLI conventions (§3.2). Use `tlc agent list --source
// config` instead. Hidden + deprecation warning kept for one release.
var AgentRegisteredCmd = &cobra.Command{
	Use:        "registered",
	Short:      "List agents registered in agents.yaml (deprecated)",
	Hidden:     true,
	Deprecated: "use 'tlc agent list --source config' instead",
	Long: `List all agents registered in ~/.config/tlc/agents.yaml (global)
and .tlc/agents.yaml (project-local). Shows merged view.

DEPRECATED: use 'tlc agent list --source config'. This alias will be
removed in a future release.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
			"warning: 'tlc agent registered' is deprecated; "+
				"use 'tlc agent list --source config' instead")
		return runAgentListConfig(cmd)
	},
}

// runAgentListConfig prints agents declared in agents.yaml (global +
// project-local merged). Shared by `agent list --source config` and the
// deprecated `agent registered` alias.
func runAgentListConfig(cmd *cobra.Command) error {
	reg := core.NewAgentRegistry()
	if err := reg.LoadDefaults(); err != nil {
		return fmt.Errorf("load agents: %w", err)
	}
	reg.TrustProject(reg.ProjectConfigPath())

	names := reg.List()
	out := cmd.OutOrStdout()
	if len(names) == 0 {
		_, _ = fmt.Fprintln(out, "No agents registered.")
		_, _ = fmt.Fprintf(out,
			"Add one with: tlc agent register <name> --binary <path>\n")
		_, _ = fmt.Fprintf(out,
			"Or edit %s directly.\n", agentsGlobalDefaultPath())
		return nil
	}

	_, _ = fmt.Fprintf(out, "Registered agents:\n")
	for _, n := range names {
		cfg, err := reg.Get(n)
		if err != nil {
			_, _ = fmt.Fprintf(out, "  %s  (error: %v)\n", n, err)
			continue
		}
		descriptor := cfg.Binary
		if descriptor == "" {
			descriptor = cfg.Image
		}
		_, _ = fmt.Fprintf(out, "  %s  %s\n", n, descriptor)
	}
	return nil
}

func init() {
	f := AgentRegisterCmd.Flags()
	f.StringVar(&agentRegisterBinary, "binary", "",
		"Absolute path to the agent binary (local exec)")
	f.StringVar(&agentRegisterImage, "image", "",
		"Container image reference (container exec)")
	f.DurationVar(&agentRegisterTimeout, "timeout", 0,
		"Default per-task timeout (e.g., 30m, 1h)")
	f.StringSliceVar(&agentRegisterEnv, "env", nil,
		"Environment variable as KEY=VALUE (repeatable)")
	f.BoolVar(&agentRegisterProject, "project", false,
		"Write to project-local .tlc/agents.yaml (default: global)")

	AgentCmd.AddCommand(AgentRegisterCmd)
	AgentCmd.AddCommand(AgentShowCmd)
	AgentCmd.AddCommand(AgentRegisteredCmd)
}

func agentsGlobalDefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "~/.config/tlc/agents.yaml"
	}
	return filepath.Join(home, ".config", "tlc", "agents.yaml")
}

func agentsConfigPath(projectScope bool) (string, error) {
	if projectScope {
		return filepath.Join(".tlc", "agents.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".config", "tlc", "agents.yaml"), nil
}

// agentConfigFileShape mirrors the on-disk YAML structure used by
// core.AgentRegistry. Kept in this package to avoid leaking write-side
// concerns into core.
type agentConfigFileShape struct {
	Agents map[string]*core.AgentConfig `yaml:"agents"`
}

func writeAgentEntry(path, name string, cfg *core.AgentConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	file := agentConfigFileShape{Agents: make(map[string]*core.AgentConfig)}
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, &file); err != nil {
			return fmt.Errorf("parse existing %s: %w", path, err)
		}
		if file.Agents == nil {
			file.Agents = make(map[string]*core.AgentConfig)
		}
	}

	file.Agents[name] = cfg

	out, err := yaml.Marshal(&file)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

func parseEnvFlags(raw []string) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(raw))
	for _, kv := range raw {
		for i, c := range kv {
			if c == '=' {
				out[kv[:i]] = kv[i+1:]
				goto next
			}
		}
		return nil, fmt.Errorf("invalid --env %q (want KEY=VALUE)", kv)
	next:
	}
	return out, nil
}

func printAgentConfig(out io.Writer, name string, cfg *core.AgentConfig) {
	_, _ = fmt.Fprintf(out, "agent: %s\n", name)
	if cfg.Binary != "" {
		_, _ = fmt.Fprintf(out, "  binary: %s\n", cfg.Binary)
	}
	if cfg.Image != "" {
		_, _ = fmt.Fprintf(out, "  image:  %s\n", cfg.Image)
	}
	if cfg.DefaultTimeout != 0 {
		_, _ = fmt.Fprintf(out, "  default_timeout: %s\n", cfg.DefaultTimeout)
	}
	if len(cfg.Env) > 0 {
		keys := make([]string, 0, len(cfg.Env))
		for k := range cfg.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		_, _ = fmt.Fprintln(out, "  env:")
		for _, k := range keys {
			_, _ = fmt.Fprintf(out, "    %s: %s\n", k, cfg.Env[k])
		}
	}
}
