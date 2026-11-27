package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"hop.top/kit/go/runtime/domain"
	"hop.top/tlc/internal/core"
)

// sortableFields and filterableFields pin the advertised sets. They are
// literals on purpose: a test that derived them from the code under test
// would pass whatever the code accepted.
var sortableFields = []string{
	"id", "seq", "title", "description", "status", "assigned_to",
	"reference", "created_at", "updated_at", "tags", "origin_system",
	"last_sync_at", "archived", "project_id", "effort", "priority",
	"stale_timeout", "blocked_reason", "stale_fired_at", "track_id",
	"due_at", "remind_at", "rrule", "no_auto_remind", "kind", "attempts",
	"claimed_at", "run_id", "step_id", "step_ordinal",
}

// injectionShapes are field-name inputs that are not task columns. Each
// either changes what the statement means if pasted into it, or is just
// not a field; none may reach SQL.
var injectionShapes = []string{
	"title;",
	"title; SELECT 1",
	"title--",
	"title -- x",
	"title/**/",
	"/* x */ title",
	"'title'",
	`"title"`,
	"`title`",
	"[title]",
	"title,",
	",title",
	"title,,id",
	"title id",
	"title DESC",
	"title DESC, id",
	"title ASC",
	"(SELECT 1)",
	"(SELECT count(*) FROM tasks)",
	"CASE WHEN 1 THEN id ELSE title END",
	"(CASE WHEN title = 'b' THEN 0 ELSE 1 END)",
	"id)",
	"(id",
	"id OR 1=1",
	"title = title OR 1=1 OR title",
	"1",
	"1=1",
	"x'",
	"tasks.id",
	"rowid",
	"nosuchcolumn",
	"meta",
	"meta.priority",
	"spec",
	"result",
	"title\x00",
	"title\nid",
	"",
	"  ",
}

func newFieldStore(t *testing.T) *SQLiteStorage {
	t.Helper()
	resetProjectDetection()
	dir := t.TempDir()
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldCwd)
		resetProjectDetection()
	})
	s, err := NewSQLiteStorage(filepath.Join(dir, "fields.db"))
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	ctx := context.Background()
	for _, task := range []*core.Task{
		{ID: "T-1", Title: "charlie", Status: core.StatusTodo},
		{ID: "T-2", Title: "alpha", Status: core.StatusTodo},
		{ID: "T-3", Title: "bravo", Status: core.StatusDone},
	} {
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed %s: %v", task.ID, err)
		}
	}
	return s
}

