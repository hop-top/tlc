package cli

// The task lifecycle commands ignored --dry-run: each funneled its write
// through saveTaskWithLog (or, for `update`, applyTaskFieldChanges) with
// no regard for the flag, so the row landed, the audit log grew, the
// projection was rewritten and the output said "Claimed task T-NNNN".
//
// Kit opts every leaf annotated write-local / destructive-local into
// --dry-run and appends "this command honors --dry-run" to its help, so
// the help advertised support the code never had.
//
// These run the real binary rather than the in-process tree: --dry-run is
// a kit-owned persistent flag registered by the production root, so an
// in-process test cmd would not even parse it.

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// taskStateProbe is the state a dry run must leave untouched. Every field
// here is one a lifecycle command writes.
//
// The nullable fields are decoded as pointers, because "unassigned" and
// "assigned to the empty string" are different facts, then rendered to
// strings for comparison — comparing the structs directly would compare
// POINTERS, and two equal-valued *string from two separate decodes never
// match. That mistake makes every assertion pass vacuously in one
// direction and fail spuriously in the other.
type taskStateProbe struct {
	Title         string  `json:"title"`
	Status        string  `json:"status"`
	AssignedTo    *string `json:"assigned_to"`
	BlockedReason *string `json:"blocked_reason"`
	Description   string  `json:"description"`
	UpdatedAt     string  `json:"updated_at"`
}

// String renders the probe by value, so two reads of an unchanged task
// compare equal.
func (p taskStateProbe) String() string {
	return fmt.Sprintf(
		"title=%q status=%q assigned=%s blocked=%s description=%q updated=%q",
		p.Title, p.Status, quoteOrNil(p.AssignedTo), quoteOrNil(p.BlockedReason),
		p.Description, p.UpdatedAt,
	)
}

func quoteOrNil(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%q", *s)
}

