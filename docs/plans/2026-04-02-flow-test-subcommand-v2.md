# Flow Test Subcommand v2 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this
> plan task-by-task.

**Goal:** Add `tlc flow test <flow-file> [run-name]` — deterministic e2e testing for
flow definitions — backed by `hop.top/xrr` for all cassette I/O, fingerprinting, and
adapter protocol.

**Architecture:** `FlowTestCmd` wraps `FlowExecutor` unchanged. A hermetic sandbox
(tmp dir + git clone + shimmed PATH) intercepts every subprocess call. Each shim binary
is a thin `xrr` client: record mode proxies to the real binary and writes a cassette;
replay mode serves the cassette. The catch-all shim handles any unrecognised binary.
Eva contracts run as a post-step hook after each step.

**Tech Stack:** Go 1.24 · `hop.top/xrr` (exec adapter) · `go:embed` for shim binaries
· Cobra · eva CLI

---

## What changed from v1

`hop.top/xrr` already ships:
- `xrr.FileCassette` — cassette read/write with YAML envelope
- `xrr.FileSession` — record / replay / passthrough dispatch
- `xrr.ErrCassetteMiss` — canonical miss sentinel
- `adapters/exec` — argv+stdin fingerprint, Request/Response types

**Deleted from scope** (xrr owns these now):
- ~~`internal/flowtest/shim.go`~~ — cassette I/O + fingerprint → `xrr.FileCassette` +
  `adapters/exec`
- ~~`internal/flowtest/recorder.go`~~ → `xrr.ModeRecord` in each shim binary
- ~~`internal/flowtest/replayer.go`~~ → `xrr.ModeReplay` in each shim binary
- ~~`internal/flowtest/catchall.go`~~ (partially) → xrr session miss path

**Remaining net-new code** (all in `internal/flowtest/`):

| File | Purpose |
|------|---------|
| `sandbox.go` | Tmp dir, git clone, shim extraction, env injection, teardown |
| `run.go` | Named run resolution: dir convention + manifest merge |
| `hook.go` | Post-step eva contract runner |
| `shims/gh/main.go` | gh — thin xrr exec client |
| `shims/git/main.go` | git — rewrites GIT_DIR/WORK_TREE; no cassette |
| `shims/claude/main.go` | LLM shims (one dir each): claude, codex, gemini, |
| `shims/copilot/main.go` | copilot, opencode, fabric, llm |
| `shims/npm/main.go` | Package manager shims: npm, npx, uv, pip, composer |
| `shims/docker/main.go` | Container shims: docker, docker-compose |
| `shims/catchall/main.go` | wildcard — xrr miss → exit 2 |
| `../cli/flow_test_cmd.go` | Cobra subcommand wired into FlowCmd |
| `examples/flows/fixtures/*/contracts/*.yaml` | Eva contracts |

### Tool routing summary

| Category | Tools | Treatment |
|----------|-------|-----------|
| Git | `git` | Real exec, sandbox GIT_DIR rewrite, no cassette |
| VCS host | `gh` | Named shim, cassette-backed |
| LLM CLIs | `claude` `codex` `gemini` `copilot` `opencode` `fabric` `llm` | Named shim, cassette-backed |
| Node pkg | `npm` `npx` | Named shim, cassette-backed |
| Python pkg | `uv` `pip` | Named shim, cassette-backed |
| PHP pkg | `composer` | Named shim, cassette-backed |
| Containers | `docker` `docker-compose` | Named shim, cassette-backed |
| Local runtimes | `node` `vite` `next` | Passthrough-always (no cassette) |
| POSIX utils | `ls` `cat` `head` `tail` `wc` `grep` `pwd` `cp` `mkdir` `chmod` `ps` `lsof` `sleep` | Passthrough-always (no cassette) |
| Everything else | `wrangler` `cargo` `just` … | Catch-all → xrr miss → exit 2 |

---

## CLI surface

```
tlc flow test <flow-file> [run-name] [flags]

  --record                 run against real infra; write/update cassettes
  --passthrough <tool,...> force live execution; repeatable or comma-sep
  --keep-sandbox           retain tmp dir after run (debugging)
  --steps <a,b>            run only named steps
```

