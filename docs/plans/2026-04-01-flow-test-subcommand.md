# Flow Test Subcommand

**`tlc flow test`** — deterministic e2e testing for flow definitions.

## Task list

- [ ] `internal/flowtest/shim.go` — cassette I/O, fingerprint, env protocol, tool mode
- [ ] `internal/flowtest/recorder.go` — record mode: proxy + write cassettes
- [ ] `internal/flowtest/replayer.go` — replay mode: fingerprint → serve cassette
- [ ] `internal/flowtest/catchall.go` — catch-all shim: block unknown tools, hard-fail
- [ ] `internal/flowtest/sandbox.go` — tmp dir, git clone, shim extraction, teardown
- [ ] `internal/flowtest/run.go` — named run resolution: dir convention + manifest merge
- [ ] `internal/flowtest/shims/gh/main.go` — gh shim
- [ ] `internal/flowtest/shims/git/main.go` — git shim, delegates to real git on sandbox repo
- [ ] `internal/flowtest/shims/claude/main.go` — LLM shim
- [ ] `internal/flowtest/shims/codex/main.go` — LLM shim
- [ ] `internal/flowtest/hook.go` — post-step eva contract runner
- [ ] `internal/cli/flow_test_cmd.go` — cobra subcommand wired into `FlowCmd`
- [ ] `examples/flows/fixtures/pr-review-loop/happy-path/contracts/` — eva contracts

---

## Problem

`tlc flow run` executes flows but provides no way to test them. The only
existing coverage is schema parsing (`flow_test.go`) and unit tests on
`FlowExecutor`. There is no way to verify that a flow behaves correctly
end-to-end — that steps transition in the right order, that tool calls
produce the expected output, and that agent responses satisfy their
contracts — without running against real infrastructure.

## Goal

Add `tlc flow test <flow-file>` as a first-class subcommand. It must:

1. Run the flow end-to-end with full isolation — no side effects outside
   the test run.
2. Support **record** mode (run once against real infra, write cassettes)
   and **replay** mode (serve cassettes deterministically, no network).
3. Handle arbitrary CLI tools (`wrangler`, `docker`, custom binaries) —
   not just known shims — by blocking unknown tools unless a cassette
   exists or `--passthrough` is set.
4. Support multiple named test runs per flow (happy path, edge cases,
   regression cases) via a convention-first layout with optional manifests.
5. Assert each step's output against an eva contract when one exists.
6. Reuse `FlowExecutor` unchanged — test mode is a thin wrapper, not a
   fork.

---

## Design

### CLI surface

```
tlc flow test <flow-file> [run-name] [flags]

Flags:
  --record                    Run against real infra; write/update cassettes
  --passthrough <tool,...>    Force live execution for named tools; update
                              their cassettes in place (repeatable or
                              comma-separated)
  --keep-sandbox              Retain tmp dir after run (debugging)
  --steps <a,b>               Run only the named steps
```

Examples:

```bash
# replay all named runs
tlc flow test examples/flows/pr-review-loop.yaml

# replay one named run
tlc flow test examples/flows/pr-review-loop.yaml happy-path

# record a new run
tlc flow test examples/flows/pr-review-loop.yaml happy-path --record

# refresh wrangler + docker cassettes in place, replay everything else
tlc flow test examples/flows/pr-review-loop.yaml happy-path \
  --passthrough wrangler,docker

# same, repeated-flag form
tlc flow test examples/flows/pr-review-loop.yaml happy-path \
  --passthrough wrangler --passthrough docker
```

Exit codes:

| Code | Meaning |
|------|---------|
| 0 | All steps executed; all contracts passed |
| 1 | Step failed or contract violated |
| 2 | Cassette miss in replay mode (unknown tool, no cassette, not passthrough) |
| 3 | Sandbox setup failure |

### Named runs and cassette layout

Each flow has a `fixtures/<flow-name>/` dir. Each named run is a subdir.
The dir name is the run name by convention. A `test.yaml` manifest is
optional — use it only when the dir name is ambiguous or you need to
declare overrides.

