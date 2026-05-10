package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"hop.top/kit/go/console/hay"
	"hop.top/tlc/internal/core"
)

// parseAgentExpr splits the composite --agent value into a bare agent
// name and an inline "key=value" override pair. The shorthand uses ':'
// to separate the name from a single override:
//
//	"claude"                            → ("claude", nil)
//	"claude:env.MODEL=opus"             → ("claude", ["env.MODEL=opus"])
//	"claude:image=ghcr.io/me/agent:dev" → ("claude", ["image=ghcr.io/me/agent:dev"])
//
// Only the FIRST ':' is treated as a separator so override values that
// contain colons (registry hostnames, image refs) survive intact. To
// layer multiple overrides, use the repeatable -A/--agent-config flag
// — `-A` overrides are applied after the inline one and win on
// duplicate keys.
//
// An empty input yields an empty name; callers should treat that as
// "not set" and fall through to the default-agent resolution chain.
func parseAgentExpr(expr string) (name string, inline []string) {
	if expr == "" {
		return "", nil
	}
	parts := strings.SplitN(expr, ":", 2)
	name = parts[0]
	if len(parts) == 1 {
		return name, nil
	}
	return name, []string{parts[1]}
}

// applyAgentOverrides mutates cfg with the union of inline (from --agent)
// and explicit (-A) overrides. Explicit overrides win on duplicate keys
// since they're applied last.
//
// Allowed keys (dotted):
//   - image
//   - binary
//   - default_timeout (parsed as time.Duration)
//   - env.<KEY>
//
// Unknown keys return an error naming the offending key.
func applyAgentOverrides(cfg *core.AgentConfig, inline, explicit []string) error {
	pairs := make([]string, 0, len(inline)+len(explicit))
	pairs = append(pairs, inline...)
	pairs = append(pairs, explicit...)
	for _, kv := range pairs {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			return fmt.Errorf("agent override %q: expected key=value", kv)
		}
		key := strings.TrimSpace(kv[:eq])
		val := kv[eq+1:]
		switch {
		case key == "image":
			cfg.Image = val
		case key == "binary":
			cfg.Binary = val
		case key == "default_timeout":
			if val == "" {
				cfg.DefaultTimeout = 0
				continue
			}
			d, err := time.ParseDuration(val)
			if err != nil {
				return fmt.Errorf("agent override default_timeout=%q: %w", val, err)
			}
			cfg.DefaultTimeout = d
		case strings.HasPrefix(key, "env."):
			envKey := strings.TrimPrefix(key, "env.")
			if envKey == "" {
				return fmt.Errorf("agent override %q: env.<KEY> requires a key", kv)
			}
			if cfg.Env == nil {
				cfg.Env = map[string]string{}
			}
			if val == "" {
				delete(cfg.Env, envKey)
			} else {
				cfg.Env[envKey] = val
			}
		default:
			return fmt.Errorf("agent override %q: unknown key (allowed: image, binary, default_timeout, env.<KEY>)", key)
		}
	}
	return nil
}

// resolveAgentName fuzzy-matches query against the registry's agent
// names. Returns the canonical name on a unique match. Mirrors the
// pattern used by chdir_project_resolver.go: ambiguity fails loudly with
// a candidate list rather than silently picking, matching tlc's
// non-interactive disambiguation convention.
func resolveAgentName(registry *core.AgentRegistry, query string) (string, error) {
	if query == "" {
		return "", errors.New("agent name is empty")
	}
	corpus := registry.List()
	if len(corpus) == 0 {
		return "", errors.New("no agents registered; run `tlc agent list` to verify config")
	}

	// Exact match short-circuits the fuzzy scorer so users typing the
	// canonical name never see ambiguity errors when the registry
	// happens to contain a similarly-spelled neighbor.
	for _, name := range corpus {
		if name == query {
			return name, nil
		}
	}

	res, err := hay.Resolve(query, corpus, hay.Options[string]{
		Score:     hay.StringScore(func(s string) string { return s }, hay.Combined),
		Policy:    hay.Policy{Action: hay.ActionList, Fail: true},
		TieMargin: 1,
	})
	if err != nil {
		return "", agentResolveError(query, err)
	}
	return res.Winner, nil
}

func agentResolveError(query string, err error) error {
	var amb *hay.ErrAmbiguous[string]
	if errors.As(err, &amb) {
		var b strings.Builder
		fmt.Fprintf(&b, "agent %q: ambiguous; matches %d candidates:\n", query, len(amb.Candidates))
		for _, c := range amb.Candidates {
			fmt.Fprintf(&b, "  %s\n", c.Item)
		}
		fmt.Fprintf(&b, "Disambiguate by typing more characters or the exact name.")
		return errors.New(b.String())
	}
	var nm *hay.ErrNoMatch
	if errors.As(err, &nm) {
		return fmt.Errorf("agent %q not found; run `tlc agent list` to see available agents", query)
	}
	return fmt.Errorf("agent %q: %w", query, err)
}