Exit codes: `0` pass · `1` step/contract fail · `2` cassette miss · `3` sandbox fail

---

## Cassette layout

```
examples/flows/
  pr-review-loop.yaml
  fixtures/
    pr-review-loop/
      happy-path/
        record/
          step-load-pr-context/
            exec-<fp>.req.yaml          ← xrr FileCassette format
            exec-<fp>.resp.yaml
          step-reviewer-pass/
            exec-<fp>.req.yaml
            exec-<fp>.resp.yaml
        contracts/
          step-reviewer-pass.yaml
          step-check-approval.yaml
      max-iterations/
        record/  ...
        test.yaml
```

The `record/` subdir is passed directly to `xrr.NewFileCassette(dir)`. No custom
file format; cassettes are readable by any xrr language port.

---

## Env protocol (injected per step subprocess)

```
HOME                       = .../home
GIT_DIR                    = .../repo/.git
GIT_WORK_TREE              = .../repo
GH_CONFIG_DIR              = .../home/.config/gh
TLC_FLOW_TEST_MODE         = record|replay
TLC_FLOW_TEST_STEP         = <step-id>
TLC_FLOW_TEST_CASSETTE_DIR = .../record/<step-id>/
TLC_FLOW_TEST_PASSTHROUGH  = wrangler,docker
```

Each shim binary reads these env vars; no flags needed.

---

## Shim binary pattern (gh / claude / codex)

Every named shim follows this template:

```go
func main() {
    mode  := xrr.Mode(os.Getenv("TLC_FLOW_TEST_MODE"))
    dir   := os.Getenv("TLC_FLOW_TEST_CASSETTE_DIR")
    pass  := strings.Split(os.Getenv("TLC_FLOW_TEST_PASSTHROUGH"), ",")

    // override mode to passthrough if this binary is in the passthrough list
    self  := filepath.Base(os.Args[0])
    if slices.Contains(pass, self) {
        mode = xrr.ModePassthrough
    }

    c := xrr.NewFileCassette(dir)
    s := xrr.NewSession(mode, c)

    req := &execadapter.Request{Argv: os.Args[1:], Stdin: readStdin()}
    resp, err := s.Record(context.Background(), execadapter.NewAdapter(), req,
        func() (xrr.Response, error) {
            return runReal(os.Args[0], os.Args[1:]) // exec real binary
        })
    if errors.Is(err, xrr.ErrCassetteMiss) {
        fmt.Fprintf(os.Stderr, "xrr: cassette miss: %v\n", os.Args)
        os.Exit(2)
    }
    // write captured stdout/stderr, exit with recorded exit code
    emit(resp)
}
```

---

## Task 1: Add `hop.top/xrr` dependency

**Files:**
- Modify: `go.mod`, `go.sum`

**Step 1: Add dependency**

```bash
cd /path/to/tlc/hops/main
go get hop.top/xrr@latest
```

**Step 2: Verify**

```bash
go build ./...
# Expected: clean
```

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "build: add hop.top/xrr dependency"
```

---

## Task 2: `internal/flowtest/run.go` — named run resolution

**Files:**
- Create: `internal/flowtest/run.go`
- Create: `internal/flowtest/run_test.go`

**Step 1: Write failing tests**

```go
func TestRunResolutionByDirName(t *testing.T) {
    // fixtures/pr-review-loop/happy-path/ exists, no manifest
    dir := setupFixtureDir(t, "pr-review-loop", "happy-path", nil)
    runs, err := flowtest.DiscoverRuns("pr-review-loop", dir)
    require.NoError(t, err)
    require.Len(t, runs, 1)
    assert.Equal(t, "happy-path", runs[0].Name)
    assert.Equal(t, 0, runs[0].ExpectedExit)
    assert.Empty(t, runs[0].Passthrough)
}

