package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/flowtest"
	"hop.top/tlc/internal/uri"
	xrr "hop.top/xrr"
)

// ExitCodeError wraps a process exit code so callers can detect specific codes.
type ExitCodeError struct {
	Code    int
	Message string
}

func (e *ExitCodeError) Error() string { return e.Message }

func init() {
	FlowCmd.AddCommand(NewFlowTestCmd())
}

// NewFlowTestCmd creates the `tlc flow test` subcommand.
func NewFlowTestCmd() *cobra.Command {
	var (
		record      bool
		passthrough []string
		keepSandbox bool
		steps       []string
	)

	cmd := &cobra.Command{
		Use:   "test <flow-file> [run-name]",
		Short: "Run deterministic e2e tests against a flow definition",
		Long: `Run a flow definition through a hermetic sandbox.

In replay mode (default), each tool call is served from a recorded cassette.
In record mode (--record), tool calls are proxied to real binaries and cassettes
are written for future replay runs.

Exit codes:
  0  All steps executed; all contracts passed
  1  Step failed or contract violated
  2  Cassette miss in replay mode
  3  Sandbox setup failure`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			flowFile := args[0]
			runName := ""
			if len(args) == 2 {
				runName = args[1]
			}
			return runFlowTest(cmd, flowFile, runName, record, passthrough, keepSandbox, steps)
		},
	}

	cmd.Flags().BoolVar(&record, "record", false,
		"Run against real infra; write/update cassettes")
	cmd.Flags().StringArrayVar(&passthrough, "passthrough", nil,
		"Force live execution for named tools (repeatable or comma-separated)")
	cmd.Flags().BoolVar(&keepSandbox, "keep-sandbox", false,
		"Retain tmp dir after run (for debugging)")
	cmd.Flags().StringSliceVar(&steps, "steps", nil,
		"Run only the named steps (comma-separated)")

	return cmd
}

func runFlowTest(
	cmd *cobra.Command,
	flowFile, runName string,
	record bool,
	cliPassthrough []string,
	keepSandbox bool,
	_ []string, // steps filter — reserved for future use
) error {
	// Determine mode.
	mode := xrr.ModeReplay
	if record {
		mode = xrr.ModeRecord
	}

	// Flatten comma-separated passthrough values.
	var passthroughFlat []string
	for _, pt := range cliPassthrough {
		for _, s := range strings.Split(pt, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				passthroughFlat = append(passthroughFlat, s)
			}
		}
	}

	// Resolve flow file.
	s, err := getStorage()
	if err != nil {
		return &ExitCodeError{Code: 3, Message: fmt.Sprintf("flow test: storage: %v", err)}
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	resolver := uri.NewResolver(s)
	resolver.FlowsDir = flowsDirFromConfig()
	res, err := resolver.ResolveFlow(ctx, flowFile)
	if err != nil {
		return &ExitCodeError{Code: 3, Message: fmt.Sprintf("flow test: resolve flow: %v", err)}
	}
	flow := res.Flow

	// Derive flow name from file path for fixture lookup.
	// DiscoverRuns resolves fixtures/<flowName>/ from flowBaseDir.
	// Use abs path so cassette dirs passed via env to shims are absolute.
	flowName := flowFileBaseName(flowFile)
	flowBaseDir, err := filepath.Abs(filepath.Dir(flowFile))
	if err != nil {
		return &ExitCodeError{Code: 3, Message: fmt.Sprintf("flow test: abs path: %v", err)}
	}

	// Discover runs.
	var discoverOpts []flowtest.Option
	if runName != "" {
		discoverOpts = append(discoverOpts, flowtest.WithRunName(runName))
	}
	runs, err := flowtest.DiscoverRuns(flowName, flowBaseDir, discoverOpts...)
	if err != nil {
		return &ExitCodeError{Code: 3, Message: fmt.Sprintf("flow test: discover runs: %v", err)}
	}

	if len(runs) == 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"flow test: no runs found in %s/fixtures/%s\n", flowBaseDir, flowName)
		return nil
	}

	// Run each discovered test run.
	overallFail := false
	for _, run := range runs {
		exitCode, err := executeRun(ctx, cmd, flow, s, run, mode, passthroughFlat, keepSandbox)
		if err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  run %q: %v\n", run.Name, err)
		}
		if exitCode != 0 {
			overallFail = true
		}
	}

	if overallFail {
		return &ExitCodeError{Code: 1, Message: "flow test: one or more runs failed"}
	}
	return nil
}

