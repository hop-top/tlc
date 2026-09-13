package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/flowtest"
	"hop.top/tlc/internal/storage"
	xrr "hop.top/xrr"
)

// Exit codes `recipe test` reports. ExitCodeError itself lives in errors.go.
const (
	recipeTestExitFailed  = 1 // a task failed, or a contract was violated
	recipeTestExitMiss    = 2 // replay found no cassette for a step
	recipeTestExitSandbox = 3 // the sandbox, the recipe or the fixtures are unusable
)

func init() {
	RecipeCmd.AddCommand(NewRecipeTestCmd())
}

// NewRecipeTestCmd creates the `tlc recipe test` subcommand.
func NewRecipeTestCmd() *cobra.Command {
	var (
		record      bool
		passthrough []string
		keepSandbox bool
		tasks       []string
	)

	cmd := &cobra.Command{
		Use:   "test <recipe> [run-name]",
		Short: "Run a recipe against recorded tool output",
		Long: `Materialize a recipe into a throwaway store and execute it inside a
hermetic sandbox: a clone of the working tree, the shim binaries ahead of
the real ones on PATH, and a home of its own.

In replay mode (the default) every tool call is served from a recorded
cassette, so a run is deterministic and offline. With --record the calls
reach the real binaries and the cassettes are written for later replays.

Fixtures live beside the recipe, one directory per run:

  fixtures/<recipe>/<run>/
    test.yaml     run name, expected_exit, vars, passthrough
    contracts/    <step-id>.yaml, one per gated step
    record/       <step-id>/ cassettes

Naming no run executes every run the recipe has.

Exit codes:
  0  every task ran and every contract passed
  1  a task failed or a contract was violated
  2  replay found no cassette for a step
  3  the sandbox, the recipe or the fixtures could not be set up`,
		Annotations: map[string]string{
			"kit/side-effect": "interactive",
			"kit/idempotent":  "yes",
		},
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			runName := ""
			if len(args) == 2 {
				runName = args[1]
			}
			return runRecipeTest(cmd, recipeTestOpts{
				Ref: args[0], RunName: runName, Record: record,
				Passthrough: passthrough, KeepSandbox: keepSandbox, Tasks: tasks,
			})
		},
	}

	cmd.Flags().BoolVar(&record, "record", false,
		"Run against real binaries and write the cassettes")
	cmd.Flags().StringArrayVar(&passthrough, "passthrough", nil,
		"Always run these tools live (repeatable or comma-separated)")
	cmd.Flags().BoolVar(&keepSandbox, "keep-sandbox", false,
		"Keep the sandbox directory after the run")
	cmd.Flags().StringArrayVar(&tasks, "task", nil,
		"Materialize only these steps: ordinals (3), ranges (1-5) or ids (lint)")

	return cmd
}

// recipeTestOpts is what one `recipe test` invocation was asked to do.
type recipeTestOpts struct {
	Ref         string
	RunName     string
	Record      bool
	Passthrough []string
	KeepSandbox bool
	Tasks       []string
}

func (o recipeTestOpts) mode() xrr.Mode {
	if o.Record {
		return xrr.ModeRecord
	}
	return xrr.ModeReplay
}

// flatten splits the comma form of --passthrough.
func flattenPassthrough(values []string) []string {
	var out []string
	for _, v := range values {
		for _, piece := range strings.Split(v, ",") {
			if piece = strings.TrimSpace(piece); piece != "" {
				out = append(out, piece)
			}
		}
	}
	return out
}

