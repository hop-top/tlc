package cli

// Coverage for nested JSON/YAML under `--group-by`: scripts consume the
// same shape humans see, `{groups: [{name, tasks: [...]}]}`.
//
// The contract has two halves and the second is the load-bearing one.
// WITHOUT --group-by the payload must stay a flat array, byte-for-byte,
// because every existing consumer of `task list -f json` iterates it
// directly. Wrapping unconditionally would break all of them at once,
// silently — `jq '.[]'` on an object errors the same way it did on the
// `null` that commit c1b573d fixed.

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// renderTasksFormat runs formatTasks with the given format and options.
func renderTasksFormat(t *testing.T, tasks []*core.Task, format string, opts ...listOption) string {
	t.Helper()
	cmd, buf := groupRenderCmd(t)
	if err := formatTasks(cmd, tasks, format, false, opts...); err != nil {
		t.Fatalf("formatTasks(%s): %v", format, err)
	}
	return buf.String()
}

// groupedJSONPayload is the decoded nested document.
type groupedJSONPayload struct {
	Groups []struct {
		Name  string  `json:"name"`
		Tasks *[]any  `json:"tasks"`
	} `json:"groups"`
}

// TestGroupedJSONNestsGroups pins the nested shape and its ordering: the
// same groups, in the same order, as the table renderer produces.
func TestGroupedJSONNestsGroups(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-0001", Seq: 1, Title: "zoe work", Status: core.StatusTodo, AssignedTo: strptr("zoe")},
		{ID: "T-0002", Seq: 2, Title: "ann work", Status: core.StatusTodo, AssignedTo: strptr("ann")},
		{ID: "T-0003", Seq: 3, Title: "orphan", Status: core.StatusTodo},
	}

	got := renderTasksFormat(t, tasks, formatJSON, withGroupBy("assignee"))

	var payload groupedJSONPayload
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("grouped json must decode as {groups:[...]}: %v\n%s", err, got)
	}
	if len(payload.Groups) != 3 {
		t.Fatalf("want 3 groups, got %d:\n%s", len(payload.Groups), got)
	}
	// Named groups ascending, "(none)" forced last — the same ordering
	// the table renders, so a script and a human never disagree.
	wantNames := []string{"ann", "zoe", noneGroupName}
	for i, want := range wantNames {
		if payload.Groups[i].Name != want {
			t.Errorf("group %d name = %q, want %q", i, payload.Groups[i].Name, want)
		}
	}
	if payload.Groups[0].Tasks == nil || len(*payload.Groups[0].Tasks) != 1 {
		t.Errorf("ann must carry its one task; got:\n%s", got)
	}
}

// TestUngroupedJSONUnchanged is the regression guard for every existing
// consumer: with no grouping key the payload stays the flat array it has
// always been, byte-for-byte. An object where an array was is a breaking
// change no version of `jq '.[]'` survives.
func TestUngroupedJSONUnchanged(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-0001", Seq: 1, Title: "first", Status: core.StatusTodo},
		{ID: "T-0002", Seq: 2, Title: "second", Status: core.StatusTodo},
	}
	for _, format := range []string{formatJSON, formatYAML} {
		t.Run(format, func(t *testing.T) {
			// No option at all — the shape every pre-existing caller uses.
			want := renderTasksFormat(t, tasks, format)

			// And with the option present but empty: the `task list` path
			// when --group-by was never set.
			if got := renderTasksFormat(t, tasks, format, withGroupBy("")); got != want {
				t.Errorf("empty --group-by altered %s.\ngot:\n%s\nwant:\n%s", format, got, want)
			}

			// A --group-limit with no grouping key must not wrap either.
			if got := renderTasksFormat(t, tasks, format, withGroupLimit(1)); got != want {
				t.Errorf("--group-limit altered ungrouped %s.\ngot:\n%s\nwant:\n%s", format, got, want)
			}

			if format == formatJSON {
				var arr []any
				if err := json.Unmarshal([]byte(want), &arr); err != nil {
					t.Fatalf("ungrouped json must stay a top-level array: %v\n%s", err, want)
				}
			}
		})
	}
}