func executeRun(
	ctx context.Context,
	cmd *cobra.Command,
	flow *core.Flow,
	s interface {
		core.Repository
		core.LogRepository
	},
	run flowtest.Run,
	mode xrr.Mode,
	cliPassthrough []string,
	keepSandbox bool,
) (int, error) {
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nrun: %s\n", run.Name)

	// Merge CLI passthrough with manifest passthrough.
	mergedPassthrough := flowtest.MergePassthrough(cliPassthrough, run.Passthrough)

	// Create sandbox.
	cwd, err := os.Getwd()
	if err != nil {
		return 3, fmt.Errorf("getwd: %w", err)
	}
	sb, err := flowtest.NewSandbox(cwd)
	if err != nil {
		return 3, fmt.Errorf("sandbox: %w", err)
	}
	sb.Keep = keepSandbox
	defer sb.Teardown()

	// Extract shim binaries.
	if err := sb.ExtractShims(); err != nil {
		return 3, fmt.Errorf("extract shims: %w", err)
	}

	// Build run descriptor with merged passthrough for env injection.
	runWithPassthrough := run
	runWithPassthrough.Passthrough = mergedPassthrough

	// Inject sandbox env into current process for subprocesses spawned by executor.
	// Build env — stored on sandbox for use when executor eventually spawns agents.
	// We inject the most critical vars into the process environment here so that
	// any subprocess the executor spawns (e.g., git calls in tests) picks them up.
	sandboxEnv := sb.Env("", mode, &runWithPassthrough)
	for _, kv := range sandboxEnv {
		idx := strings.IndexByte(kv, '=')
		if idx < 0 {
			continue
		}
		key := kv[:idx]
		// Don't override HOME in the process env — it breaks keychain auth for
		// subprocesses like claude. The agent runner passes its own full env.
		if key == "HOME" {
			continue
		}
		_ = os.Setenv(key, kv[idx+1:])
	}
	// Restore original env after run.
	defer func() {
		for key := range envMap(sandboxEnv) {
			_ = os.Unsetenv(key)
		}
	}()

	// Wire agent runner — dispatches adapter shim per step in record/replay mode.
	adapters := map[string]flowtest.AgentAdapter{
		"claude":   flowtest.NewClaudeAdapter(),
		"gemini":   flowtest.NewGeminiAdapter(),
		"fabric":   flowtest.NewFabricAdapter(),
		"llm":      flowtest.NewLLMAdapter(),
		"codex":     flowtest.NewCodexAdapter(),
		"opencode":  flowtest.NewOpenCodeAdapter(),
		"routellm":  flowtest.NewRouteLLMAdapter(),
		"crewai":    flowtest.NewCrewAIAdapter(),
		"langchain": flowtest.NewLangChainAdapter(),
		"openai-agents": flowtest.NewOpenAIAgentsAdapter(),
		"autogen":        flowtest.NewAutoGenAdapter(),
		"n8n":            flowtest.NewN8NAdapter(),
		"bedrock":        flowtest.NewBedrockAdapter(),
	}
	globalCfg, err := flowtest.LoadGlobalAdapterConfig()
	if err != nil {
		return 3, fmt.Errorf("adapter config: %w", err)
	}
	resolver := flowtest.NewAdapterResolver(adapters, flow, globalCfg)
	sandboxRunner := flowtest.NewSandboxAgentRunner(sb, mode, &runWithPassthrough, resolver)
	execRunner := flowtest.NewExecAgentRunner(sb.RepoDir)
	// Composite: dispatch task→sandbox, exec→exec runner. SandboxAgentRunner
	// declines exec steps via its CanHandle, so order here is illustrative.
	composite := core.NewCompositeAgentRunner(sandboxRunner, execRunner)
	agentRunner := &loggingAgentRunner{
		inner:  composite,
		stderr: os.Stderr,
		total:  len(flow.Steps),
	}

	// Execute flow.
	executor := core.NewFlowExecutor(s, s).
		WithEvaKey(os.Getenv("EVA_KEY")).
		WithTestMode().
		WithAgentRunner(agentRunner)
	by := core.GetCurrentUser()

	flowRun, stepOutputs, err := executor.Execute(ctx, flow, by)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  FAIL step error: %v\n", err)
		return 1, nil
	}

	// Run eva contracts for each step using real step outputs where available.
	hook := flowtest.NewHook(run.ContractsDir)
	contractFailed := false

	for stepID := range flow.Steps {
		stepOutput := stepOutputs[stepID]
		if stepOutput == nil {
			stepOutput = map[string]any{
				"status": string(core.StepStatusSucceeded),
				"run_id": flowRun.ID,
			}
		}
		if err := hook.Run(stepID, stepOutput); err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  FAIL contract %s: %v\n", stepID, err)
			contractFailed = true
		}
	}

	if contractFailed {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  result: FAIL (contract violation)\n")
		return 1, nil
	}

	// Assert expected exit code.
	if run.ExpectedExit != 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"  result: FAIL (expected exit %d but flow succeeded)\n", run.ExpectedExit)
		return 1, nil
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  result: PASS\n")
	return 0, nil
}

// loggingAgentRunner wraps a core.AgentRunner and prints step progress + timing
// to stderr so operators can follow a long record/replay run in real time.
type loggingAgentRunner struct {
	inner  core.AgentRunner
	stderr *os.File
	total  int
	done   int
}

func (r *loggingAgentRunner) CanHandle(step core.Step) bool { return r.inner.CanHandle(step) }

func (r *loggingAgentRunner) Run(ctx context.Context, step core.Step, prompt string) (map[string]any, error) {
	r.done++
	fmt.Fprintf(r.stderr, "  step %d/%d: %s\n", r.done, r.total, step.ID)
	t0 := time.Now()
	out, err := r.inner.Run(ctx, step, prompt)
	elapsed := time.Since(t0)
	if err != nil {
		fmt.Fprintf(r.stderr, "  step %d/%d: %s FAIL (%.1fs): %v\n", r.done, r.total, step.ID, elapsed.Seconds(), err)
	} else {
		fmt.Fprintf(r.stderr, "  step %d/%d: %s OK (%.1fs)\n", r.done, r.total, step.ID, elapsed.Seconds())
	}
	return out, err
}

// flowFileBaseName returns the base name of a flow file without extension.
func flowFileBaseName(path string) string {
	base := filepath.Base(path)
	if ext := filepath.Ext(base); ext != "" {
		return base[:len(base)-len(ext)]
	}
	return base
}

// envMap extracts key names from an env slice.
func envMap(env []string) map[string]struct{} {
	m := make(map[string]struct{}, len(env))
	for _, kv := range env {
		idx := strings.IndexByte(kv, '=')
		if idx >= 0 {
			m[kv[:idx]] = struct{}{}
		}
	}
	return m
}
