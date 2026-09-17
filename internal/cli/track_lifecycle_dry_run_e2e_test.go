package cli

// The track lifecycle verbs ignored --dry-run. `update`, `delete`,
// `abandon` and `archive` all reached storage and wrote, while kit's
// generated help advertised dry-run support for every one of them
// (kit tags any leaf annotated write-local / destructive-local and
// appends "Dry-run support: this command honors --dry-run."). Honoring
// the flag is the adopter's job and nothing verified it, so the help
// actively lied: `track abandon --dry-run` skipped every linked task.
//
// These run the real binary rather than the in-process tree: --dry-run
// is a kit-owned persistent flag registered by the production root, so
// an in-process test cmd would not even parse it.
//
// Every case asserts on STATE read back afterwards, not on the exit
// code. The defect exited 0 while writing, so an exit-code-only
// assertion passes against it.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dryRunTrackRow is the shape `track list --format json` emits, narrowed to
// the fields these tests assert on. Slugs are derived from the title
// ("probe track" -> "probe-track"), so the title is not itself a usable
// handle and the row is read back rather than guessed at.
type dryRunTrackRow struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// allTrackStatuses names every track status explicitly. `track list`
// has no --archived flag and its default view hides archived rows, so
// an archive would otherwise read as a disappearance rather than as the
// status change it is.
var allTrackStatuses = []string{
	"pending", "active", "completed", "abandoned", "archived",
}

// listTracks reads the track table back through the CLI's own JSON
// output, across every status.
func dryRunListTracks(t *testing.T, bin, home string, env []string) []dryRunTrackRow {
	t.Helper()
	args := make([]string, 0, 4+2*len(allTrackStatuses))
	args = append(args, "track", "list")
	for _, st := range allTrackStatuses {
		args = append(args, "--status", st)
	}
	args = append(args, "--format", "json")
	out := runTLCOK(t, bin, home, env, args...)
	var rows []dryRunTrackRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("parse track list json: %v\n%s", err, out)
	}
	return rows
}

// trackStatus returns the status of the track with the given slug, or
// "" when no such row exists (which is how a delete reads).
func dryRunTrackStatus(t *testing.T, bin, home string, env []string, slug string) string {
	t.Helper()
	for _, r := range dryRunListTracks(t, bin, home, env) {
		if r.Slug == slug {
			return r.Status
		}
	}
	return ""
}

// dryRunTaskRow narrows `task list --format json` to the fields under test.
type dryRunTaskRow struct {
	Title  string `json:"title"`
	Status string `json:"status"`
}

// listTasks reads every task back regardless of status. `task list`
// defaults to unfinished work only, so a task the dry run wrongly
// skipped would vanish from the default view rather than show up as
// SKIPPED — the statuses are named explicitly to see both.
func dryRunListTasks(t *testing.T, bin, home string, env []string) []dryRunTaskRow {
	t.Helper()
	out := runTLCOK(t, bin, home, env, "task", "list",
		"--status", "TODO", "--status", "IN_PROGRESS",
		"--status", "IN_REVIEW", "--status", "DONE", "--status", "SKIPPED",
		"--format", "json")
	var rows []dryRunTaskRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("parse task list json: %v\n%s", err, out)
	}
	return rows
}

// taskStatus returns the status of the task with the given title, or ""
// when absent.
func dryRunTaskStatus(t *testing.T, bin, home string, env []string, title string) string {
	t.Helper()
	for _, r := range dryRunListTasks(t, bin, home, env) {
		if r.Title == title {
			return r.Status
		}
	}
	return ""
}

// seedTrack creates one track and returns the slug the CLI derived for
// it. The slug comes from the title rather than being the title, so it
// is resolved from the store instead of assumed.
func dryRunSeedTrack(t *testing.T, bin, home string, env []string, title string) string {
	t.Helper()
	runTLCOK(t, bin, home, env, "track", "create", title)
	for _, r := range dryRunListTracks(t, bin, home, env) {
		if r.Title == title {
			return r.Slug
		}
	}
	t.Fatalf("seeded track %q not found in listing", title)
	return ""
}

// TestTrackUpdateDryRunWritesNothing is the headline regression for
// `track update`: it called svc.UpdateTrack unconditionally, so the row
// changed and the output said "Updated track <slug>".
func TestTrackUpdateDryRunWritesNothing(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	slug := dryRunSeedTrack(t, bin, home, env, "probe track")

	out := runTLCOK(t, bin, home, env,
		"track", "update", slug, "--title", "mutated title", "--dry-run")

	if !strings.Contains(out, "Dry run") {
		t.Errorf("dry-run output does not announce itself:\n%s", out)
	}
	if strings.Contains(out, "Updated track") {
		t.Errorf("dry-run reported an update:\n%s", out)
	}

	for _, r := range dryRunListTracks(t, bin, home, env) {
		if r.Title == "mutated title" {
			t.Fatalf("dry-run wrote the new title:\n%+v", r)
		}
	}
	if got := dryRunTrackStatus(t, bin, home, env, slug); got != "pending" {
		t.Errorf("track status changed under dry run: got %q, want pending", got)
	}
}

