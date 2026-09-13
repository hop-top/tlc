package cli

import (
	"os"
	"strings"

	"hop.top/tlc/internal/core"
)

// buildOpts assembles context-builder options from the `agent run`
// prompt/context flags for the given repo root.
func buildOpts(repoRoot string) core.BuildOpts {
	return core.BuildOpts{
		PromptFile:   agentRunPrompt,
		ContextFiles: agentRunContext,
		RepoRoot:     repoRoot,
	}
}

// repoRootForMode returns the working directory for agent execution: the
// host CWD in local mode, /workspace (where the repo is mounted) in a
// container.
func repoRootForMode(local bool) string {
	if !local {
		return "/workspace"
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func parseMounts(specs []string) []core.MountSpec {
	mounts := make([]core.MountSpec, 0, len(specs))
	for _, spec := range specs {
		m := core.MountSpec{}
		parts := splitMount(spec)
		if len(parts) >= 2 {
			m.Source = parts[0]
			m.Target = parts[1]
		}
		if len(parts) >= 3 {
			m.Mode = parts[2]
		}
		if m.Source != "" && m.Target != "" {
			mounts = append(mounts, m)
		}
	}
	return mounts
}

func splitMount(s string) []string {
	var parts []string
	var current []byte
	for i := 0; i < len(s); i++ {
		if s[i] == ':' && len(parts) < 2 {
			parts = append(parts, string(current))
			current = nil
		} else {
			current = append(current, s[i])
		}
	}
	parts = append(parts, string(current))
	return parts
}

// mergeEnvVars overlays KEY=VALUE pairs on a copy of the agent config
// env; the overlay wins on duplicate keys and entries without '=' are
// dropped.
func mergeEnvVars(cfgEnv map[string]string, overlay []string) map[string]string {
	merged := make(map[string]string, len(cfgEnv)+len(overlay))
	for k, v := range cfgEnv {
		merged[k] = v
	}
	for _, kv := range overlay {
		if k, v, ok := strings.Cut(kv, "="); ok {
			merged[k] = v
		}
	}
	return merged
}
