package cli

// Behavioral conformance gate for kit's --dry-run contract.
//
// WHY THIS EXISTS
//
// Per kit ADR-0020 dry-run is default-allow BY TIER: any leaf annotated
// kit/side-effect: write-local | write-shared | destructive-local is
// automatically opted in, and kit appends
//
//	"Dry-run support: this command honors --dry-run."
//
// to that leaf's help. Kit tags the context; actually honoring the tag is
// the adopter's job. Nothing verified that it happened, so the help
// advertised a guarantee the code did not keep: commands exited 0, printed
// "Deleted task T-0002", and wrote the row anyway.
//
// The repo's two existing gates both miss this:
//
//   - TestStrictValidationPasses checks ANNOTATIONS, never behavior. It
//     passes with every one of those defects in place — asserting a command
//     CLAIMS a side-effect tier is not asserting it honors the claim.
//   - The 12fcc workflow scans cmd/, which is a 15-line shim. Every command
//     lives in internal/cli, so the scanned tree holds no command surface.
//
// WHAT THIS GATE DOES
//
//  1. Discovers the advertising set from the LIVE command tree (RootCmd) via
//     kitcli.IsDryRunSupported — the same predicate kit uses to decide
//     whether to append the help line. No hand-maintained list: a command
//     added tomorrow is discovered tomorrow.
//  2. Runs each exercisable command from dryRunExercises against a real
//     binary with --dry-run and asserts the STORE IS BYTE-IDENTICAL
//     afterwards. Exit codes are not evidence: every known defect exited 0.
//  3. Fails when a discovered command is in NEITHER the exercised set NOR
//     dryRunExemptions. A new advertising command therefore forces a
//     decision — write an exercise or write a justified exemption — instead
//     of silently inheriting a guarantee nobody checked.
//
// WHY A SUBPROCESS
//
// --dry-run is registered by the production root's persistent flag set, so
// an in-process cobra command constructed by a test will not parse it. The
// binary is the only surface where the flag exists as a user sees it.
//
// WHY THE SNAPSHOT IS GENERIC
//
// Hashing the DB file does not work: SQLite runs in WAL mode, so a mutation
// can live in the -wal sidecar while the main file's bytes are unchanged —
// a file hash reports "clean" for a command that just printed "Deleted
// track". Nor is a hand-picked field list safe: `task block --dry-run`
// writes blocked_reason AND a task_logs row, and a snapshot that named
// neither column called it clean. So snapshotStore dumps EVERY column of
// EVERY row of tasks, tracks and task_logs, ordered deterministically. A
// column added later is covered without anyone remembering to add it.

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	kitcli "hop.top/kit/go/console/cli"
	_ "modernc.org/sqlite"
)

// dryRunExercise describes how to drive one advertising command hard enough
// that a missing dry-run guard shows up as a store mutation.
//
// setup runs WITHOUT --dry-run to build the preconditions (a task in the
// right status, a linked track, ...). args is then run WITH --dry-run
// appended, and the store must not change.
type dryRunExercise struct {
	// command is the space-joined path as CommandPath renders it minus the
	// binary name, e.g. "task complete". It must match a discovered leaf.
	command string
	// setup runs before the snapshot. Each entry is a full argv.
	setup [][]string
	// args is the argv under test; "--dry-run" is appended by the runner.
	args []string
	// wantExit is the exit code the dry run should produce. Nearly always
	// 0 — a preview that errors is usually a broken fixture, not a pass.
	wantExit int
}