func runRecipeTest(cmd *cobra.Command, opts recipeTestOpts) error {
	recipe, expanded, err := loadExpandedRecipe(opts.Ref)
	if err != nil {
		return &ExitCodeError{Code: recipeTestExitSandbox, Message: fmt.Sprintf("recipe test: %v", err)}
	}
	// Fixtures sit beside the recipe file; absolute so the cassette dirs
	// the shims read from the env survive the sandbox's own cwd.
	baseDir, err := filepath.Abs(filepath.Dir(recipe.Path))
	if err != nil {
		return &ExitCodeError{Code: recipeTestExitSandbox, Message: fmt.Sprintf("recipe test: %v", err)}
	}

	var discoverOpts []flowtest.Option
	if opts.RunName != "" {
		discoverOpts = append(discoverOpts, flowtest.WithRunName(opts.RunName))
	}
	runs, err := flowtest.DiscoverRuns(recipe.Name, baseDir, discoverOpts...)
	if err != nil {
		return &ExitCodeError{Code: recipeTestExitSandbox, Message: fmt.Sprintf("recipe test: %v", err)}
	}
	if len(runs) == 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"recipe test: no runs in %s\n", filepath.Join(baseDir, "fixtures", recipe.Name))
		return nil
	}

	worst := 0
	for _, run := range runs {
		code, err := executeRecipeRun(cmd, recipe, expanded, run, opts)
		if err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  run %q: %v\n", run.Name, err)
		}
		if code > worst {
			worst = code
		}
	}
	switch worst {
	case 0:
		return nil
	case recipeTestExitMiss:
		return &ExitCodeError{Code: recipeTestExitMiss, Message: "recipe test: cassette miss; re-run with --record"}
	case recipeTestExitSandbox:
		return &ExitCodeError{Code: recipeTestExitSandbox, Message: "recipe test: sandbox setup failed"}
	default:
		return &ExitCodeError{Code: recipeTestExitFailed, Message: "recipe test: one or more runs failed"}
	}
}

// recipeTestRun is one run's sandbox and the store materialized into it.
type recipeTestRun struct {
	sandbox *Sandbox
	store   *storage.SQLiteStorage
}

// Sandbox aliases the flowtest sandbox so this file reads in one vocabulary.
type Sandbox = flowtest.Sandbox

// executeRecipeRun materializes the recipe into a throwaway store inside a
// fresh sandbox, executes every task, then checks the contracts.
func executeRecipeRun(
	cmd *cobra.Command, recipe *core.Recipe, expanded []core.RecipeStep,
	run flowtest.Run, opts recipeTestOpts,
) (int, error) {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "\nrun: %s\n", run.Name)

	fixture := run
	fixture.Passthrough = flowtest.MergePassthrough(flattenPassthrough(opts.Passthrough), run.Passthrough)

	env, cleanup, err := setupRecipeTestRun(&fixture, opts)
	if err != nil {
		return recipeTestExitSandbox, err
	}
	defer cleanup()

	ctx := context.Background()
	res, err := materializeForTest(ctx, cmd, recipe, expanded, fixture, env.store, opts)
	if err != nil {
		return recipeTestExitSandbox, err
	}
	taskIDs := createdTaskIDs(res)
	if len(taskIDs) == 0 {
		_, _ = fmt.Fprintf(out, "  result: FAIL (recipe materialized no tasks)\n")
		return recipeTestExitFailed, nil
	}

	report, err := executeTestTasks(ctx, cmd, env, &fixture, recipe, taskIDs, opts)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  FAIL %v\n", err)
		return recipeTestExitFailed, nil
	}
	return gradeRecipeRun(ctx, cmd, env.store, fixture, taskIDs, report)
}

// setupRecipeTestRun builds the sandbox, extracts the shims, exports the
// sandbox env to this process, and opens the throwaway store.
func setupRecipeTestRun(fixture *flowtest.Run, opts recipeTestOpts) (*recipeTestRun, func(), error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, fmt.Errorf("getwd: %w", err)
	}
	sb, err := flowtest.NewSandbox(cwd)
	if err != nil {
		return nil, nil, fmt.Errorf("sandbox: %w", err)
	}
	sb.Keep = opts.KeepSandbox
	if err := sb.ExtractShims(); err != nil {
		sb.Teardown()
		return nil, nil, fmt.Errorf("extract shims: %w", err)
	}

	restore := exportSandboxEnv(sb, fixture, opts.mode())

	store, err := storage.NewSQLiteStorage(filepath.Join(sb.RootDir, "recipe-test.sqlite"))
	if err != nil {
		restore()
		sb.Teardown()
		return nil, nil, fmt.Errorf("test store: %w", err)
	}
	env := &recipeTestRun{sandbox: sb, store: store}
	return env, func() {
		_ = store.Close()
		restore()
		sb.Teardown()
	}, nil
}