// TestGroupedJSONEmptyGroupIsArrayNotNull: a group carrying no tasks must
// serialize `tasks: []`, never `null`. Same contract commit c1b573d
// established repo-wide, applied per group — normalizeEmptySlices is the
// one helper that does it, so grouping runs it rather than reimplementing
// nil-to-empty a second time.
func TestGroupedJSONEmptyGroupIsArrayNotNull(t *testing.T) {
	groups := []TaskGroup{
		{Name: "ann", Tasks: []*core.Task{{ID: "T-0001", Seq: 1, Title: "x", Status: core.StatusTodo}}},
		{Name: "empty", Tasks: nil},
	}
	doc := groupedPayload(groups)

	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal grouped payload: %v", err)
	}
	// Scoped to the tasks field: a task's own nullable columns
	// (assigned_to and friends) are legitimately null and are not what
	// this contract is about.
	if bytes.Contains(raw, []byte(`"tasks":null`)) {
		t.Errorf("no group may serialize null tasks; got: %s", raw)
	}

	var payload groupedJSONPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, g := range payload.Groups {
		if g.Tasks == nil {
			t.Errorf("group %q rendered tasks:null, want []; got: %s", g.Name, raw)
		}
	}
}

// TestGroupedYAMLNestsGroups: YAML travels the same branch as JSON, so
// the nesting must reach it too rather than being a JSON-only shape.
func TestGroupedYAMLNestsGroups(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-0001", Seq: 1, Title: "ann work", Status: core.StatusTodo, AssignedTo: strptr("ann")},
	}
	got := renderTasksFormat(t, tasks, formatYAML, withGroupBy("assignee"))
	for _, want := range []string{"groups:", "name:", "tasks:"} {
		if !strings.Contains(got, want) {
			t.Errorf("grouped yaml must carry %q; got:\n%s", want, got)
		}
	}
}

// TestGroupedJSONIgnoresGroupLimit pins the decision: --group-limit is a
// DISPLAY cap, and JSON is data. A machine consumer asking for JSON gets
// every task in every group, capped or not.
//
// The alternative — honouring the cap — writes a truncated task list into
// a payload that carries no marker of the truncation, so a script that
// re-serializes it silently persists a subset as if it were the whole.
// That is data corruption, not a display choice. Ignoring a flag the user
// typed is its own hazard, so it is not ignored silently: the command
// prints a stderr note (TestGroupLimitJSONNote).
func TestGroupedJSONIgnoresGroupLimit(t *testing.T) {
	tasks := append(groupLimitTasks("ann", 1, 5), groupLimitTasks("bob", 11, 2)...)

	capped := renderTasksFormat(t, tasks, formatJSON, withGroupBy("assignee"), withGroupLimit(2))
	uncapped := renderTasksFormat(t, tasks, formatJSON, withGroupBy("assignee"))
	if capped != uncapped {
		t.Errorf("--group-limit must not truncate JSON data.\ncapped:\n%s\nuncapped:\n%s", capped, uncapped)
	}

	var payload groupedJSONPayload
	if err := json.Unmarshal([]byte(capped), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, g := range payload.Groups {
		want := 5
		if g.Name == "bob" {
			want = 2
		}
		if g.Tasks == nil || len(*g.Tasks) != want {
			t.Errorf("group %q must carry all %d tasks; got:\n%s", g.Name, want, capped)
		}
	}
}

// TestGroupLimitJSONNote: ignoring a flag the user typed is announced on
// stderr, so the omission is visible without polluting the stdout payload
// a script parses.
func TestGroupLimitJSONNote(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer func() { _ = s.Close() }()

	for i := 1; i <= 3; i++ {
		task := &core.Task{
			ID: fmtTaskID(i), Seq: int64(i), Title: "seeded",
			Status: core.StatusTodo, AssignedTo: strptr("ann"),
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
	}

	// Set through viper, as the other JSON contract tests do: a bare
	// `-f json` on the test root can be overridden by an output.format
	// another test left behind on the shared viper instance.
	viper.Set("output.format", formatJSON)
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{
		"task", "list",
		"--group-by", "assignee", "--group-limit", "1",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("grouped json list failed: %v\n%s", err, errBuf.String())
	}

	// stdout stays pure payload.
	var payload groupedJSONPayload
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("stdout must be the payload alone: %v\n%s", err, out.String())
	}
	if len(payload.Groups) != 1 || payload.Groups[0].Tasks == nil ||
		len(*payload.Groups[0].Tasks) != 3 {
		t.Errorf("json must carry all 3 tasks despite the cap; got:\n%s", out.String())
	}
	// The ignored flag is announced, on stderr.
	note := errBuf.String()
	if !strings.Contains(note, "--group-limit") {
		t.Errorf("ignoring --group-limit must be announced on stderr; got: %q", note)
	}
}

// fmtTaskID builds the legacy alias form the fixtures use.
func fmtTaskID(n int) string {
	return groupLimitTasks("x", n, 1)[0].ID
}