// dryRunExercises is the exercised set: advertising commands that can be
// driven generically against an isolated store.
//
// Not every advertising leaf belongs here, and forcing one in would buy a
// flaky gate, which is worse than no gate. Anything not exercisable needs an
// entry in dryRunExemptions with a reason instead.
var dryRunExercises = []dryRunExercise{
	// task create: the plain-title path. Only the --recipe branch ever read
	// the flag, so a plain create wrote the row and burned a sequence.
	{
		command: "task create",
		args:    []string{"task", "create", "preview only"},
	},

	// The claim/assign family. Each writes the task row plus a task_logs
	// entry, and several print a per-task success line.
	{
		command: "task claim",
		setup:   [][]string{{"task", "create", "claimable"}},
		args:    []string{"task", "claim", "T-0001"},
	},
	{
		command: "task unclaim",
		setup: [][]string{
			{"task", "create", "unclaimable"},
			{"task", "claim", "T-0001"},
		},
		// unclaim is destructive-local: kit's confirm gate refuses it at
		// exit 5 on closed stdin unless the bridged --no-prompt is passed.
		args: []string{"task", "unclaim", "T-0001", "--no-prompt"},
	},
	{
		command: "task assign",
		setup:   [][]string{{"task", "create", "assignable"}},
		args:    []string{"task", "assign", "alice", "T-0001"},
	},
	{
		command: "task unassign",
		setup: [][]string{
			{"task", "create", "unassignable"},
			{"task", "assign", "alice", "T-0001"},
		},
		// --note is required by the command itself; --no-prompt clears the
		// destructive-local confirm gate.
		args: []string{"task", "unassign", "T-0001", "--note", "reassigning", "--no-prompt"},
	},

	// Status transitions. complete needs IN_PROGRESS first — the state
	// machine refuses TODO -> DONE, and a run that dies at the state
	// machine proves nothing about the guard behind it.
	{
		command: "task complete",
		setup: [][]string{
			{"task", "create", "completable"},
			{"task", "claim", "T-0001"},
		},
		args: []string{"task", "complete", "T-0001"},
	},
	{
		command: "task reopen",
		setup: [][]string{
			{"task", "create", "reopenable"},
			{"task", "claim", "T-0001"},
			{"task", "complete", "T-0001"},
		},
		args: []string{"task", "reopen", "T-0001", "--note", "more work found"},
	},
	{
		command: "task skip",
		setup:   [][]string{{"task", "create", "skippable"}},
		args:    []string{"task", "skip", "T-0001"},
	},

	// block writes blocked_reason AND a task_logs row. It is the case that
	// caught an earlier narrower snapshot calling a mutation clean.
	{
		command: "task block",
		setup:   [][]string{{"task", "create", "blockable"}},
		args:    []string{"task", "block", "T-0001", "--reason", "waiting upstream"},
	},
	{
		command: "task unblock",
		setup: [][]string{
			{"task", "create", "unblockable"},
			{"task", "block", "T-0001", "--reason", "waiting upstream"},
		},
		args: []string{"task", "unblock", "T-0001"},
	},

	{
		command: "task update",
		setup:   [][]string{{"task", "create", "updatable"}},
		args:    []string{"task", "update", "T-0001", "--title", "renamed by a preview"},
	},
	{
		command: "task delete",
		setup:   [][]string{{"task", "create", "deletable"}},
		// --note satisfies the delete-requires-note policy; --no-prompt the
		// confirm gate. Without both the run dies before reaching the code
		// under test.
		args: []string{"task", "delete", "T-0001", "--note", "obsolete", "--no-prompt"},
	},

	// Track verbs. update and delete need no gate; abandon is
	// destructive-local and takes the bridged --no-prompt.
	{
		command: "track update",
		setup:   [][]string{{"track", "create", "renameable track", "--type", "feature"}},
		args:    []string{"track", "update", "renameable-track", "--title", "renamed by a preview"},
	},
	{
		command: "track delete",
		setup:   [][]string{{"track", "create", "deletable track", "--type", "feature"}},
		args:    []string{"track", "delete", "deletable-track"},
	},
	{
		command: "track abandon",
		setup:   [][]string{{"track", "create", "abandonable track", "--type", "feature"}},
		args:    []string{"track", "abandon", "abandonable-track", "--no-prompt"},
	},

	// track archive only accepts completed -> archived, and a track reaches
	// completed only with every linked task done. The setup is long because
	// the state machine demands it; shortening it would land the run on a
	// transition error instead of on the guard.
	{
		command: "track archive",
		setup: [][]string{
			{"track", "create", "archivable track", "--type", "feature"},
			{"task", "create", "linked work"},
			{"task", "update", "T-0001", "--track", "archivable-track"},
			{"track", "update", "archivable-track", "--status", "active"},
			{"task", "claim", "T-0001"},
			{"task", "complete", "T-0001"},
			{"track", "update", "archivable-track", "--status", "completed"},
		},
		args: []string{"track", "archive", "archivable-track", "--confirm", "yes"},
	},

	// track create already honors the flag. Kept exercised, not exempted:
	// this is the regression guard that keeps it honoring it.
	{
		command: "track create",
		args:    []string{"track", "create", "preview track", "--type", "feature"},
	},
}

