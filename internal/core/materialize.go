package core

import (
	"context"
	"fmt"
)

// specTasks describes a batch of tasks to create from an ordered list of
// specifications — plan frontmatter entries or recipe steps. Every id and
// sequence number is allocated before the first task is built, so a
// specification may reference a later one (a forward blocked-by) and
// resolve it through the ids table it is handed.
type specTasks struct {
	IDGen     IDGenerator
	ProjectID string
	// Count is the number of specifications; Build is called once per
	// index in order.
	Count int
	// Label names specification i in errors ("plan task 2 (\"Title\")",
	// "step lint"); the helper appends what went wrong.
	Label func(i int) string
	// Build returns the task for specification i. ids and seqs hold the
	// allocation for the whole batch; the task must use ids[i] and seqs[i].
	Build func(i int, ids []string, seqs []int64) (*Task, error)
	// PreCreate, when set, runs on each built task before it is persisted
	// and aborts the batch when it returns an error.
	PreCreate func(*Task) error
}

// createSpecTasks allocates ids and sequence numbers for the whole batch,
// then builds, gates and persists each task in order. On error it returns
// the ids of the tasks it did create (a prefix of the batch) alongside
// the error so the caller can account for them; nothing is rolled back.
func createSpecTasks(ctx context.Context, repo Repository, batch specTasks) ([]string, error) {
	ids := make([]string, batch.Count)
	seqs := make([]int64, batch.Count)
	for i := range ids {
		seq, err := batch.IDGen.GetNextSequenceID(ctx, batch.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to allocate sequence; %w", batch.Label(i), err)
		}
		ids[i] = NewTaskID()
		seqs[i] = int64(seq)
	}

	for i := range ids {
		task, err := batch.Build(i, ids, seqs)
		if err != nil {
			return ids[:i], err
		}
		if batch.PreCreate != nil {
			if err := batch.PreCreate(task); err != nil {
				return ids[:i], fmt.Errorf("%s: %w", batch.Label(i), err)
			}
		}
		if err := repo.CreateTask(ctx, task); err != nil {
			return ids[:i], fmt.Errorf("%s: failed to create; %w", batch.Label(i), err)
		}
	}
	return ids, nil
}

// mergeOwnedBlockedBy computes a task's new blocked_by set for a source
// (plan ingest, recipe run) that declares edges and records the ones it
// authored under ownerKey.
//
// The source owns only the edges a previous pass recorded under ownerKey.
// Those are dropped and replaced by edges; every other current edge is
// out-of-band (added via `task update --add-blocked-by`, a sync plugin or
// another owner) and is preserved untouched. Ordering keeps the surviving
// edges first so a re-run does not churn unrelated entries.
func mergeOwnedBlockedBy(task *Task, ownerKey string, edges []string) []string {
	priorOwned := make(map[string]struct{})
	if task.Meta != nil {
		for _, id := range NormalizeStringSliceMeta(task.Meta[ownerKey]) {
			priorOwned[id] = struct{}{}
		}
	}

	merged := make([]string, 0, len(edges))
	for _, id := range task.BlockedBy() {
		if _, owned := priorOwned[id]; owned {
			continue // retractable — re-added below only if still declared
		}
		merged = append(merged, id)
	}
	return append(merged, edges...)
}

// assigneeFromSpec turns an authored assignee ("@lead", "lead", "") into
// the stored form: the leading "@" is dropped and an empty value is nil.
func assigneeFromSpec(raw string) *string {
	if raw == "" {
		return nil
	}
	if len(raw) > 1 && raw[0] == '@' {
		raw = raw[1:]
	}
	return &raw
}

// optString returns nil for "" so optional columns stay NULL rather than
// carrying an empty marker.
func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