// exportSandboxEnv puts the sandbox env on this process so anything it
// spawns inherits the shim PATH and the cassette settings. HOME is left
// alone: overriding it breaks keychain access for the real binaries in
// record mode. The returned func unsets what was set.
func exportSandboxEnv(sb *flowtest.Sandbox, fixture *flowtest.Run, mode xrr.Mode) func() {
	// Remember what each key was so the restore puts it back rather than
	// dropping it: PATH in particular must survive the run.
	previous := map[string]*string{}
	for key, value := range sb.Overrides("", mode, fixture) {
		if key == "HOME" {
			continue
		}
		if old, ok := os.LookupEnv(key); ok {
			kept := old
			previous[key] = &kept
		} else {
			previous[key] = nil
		}
		_ = os.Setenv(key, value)
	}
	return func() {
		for key, old := range previous {
			if old == nil {
				_ = os.Unsetenv(key)
				continue
			}
			_ = os.Setenv(key, *old)
		}
	}
}

// materializeForTest renders the recipe with the run's vars and creates
// its tasks in the throwaway store, trackless and with no subject.
func materializeForTest(
	ctx context.Context, cmd *cobra.Command, recipe *core.Recipe, expanded []core.RecipeStep,
	fixture flowtest.Run, store *storage.SQLiteStorage, opts recipeTestOpts,
) (*core.MaterializeResult, error) {
	createOpts := recipeCreateOpts{Recipe: opts.Ref, Tasks: opts.Tasks}
	plan, err := renderRecipePlan(cmd, recipe, expanded, createOpts, nil, fixture.Vars)
	if err != nil {
		return nil, fmt.Errorf("materialize: %w", err)
	}
	// The throwaway store reads back through the ambient project the same
	// way the real one does, so create the tasks under it: trackless, but
	// in the project bucket a later GetTask will look in.
	in := core.MaterializeInput{
		Recipe: plan.Recipe, Steps: plan.Steps, RunID: plan.RunID,
		ProjectID: currentProjectID(), By: core.GetCurrentUser(), Vars: plan.Vars,
		Selection: plan.Selection, DroppedDeps: plan.Dropped,
	}
	m := &core.Materializer{Repo: store, IDGen: store, Runs: store}
	res, err := m.Materialize(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("materialize: %w", err)
	}
	for _, w := range append(plan.Warnings, res.Warnings...) {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  warning: %s\n", w)
	}
	return res, nil
}

// executeTestTasks runs the materialized tasks with both dispatchers
// pointed at the sandbox, one task at a time so the output stays readable
// and the cassettes are hit in a fixed order.
func executeTestTasks(
	ctx context.Context, cmd *cobra.Command, env *recipeTestRun, fixture *flowtest.Run,
	recipe *core.Recipe, taskIDs []string, opts recipeTestOpts,
) (*core.ExecReport, error) {
	resolver := flowtest.NewAdapterResolver(flowtest.DefaultAdapters(), recipe, mustGlobalAdapterConfig())
	agentExec := flowtest.NewSandboxAgentExec(env.sandbox, opts.mode(), fixture, resolver)
	agentExec.Log = cmd.ErrOrStderr()

	executor := core.NewExecutor(env.store, env.store, env.store, core.DefaultWorkflow(), core.ExecutorOpts{
		Actor:       core.GetCurrentUser(),
		Concurrency: 1,
		EvaKey:      os.Getenv("EVA_KEY"),
		Out:         cmd.OutOrStdout(),
	})
	executor.Register(core.TaskKindAgent, &sandboxAgentDispatcher{exec: agentExec})
	executor.Register(core.TaskKindExec, flowtest.NewSandboxExecDispatcher(env.sandbox, opts.mode(), fixture))

	report, err := executor.RunTasks(ctx, taskIDs)
	if err != nil {
		return nil, fmt.Errorf("execute: %w", err)
	}
	return report, nil
}