// TestTrackUpdateDryRunStillValidates keeps --dry-run from becoming an
// escape hatch around the rules. An invalid type is refused on a real
// run, so it must be refused on a preview too — otherwise the preview
// reports success for an update that cannot happen.
func TestTrackUpdateDryRunStillValidates(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	slug := dryRunSeedTrack(t, bin, home, env, "probe track")

	out, code := runTLC(t, bin, home, env,
		"track", "update", slug, "--type", "NOPE", "--dry-run")
	if code == 0 {
		t.Errorf("dry-run accepted an invalid track type (exit 0):\n%s", out)
	}
}

// TestTrackUpdateDryRunRefusesIllegalTransition pins the state machine
// half: pending -> archived is not a legal track transition, and the
// preview must be refused by exactly what would refuse the real run.
func TestTrackUpdateDryRunRefusesIllegalTransition(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	slug := dryRunSeedTrack(t, bin, home, env, "probe track")

	out, code := runTLC(t, bin, home, env,
		"track", "update", slug, "--status", "archived", "--dry-run")
	if code == 0 {
		t.Errorf("dry-run accepted an illegal transition (exit 0):\n%s", out)
	}
	if !strings.Contains(out, "transition") {
		t.Errorf("refusal does not name the transition rule:\n%s", out)
	}
}

// TestTrackUpdateAddPlanDryRunCreatesNoTasks covers the compound half:
// --add-plan links the plan AND ingests tasks from its frontmatter, so
// a dry run that only suppressed the track write would still create
// every task in the plan.
func TestTrackUpdateAddPlanDryRunCreatesNoTasks(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	slug := dryRunSeedTrack(t, bin, home, env, "plan track")

	planPath := filepath.Join(home, "plan.md")
	plan := "---\ntasks:\n  - title: plan task alpha\n  - title: plan task beta\n---\n\n# Plan\n"
	if err := os.WriteFile(planPath, []byte(plan), 0o600); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	out := runTLCOK(t, bin, home, env,
		"track", "update", slug, "--add-plan", planPath, "--dry-run")

	if !strings.Contains(out, "Dry run") {
		t.Errorf("dry-run output does not announce itself:\n%s", out)
	}
	// Requirement 4: a compound preview must state its blast radius.
	if !strings.Contains(out, "plan task alpha") {
		t.Errorf("dry-run does not name the tasks it would create:\n%s", out)
	}

	for _, r := range dryRunListTasks(t, bin, home, env) {
		if strings.HasPrefix(r.Title, "plan task ") {
			t.Fatalf("dry-run created tasks from the plan:\n%+v", r)
		}
	}
}

// TestTrackDeleteDryRunWritesNothing: delete removed the row and said
// "Deleted track <slug>".
func TestTrackDeleteDryRunWritesNothing(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	slug := dryRunSeedTrack(t, bin, home, env, "probe track")

	// --confirm yes satisfies kit's own destructive-tier gate, which
	// refuses these verbs outright in a non-TTY. That gate runs in
	// kit's RunE wrapper, ahead of anything here; without it the run
	// never reaches the code under test.
	out := runTLCOK(t, bin, home, env,
		"track", "delete", slug, "--dry-run", "--confirm", "yes")

	if !strings.Contains(out, "Dry run") {
		t.Errorf("dry-run output does not announce itself:\n%s", out)
	}
	if strings.Contains(out, "Deleted track") {
		t.Errorf("dry-run reported a delete:\n%s", out)
	}
	if got := dryRunTrackStatus(t, bin, home, env, slug); got == "" {
		t.Errorf("dry-run deleted the track; it is gone from the listing")
	}
}

// TestTrackDeleteDryRunRefusesLinkedTasks: delete is refused while the
// track still owns tasks, and that refusal lives in the storage
// transaction the dry run never reaches. A preview that skipped it
// would report a would-delete for a delete that cannot happen.
func TestTrackDeleteDryRunRefusesLinkedTasks(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	slug := dryRunSeedTrack(t, bin, home, env, "probe track")
	runTLCOK(t, bin, home, env, "task", "create", "linked one", "--track", slug)

	out, code := runTLC(t, bin, home, env,
		"track", "delete", slug, "--dry-run", "--confirm", "yes")
	if code == 0 {
		t.Errorf("dry-run accepted deleting a track with linked tasks (exit 0):\n%s", out)
	}
	if !strings.Contains(out, "linked task") {
		t.Errorf("refusal does not name the linked-task rule:\n%s", out)
	}
}

