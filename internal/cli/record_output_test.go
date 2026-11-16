package cli

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

// recordCmd is a bare command carrying the record flags, as the real
// list/show/track commands do.
func recordCmd(t *testing.T, fields []string, flags ...string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	cmd, buf := groupRenderCmd(t)
	registerRecordFlags(cmd, fields)
	cmd.Flags().String(flagTemplate, "", "")
	for i := 0; i+1 < len(flags); i += 2 {
		if err := cmd.Flags().Set(flags[i], flags[i+1]); err != nil {
			t.Fatalf("set --%s: %v", flags[i], err)
		}
	}
	return cmd, buf
}

func sampleTasks() []*core.Task {
	due := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return []*core.Task{
		{
			ID: "task_01aaaaaaaaaaaaaaaaaaaaaaaa", Seq: 1, Title: "full",
			Status: core.StatusTodo, Priority: "P0", Effort: "M",
			TrackID: strptr("track_x"), DueAt: &due, Tags: []string{"a", "b"},
			Meta: map[string]interface{}{"k": "v"},
		},
		{ID: "task_01bbbbbbbbbbbbbbbbbbbbbbbb", Seq: 2, Title: "bare", Status: core.StatusTodo},
	}
}

// TestTaskColumnsHaveJSONFields is the schema test: every table column
// must resolve to a field the JSON record carries.
func TestTaskColumnsHaveJSONFields(t *testing.T) {
	have := map[string]bool{}
	for _, f := range taskRecordFields {
		have[f] = true
	}
	for key := range taskColumnHeaders {
		f, ok := taskColumnFields[key]
		if !ok {
			t.Errorf("table column %q has no entry in taskColumnFields", key)
			continue
		}
		if !have[f] {
			t.Errorf("table column %q maps to JSON field %q, absent from taskRecord", key, f)
		}
	}
}

func TestTrackColumnsHaveJSONFields(t *testing.T) {
	have := map[string]bool{}
	for _, f := range trackRecordFields {
		have[f] = true
	}
	for key := range trackColumnHeaders {
		f, ok := trackColumnFields[key]
		if !ok {
			t.Errorf("track column %q has no entry in trackColumnFields", key)
			continue
		}
		if !have[f] {
			t.Errorf("track column %q maps to JSON field %q, absent from trackRecord", key, f)
		}
	}
}

// TestRecordsCoverModelFields fails when a field is added to the model
// without a record field, so JSON cannot silently fall behind the model.
func TestRecordsCoverModelFields(t *testing.T) {
	have := map[string]bool{}
	for _, f := range taskRecordFields {
		have[f] = true
	}
	for _, f := range fieldsOf(reflect.TypeOf(core.Task{})) {
		if !have[f] {
			t.Errorf("core.Task field %q missing from taskRecord", f)
		}
	}
	have = map[string]bool{}
	for _, f := range trackRecordFields {
		have[f] = true
	}
	for _, f := range fieldsOf(reflect.TypeOf(core.Track{})) {
		if !have[f] {
			t.Errorf("core.Track field %q missing from trackRecord", f)
		}
	}
}