// readTaskState reads a task back through `task show --format json`.
//
// The assertion has to be on stored state, not on the command's exit
// code: the defect exited 0 while writing, so a code-only check passes
// against the bug.
func readTaskState(t *testing.T, bin, home string, env []string, alias string) (taskStateProbe, int) {
	t.Helper()

	out := runTLCOK(t, bin, home, env, "task", "show", alias, "--format", "json")

	var payload struct {
		Task taskStateProbe `json:"task"`
		Logs []struct {
			ID int `json:"id"`
		} `json:"logs"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse task show json for %s: %v\n%s", alias, err, out)
	}
	return payload.Task, len(payload.Logs)
}

// projectionSnapshot renders every todo.txt projection under home as
// path + contents + modification time, so a comparison catches a rewrite
// wherever the projection actually lands.
//
// The location is deliberately discovered rather than asserted:
// writeProjection resolves it through viper (task.todo_file) and through
// project detection, so hard-coding a path would pin the test to today's
// config resolution instead of to the behavior under test.
//
// The mtime is what makes this an assertion about the WRITE rather than
// about the contents. writeProjection re-renders the whole file from the
// store, so when the store is unchanged the bytes it writes are
// identical — a contents-only comparison passes while the file is being
// rewritten on every dry run, which is exactly the side effect --dry-run
// exists to suppress.
func projectionSnapshot(t *testing.T, home string) string {
	t.Helper()

	var parts []string
	err := filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".txt") {
			return nil //nolint:nilerr // an unreadable entry is not a projection
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil //nolint:nilerr // an unreadable entry is not a projection
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil //nolint:nilerr // an unstattable entry is not a projection
		}
		rel, relErr := filepath.Rel(home, path)
		if relErr != nil {
			rel = path
		}
		parts = append(parts, fmt.Sprintf("%s@%s:%s", rel, info.ModTime().UTC().Format(time.RFC3339Nano), body))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", home, err)
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

// dryRunCase is one lifecycle command, seeded into the state it needs and
// then invoked under --dry-run.
type dryRunCase struct {
	name string
	// setup moves the seeded task into the precondition this command
	// needs (e.g. reopen needs a terminal task).
	setup [][]string
	// args is the dry-run invocation, minus the trailing --dry-run.
	args []string
	// verb is the word the preview must use, as in "would claim".
	verb string
	// success is the line a real run prints; a dry run must not.
	success string
	// config overrides the status vocabulary for this case. Empty means
	// customStatusConfig, whose state machine is deliberately strict.
	config string
}

// lifecycleDryRunCases covers all ten commands the kit help advertises
// dry-run support for. T-0001 is the seeded task in every case.
func lifecycleDryRunCases() []dryRunCase {
	return []dryRunCase{
		{
			name:    "claim",
			args:    []string{"task", "claim", "T-0001"},
			verb:    "would claim",
			success: "Claimed task",
		},
		{
			// The built-in vocabulary, because unclaim walks
			// IN_PROGRESS -> TODO and passes force=false;
			// customStatusConfig forbids that edge, so the real path
			// refuses there too and the case would prove the state
			// machine rather than the dry-run guard.
			name:    "unclaim",
			config:  defaultStatusConfig,
			setup:   [][]string{{"task", "claim", "T-0001"}},
			args:    []string{"task", "unclaim", "T-0001"},
			verb:    "would unclaim",
			success: "Unclaimed task",
		},
		{
			name:    "assign",
			args:    []string{"task", "assign", "someone", "T-0001"},
			verb:    "would assign",
			success: "Assigned task",
		},
		{
			name:    "unassign",
			setup:   [][]string{{"task", "assign", "someone", "T-0001"}},
			args:    []string{"task", "unassign", "T-0001", "--note", "reason"},
			verb:    "would unassign",
			success: "Unassigned task",
		},
		{
			name:    "complete",
			setup:   [][]string{{"task", "claim", "T-0001"}},
			args:    []string{"task", "complete", "T-0001"},
			verb:    "would complete",
			success: "Completed task",
		},
		{
			name: "reopen",
			setup: [][]string{
				{"task", "claim", "T-0001"},
				{"task", "complete", "T-0001"},
			},
			args:    []string{"task", "reopen", "T-0001", "--note", "reason"},
			verb:    "would reopen",
			success: "Reopened task",
		},
		{
			// --no-verify because the fixture vocabulary forbids
			// TODO -> SKIPPED (TODO: [IN_PROGRESS]). Without it the
			// real path refuses too, and the case would prove the
			// state-machine gate rather than the dry-run guard.
			name:    "skip",
			args:    []string{"task", "skip", "T-0001", "--no-verify"},
			verb:    "would skip",
			success: "Skipped task",
		},
		{
			name:    "block",
			args:    []string{"task", "block", "T-0001", "--note", "waiting on upstream"},
			verb:    "would block",
			success: "Blocked task",
		},
		{
			name:    "unblock",
			setup:   [][]string{{"task", "block", "T-0001", "--note", "waiting"}},
			args:    []string{"task", "unblock", "T-0001"},
			verb:    "would unblock",
			success: "Unblocked task",
		},
		{
			name:    "update",
			args:    []string{"task", "update", "T-0001", "--title", "renamed by dry run"},
			verb:    "would update",
			success: "Updated task",
		},
		{
			// A status update takes a different route through
			// applyTaskFieldChanges than a plain field edit: the
			// transition writes its own log row BEFORE the task row is
			// written, so a guard placed only at the final write would
			// still leave a transition row behind.
			name:    "update --status",
			args:    []string{"task", "update", "T-0001", "--status", "IN_PROGRESS"},
			verb:    "would update",
			success: "Updated task",
		},
	}
}

// TestTaskLifecycleDryRunWritesNothing is the headline regression across
// all ten commands: after a dry run the task must be byte-identical to
// what it was before, log count included.
func TestTaskLifecycleDryRunWritesNothing(t *testing.T) {
	for _, tc := range lifecycleDryRunCases() {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.config
			if cfg == "" {
				cfg = customStatusConfig
			}
			bin, home, env := statusVocabFixture(t, cfg)
			runTLCOK(t, bin, home, env, "task", "create", "lifecycle probe")
			for _, step := range tc.setup {
				runTLCOK(t, bin, home, env, step...)
			}

			before, beforeLogs := readTaskState(t, bin, home, env, "T-0001")

			args := append(append([]string{}, tc.args...), "--dry-run")
			out, code := runTLC(t, bin, home, env, args...)
			if code != 0 {
				t.Fatalf("dry run exited %d:\n%s", code, out)
			}

			// Say what would happen, in the house phrasing `track
			// create --dry-run` and `task create --dry-run` use. The
			// success line here is the defect speaking.
			if !strings.Contains(out, "Dry run") {
				t.Errorf("dry-run output does not announce itself:\n%s", out)
			}
			if !strings.Contains(out, tc.verb) {
				t.Errorf("dry-run output does not say %q:\n%s", tc.verb, out)
			}
			if strings.Contains(out, tc.success) {
				t.Errorf("dry-run reported a real write (%q):\n%s", tc.success, out)
			}

			after, afterLogs := readTaskState(t, bin, home, env, "T-0001")
			if after.String() != before.String() {
				t.Errorf("dry run mutated the task\nbefore: %s\nafter:  %s", before, after)
			}
			if afterLogs != beforeLogs {
				t.Errorf("dry run wrote %d log entries", afterLogs-beforeLogs)
			}
		})
	}
}