// dryRunExemptions lists advertising commands deliberately NOT exercised,
// each with the reason. An entry here is a decision on the record, not an
// oversight — which is the whole point of failing on anything in neither
// set.
//
// Every exemption is a standing invitation to write a real exercise. None of
// these commands is known-correct; they are merely out of reach of a
// hermetic, non-flaky subprocess fixture.
var dryRunExemptions = map[string]string{
	// --- binds a port / blocks forever ---
	"serve": "starts an HTTP server and blocks; no store assertion available in a subprocess run",

	// --- reaches an external system ---
	"sync pull":   "contacts a remote issue tracker; hermetic fixture would need a full fake server",
	"sync push":   "contacts a remote issue tracker; also shadows kit's global flag with a local --dry-run",
	"sync config": "writes credentials for a remote system; needs a configured provider to reach the write",
	"auth logout": "mutates credential storage for an external provider, not the task store",

	// --- scaffolds a project / writes outside the store ---
	"init":           "scaffolds a project tree and config; effect is on the filesystem, not the store snapshot",
	"project init":   "same scaffolding path as init, one directory deeper",
	"project prune":  "operates on the global project registry rather than the project store the snapshot covers",
	"project import": "requires an export file fixture and rewrites the registry; not store-shaped",
	"project export": "writes an export file, not the store; nothing in the snapshot can move",

	// --- needs stdin, a binary, or an external artifact ---
	"audit record":   "requires --stdin with a JSON payload and --tool; also annotated kit/side-effect: write, outside the ADR-0020 tier set",
	"agent register": "requires a --binary or --image that must exist on the host",
	"agent cancel":   "requires a live async job id; nothing to cancel in a fresh store",
	"recipe import":  "requires a recipe file or URL fixture; the honoring path is covered by recipe_import_track_e2e_test.go",

	// --- config / registry surfaces outside the three snapshotted tables ---
	"config set":   "writes the config file, not the store",
	"alias add":    "writes the alias registry in config, not the store",
	"alias remove": "writes the alias registry in config, not the store",
	"uri register": "registers a URI handler with the OS, not the store",
	"label init":   "seeds label config; covered behaviorally by label_init_seed_e2e_test.go",

	// --- honors dry-run through its OWN local --dry-run flag ---
	// These register a local --dry-run pflag that shadows kit's global one.
	// The flag works from a user's point of view, but the value never
	// reaches kit, so kit's own opt-in is inert and this gate's mechanism
	// does not apply. Worth reconciling with ADR-0020 separately.
	"task reprioritise":    "local --dry-run flag shadows kit's global; honored, but not through kit's mechanism",
	"task sync-projection": "local --dry-run flag shadows kit's global; also a filesystem projection, not a store write",
	"tasks sync":           "deprecated alias with its own local --dry-run flag shadowing kit's global",

	// --- needs a task shape the fixture cannot make ---
	"task approve": "only applies to human-decision tasks; task create makes agent tasks, so the run dies before the guard",
	"task reject":  "only applies to human-decision tasks; same unreachable-guard problem as approve",

	// --- inbox ---
	"inbox process": "consumes an inbox directory fixture; the write path needs queued items that no store seed creates",
}

// discoverDryRunAdvertisers walks the production command tree and returns
// every leaf kit would append the dry-run help line to.
//
// This is deliberately in-process against RootCmd rather than scraped from
// `tlc help` output: help scraping missed four commands (alias add, alias
// remove, config set, tasks sync) whose help rendering differs, and a gate
// that under-reports the set it is supposed to police is no gate.
func discoverDryRunAdvertisers() []string {
	var found []string
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		children := 0
		for _, kid := range c.Commands() {
			// cobra's generated help/completion commands are not part of
			// the adopter's surface and carry no annotations.
			if kid.Name() == "help" || kid.Name() == "completion" {
				continue
			}
			children++
			walk(kid)
		}
		if children > 0 {
			return // not a leaf
		}
		if kitcli.IsDryRunSupported(c) {
			found = append(found, strings.TrimPrefix(c.CommandPath(), RootCmd.Name()+" "))
		}
	}
	walk(RootCmd)
	sort.Strings(found)
	return found
}