// mustGlobalAdapterConfig loads the user's adapter config, falling back to
// an empty one: a missing or unreadable file is not a test failure.
func mustGlobalAdapterConfig() *flowtest.GlobalAdapterConfig {
	cfg, err := flowtest.LoadGlobalAdapterConfig()
	if err != nil || cfg == nil {
		return &flowtest.GlobalAdapterConfig{}
	}
	return cfg
}

// sandboxAgentDispatcher dispatches agent-kind tasks to the sandbox shim
// for their step, in place of the real agent runtime.
type sandboxAgentDispatcher struct {
	exec *flowtest.SandboxAgentExec
}

// Dispatch implements core.Dispatcher.
func (d *sandboxAgentDispatcher) Dispatch(ctx context.Context, task *core.Task) (*core.DispatchResult, error) {
	result, err := d.exec.Run(ctx, flowtest.TaskStepRef(task), task.Description)
	if err != nil {
		return nil, fmt.Errorf("task %s: %w", task.ID, err)
	}
	return &core.DispatchResult{Status: result.Status, Summary: result.Summary, Result: result.Outputs}, nil
}

var _ core.Dispatcher = (*sandboxAgentDispatcher)(nil)

// gradeRecipeRun turns the executed tasks into an exit code: a cassette
// miss outranks a plain failure, then the contracts, then the manifest's
// expected exit.
func gradeRecipeRun(
	ctx context.Context, cmd *cobra.Command, store *storage.SQLiteStorage,
	fixture flowtest.Run, taskIDs []string, report *core.ExecReport,
) (int, error) {
	out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
	hook := flowtest.NewHook(fixture.ContractsDir)
	miss, failed, contractFailed := false, len(report.Done) != len(taskIDs), false

	for _, id := range taskIDs {
		task, err := store.GetTask(ctx, id)
		if err != nil || task == nil {
			return recipeTestExitSandbox, fmt.Errorf("read task %s: %w", id, err)
		}
		if flowtest.IsCassetteMiss(task.Result) {
			_, _ = fmt.Fprintf(errOut, "  MISS %s: no cassette; re-run with --record\n", task.StepID)
			miss = true
		}
		if err := hook.Run(task.StepID, contractPayload(task)); err != nil {
			_, _ = fmt.Fprintf(errOut, "  FAIL contract %s: %v\n", task.StepID, err)
			contractFailed = true
		}
	}

	switch {
	case miss:
		_, _ = fmt.Fprintf(out, "  result: FAIL (cassette miss)\n")
		return recipeTestExitMiss, nil
	case failed:
		_, _ = fmt.Fprintf(out, "  result: FAIL (%d of %d tasks done)\n", len(report.Done), len(taskIDs))
		return recipeTestExitFailed, nil
	case contractFailed:
		_, _ = fmt.Fprintf(out, "  result: FAIL (contract violation)\n")
		return recipeTestExitFailed, nil
	case fixture.ExpectedExit != 0:
		_, _ = fmt.Fprintf(out, "  result: FAIL (expected exit %d, the run succeeded)\n", fixture.ExpectedExit)
		return recipeTestExitFailed, nil
	}
	_, _ = fmt.Fprintf(out, "  result: PASS\n")
	return 0, nil
}

// contractPayload is what a step's contract is evaluated against: the
// task's own result, or its terminal state when it produced none.
func contractPayload(task *core.Task) map[string]any {
	if len(task.Result) > 0 {
		return task.Result
	}
	return map[string]any{"status": string(task.Status), "run_id": task.RunID}
}
