package cli

import (
	"os"

	"hop.top/tlc/internal/core"
)

func buildOpts() core.BuildOpts {
	return core.BuildOpts{
		PromptFile:   agentRunPrompt,
		ContextFiles: agentRunContext,
		RepoRoot:     repoRoot(),
	}
}

func repoRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "/workspace"
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

func mergeEnvVars(cfgEnv map[string]string) map[string]string {
	merged := make(map[string]string, len(cfgEnv)+len(agentRunEnv))
	for k, v := range cfgEnv {
		merged[k] = v
	}
	for _, kv := range agentRunEnv {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				merged[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	return merged
}