func TestRunResolutionWithManifest(t *testing.T) {
    manifest := `
expected_exit: 1
passthrough: [docker]
description: hits max_iterations cap
`
    dir := setupFixtureDir(t, "pr-review-loop", "max-iterations",
        []byte(manifest))
    runs, err := flowtest.DiscoverRuns("pr-review-loop", dir)
    require.NoError(t, err)
    assert.Equal(t, 1, runs[0].ExpectedExit)
    assert.Equal(t, []string{"docker"}, runs[0].Passthrough)
}

func TestRunResolutionSingleNamed(t *testing.T) {
    dir := setupFixtureDir(t, "pr-review-loop", "happy-path", nil)
    setupFixtureDir(t, "pr-review-loop", "edge-case", nil)  // should be ignored
    runs, err := flowtest.DiscoverRuns("pr-review-loop", dir,
        flowtest.WithRunName("happy-path"))
    require.NoError(t, err)
    require.Len(t, runs, 1)
    assert.Equal(t, "happy-path", runs[0].Name)
}
```

**Step 2: Run to verify FAIL**

```bash
go test ./internal/flowtest/... 2>&1 | head -20
# Expected: package not found
```

**Step 3: Implement**

```go
package flowtest

type RunManifest struct {
    Name         string   `yaml:"name"`
    Description  string   `yaml:"description,omitempty"`
    ExpectedExit int      `yaml:"expected_exit"`
    Passthrough  []string `yaml:"passthrough,omitempty"`
}

type Run struct {
    Name         string
    Dir          string   // path to run dir (contains record/ and contracts/)
    RecordDir    string   // path to record/ subdir → passed to xrr.NewFileCassette
    ContractsDir string   // path to contracts/ subdir
    ExpectedExit int
    Passthrough  []string
}

// DiscoverRuns finds all runs for flowName under baseDir. If WithRunName
// option is set, returns only that run.
func DiscoverRuns(flowName, baseDir string, opts ...Option) ([]Run, error)
```

**Step 4: Run to verify PASS**

```bash
go test ./internal/flowtest/... -run TestRun -v
```

**Step 5: Commit**

```bash
git commit -m "feat(flowtest): run resolution with dir convention + manifest"
```

---

## Task 3: `internal/flowtest/sandbox.go` — hermetic sandbox

**Files:**
- Create: `internal/flowtest/sandbox.go`
- Create: `internal/flowtest/sandbox_test.go`

**Step 1: Write failing tests**

```go
func TestSandboxCreate(t *testing.T) {
    sb, err := flowtest.NewSandbox(t.TempDir())
    require.NoError(t, err)
    defer sb.Teardown()
    // repo/ must be a valid git repo
    _, err = os.Stat(filepath.Join(sb.RepoDir, ".git"))
    assert.NoError(t, err)
    // bin/ must exist
    _, err = os.Stat(sb.BinDir)
    assert.NoError(t, err)
    // home/ must exist
    _, err = os.Stat(sb.HomeDir)
    assert.NoError(t, err)
}

func TestSandboxEnv(t *testing.T) {
    sb, _ := flowtest.NewSandbox(t.TempDir())
    defer sb.Teardown()
    run := &flowtest.Run{
        RecordDir:   t.TempDir(),
        Passthrough: []string{"docker"},
    }
    env := sb.Env("step-foo", xrr.ModeReplay, run)
    assert.Equal(t, sb.HomeDir, envGet(env, "HOME"))
    assert.Equal(t, sb.RepoDir+"/.git", envGet(env, "GIT_DIR"))
    assert.Equal(t, "replay", envGet(env, "TLC_FLOW_TEST_MODE"))
    assert.Equal(t, "step-foo", envGet(env, "TLC_FLOW_TEST_STEP"))
    assert.Equal(t, run.RecordDir, envGet(env, "TLC_FLOW_TEST_CASSETTE_DIR"))
    assert.Equal(t, "docker", envGet(env, "TLC_FLOW_TEST_PASSTHROUGH"))
}