// TestDryRunAdvertisersAreAccountedFor is the coverage half of the gate.
//
// It does not run any command. It asserts only that the discovered
// advertising set is fully partitioned by dryRunExercises + dryRunExemptions
// — so adding a command that advertises dry-run support fails the build
// until someone either proves it honors the claim or records why it cannot
// be proven here.
//
// It also fails on stale entries pointing at commands that no longer
// advertise, which is how the registry avoids rotting in the other
// direction.
func TestDryRunAdvertisersAreAccountedFor(t *testing.T) {
	advertised := discoverDryRunAdvertisers()
	if len(advertised) == 0 {
		t.Fatal("discovered no dry-run advertising commands; the walk is broken, " +
			"not the tree — kit opts in every write|destructive leaf by tier")
	}

	exercised := map[string]bool{}
	for _, ex := range dryRunExercises {
		if exercised[ex.command] {
			t.Errorf("duplicate exercise for %q", ex.command)
		}
		exercised[ex.command] = true
	}

	advertisedSet := map[string]bool{}
	for _, cmd := range advertised {
		advertisedSet[cmd] = true
	}

	var unaccounted []string
	for _, cmd := range advertised {
		if exercised[cmd] {
			continue
		}
		if _, ok := dryRunExemptions[cmd]; ok {
			continue
		}
		unaccounted = append(unaccounted, cmd)
	}
	if len(unaccounted) > 0 {
		t.Errorf("these commands advertise \"honors --dry-run\" in their help but are "+
			"neither exercised nor exempted:\n  %s\n\n"+
			"kit opts a leaf in automatically once it is annotated kit/side-effect: "+
			"write-local | write-shared | destructive-local, and appends the support "+
			"line to its help. Either add a dryRunExercise proving it honors the flag, "+
			"or add a dryRunExemptions entry saying why it cannot be exercised here.",
			strings.Join(unaccounted, "\n  "))
	}

	for cmd := range dryRunExemptions {
		if !advertisedSet[cmd] {
			t.Errorf("stale exemption %q: no such command advertises dry-run support "+
				"any more; delete the entry", cmd)
		}
	}
	for _, ex := range dryRunExercises {
		if !advertisedSet[ex.command] {
			t.Errorf("stale exercise %q: no such command advertises dry-run support "+
				"any more; delete the exercise", ex.command)
		}
	}
	for cmd := range dryRunExemptions {
		if exercised[cmd] {
			t.Errorf("%q is both exercised and exempted; drop the exemption", cmd)
		}
	}
}

// TestDryRunDoesNotMutateStore is the behavioral half: for each exercised
// command, snapshot the store, run it with --dry-run, snapshot again, and
// require the two to be identical.
//
// The assertion is on STATE, not on the exit code. Every defect this gate
// was written to catch exited 0 while writing.
func TestDryRunDoesNotMutateStore(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("non-TTY pipe and confirm-gate assumptions are unix-only")
	}

	bin := buildTLCBinary(t)

	for _, ex := range dryRunExercises {
		t.Run(ex.command, func(t *testing.T) {
			t.Parallel()

			home := t.TempDir()
			dbPath := filepath.Join(home, "tasks.db")
			env := e2eEnv(t, home, dbPath)

			// Materialize the store before anything is snapshotted. On a
			// case with no setup the DB file does not exist yet, and the
			// command under test creates it by running migrations — so the
			// before/after pair would differ as "<unreadable>" vs "rows=0"
			// for a command that wrote nothing. That is file creation, not
			// mutation, and reporting it as a defect would make the gate
			// cry wolf on the one command that already honors the flag.
			runTLCOK(t, bin, home, env, "task", "list")

			for _, argv := range ex.setup {
				out, code := runTLC(t, bin, home, env, argv...)
				if code != 0 {
					t.Fatalf("setup %v failed with exit %d — the fixture is broken, "+
						"so this case proves nothing about %q:\n%s",
						argv, code, ex.command, out)
				}
			}

			before := snapshotStore(t, dbPath)

			args := append(append([]string{}, ex.args...), "--dry-run")
			out, code := runTLC(t, bin, home, env, args...)
			if code != ex.wantExit {
				t.Fatalf("`tlc %s` exited %d, want %d — a dry run that cannot even "+
					"reach the command body proves nothing:\n%s",
					strings.Join(args, " "), code, ex.wantExit, out)
			}

			after := snapshotStore(t, dbPath)

			if diff := snapshotDiff(before, after); diff != "" {
				t.Errorf("`tlc %s` MUTATED the store.\n\n"+
					"Its help says \"Dry-run support: this command honors --dry-run.\" "+
					"(kit appends that line to every leaf annotated write-local | "+
					"write-shared | destructive-local). The command exited %d and wrote "+
					"anyway, so the help is lying to users.\n\n"+
					"Command output:\n%s\nStore changes:\n%s",
					strings.Join(args, " "), code, indent(out), diff)
			}
		})
	}
}