// TestTrackArchiveDryRunWritesNothing. Archive could not be reached
// from a fresh track: pending -> archived is refused by the state
// machine, so the track is first walked to abandoned, from which
// archive is legal. It then wrote: abandoned -> archived, exit 0,
// "Archived track <slug>".
func TestTrackArchiveDryRunWritesNothing(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	slug := dryRunSeedTrack(t, bin, home, env, "probe track")

	// Walk to a status archive is legal from, for real.
	runTLCOK(t, bin, home, env, "track", "abandon", slug, "--no-prompt")
	if got := dryRunTrackStatus(t, bin, home, env, slug); got != "abandoned" {
		t.Fatalf("setup: track is %q, want abandoned", got)
	}

	out := runTLCOK(t, bin, home, env,
		"track", "archive", slug, "--dry-run", "--confirm", "yes")

	if !strings.Contains(out, "Dry run") {
		t.Errorf("dry-run output does not announce itself:\n%s", out)
	}
	if strings.Contains(out, "Archived track") {
		t.Errorf("dry-run reported an archive:\n%s", out)
	}
	if got := dryRunTrackStatus(t, bin, home, env, slug); got != "abandoned" {
		t.Errorf("dry-run archived the track: status is %q, want abandoned", got)
	}
}

// TestTrackArchiveDryRunRefusesIllegalTransition: the preview of an
// archive the state machine would refuse must be refused too, rather
// than reporting a would-archive that cannot happen.
func TestTrackArchiveDryRunRefusesIllegalTransition(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	slug := dryRunSeedTrack(t, bin, home, env, "probe track")

	out, code := runTLC(t, bin, home, env,
		"track", "archive", slug, "--dry-run", "--confirm", "yes")
	if code == 0 {
		t.Errorf("dry-run accepted archiving a pending track (exit 0):\n%s", out)
	}
	if !strings.Contains(out, "transition") {
		t.Errorf("refusal does not name the transition rule:\n%s", out)
	}
}

// TestTrackAbandonDryRunSkipsNoTasks is the highest-risk case in the
// set. abandon is compound: it transitions the track AND skips every
// linked non-terminal task. Under --dry-run it did both — two TODO
// tasks came back SKIPPED with no way to undo them.
func TestTrackAbandonDryRunSkipsNoTasks(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	slug := dryRunSeedTrack(t, bin, home, env, "blast track")

	runTLCOK(t, bin, home, env, "task", "create", "linked one", "--track", slug)
	runTLCOK(t, bin, home, env, "task", "create", "linked two", "--track", slug)

	out := runTLCOK(t, bin, home, env,
		"track", "abandon", slug, "--dry-run", "--no-prompt")

	if !strings.Contains(out, "Dry run") {
		t.Errorf("dry-run output does not announce itself:\n%s", out)
	}
	if strings.Contains(out, "Abandoned track") {
		t.Errorf("dry-run reported an abandon:\n%s", out)
	}
	// Requirement 4: the preview states the full blast radius.
	if !strings.Contains(out, "linked one") || !strings.Contains(out, "linked two") {
		t.Errorf("dry-run does not name the tasks it would skip:\n%s", out)
	}

	if got := dryRunTrackStatus(t, bin, home, env, slug); got != "pending" {
		t.Errorf("dry-run abandoned the track: status %q, want pending", got)
	}
	for _, title := range []string{"linked one", "linked two"} {
		if got := dryRunTaskStatus(t, bin, home, env, title); got != "TODO" {
			t.Errorf("dry-run skipped task %q: status %q, want TODO", title, got)
		}
	}
}

// TestTrackAbandonDryRunRefusedByConfirmation keeps the confirmation
// gate in front of the preview. A dry run is refused by exactly what
// refuses a real run, and the destructive-tier gate is one of those
// things — otherwise --dry-run becomes a way around a gate the
// command's own help says is there.
//
// The gate is kit's, installed on the destructive tier and enforced in
// its RunE wrapper, so it refuses before reaching this package. The
// test pins that it is still reached on a dry run rather than skipped
// as "no side effects, no confirmation needed".
func TestTrackAbandonDryRunRefusedByConfirmation(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)
	slug := dryRunSeedTrack(t, bin, home, env, "blast track")
	runTLCOK(t, bin, home, env, "task", "create", "linked one", "--track", slug)

	// Neither --no-prompt (which bridges to --confirm=yes) nor an
	// explicit --confirm: the non-TTY default is "no".
	out, code := runTLC(t, bin, home, env, "track", "abandon", slug, "--dry-run")
	if code == 0 {
		t.Errorf("dry-run bypassed the confirmation gate (exit 0):\n%s", out)
	}

	// And it stopped at the gate rather than partially applying.
	if got := dryRunTrackStatus(t, bin, home, env, slug); got != "pending" {
		t.Errorf("refused run still changed the track: status %q, want pending", got)
	}
	if got := dryRunTaskStatus(t, bin, home, env, "linked one"); got != "TODO" {
		t.Errorf("refused run still skipped the task: status %q, want TODO", got)
	}
}