func TestSandboxKeep(t *testing.T) {
    sb, _ := flowtest.NewSandbox(t.TempDir())
    sb.Keep = true
    dir := sb.RootDir
    sb.Teardown()
    _, err := os.Stat(dir)
    assert.NoError(t, err) // must still exist
}
```

**Step 2: Run to verify FAIL**

```bash
go test ./internal/flowtest/... -run TestSandbox 2>&1 | head -20
```

**Step 3: Implement**

```go
type Sandbox struct {
    RootDir string
    RepoDir string
    BinDir  string
    HomeDir string
    Keep    bool
}

// NewSandbox creates tmp/tlc-flow-test-<uuid>/{repo,bin,home} and
// git clone --local <cwd> into repo/.
func NewSandbox(cwd string) (*Sandbox, error)

// Env returns the os.Environ()-style slice to inject into each step subprocess.
func (s *Sandbox) Env(stepID string, mode xrr.Mode, run *Run) []string

// ExtractShims copies embedded shim binaries into BinDir and creates
// catch-all symlinks for any name not already present.
func (s *Sandbox) ExtractShims() error

// Teardown removes the sandbox root unless Keep is set.
func (s *Sandbox) Teardown()
```

Shims are embedded via:

```go
//go:embed shims/bin/*
var shimsFS embed.FS
```

(Shim binaries compiled in Tasks 4–8; embedded in Task 9.)

**Step 4: Run to verify PASS**

```bash
go test ./internal/flowtest/... -run TestSandbox -v
```

**Step 5: Commit**

```bash
git commit -m "feat(flowtest): hermetic sandbox with git clone + env injection"
```

---

## Task 4: `shims/git/main.go` — git shim

**Files:**
- Create: `internal/flowtest/shims/git/main.go`

**Step 1: Implement**

No cassette. Rewrites env and delegates to real git:

```go
func main() {
    sb := os.Getenv("GIT_WORK_TREE") // set by sandbox
    real, err := exec.LookPath("git")
    // ... error handling
    cmd := exec.Command(real, os.Args[1:]...)
    cmd.Env = append(os.Environ(),
        "GIT_DIR="+sb+"/.git",
        "GIT_WORK_TREE="+sb,
    )
    cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
    cmd.Run()
    os.Exit(cmd.ProcessState.ExitCode())
}
```

**Step 2: Build + smoke test**

```bash
go build -o /tmp/test-git-shim ./internal/flowtest/shims/git/
GIT_WORK_TREE=/tmp /tmp/test-git-shim --version
# Expected: git version x.y.z
```

**Step 3: Commit**

```bash
git commit -m "feat(flowtest/shims): git shim — rewrites GIT_DIR to sandbox"
```

---

## Task 5: `shims/gh/main.go` — gh shim

**Files:**
- Create: `internal/flowtest/shims/gh/main.go`

**Step 1: Implement** using the shim template from the design section above.
`runReal` execs the real `gh` binary; `emit` writes stdout/stderr and exits with
the recorded exit code.

**Step 2: Build + smoke test (passthrough mode)**

```bash
go build -o /tmp/test-gh-shim ./internal/flowtest/shims/gh/
TLC_FLOW_TEST_MODE=passthrough \
TLC_FLOW_TEST_CASSETTE_DIR=/tmp/cassettes \
/tmp/test-gh-shim --version
# Expected: gh version x.y.z (passthrough)
```

**Step 3: Test replay miss → exit 2**

```bash
TLC_FLOW_TEST_MODE=replay \
TLC_FLOW_TEST_CASSETTE_DIR=/tmp/empty-dir \
/tmp/test-gh-shim pr view 1
echo "exit: $?"
# Expected: exit: 2
```

**Step 4: Commit**

```bash
git commit -m "feat(flowtest/shims): gh shim"
```

---

## Task 6: LLM shims — claude, codex, gemini, copilot, opencode

Same pattern as Task 5. All five are thin wrappers around `runReal` + xrr session.

Binaries: `claude`, `codex`, `gemini`, `copilot` (note: `gh copilot` is a gh
extension; shim intercepts the `copilot` argv prefix), `opencode`,
`fabric` (danielmiessler/fabric — Go), `llm` (simonw/llm — Python).

**Step 1: Implement all** (copy gh shim template; only `self` name differs)

**Step 2: Build all**

```bash
go build -o /tmp/test-claude-shim   ./internal/flowtest/shims/claude/
go build -o /tmp/test-codex-shim    ./internal/flowtest/shims/codex/
go build -o /tmp/test-gemini-shim   ./internal/flowtest/shims/gemini/
go build -o /tmp/test-copilot-shim  ./internal/flowtest/shims/copilot/
go build -o /tmp/test-opencode-shim ./internal/flowtest/shims/opencode/
go build -o /tmp/test-fabric-shim   ./internal/flowtest/shims/fabric/
go build -o /tmp/test-llm-shim      ./internal/flowtest/shims/llm/
```

**Step 3: Commit**

```bash
git commit -m "feat(flowtest/shims): LLM shims (claude, codex, gemini, copilot, opencode, fabric, llm)"
```

---

## Task 7: `shims/catchall/main.go` — wildcard shim

**Files:**
- Create: `internal/flowtest/shims/catchall/main.go`

**Step 1: Implement**

```go
func main() {
    mode := xrr.Mode(os.Getenv("TLC_FLOW_TEST_MODE"))
    dir  := os.Getenv("TLC_FLOW_TEST_CASSETTE_DIR")
    pass := strings.Split(os.Getenv("TLC_FLOW_TEST_PASSTHROUGH"), ",")
    self := filepath.Base(os.Args[0]) // the name it was invoked as

    if slices.Contains(pass, self) {
        mode = xrr.ModePassthrough
    }

    c := xrr.NewFileCassette(dir)
    s := xrr.NewSession(mode, c)

    req := &execadapter.Request{Argv: os.Args, Stdin: readStdin()}
    resp, err := s.Record(context.Background(), execadapter.NewAdapter(), req,
        func() (xrr.Response, error) { return runReal(self, os.Args[1:]) })

    if errors.Is(err, xrr.ErrCassetteMiss) {
        fmt.Fprintf(os.Stderr,
            "tlc-flow-test: cassette miss for %q; re-run with --record\n", self)
        os.Exit(2)
    }
    emit(resp)
}
```

**Step 2: Build + test miss path**

```bash
go build -o /tmp/tlc-shim-catchall ./internal/flowtest/shims/catchall/
TLC_FLOW_TEST_MODE=replay \
TLC_FLOW_TEST_CASSETTE_DIR=/tmp/empty \
/tmp/tlc-shim-catchall wrangler deploy
echo "exit: $?"   # Expected: 2
```

**Step 3: Commit**

```bash
git commit -m "feat(flowtest/shims): catch-all shim using xrr miss path"
```

---

## Task 8: `internal/flowtest/hook.go` — eva contract runner

**Files:**
- Create: `internal/flowtest/hook.go`
- Create: `internal/flowtest/hook_test.go`

**Step 1: Write failing tests**

```go
func TestHookNoContract(t *testing.T) {
    h := flowtest.NewHook(t.TempDir()) // empty contracts dir
    err := h.Run("step-foo", map[string]any{"result": "ok"})
    assert.NoError(t, err) // no contract → pass silently
}