func titles(tasks []*core.Task) []string {
	out := make([]string, len(tasks))
	for i, task := range tasks {
		out[i] = task.Title
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestListTasks_SortFieldAllowlist_AcceptsEveryField(t *testing.T) {
	s := newFieldStore(t)
	for _, field := range sortableFields {
		for _, dir := range []string{"", "asc", "desc", "ASC", "Desc"} {
			if _, err := s.ListTasks(context.Background(),
				core.Query{AllProjects: true, SortBy: field, SortDirection: dir}); err != nil {
				t.Errorf("sort %q %q rejected: %v", field, dir, err)
			}
		}
	}
}

func TestListTasks_SortFieldAllowlist_RejectsInjectionShapes(t *testing.T) {
	s := newFieldStore(t)
	for _, bad := range injectionShapes {
		if bad == "" {
			continue // empty means "no sort key"; covered separately
		}
		got, err := s.ListTasks(context.Background(),
			core.Query{AllProjects: true, SortBy: bad, SortDirection: "asc"})
		if !errors.Is(err, core.ErrInvalidSortField) {
			t.Errorf("sort %q: err = %v, want ErrInvalidSortField", bad, err)
		}
		if got != nil {
			t.Errorf("sort %q: returned %d rows alongside the rejection", bad, len(got))
		}
	}
}

func TestListTasks_SortEmptyKeepsDefaultOrder(t *testing.T) {
	s := newFieldStore(t)
	got, err := s.ListTasks(context.Background(), core.Query{AllProjects: true})
	if err != nil {
		t.Fatalf("no sort key: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("no sort key: %d rows, want 3", len(got))
	}
}

func TestListTasks_SortDirectionAllowlist(t *testing.T) {
	s := newFieldStore(t)
	for _, bad := range []string{"sideways", "asc; SELECT 1", "desc,", "ASC DESC", "asc--", "(asc)", "1"} {
		got, err := s.ListTasks(context.Background(),
			core.Query{AllProjects: true, SortBy: "title", SortDirection: bad})
		if !errors.Is(err, core.ErrInvalidSortDirection) {
			t.Errorf("direction %q: err = %v, want ErrInvalidSortDirection", bad, err)
		}
		if got != nil {
			t.Errorf("direction %q: returned rows alongside the rejection", bad)
		}
	}
	// A bad direction is rejected even with no sort key: it must not be
	// silently dropped just because nothing consumed it.
	if _, err := s.ListTasks(context.Background(),
		core.Query{AllProjects: true, SortDirection: "sideways"}); !errors.Is(err, core.ErrInvalidSortDirection) {
		t.Errorf("direction without sort key: err = %v, want ErrInvalidSortDirection", err)
	}
}

func TestListTasks_SortOrdersRows(t *testing.T) {
	s := newFieldStore(t)
	read := func(by, dir string) []string {
		got, err := s.ListTasks(context.Background(),
			core.Query{AllProjects: true, SortBy: by, SortDirection: dir})
		if err != nil {
			t.Fatalf("sort %q %q: %v", by, dir, err)
		}
		return titles(got)
	}
	if got, want := read("title", "asc"), []string{"alpha", "bravo", "charlie"}; !sameStrings(got, want) {
		t.Errorf("title asc = %v, want %v", got, want)
	}
	if got, want := read("title", "desc"), []string{"charlie", "bravo", "alpha"}; !sameStrings(got, want) {
		t.Errorf("title desc = %v, want %v", got, want)
	}
	// Names resolve case-insensitively, as column names always did.
	if got, want := read("TITLE", "asc"), []string{"alpha", "bravo", "charlie"}; !sameStrings(got, want) {
		t.Errorf("TITLE asc = %v, want %v", got, want)
	}
	// A comma-separated list is each key through the allowlist: status
	// first (done sorts before todo), then title within it.
	if got, want := read("status,title", "asc"), []string{"bravo", "alpha", "charlie"}; !sameStrings(got, want) {
		t.Errorf("status,title asc = %v, want %v", got, want)
	}
	if got, want := read("status, title", "desc"), []string{"charlie", "alpha", "bravo"}; !sameStrings(got, want) {
		t.Errorf("status, title desc = %v, want %v", got, want)
	}
}

func TestListTasks_SortListRejectsAnyBadKey(t *testing.T) {
	s := newFieldStore(t)
	for _, bad := range []string{"title,nosuchcolumn", "nosuchcolumn,title", "title,", "title,(SELECT 1)"} {
		if _, err := s.ListTasks(context.Background(),
			core.Query{AllProjects: true, SortBy: bad}); !errors.Is(err, core.ErrInvalidSortField) {
			t.Errorf("sort %q: err = %v, want ErrInvalidSortField", bad, err)
		}
	}
}

// operatorForms covers every branch of the filter builder that pastes the
// field name into a clause.
var operatorForms = []struct {
	name string
	op   core.Operator
	val  any
}{
	{"eq", core.OpEq, "x"},
	{"equal", core.OpEqual, "x"},
	{"empty-op", "", "x"},
	{"not-eq", core.OpNotEq, "x"},
	{"gt", core.OpGt, "x"},
	{"gte", core.OpGte, "x"},
	{"lt", core.OpLt, "x"},
	{"lte", core.OpLte, "x"},
	{"contains", core.OpContains, "x"},
	{"start", core.OpStart, "x"},
	{"end", core.OpEnd, "x"},
	{"in", core.OpIn, "a,b"},
	{"eq-nil", core.OpEq, nil},
	{"not-eq-nil", core.OpNotEq, nil},
}

func TestTaskFilters_FieldAllowlist_AcceptsEveryField(t *testing.T) {
	s := newFieldStore(t)
	ctx := context.Background()
	for _, field := range sortableFields {
		for _, form := range operatorForms {
			q := core.Query{AllProjects: true, Filters: []core.FieldFilter{
				{Field: field, Operator: form.op, Value: form.val},
			}}
			if _, err := s.ListTasks(ctx, q); err != nil {
				t.Errorf("filter %s %s rejected: %v", field, form.name, err)
			}
		}
	}
}

func TestTaskFilters_FieldAllowlist_RejectsInjectionShapes(t *testing.T) {
	s := newFieldStore(t)
	ctx := context.Background()
	for _, bad := range injectionShapes {
		for _, form := range operatorForms {
			q := core.Query{AllProjects: true, Filters: []core.FieldFilter{
				{Field: bad, Operator: form.op, Value: form.val},
			}}
			rows, err := s.ListTasks(ctx, q)
			if !errors.Is(err, core.ErrInvalidFilterField) {
				t.Errorf("ListTasks filter %q %s: err = %v, want ErrInvalidFilterField", bad, form.name, err)
			}
			if rows != nil {
				t.Errorf("ListTasks filter %q %s: returned rows alongside the rejection", bad, form.name)
			}
			if _, err := s.CountTasks(ctx, q); !errors.Is(err, core.ErrInvalidFilterField) {
				t.Errorf("CountTasks filter %q %s: err = %v, want ErrInvalidFilterField", bad, form.name, err)
			}
			if _, err := s.CountTasksByStatus(ctx, q); !errors.Is(err, core.ErrInvalidFilterField) {
				t.Errorf("CountTasksByStatus filter %q %s: err = %v, want ErrInvalidFilterField", bad, form.name, err)
			}
			if _, err := s.CountTasksByProjectAndStatus(ctx, q); !errors.Is(err, core.ErrInvalidFilterField) {
				t.Errorf("CountTasksByProjectAndStatus filter %q %s: err = %v, want ErrInvalidFilterField", bad, form.name, err)
			}
		}
	}
}

// A tautology pasted as a field name would match every row; the filter
// must fail rather than silently widen the result set.
func TestTaskFilters_TautologyFieldDoesNotWidenResults(t *testing.T) {
	s := newFieldStore(t)
	got, err := s.ListTasks(context.Background(), core.Query{AllProjects: true, Filters: []core.FieldFilter{
		{Field: "title = title OR 1=1 OR title", Operator: core.OpEq, Value: "no-such-title"},
	}})
	if err == nil {
		t.Fatalf("tautology field accepted; matched %d rows", len(got))
	}
}

func TestTaskFilters_FieldNameIsCaseInsensitiveAndStillFilters(t *testing.T) {
	s := newFieldStore(t)
	got, err := s.ListTasks(context.Background(), core.Query{AllProjects: true, Filters: []core.FieldFilter{
		{Field: "Title", Operator: core.OpEq, Value: "alpha"},
		{Field: "STATUS", Operator: core.OpEq, Value: "todo"},
	}})
	if err != nil {
		t.Fatalf("mixed-case fields rejected: %v", err)
	}
	if want := []string{"alpha"}; !sameStrings(titles(got), want) {
		t.Errorf("rows = %v, want %v", titles(got), want)
	}
}

// The domain repository forwards its sort key straight into the query, so
// a caller of the generic repository API gets the same gate.
func TestTaskDomainRepo_ListRejectsBadSort(t *testing.T) {
	s := newFieldStore(t)
	repo := NewTaskDomainRepo(s)
	if _, err := repo.List(context.Background(), domain.Query{Sort: "title; SELECT 1"}); !errors.Is(err, core.ErrInvalidSortField) {
		t.Errorf("err = %v, want ErrInvalidSortField", err)
	}
	got, err := repo.List(context.Background(), domain.Query{Sort: "title"})
	if err != nil {
		t.Fatalf("valid sort rejected: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("rows = %d, want 3", len(got))
	}
}
