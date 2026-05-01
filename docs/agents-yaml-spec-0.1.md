# agents.yaml Spec 0.1

**Status**: paper
**Author**: jadb
**Task**: T-0198

## Purpose

`agents.yaml` is the agent registry consulted by `tlc flow run --agent
<name>` and `tlc agent run --agent <name>` to map a human-friendly
agent name to an executable binary or container image. Without this
file, `tlc flow run` cannot dispatch `task_template` or `exec` steps
(failure mode: "no agent runner registered"). The registry is the
single source of truth for agent dispatch in the local install.

## Locations

The registry is layered. Both files are optional; missing files are
silently skipped. When both exist, project-local overrides global on
scalar fields; env maps merge (project wins on key conflict).

| Scope | Path | Trust | Env expansion |
|-------|------|-------|---------------|
| Global | `~/.config/tlc/agents.yaml` | implicit | `${VAR}` expanded |
| Project-local | `<repo>/.tlc/agents.yaml` | requires `--trust-project` | literal (no expansion) |

Project-local is literal-only by design: this prevents secret
exfiltration via committed config (an agent's `env: {API_KEY:
"${REAL_KEY}"}` would otherwise leak the host's env into the
container).

## Schema

```yaml
agents:
  <agent-name>:           # arbitrary slug; used by --agent flag
    binary: /abs/path     # absolute path; required if image is empty
    image: ghcr.io/...    # container image ref; required if binary is empty
    env:                  # optional; KEY: VALUE (string only)
      KEY: VALUE
    default_timeout: 30m  # optional; Go duration; default per-task timeout
```

### Required fields

- Either `binary` OR `image` must be set. Both is allowed (project may
  override global to switch from container to local exec, etc).
- `<agent-name>` must be a stable slug. By convention: lowercase
  alphanum + hyphens.

### Optional fields

- `env` — string-keyed map of environment variables injected into
  agent process. Global values support `${VAR}` expansion at load
  time. Project-local values are literal.
- `default_timeout` — Go duration string (`30m`, `1h`, `90s`). Used
  as default per-task timeout. Per-invocation `--timeout` overrides.

## Example

Global config (`~/.config/tlc/agents.yaml`):

```yaml
agents:
  claude:
    binary: /usr/local/bin/claude
    image: ghcr.io/hop-top/pod-claude:latest
    env:
      ANTHROPIC_API_KEY: "${ANTHROPIC_API_KEY}"
    default_timeout: 30m

  codex:
    binary: /usr/local/bin/codex
    default_timeout: 1h
```

Project-local override (`./.tlc/agents.yaml`):

```yaml
agents:
  claude:
    image: ghcr.io/custom/claude-dev:nightly  # override image only
```

After load + trust, `tlc agent show claude` reports `image=...nightly`,
`binary=/usr/local/bin/claude`, env is merged.

## CLI surface

```
tlc agent register <name> --binary <path>   # write to ~/.config/tlc/agents.yaml
tlc agent register <name> --binary <path> --project   # write to .tlc/agents.yaml
tlc agent show <name>                       # print merged config
tlc agent registered                        # list all registered agents
```

`tlc agent list` (note: distinct surface) lists agent **execution
runs** (audit records), not registered agents.

## Trust model

When a project-local config exists, calling `Get(name)` returns
`ErrTrustRequired` until the path is trusted. Trust is per-process
(in-memory) — each `tlc` invocation prompts (or accepts
`--trust-project`). Global config is always trusted.

## Backward compat

- Missing both files → registry returns "agent not found" with the
  list of known agents (empty list). Flow run with `--agent` fails
  with actionable error pointing to `tlc agent register`.
- Missing `--agent` flag on `tlc flow run` → no runner registered →
  `task_template` steps fall back to DB-only ephemeral path; `exec`
  steps fail fast (per spec — exec requires a runner).

## Future (post-0.1)

- `capabilities: [...]` per agent for capability-based step dispatch.
- `agents.lock` for pinning image digests.
- Sourcing from external secret managers (1Password, Doppler) via
  loader hooks.

## References

- `internal/core/agent_registry.go` — loader + merge logic
- `internal/cli/agent_registry.go` — register/show/registered commands
- `internal/cli/agent_runner.go` — ContainerAgentRunner consumption
- task `hop-top/tlc#T-0198` — original wiring task
- task `hop-top/tlc#T-0113` — surfacing scenario that drove this