func TestHookContractPass(t *testing.T) {
    dir := t.TempDir()
    writeContract(t, dir, "step-foo.yaml", `rules: []`) // trivial pass
    h := flowtest.NewHook(dir)
    err := h.Run("step-foo", map[string]any{"result": "ok"})
    assert.NoError(t, err)
}

func TestHookContractFail(t *testing.T) {
    // requires eva binary in PATH; skip if absent
    if _, err := exec.LookPath("eva"); err != nil {
        t.Skip("eva not in PATH")
    }
    dir := t.TempDir()
    writeContract(t, dir, "step-foo.yaml", `
rules:
  - name: must_have_field
    expr: "output.missing_field != null"
`)
    h := flowtest.NewHook(dir)
    err := h.Run("step-foo", map[string]any{"result": "ok"})
    var ce *flowtest.ContractError
    require.ErrorAs(t, err, &ce)
    assert.Equal(t, "step-foo", ce.StepID)
}
```

**Step 2: Run to verify FAIL**

```bash
go test ./internal/flowtest/... -run TestHook 2>&1 | head -20
```

**Step 3: Implement**

```go
type Hook struct { contractsDir string }
func NewHook(contractsDir string) *Hook

type ContractError struct {
    StepID  string
    Rule    string
    Message string
}
func (e *ContractError) Error() string