```
examples/flows/
  pr-review-loop.yaml
  fixtures/
    pr-review-loop/

      happy-path/                  ← run name by convention; no manifest needed
        record/
          step-load-pr-context/
            gh-pr-view.<fp>.req.json
            gh-pr-view.<fp>.resp.json
            gh-pr-diff.<fp>.req.txt
            gh-pr-diff.<fp>.resp.patch
          step-reviewer-pass/
            claude.<fp>.req.json
            claude.<fp>.resp.json
          step-fixer-pass/
            claude.<fp>.req.json
            claude.<fp>.resp.json
        contracts/
          step-reviewer-pass.yaml
          step-check-approval.yaml

      max-iterations/              ← non-obvious exit code; manifest needed
        record/
          ...
        contracts/
          ...
        test.yaml
          name: max-iterations
          description: Reviewer never approves; hits max_iterations cap
          expected_exit: 1

      pr-123-regression/           ← non-conventional name; manifest maps it
        record/
          ...
        test.yaml
          name: regression-pr-123
          description: Reproduces the double-commit bug from PR #123
          passthrough: [docker]
```

**Manifest fields** (all optional):

| Field | Default | Purpose |
|-------|---------|---------|
| `name` | dir name | Human-readable run name |
| `description` | — | Why this run exists |
| `expected_exit` | 0 | Assert final exit code |
| `passthrough` | [] | Tools to execute live in this run |

CLI `--passthrough` takes precedence over manifest `passthrough`.

### Tool routing

Every CLI call a step makes is intercepted by the shim layer. The routing
decision for each tool:

```
tool called by step
  │
  ├─ is it git?
  │     → git shim: rewrite GIT_DIR/GIT_WORK_TREE → exec real git on
  │       sandbox repo (no cassette; always isolated)
  │
  ├─ is it in --passthrough (or manifest passthrough)?
  │     → exec real binary → write/update cassette in place
  │
  ├─ cassette exists for this tool + argv fingerprint?
  │     → serve cassette response; return recorded exit code
  │
  └─ no cassette, not passthrough, not git
        → catch-all shim: exit 2 (TLC_SHIM_MISS); halt flow immediately
```

The catch-all shim is a single binary installed as a wildcard via
`$PATH` — it intercepts any executable not already shimmed and applies
the routing logic above. This means `wrangler`, `docker`, `cargo`,
`just`, and any other tool a flow step calls are all covered without
naming them in advance.

### Sandbox

`FlowTestCmd` creates a hermetic sandbox before running any step:

```
tmp/
  tlc-flow-test-<uuid>/
    repo/      ← git clone --local <cwd>
    home/      ← fake $HOME
    bin/       ← extracted shim binaries + catch-all shim
    cassettes/ ← symlink to fixtures/<flow-name>/<run-name>/record/
```

Every step subprocess inherits:

```
HOME                       = .../home
GIT_DIR                    = .../repo/.git
GIT_WORK_TREE              = .../repo
GH_CONFIG_DIR              = .../home/.config/gh
TLC_FLOW_TEST_MODE         = record|replay
TLC_FLOW_TEST_STEP         = <step-id>
TLC_FLOW_TEST_CASSETTE_DIR = .../cassettes/
TLC_FLOW_TEST_PASSTHROUGH  = wrangler,docker   ← merged from flag + manifest
```

Nothing the step does can affect the caller's working tree, home
directory, or git state.

### Shims

Shim binaries are embedded in `tlc` via `go:embed`, extracted to
`sandbox/bin/` at test-start, and prepended to `$PATH`.

**`git` shim** — always isolated, never cassettted:

```
rewrite GIT_DIR/GIT_WORK_TREE to sandbox paths → exec real git
```

All git operations execute for real against the sandbox repo.

**`gh`, `claude`, `codex` shims** — named shims with cassette protocol:

```
record / passthrough: proxy to real binary → write/update cassette
replay:               fingerprint argv+stdin → serve cassette response
miss:                 exit 2; halt flow
```

