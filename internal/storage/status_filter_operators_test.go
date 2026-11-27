package storage

import (
	"context"
	"testing"

	"hop.top/tlc/internal/core"
)

// The legacy fixture (legacySeed, openSeededLegacy) holds rows spelled
// TODO, IN_PROGRESS, DONE, SKIPPED, BACKLOG, Todo, todo and in_review. Any
// future work on status handling should run its predicates against it:
// a fixture of lowercase rows only passes whether or not legacy labels
// still match.

// sf builds a status filter; op "" is what the CLI and REST callers send.
func sf(op core.Operator, v any) core.FieldFilter {
	return core.FieldFilter{Field: "status", Operator: op, Value: v}
}

// Every operator form a caller can put on a status filter, including the
// empty one, must see legacy and current spellings alike.
func TestQuery_StatusFilterEveryOperatorForm(t *testing.T) {
	withDefaultVocab(t)
	s := openSeededLegacy(t)

	todos := []string{"T-1", "T-6", "T-7"}
	cases := []struct {
		name    string
		filters []core.FieldFilter
		want    []string
	}{
		{"empty op lowercase", []core.FieldFilter{sf("", "todo")}, todos},
		{"empty op uppercase", []core.FieldFilter{sf("", "TODO")}, todos},
		{"empty op mixed case", []core.FieldFilter{sf("", "Todo")}, todos},
		{"empty op in_progress", []core.FieldFilter{sf("", "in_progress")}, []string{"T-2"}},
		{"empty op cancelled reads legacy skipped", []core.FieldFilter{sf("", string(core.StatusCancelled))}, []string{"T-4"}},
		{"empty op skipped spelling", []core.FieldFilter{sf("", "SKIPPED")}, []string{"T-4"}},
		{"empty op repeated ORs", []core.FieldFilter{sf("", "todo"), sf("", "in_progress")}, []string{"T-1", "T-2", "T-6", "T-7"}},
		{"empty op unfinished set", []core.FieldFilter{
			sf("", "backlog"), sf("", "todo"), sf("", "in_progress"), sf("", "in_review"),
		}, []string{"T-1", "T-2", "T-5", "T-6", "T-7", "T-8"}},
		{"colon", []core.FieldFilter{sf(core.OpEq, "TODO")}, todos},
		{"equals", []core.FieldFilter{sf(core.OpEqual, "todo")}, todos},
		{"not equals", []core.FieldFilter{sf(core.OpNotEq, "TODO")}, []string{"T-2", "T-3", "T-4", "T-5", "T-8"}},
		{"in", []core.FieldFilter{sf(core.OpIn, "DONE, backlog")}, []string{"T-3", "T-5"}},
		{"contains", []core.FieldFilter{sf(core.OpContains, "prog")}, []string{"T-2"}},
		{"contains upper", []core.FieldFilter{sf(core.OpContains, "ONE")}, []string{"T-3"}},
		{"contains alias target", []core.FieldFilter{sf(core.OpContains, "cancel")}, []string{"T-4"}},
		{"starts with", []core.FieldFilter{sf(core.OpStart, "in_")}, []string{"T-2", "T-8"}},
		{"starts with alias target", []core.FieldFilter{sf(core.OpStart, "canc")}, []string{"T-4"}},
		{"ends with", []core.FieldFilter{sf(core.OpEnd, "log")}, []string{"T-5"}},
		{"not contains", []core.FieldFilter{sf(core.OpNotCont, "o")}, []string{"T-4", "T-8"}},
		{"status and other field", []core.FieldFilter{
			sf("", "todo"), {Field: "title", Operator: core.OpEq, Value: "title-T-6"},
		}, []string{"T-6"}},
	}
	for _, c := range cases {
		got := queryIDs(t, s, core.Query{Filters: c.filters})
		if !sameSet(got, c.want...) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// The same cases against a custom vocabulary must keep exact matching:
// the compat layer applies to the default vocabulary only.
func TestQuery_StatusFilterEmptyOpCustomVocabularyExact(t *testing.T) {
	withLegacyVocab(t)
	s := openSeededLegacy(t)
	if got := queryIDs(t, s, core.Query{Filters: []core.FieldFilter{sf("", "TODO")}}); !sameSet(got, "T-1") {
		t.Errorf("exact TODO: %v, want [T-1]", got)
	}
	if got := queryIDs(t, s, core.Query{Filters: []core.FieldFilter{sf("", "todo")}}); !sameSet(got, "T-7") {
		t.Errorf("exact todo: %v, want [T-7]", got)
	}
}

func TestQuery_SortByStatusFoldsLegacySpellings(t *testing.T) {
	withDefaultVocab(t)
	s := openSeededLegacy(t)
	read := func(dir string) []string {
		tasks, err := s.ListTasks(context.Background(), core.Query{AllProjects: true, SortBy: "status", SortDirection: dir})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		var out []string
		for _, task := range tasks {
			if n := len(out); n == 0 || out[n-1] != string(task.Status) {
				out = append(out, string(task.Status))
			}
		}
		return out
	}
	want := []string{"backlog", string(core.StatusCancelled), "done", "in_progress", "in_review", "todo"}
	got := read("asc")
	if len(got) != len(want) {
		t.Fatalf("asc: %v, want %v (a status split into several runs)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("asc: %v, want %v", got, want)
		}
	}
	desc := read("desc")
	for i := range want {
		if desc[len(want)-1-i] != want[i] {
			t.Fatalf("desc: %v, want reverse of %v", desc, want)
		}
	}
}

// A store that has been written to by the current build holds lowercase
// rows beside legacy ones; every predicate must see it as one set.
func TestQuery_MixedCaseStoreFiltersAsOneSet(t *testing.T) {
	withDefaultVocab(t)
	s := openSeededLegacy(t)
	ctx := context.Background()
	for _, id := range []string{"T-1", "T-3", "T-4"} {
		task, err := s.GetTaskInProject(ctx, id, "")
		if err != nil || task == nil {
			t.Fatalf("get %s: %v", id, err)
		}
		task.Title += " edited"
		if err := s.UpdateTask(ctx, task); err != nil {
			t.Fatalf("update %s: %v", id, err)
		}
	}
	if raw := rawStatus(t, s, "T-1"); raw != "todo" {
		t.Fatalf("fixture not mixed: T-1 stored %q", raw)
	}
	if raw := rawStatus(t, s, "T-2"); raw != "IN_PROGRESS" {
		t.Fatalf("fixture not mixed: T-2 stored %q", raw)
	}
	for name, c := range map[string]struct {
		f    core.FieldFilter
		want []string
	}{
		"todo":      {sf("", "todo"), []string{"T-1", "T-6", "T-7"}},
		"TODO":      {sf("", "TODO"), []string{"T-1", "T-6", "T-7"}},
		"progress":  {sf("", "in_progress"), []string{"T-2"}},
		"done":      {sf("", "done"), []string{"T-3"}},
		"cancelled": {sf("", string(core.StatusCancelled)), []string{"T-4"}},
		"in":        {sf(core.OpIn, "todo,done"), []string{"T-1", "T-3", "T-6", "T-7"}},
	} {
		if got := queryIDs(t, s, core.Query{Filters: []core.FieldFilter{c.f}}); !sameSet(got, c.want...) {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
	counts, err := s.CountTasksByStatus(ctx, core.Query{AllProjects: true})
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if counts["todo"] != 3 || counts["done"] != 1 || counts["in_progress"] != 1 || counts[string(core.StatusCancelled)] != 1 {
		t.Errorf("counts over mixed store: %v", counts)
	}
}