// TestTaskLifecycleDryRunPreviewsEveryTask pins the batch half. These
// commands take `<task-id|pattern>...` and accumulate errors across the
// batch, so a preview that stopped after the first resolved task would
// under-report what a real run does.
func TestTaskLifecycleDryRunPreviewsEveryTask(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	runTLCOK(t, bin, home, env, "task", "create", "batch one")
	runTLCOK(t, bin, home, env, "task", "create", "batch two")
	runTLCOK(t, bin, home, env, "task", "create", "batch three")

	out := runTLCOK(t, bin, home, env,
		"task", "claim", "T-0001", "T-0002", "T-0003", "--dry-run")

	for _, alias := range []string{"T-0001", "T-0002", "T-0003"} {
		if !strings.Contains(out, alias) {
			t.Errorf("dry run did not preview %s:\n%s", alias, out)
		}
		state, _ := readTaskState(t, bin, home, env, alias)
		if state.Status != "TODO" {
			t.Errorf("%s left TODO? got status %q", alias, state.Status)
		}
		if state.AssignedTo != nil {
			t.Errorf("%s was assigned by a dry run: %q", alias, *state.AssignedTo)
		}
	}
}

// TestTaskLifecycleDryRunStillRefusesInvalidTransitions keeps the flag
// from becoming an escape hatch around the workflow rules. A dry run is a
// preview of a real run, so a transition the real path refuses must be
// refused here too — otherwise --dry-run reports success for a change
// that cannot happen.
//
// The seeded vocabulary forbids TODO -> DONE (TODO: [IN_PROGRESS]), so a
// bare `complete` on a fresh task is refused on the real path.
func TestTaskLifecycleDryRunStillRefusesInvalidTransitions(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	runTLCOK(t, bin, home, env, "task", "create", "transition probe")

	out, code := runTLC(t, bin, home, env, "task", "complete", "T-0001", "--dry-run")
	if code == 0 {
		t.Errorf("dry run accepted a transition the workflow forbids (exit 0):\n%s", out)
	}
}

// TestTaskLifecycleDryRunStillRequiresNote pins the same principle for
// the note gates: `unassign` and `reopen` refuse without --note, and a
// preview must be refused by exactly what refuses the real run.
func TestTaskLifecycleDryRunStillRequiresNote(t *testing.T) {
	for _, args := range [][]string{
		{"task", "unassign", "T-0001"},
		{"task", "reopen", "T-0001"},
		{"task", "block", "T-0001"},
	} {
		t.Run(args[1], func(t *testing.T) {
			bin, home, env := statusVocabFixture(t, customStatusConfig)
			runTLCOK(t, bin, home, env, "task", "create", "note probe")

			out, code := runTLC(t, bin, home, env, append(args, "--dry-run")...)
			if code == 0 {
				t.Errorf("dry run skipped the required-note gate (exit 0):\n%s", out)
			}
		})
	}
}