// Run looks for <contractsDir>/<stepID>.yaml; if present, writes output to a
// tmp file and execs: eva run --dataset <tmp> --contract <contract>
// Eva exit 0 → nil. Exit 1 → ContractError. Missing contract → nil.
func (h *Hook) Run(stepID string, output map[string]any) error
```

**Step 4: Run to verify PASS**

```bash
go test ./internal/flowtest/... -run TestHook -v
```

**Step 5: Commit**

```bash
git commit -m "feat(flowtest): post-step eva contract hook"
```

---

## Task 9: Embed shim binaries + wire sandbox extraction

**Files:**
- Modify: `internal/flowtest/sandbox.go`
- Create: `internal/flowtest/embed.go`

**Step 1: Build shims to embedded path**

Add a `//go:generate` or `Makefile` target that compiles each shim for the host OS/arch
into `internal/flowtest/shims/bin/`:

```bash
GOOS=darwin GOARCH=arm64 go build \
  -o internal/flowtest/shims/bin/gh \
  ./internal/flowtest/shims/gh/
# repeat for git, claude, codex, tlc-shim-catchall
```

**Step 2: Add embed directive**

```go
// embed.go
package flowtest

import "embed"

//go:embed shims/bin/*
var shimsFS embed.FS
```

**Step 3: Implement `ExtractShims`**

Extract each binary from `shimsFS` into `sandbox.BinDir`, `chmod +x`.
Create catch-all symlinks for common tools not already in BinDir:
`wrangler`, `docker`, `cargo`, `just`, `npx` → `tlc-shim-catchall`.

**Step 4: Test extraction**

```go
func TestSandboxExtractShims(t *testing.T) {
    sb, _ := flowtest.NewSandbox(t.TempDir())
    defer sb.Teardown()
    require.NoError(t, sb.ExtractShims())
    for _, name := range []string{"gh", "git", "claude", "codex",
        "tlc-shim-catchall", "wrangler", "docker"} {
        path := filepath.Join(sb.BinDir, name)
        info, err := os.Lstat(path)
        require.NoError(t, err, "missing: %s", name)
        // must be executable or symlink
        assert.True(t, info.Mode()&0o111 != 0 || info.Mode()&os.ModeSymlink != 0)
    }
}
```

**Step 5: Commit**

```bash
git commit -m "feat(flowtest): embed + extract shim binaries into sandbox"
```

---

## Task 10: `internal/cli/flow_test_cmd.go` — Cobra subcommand

**Files:**
- Create: `internal/cli/flow_test_cmd.go`
- Create: `internal/cli/flow_test_cmd_test.go`

**Step 1: Write e2e test (uses real sandbox, committed fixture cassettes)**

```go
func TestFlowTestReplay(t *testing.T) {
    // requires examples/flows/fixtures/pr-review-loop/happy-path/record/ to be
    // populated (committed cassettes)
    cmd := newTestCmd(t)
    err := cmd.Execute([]string{
        "flow", "test",
        "examples/flows/pr-review-loop.yaml",
        "happy-path",
    })
    assert.NoError(t, err)
}

func TestFlowTestCassetteMiss(t *testing.T) {
    cmd := newTestCmd(t)
    err := cmd.Execute([]string{
        "flow", "test",
        "examples/flows/pr-review-loop.yaml",
        "happy-path",
    })
    // cassettes not present → exit 2
    var exitErr *ExitCodeError
    require.ErrorAs(t, err, &exitErr)
    assert.Equal(t, 2, exitErr.Code)
}
```

**Step 2: Run to verify FAIL**

