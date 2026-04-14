package core

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// fileRefPattern matches file paths in task descriptions. Captures paths
// like internal/foo/bar.go, cmd/main.go, etc.
var fileRefPattern = regexp.MustCompile(
	`(?:^|\s)((?:[a-zA-Z0-9_.-]+/)+[a-zA-Z0-9_.-]+\.\w+)`,
)

// BuildOpts configures context construction.
type BuildOpts struct {
	PromptFile   string   // --prompt override; empty = auto-generate
	ContextFiles []string // --context extra files
	RepoRoot     string   // default: "/workspace"
}

func (o *BuildOpts) repoRoot() string {
	if o.RepoRoot != "" {
		return o.RepoRoot
	}
	return "/workspace"
}

// ContextBuilder derives AgentContext from task, flow, or track metadata.
type ContextBuilder struct {
	repo Repository
}

// NewContextBuilder returns a builder backed by the given repository.
func NewContextBuilder(repo Repository) *ContextBuilder {
	return &ContextBuilder{repo: repo}
}

// BuildForTask creates an AgentContext from a stored task.
func (b *ContextBuilder) BuildForTask(
	ctx context.Context,
	taskID string,
	opts BuildOpts,
) (*AgentContext, error) {
	task, err := b.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("context builder: get task: %w", err)
	}
	if task == nil {
		return nil, fmt.Errorf(
			"context builder: task %s not found; run 'tlc task list' to see available tasks",
			taskID,
		)
	}

	files := extractFileRefs(task.Description)
	files = append(files, opts.ContextFiles...)

	prompt := opts.PromptFile
	if prompt == "" {
		prompt = buildTaskPrompt(task, files)
	}

	var trackID string
	if task.TrackID != nil {
		trackID = *task.TrackID
	}

	ac := &AgentContext{
		Version:         AgentContextVersion,
		TaskID:          task.ID,
		TaskTitle:       task.Title,
		TaskDescription: task.Description,
		Tags:            task.Tags,
		TrackID:         trackID,
		RepoRoot:        opts.repoRoot(),
		Files:           files,
		Prompt:          prompt,
	}
	return ac, nil
}

// BuildForFlowStep creates an AgentContext from a flow step definition.
func (b *ContextBuilder) BuildForFlowStep(
	ctx context.Context,
	flow *Flow,
	stepID string,
	opts BuildOpts,
) (*AgentContext, error) {
	if flow == nil {
		return nil, fmt.Errorf("context builder: flow is nil")
	}
	step, ok := flow.Steps[stepID]
	if !ok {
		return nil, fmt.Errorf(
			"context builder: step %q not found in flow %s",
			stepID, flow.ID,
		)
	}

	prompt := opts.PromptFile
	if prompt == "" {
		prompt = buildFlowStepPrompt(flow, &step)
	}

	ac := &AgentContext{
		Version:    AgentContextVersion,
		FlowID:     flow.ID,
		FlowStepID: stepID,
		StepType:   string(step.Type),
		StepTitle:  step.Title,
		RepoRoot:   opts.repoRoot(),
		Files:      opts.ContextFiles,
		Prompt:     prompt,
	}
	return ac, nil
}

// BuildForTrack resolves linked TODO tasks in blocked-by order and
// returns one AgentContext per task.
func (b *ContextBuilder) BuildForTrack(
	ctx context.Context,
	trackID string,
	opts BuildOpts,
) ([]*AgentContext, error) {
	tasks, err := b.repo.ListTasks(ctx, Query{
		Filters: []FieldFilter{
			{Field: "track_id", Operator: OpEq, Value: trackID},
			{Field: "status", Operator: OpEq, Value: StatusTodo},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("context builder: list track tasks: %w", err)
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf(
			"context builder: no TODO tasks for track %s",
			trackID,
		)
	}

	sorted := sortByBlockedBy(tasks)

	contexts := make([]*AgentContext, 0, len(sorted))
	for _, task := range sorted {
		ac, err := b.BuildForTask(ctx, task.ID, opts)
		if err != nil {
			return nil, err
		}
		contexts = append(contexts, ac)
	}
	return contexts, nil
}

// extractFileRefs parses file paths from task description text.
func extractFileRefs(desc string) []string {
	matches := fileRefPattern.FindAllStringSubmatch(desc, -1)
	seen := make(map[string]struct{}, len(matches))
	var files []string
	for _, m := range matches {
		path := strings.TrimSpace(m[1])
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		files = append(files, path)
	}
	return files
}

func buildTaskPrompt(task *Task, files []string) string {
	var sb strings.Builder
	sb.WriteString("## Task\n\n")
	sb.WriteString(task.Title)
	sb.WriteString("\n\n")
	if task.Description != "" {
		sb.WriteString("### Description\n\n")
		sb.WriteString(task.Description)
		sb.WriteString("\n\n")
	}
	if len(files) > 0 {
		sb.WriteString("### Relevant Files\n\n")
		for _, f := range files {
			sb.WriteString("- ")
			sb.WriteString(f)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("### Instructions\n\n")
	sb.WriteString("Complete the task described above. ")
	sb.WriteString("Write results to /workspace/.tlc/results.json\n")
	return sb.String()
}

func buildFlowStepPrompt(flow *Flow, step *Step) string {
	var sb strings.Builder
	sb.WriteString("## Flow Step\n\n")
	sb.WriteString("Flow: ")
	sb.WriteString(flow.Name)
	sb.WriteString("\nStep: ")
	sb.WriteString(step.Title)
	sb.WriteString("\n\n")
	if step.TaskTemplate != nil && step.TaskTemplate.Description != "" {
		sb.WriteString("### Description\n\n")
		sb.WriteString(step.TaskTemplate.Description)
		sb.WriteString("\n\n")
	}
	sb.WriteString("### Instructions\n\n")
	sb.WriteString("Complete the step described above. ")
	sb.WriteString("Write results to /workspace/.tlc/results.json\n")
	return sb.String()
}

// sortByBlockedBy orders tasks so blockers come before dependents.
// Simple topological sort; cycles are broken arbitrarily.
func sortByBlockedBy(tasks []*Task) []*Task {
	byID := make(map[string]*Task, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t
	}

	visited := make(map[string]bool, len(tasks))
	var result []*Task

	var visit func(t *Task)
	visit = func(t *Task) {
		if visited[t.ID] {
			return
		}
		visited[t.ID] = true
		for _, dep := range t.BlockedBy() {
			if dt, ok := byID[dep]; ok {
				visit(dt)
			}
		}
		result = append(result, t)
	}

	for _, t := range tasks {
		visit(t)
	}
	return result
}
