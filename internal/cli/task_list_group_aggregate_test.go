package cli

// Coverage for the --group-by / aggregate-format conflict.
//
// --summary and --counters aggregate the whole match set into counts;
// --group-by partitions it into row tables. Both claim the output shape,
// and there is no reading of the pair that satisfies either. Resolving it
// silently — grouping wins, or the aggregate wins — hands the user output
// for a command they did not type.
//
// THE GATE MUST KEY ON aggregateFormat(), NOT on the taskListSummary /
// taskListCounters bools. An aggregate has three spellings: the flags,
// `-f summary` / `--format counters`, and a config `output.format:
// summary`. aggregateFormat resolves all three; the bools see only the
// first. Keying on the bools once left the format spellings on the
// paginated path (see its doc comment), and the same mistake here would
// let `-f summary --group-by track` through the check to produce
// whichever shape the renderer happened to reach first. Every test below
// therefore exercises all three spellings.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// aggregateConflictSpellings are the three ways an aggregate format is
// selected, each paired with the args/config that select it.
type aggregateSpelling struct {
	name string
	// args are appended to `task list --group-by track`.
	args []string
	// configFormat, when set, is seeded on output.format instead.
	configFormat string
	// wantFlag is the flag name the rejection must name.
	wantFlag string
}

func aggregateConflictSpellings() []aggregateSpelling {
	return []aggregateSpelling{
		{name: "SummaryFlag", args: []string{"--summary"}, wantFlag: "summary"},
		{name: "CountersFlag", args: []string{"--counters"}, wantFlag: "counters"},
		// `-f summary` / `--format counters` resolve THROUGH
		// output.format — that is the whole reason the gate cannot read
		// the bools. The flag is passed AND the resolved value seeded,
		// because this package's shared viper does not retain a
		// per-command -f binding across tests (see runGroupByAggregate);
		// what matters to the gate is the resolved value either way.
		{
			name: "ShortFormatSummary", args: []string{"-f", "summary"},
			configFormat: formatSummary, wantFlag: "summary",
		},
		{
			name: "LongFormatCounters", args: []string{"--format", "counters"},
			configFormat: formatCounters, wantFlag: "counters",
		},
		{name: "ConfigOutputFormat", configFormat: formatSummary, wantFlag: "summary"},
	}
}

