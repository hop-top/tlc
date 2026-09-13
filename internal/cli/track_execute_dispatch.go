package cli

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"hop.top/tlc/internal/core"
)

// agentDispatcher runs agent-kind tasks through the same path as
// `tlc task execute`: registry lookup, full task context, audit record
// and bus events — but without a state transition, which the executor
// owns.
//
// Container dispatches run concurrently up to the executor's bound: each
// carries its own execParams and gets its own pod. Local dispatches share
// the host working tree, so localMu lets only one run at a time whatever
// --concurrency says.
type agentDispatcher struct {
	s interface {
		core.Repository
		core.LogRepository
	}
	registry      *core.AgentRegistry
	defaultAgent  string
	ctxtRefs      []string
	imageOverride string
	params        execParams
	updater       *core.StateUpdater
	exec          agentExecFunc // nil selects execTaskWithAgent

	localMu sync.Mutex
}

// Dispatch implements core.Dispatcher.
func (d *agentDispatcher) Dispatch(ctx context.Context, task *core.Task) (*core.DispatchResult, error) {
	if d.params.local {
		d.localMu.Lock()
		defer d.localMu.Unlock()
	}

	name := d.defaultAgent
	if task.Spec != nil && task.Spec.Agent != "" {
		name = task.Spec.Agent
	}
	if name == "" {
		return nil, fmt.Errorf("no agent for %s: set agent on the recipe step or pass --agent", formatTaskAlias(task))
	}
	resolved, err := resolveAgentName(d.registry, name)
	if err != nil {
		return nil, err
	}
	cfg, err := d.registry.Get(resolved)
	if err != nil {
		var trust *core.ErrTrustRequired
		if errors.As(err, &trust) {
			return nil, fmt.Errorf("%w; re-run with --trust-project to approve", err)
		}
		return nil, fmt.Errorf("agent %s: %w", resolved, err)
	}
	if d.imageOverride != "" {
		copied := *cfg
		copied.Image = d.imageOverride
		cfg = &copied
	}

	p := d.params
	p.agent = resolved
	ac, err := core.NewContextBuilder(d.s).BuildForTask(ctx, task.ID, core.BuildOpts{
		CtxtRefs: d.ctxtRefs,
		RepoRoot: p.repoRoot,
	})
	if err != nil {
		return nil, fmt.Errorf("build context for %s: %w", formatTaskAlias(task), err)
	}

	run := d.exec
	if run == nil {
		run = execTaskWithAgent
	}
	result, err := run(ctx, p, task.ID, ac, cfg, d.updater)
	if err != nil {
		return nil, err
	}
	return &core.DispatchResult{
		Status:  result.Status,
		Summary: result.Summary,
		Result:  result.Outputs,
	}, nil
}

var _ core.Dispatcher = (*agentDispatcher)(nil)