// TestTaskLifecycleDryRunSkipsProjection pins the subtler side effect.
// Every one of these commands ends in writeProjection(), which rewrites
// the todo.txt projection from the store — a write of its own, and a
// side effect of the flag whose whole job is to suppress side effects.
//
// The projection file must not appear at all: nothing before the dry run
// wrote one, so its existence afterwards can only come from the dry run.
func TestTaskLifecycleDryRunSkipsProjection(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	runTLCOK(t, bin, home, env, "task", "create", "projection probe")

	before := projectionSnapshot(t, home)
	// Put a gap between the seeding write and the dry run so a rewrite
	// lands on a distinguishable mtime. Without it a filesystem with
	// coarse timestamp granularity could stamp both writes identically
	// and the assertion would pass against a real rewrite.
	time.Sleep(1100 * time.Millisecond)
	runTLCOK(t, bin, home, env, "task", "claim", "T-0001", "--dry-run")
	after := projectionSnapshot(t, home)

	if after != before {
		t.Errorf("dry run rewrote the projection\nbefore: %q\nafter:  %q", before, after)
	}
}

// TestTaskUpdateDryRunDoesNotAutoCreateTrack covers the one write in the
// update path that is neither the task row nor a log row: `--track
// <name>` creates a track when the name does not resolve, mid-apply and
// before anything else is persisted. A preview that created it would
// leave a real track behind for an edit that never happened.
func TestTaskUpdateDryRunDoesNotAutoCreateTrack(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	runTLCOK(t, bin, home, env, "task", "create", "track probe")

	// --no-prompt is what lets auto-create fire at all off a TTY; without
	// it the command refuses before reaching the create and the case
	// would pass without ever exercising the guard.
	out, code := runTLC(t, bin, home, env,
		"task", "update", "T-0001", "--track", "invented-by-dry-run", "--no-prompt", "--dry-run")

	listed := runTLCOK(t, bin, home, env, "track", "list")
	if strings.Contains(listed, "invented-by-dry-run") {
		t.Errorf("dry run created a track (exit %d):\ndry run said:\n%s\ntracks:\n%s", code, out, listed)
	}
}

// TestTaskUpdateAmendDryRunWritesNothing covers `task update --amend`,
// the one update path that never reaches applyTaskFieldChanges: it
// rewrites the most recent log row in place through UpdateLogNote, a
// write no guard on the field-change path can see.
func TestTaskUpdateAmendDryRunWritesNothing(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	runTLCOK(t, bin, home, env, "task", "create", "amend probe")
	runTLCOK(t, bin, home, env, "task", "update", "T-0001", "--note", "original note")

	before := runTLCOK(t, bin, home, env, "task", "show", "T-0001", "--format", "json")

	out := runTLCOK(t, bin, home, env,
		"task", "update", "T-0001", "--amend", "--note", "rewritten by dry run", "--dry-run")

	if !strings.Contains(out, "Dry run") {
		t.Errorf("dry-run output does not announce itself:\n%s", out)
	}
	if strings.Contains(out, "Amended latest log") {
		t.Errorf("dry-run reported a real amend:\n%s", out)
	}

	after := runTLCOK(t, bin, home, env, "task", "show", "T-0001", "--format", "json")
	if after != before {
		t.Errorf("dry run rewrote a log note\nbefore: %s\nafter:  %s", before, after)
	}
	if strings.Contains(after, "rewritten by dry run") {
		t.Errorf("the amended note landed on a dry run:\n%s", after)
	}
}

// TestTaskUpdateAmendDryRunStillRefusesTerminal keeps --amend's own gate
// in force under the flag: amending a DONE task requires --force, and a
// preview must be refused by exactly what refuses the real amend.
func TestTaskUpdateAmendDryRunStillRefusesTerminal(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	runTLCOK(t, bin, home, env, "task", "create", "terminal amend probe")
	runTLCOK(t, bin, home, env, "task", "claim", "T-0001")
	runTLCOK(t, bin, home, env, "task", "complete", "T-0001")

	out, code := runTLC(t, bin, home, env,
		"task", "update", "T-0001", "--amend", "--note", "nope", "--dry-run")
	if code == 0 {
		t.Errorf("dry run skipped the terminal-task gate (exit 0):\n%s", out)
	}
}

// TestTaskLifecycleDryRunExitsZero states the contract plainly: a
// successful preview is a success.
func TestTaskLifecycleDryRunExitsZero(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	runTLCOK(t, bin, home, env, "task", "create", "exit probe")

	if out, code := runTLC(t, bin, home, env, "task", "claim", "T-0001", "--dry-run"); code != 0 {
		t.Errorf("successful dry run exited %d:\n%s", code, out)
	}
}