// snapshotStore returns a stable, complete rendering of every row in the
// three tables a task-store mutation can land in.
//
// Every column is dumped rather than a chosen subset. A list of interesting
// fields is a list that goes stale the first time someone adds a column, and
// the failure mode is silent: the gate keeps passing while no longer
// covering the new field.
//
// Both tasks and task_logs matter. `task block --dry-run` wrote
// blocked_reason on the row AND appended a task_logs entry; a snapshot
// covering either one alone would have reported half a mutation, and a
// snapshot covering neither reported none. Note the audit table is
// task_logs, not logs.
func snapshotStore(t *testing.T, dbPath string) map[string]string {
	t.Helper()

	// Read-only, and through the driver rather than over the file bytes:
	// the DB is in WAL mode, so a committed write can sit in the -wal
	// sidecar with the main file unchanged. Hashing the file reported
	// "clean" for a command that had just printed "Deleted track".
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatalf("open store read-only: %v", err)
	}
	defer func() { _ = db.Close() }()

	snap := map[string]string{}
	for _, table := range []string{"tasks", "tracks", "task_logs"} {
		snap[table] = dumpTable(t, db, table)
	}
	return snap
}

// dumpTable renders every column of every row of one table, ordered by the
// full row text so the result does not depend on physical row order.
func dumpTable(t *testing.T, db *sql.DB, table string) string {
	t.Helper()

	// Table names cannot be bound as query parameters. These three are
	// compile-time constants supplied by snapshotStore, never user input.
	rows, err := db.QueryContext(t.Context(), "SELECT * FROM "+table)
	if err != nil {
		// Fatal, never a recorded value. An unreadable table that compared
		// equal on both sides would let a whole table drop out of the gate
		// unnoticed; one that compared unequal would report file creation
		// as a mutation. The caller materializes the store first, so a
		// failure here means the fixture is broken.
		t.Fatalf("read %s (the store should already exist by now): %v", table, err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("columns of %s: %v", table, err)
	}

	var lines []string
	for rows.Next() {
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatalf("scan %s: %v", table, err)
		}
		parts := make([]string, 0, len(cols))
		for i, c := range cols {
			parts = append(parts, fmt.Sprintf("%s=%v", c, cells[i]))
		}
		lines = append(lines, strings.Join(parts, " "))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate %s: %v", table, err)
	}

	sort.Strings(lines)
	return fmt.Sprintf("rows=%d\n%s", len(lines), strings.Join(lines, "\n"))
}

// snapshotDiff renders the difference between two snapshots, or "" when they
// match. The output names the table and shows both sides so a failure says
// what moved rather than only that something did.
func snapshotDiff(before, after map[string]string) string {
	var b strings.Builder
	tables := make([]string, 0, len(before))
	for table := range before {
		tables = append(tables, table)
	}
	sort.Strings(tables)

	for _, table := range tables {
		if before[table] == after[table] {
			continue
		}
		fmt.Fprintf(&b, "  table %s changed:\n", table)
		fmt.Fprintf(&b, "    before:\n%s\n", indentBy(before[table], "      "))
		fmt.Fprintf(&b, "    after:\n%s\n", indentBy(after[table], "      "))
	}
	return b.String()
}

func indent(s string) string { return indentBy(s, "  ") }

func indentBy(s, pad string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}