```bash
go test ./internal/cli/... -run TestFlowTest 2>&1 | head -20
```

**Step 3: Implement**

```go
func NewFlowTestCmd() *cobra.Command {
    var record      bool
    var passthrough []string
    var keepSandbox bool
    var steps       []string

    cmd := &cobra.Command{
        Use:   "test <flow-file> [run-name]",
        Short: "Run deterministic e2e tests against a flow definition",
        Args:  cobra.RangeArgs(1, 2),
        RunE:  runFlowTest,
    }
    cmd.Flags().BoolVar(&record, "record", false, "...")
    cmd.Flags().StringArrayVar(&passthrough, "passthrough", nil, "...")
    cmd.Flags().BoolVar(&keepSandbox, "keep-sandbox", false, "...")
    cmd.Flags().StringSliceVar(&steps, "steps", nil, "...")
    return cmd
}
```

`runFlowTest`:
1. `flowtest.DiscoverRuns(flowName, fixturesDir, opts...)`
2. For each run: `flowtest.NewSandbox(cwd)` → `sb.ExtractShims()` →
   `FlowExecutor.Execute()` with shimmed env → post-step `hook.Run()` →
   `sb.Teardown()`
3. Assert `run.ExpectedExit` against actual exit
4. Print step table (pass/fail/skipped + mode used)
5. Return appropriate exit code

Wire into `FlowCmd` as `test` subcommand.

**Step 4: Run to verify PASS**

```bash
go test ./internal/cli/... -run TestFlowTest -v
```

**Step 5: Commit**

```bash
git commit -m "feat(cli): flow test subcommand"
```

---

## Task 11: Eva contracts for pr-review-loop happy-path

**Files:**
- Create: `examples/flows/fixtures/pr-review-loop/happy-path/contracts/step-reviewer-pass.yaml`
- Create: `examples/flows/fixtures/pr-review-loop/happy-path/contracts/step-check-approval.yaml`

**Step 1: Inspect pr-review-loop.yaml step outputs**

```bash
cat examples/flows/pr-review-loop.yaml
```

Identify what each step writes to its output JSON and what properties should be
asserted (e.g. `approved == true`, `review_comment != ""`).

**Step 2: Write contracts** using eva YAML contract format (same as `-validated.yaml`
gateway flows in `examples/flows/`).

**Step 3: Run against a live record (one-time)**

```bash
tlc flow test examples/flows/pr-review-loop.yaml happy-path --record
```

**Step 4: Run in replay to confirm contracts pass**

```bash
tlc flow test examples/flows/pr-review-loop.yaml happy-path
```

**Step 5: Commit**

```bash
git commit -m "test(flowtest): eva contracts + cassettes for pr-review-loop happy-path"
```

---

## Task 12: Update AGENTS.md doc validation script

**Files:**
- Modify: `scripts/validate-doc-commands.sh`
- Modify: `AGENTS.md` (add `flow test` subcommand docs)

**Step 1: Add `flow test` to AGENTS.md CLI surface table**

**Step 2: Add check to validation script**

```bash
tlc flow test --help | grep -q "run-name" || fail "flow test missing run-name arg"
```

**Step 3: Run validation**

```bash
make validate-docs
```

**Step 4: Commit**

```bash
git commit -m "docs: add flow test subcommand to AGENTS.md + validation"
```

---

## Sequencing

```
Task 1  (add xrr dep)
  ├── Task 2 (run.go)
  ├── Task 3 (sandbox.go — no shims yet)
  ├── Task 4 (git shim)
  ├── Task 5 (gh shim)
  ├── Task 6 (claude + codex shims)
  └── Task 7 (catchall shim)
        └── Task 9 (embed + extract) ← needs all shims compiled
              ├── Task 8 (hook.go) ← independent; can run earlier
              └── Task 10 (flow_test_cmd.go) ← needs Task 2+3+8+9
                    └── Task 11 (eva contracts + cassettes)
                          └── Task 12 (docs + validation)
```

Tasks 2–7 are independent of each other after Task 1 and can run in parallel.