// runGroupByAggregate runs `task list --group-by track` under one
// aggregate spelling and returns the error and combined output.
func runGroupByAggregate(t *testing.T, sp aggregateSpelling) (error, string) {
	t.Helper()
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer func() { _ = s.Close() }()

	// A non-empty store, so a run that wrongly succeeds produces real
	// output rather than passing on emptiness.
	if err := s.CreateTask(ctx, &core.Task{
		ID: "T-0001", Seq: 1, Title: "seeded", Status: core.StatusTodo,
		TrackID: strptr("alpha"),
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	args := append([]string{"task", "list", "--group-by", "track"}, sp.args...)
	cmd.SetArgs(args)

	// Seeded AFTER newTestCmd: the test root re-binds output.format to
	// its own -f flag, and a bind installed against a previous test's
	// command is dropped by the viper.Reset in setupTestDir. Setting the
	// value here is what makes the config spelling reach the resolver,
	// and it is also why the -f spellings pass their value explicitly
	// rather than relying on a binding this harness does not keep.
	if sp.configFormat != "" {
		viper.Set("output.format", sp.configFormat)
	}

	return cmd.Execute(), buf.String()
}

// TestGroupByWithAggregateFormatIsRejected: every spelling fails, and the
// message names BOTH flags so the user can see which pair conflicts.
func TestGroupByWithAggregateFormatIsRejected(t *testing.T) {
	for _, sp := range aggregateConflictSpellings() {
		t.Run(sp.name, func(t *testing.T) {
			err, out := runGroupByAggregate(t, sp)
			if err == nil {
				t.Fatalf("--group-by with %s must fail; got success:\n%s", sp.name, out)
			}
			msg := err.Error()
			if !strings.Contains(msg, "--group-by") {
				t.Errorf("rejection must name --group-by; got: %s", msg)
			}
			if !strings.Contains(msg, sp.wantFlag) {
				t.Errorf("rejection must name the aggregate (%s); got: %s", sp.wantFlag, msg)
			}
			// The conflict is reported INSTEAD of output, not alongside
			// it: a user who sees a summary table plus an error cannot
			// tell whether the numbers are trustworthy.
			if strings.Contains(out, "Total") {
				t.Errorf("no aggregate output may render before the rejection; got:\n%s", out)
			}
		})
	}
}

// TestAggregateWithoutGroupByStillWorks is the other half of the gate. A
// check that rejected every aggregate, or every --group-by, would pass
// the test above while breaking both features.
func TestAggregateWithoutGroupByStillWorks(t *testing.T) {
	for _, sp := range aggregateConflictSpellings() {
		t.Run(sp.name, func(t *testing.T) {
			ctx, cleanup := setupTestDir(t)
			defer cleanup()
			s, err := getStorageRaw()
			if err != nil {
				t.Fatalf("getStorageRaw: %v", err)
			}
			defer func() { _ = s.Close() }()

			if err := s.CreateTask(ctx, &core.Task{
				ID: "T-0001", Seq: 1, Title: "seeded", Status: core.StatusTodo,
			}); err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
			if sp.configFormat != "" {
				viper.Set("output.format", sp.configFormat)
			}

			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(append([]string{"task", "list"}, sp.args...))

			if err := cmd.Execute(); err != nil {
				t.Fatalf("%s alone must still work: %v\n%s", sp.name, err, buf.String())
			}
		})
	}
}

// TestGroupByWithoutAggregateStillWorks: the grouping path must survive
// the gate too.
func TestGroupByWithoutAggregateStillWorks(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer func() { _ = s.Close() }()

	if err := s.CreateTask(ctx, &core.Task{
		ID: "T-0001", Seq: 1, Title: "seeded", Status: core.StatusTodo,
		TrackID: strptr("alpha"),
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--group-by", "track"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("--group-by alone must still work: %v\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "alpha") {
		t.Errorf("grouped listing must render its group heading; got:\n%s", buf.String())
	}
}

// TestGroupByAggregateGateReadsResolvedFormat is the direct mutation
// guard for the gate's KEY. It asserts on aggregateFormat() rather than
// on the bools: under `-f summary` the bools are both false, so a gate
// keyed on them sees no aggregate at all and lets the conflict through.
func TestGroupByAggregateGateReadsResolvedFormat(t *testing.T) {
	defer resetTaskFlags()

	// The flag spelling: bools true, resolver agrees.
	resetTaskFlags()
	viper.Set("output.format", formatTable)
	taskListSummary = true
	if got := aggregateFormat(); got != formatSummary {
		t.Errorf("--summary must resolve to %q, got %q", formatSummary, got)
	}

	// The format spelling: bools BOTH FALSE, and the resolver is the
	// only thing that still sees the aggregate. A gate reading
	// taskListSummary || taskListCounters here reads false and passes
	// the conflicting invocation straight through.
	resetTaskFlags()
	viper.Set("output.format", formatSummary)
	if taskListSummary || taskListCounters {
		t.Fatal("fixture broken: the format spelling must leave both bools false")
	}
	if got := aggregateFormat(); got != formatSummary {
		t.Errorf("output.format summary must resolve to %q, got %q", formatSummary, got)
	}

	// And a non-aggregate format resolves to "", so the gate does not
	// fire on an ordinary listing.
	resetTaskFlags()
	viper.Set("output.format", formatJSON)
	if got := aggregateFormat(); got != "" {
		t.Errorf("json must resolve to no aggregate, got %q", got)
	}
}