**Catch-all shim** — covers every other binary:

```
check TLC_FLOW_TEST_PASSTHROUGH → passthrough path if listed
check cassette dir → serve if found
else → exit 2 (TLC_SHIM_MISS)
```

The catch-all is installed as `$PATH/tlc-shim-catchall` and symlinked
to every name not already claimed by a named shim. On macOS/Linux this
is done via a single wrapper script at sandbox setup time.

### Fingerprint

Each cassette filename encodes a short SHA256 of `argv + stdin`:

```
<tool>-<sha256[:8]>.{req,resp}.<ext>
```

Same argv + same stdin → same fingerprint → deterministic cassette
lookup across replay runs. Different invocations of the same tool in
the same step get distinct cassette files.

### Eva contract wiring

After each step, `FlowExecutor` runs a post-step hook (test mode only):

1. Step writes output to `tmp/.../step-<id>.output.json`.
2. Hook looks up `contracts/<step-id>.yaml` in the run dir.
3. If contract exists: `eva run --dataset <output> --contract <file>`.
4. Eva exit 0 → next step. Exit 1 → `FlowStatusFailed`; surface
   violated rule name and offending value.
5. No contract → continue silently.

Eva contracts are plain YAML, reusable across `flow test` and the
`-validated.yaml` gateway flows.

### End-to-end sequence

```
tlc flow test pr-review-loop.yaml happy-path --passthrough wrangler
  │
  ▼
FlowTestCmd
  resolve run: fixtures/pr-review-loop/happy-path/
  load manifest (if present); merge --passthrough flag
  create sandbox (tmp/tlc-flow-test-<uuid>/)
  git clone --local → sandbox/repo/
  extract shims + catch-all → sandbox/bin/
  inject env vars
  │
  ▼
FlowExecutor.Execute()          ← same code as `tlc flow run`
  for each step (dep order):
    inject TLC_FLOW_TEST_STEP=<step-id>
    run subprocess (shimmed PATH + sandbox env)
      git calls     → git shim     → sandbox/repo/ (always real, isolated)
      gh calls      → gh shim      → cassette or passthrough
      LLM calls     → llm shim     → cassette or passthrough
      wrangler call → catch-all    → passthrough (in TLC_FLOW_TEST_PASSTHROUGH)
      docker call   → catch-all    → cassette lookup → miss → exit 2
    step output → tmp/step-<id>.output.json
    post-step hook → eva contract (if exists)
    pass → next step
    fail → FlowStatusFailed
  │
  ▼
FlowTestCmd teardown
  print step table (pass/fail/skipped + tool mode used)
  assert expected_exit (from manifest) against actual exit
  rm -rf sandbox (unless --keep-sandbox)
  exit 0|1|2|3
```

### What stays unchanged

- `flow.yaml` — never modified
- `FlowExecutor` core logic — same code path as `tlc flow run`
- Eva contracts — same format as `-validated.yaml` gateway flows

### Net-new code

| Path | Purpose |
|------|---------|
| `internal/flowtest/shim.go` | Cassette I/O, fingerprint, env protocol, tool routing |
| `internal/flowtest/recorder.go` | Record/passthrough: proxy + write cassette |
| `internal/flowtest/replayer.go` | Replay: fingerprint → serve cassette |
| `internal/flowtest/catchall.go` | Catch-all shim: block unknown tools |
| `internal/flowtest/sandbox.go` | Tmp dir, clone, shim extract, teardown |
| `internal/flowtest/run.go` | Named run resolution: dir convention + manifest merge |
| `internal/flowtest/shims/gh/main.go` | gh shim |
| `internal/flowtest/shims/git/main.go` | git shim |
| `internal/flowtest/shims/claude/main.go` | LLM shim |
| `internal/flowtest/shims/codex/main.go` | LLM shim |
| `internal/flowtest/hook.go` | Post-step eva runner |
| `internal/cli/flow_test_cmd.go` | Cobra subcommand |
| `examples/flows/fixtures/*/happy-path/contracts/*.yaml` | Eva contracts |
