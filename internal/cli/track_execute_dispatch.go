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
// Dispatches are serialized: the agent runner reads process-level
// settings (agentRun*), and local mode writes one shared
// .tlc/context.json, so two agents cannot run at once in this process.
type agentDispatcher struct {
	s interface {
		core.Repository
		core.LogRepository
	}
	registry      *core.AgentRegistry
	defaultAgent  string
	ctxtRefs      []string
	local         bool
	imageOverride string
	updater       *core.StateUpdater

	mu sync.Mutex
}

// Dispatch implements core.Dispatcher.
func (d *agentDispatcher) Dispatch(ctx context.Context, task *core.Task) (*core.DispatchResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

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

	ac, err := core.NewContextBuilder(d.s).BuildForTask(ctx, task.ID, core.BuildOpts{
		CtxtRefs: d.ctxtRefs,
		RepoRoot: repoRootForMode(d.local),
	})
	if err != nil {
		return nil, fmt.Errorf("build context for %s: %w", formatTaskAlias(task), err)
	}

	result, err := execTaskWithAgent(ctx, resolved, task.ID, ac, cfg, d.updater, d.local)
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