// TestListJSONRowsCarryEveryField: every row has every field, nulls kept.
func TestListJSONRowsCarryEveryField(t *testing.T) {
	cmd, buf := recordCmd(t, taskRecordFields)
	if err := formatTasks(cmd, sampleTasks(), formatJSON, false); err != nil {
		t.Fatal(err)
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("%v\n%s", err, buf)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	for i, r := range rows {
		for _, f := range taskRecordFields {
			if _, ok := r[f]; !ok {
				t.Errorf("row %d missing field %q", i, f)
			}
		}
	}
	for _, f := range []string{"priority", "effort", "track_id", "due_at"} {
		if string(rows[1][f]) != "null" {
			t.Errorf("unset %s = %s, want null", f, rows[1][f])
		}
	}
	if string(rows[1]["meta"]) != "{}" || string(rows[1]["tags"]) != "[]" || string(rows[1]["blocked_by"]) != "[]" {
		t.Errorf("collections must be empty, not null: meta=%s tags=%s blocked_by=%s",
			rows[1]["meta"], rows[1]["tags"], rows[1]["blocked_by"])
	}
}

// TestJSONFieldSelection: --json f1,f2 keeps exactly those keys, in order.
func TestJSONFieldSelection(t *testing.T) {
	cmd, buf := recordCmd(t, taskRecordFields, flagRecordJSON, "track_id,priority,effort")
	if err := formatTasks(cmd, sampleTasks(), formatJSON, false); err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	for i, r := range rows {
		if len(r) != 3 {
			t.Errorf("row %d has %d keys, want 3: %v", i, len(r), r)
		}
	}
	if rows[0]["priority"] != "P0" || rows[1]["priority"] != nil {
		t.Errorf("priority wrong: %v", rows)
	}
	s := buf.String()
	if strings.Index(s, "track_id") >= strings.Index(s, "priority") ||
		strings.Index(s, "priority") >= strings.Index(s, "effort") {
		t.Errorf("selected order not preserved:\n%s", s)
	}
}

func TestJSONUnknownFieldRejected(t *testing.T) {
	cmd, _ := recordCmd(t, taskRecordFields, flagRecordJSON, "nope")
	err := formatTasks(cmd, sampleTasks(), formatJSON, false)
	if err == nil || !strings.Contains(err.Error(), "available fields") {
		t.Fatalf("want unknown-field error listing fields, got %v", err)
	}
}

// TestBareJSONListsFields: `--json` with no value fails and names fields.
func TestBareJSONListsFields(t *testing.T) {
	cmd, _ := groupRenderCmd(t)
	registerRecordFlags(cmd, taskRecordFields)
	cmd.RunE = func(*cobra.Command, []string) error { return nil }
	cmd.SetArgs([]string{"--json"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("bare --json must fail")
	}
	for _, f := range []string{"priority", "effort", "track_id", "blocked_by"} {
		if !strings.Contains(err.Error(), f) {
			t.Errorf("bare --json error does not list %q: %v", f, err)
		}
	}
}

func TestJQAndTemplate(t *testing.T) {
	cmd, buf := recordCmd(t, taskRecordFields, flagRecordJSON, "title,priority", flagRecordJQ, ".[] | select(.priority != null) | .title")
	if err := formatTasks(cmd, sampleTasks(), formatJSON, false); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "full" {
		t.Errorf("jq output = %q, want full", got)
	}

	cmd, buf = recordCmd(t, taskRecordFields, flagRecordJSON, "display_id,title",
		flagTemplate, `{{range .Items}}{{.display_id}}={{.title}};{{end}}`)
	if err := formatTasks(cmd, sampleTasks(), formatTable, false); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "T-0001=full;T-0002=bare;" {
		t.Errorf("template output = %q", got)
	}
}

// TestCSVHeadersStable: csv is real CSV whose header is the record field
// list, identical to the JSON field set.
func TestCSVHeadersStable(t *testing.T) {
	cmd, buf := recordCmd(t, taskRecordFields)
	if err := formatTasks(cmd, sampleTasks(), formatCSV, false); err != nil {
		t.Fatal(err)
	}
	recs, err := csv.NewReader(bytes.NewReader(buf.Bytes())).ReadAll()
	if err != nil {
		t.Fatalf("not valid csv: %v\n%s", err, buf)
	}
	if !reflect.DeepEqual(recs[0], taskRecordFields) {
		t.Errorf("csv header %v != record fields %v", recs[0], taskRecordFields)
	}
	if len(recs) != 3 {
		t.Errorf("want header + 2 rows, got %d", len(recs))
	}
}

// TestYAMLMatchesJSON: same keys, same order, same values.
func TestYAMLMatchesJSON(t *testing.T) {
	cmd, ybuf := recordCmd(t, taskRecordFields)
	if err := formatTasks(cmd, sampleTasks(), formatYAML, false); err != nil {
		t.Fatal(err)
	}
	y := ybuf.String()
	for _, want := range []string{"priority: P0", "priority: null", "track_id: null", "display_id: T-0001", "blocked_by: []"} {
		if !strings.Contains(y, want) {
			t.Errorf("yaml missing %q:\n%s", want, y)
		}
	}
	// Key order is the JSON record order.
	last := -1
	for _, f := range taskRecordFields {
		i := strings.Index(y, "\n"+f+":")
		if i < 0 && strings.HasPrefix(y, f+":") {
			i = 0
		}
		if i < last {
			t.Errorf("yaml key %q out of record order", f)
		}
		if i > last {
			last = i
		}
	}
}

// TestDisplayIDMatchesHumanView: a legacy row whose stored ID is a T-NNNN
// alias differing from its seq shows that alias in the table; the JSON
// display_id must be the same string so scripts can round-trip it.
func TestDisplayIDMatchesHumanView(t *testing.T) {
	task := &core.Task{ID: "T-0734", Seq: 983, Title: "legacy", Status: core.StatusTodo}
	rec := newTaskRecord(task, nil)
	if rec.DisplayID != "T-0734" || rec.Seq != 983 {
		t.Fatalf("display_id=%q seq=%d, want T-0734/983", rec.DisplayID, rec.Seq)
	}
	if rec.DisplayID != formatTaskAlias(task) {
		t.Errorf("display_id %q != table ID %q", rec.DisplayID, formatTaskAlias(task))
	}
	typed := &core.Task{ID: "task_01aaaaaaaaaaaaaaaaaaaaaaaa", Seq: 7, Title: "t", Status: core.StatusTodo}
	if got := newTaskRecord(typed, nil).DisplayID; got != formatTaskAlias(typed) || got != "T-0007" {
		t.Errorf("typed display_id = %q", got)
	}
}

// TestListAndShowRecordShapeMatch: list rows and show's record are the
// same type, so the same keys in the same order.
func TestListAndShowRecordShapeMatch(t *testing.T) {
	task := sampleTasks()[0]
	list := taskRecords([]*core.Task{task})[0]
	show := newTaskRecord(task, nil)
	a, _ := json.Marshal(list)
	b, _ := json.Marshal(show)
	if string(a) != string(b) {
		t.Errorf("list and show records differ:\n%s\n%s", a, b)
	}
}

func TestTrackProgressIsNumeric(t *testing.T) {
	tr := &core.Track{ID: "track_x", Seq: 1, Slug: "abc", Title: "T", Type: "feature", Status: core.TrackStatusActive}
	p := core.TrackProgress{TotalTasks: 4, CompletedTasks: 1, TotalPhases: 2, CurrentPhase: 1}
	rec := newTrackRecord(tr, nil, p)
	b, _ := json.Marshal(rec)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	prog, ok := m["progress"].(map[string]any)
	if !ok {
		t.Fatalf("progress must be an object: %s", b)
	}
	if prog["done"] != float64(1) || prog["total"] != float64(4) || prog["percent"] != float64(25) {
		t.Errorf("numeric progress wrong: %v", prog)
	}
	if prog["text"] != "1/4 (P1/2)" {
		t.Errorf("progress text = %v", prog["text"])
	}
	if m["assigned_to"] != nil {
		t.Errorf("assigned_to must be null, got %v", m["assigned_to"])
	}
}
