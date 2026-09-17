package cli

// The plain-title `task create` path ignored --dry-run: it called
// saveTask unconditionally, so the row landed, the per-project sequence
// number was consumed and the output said "Created task T-NNNN". Only
// the --recipe branch honored the flag, while both the long help and
// kit's generated help advertised dry-run support for the whole command.
//
// These run the real binary rather than the in-process tree: --dry-run
// is a kit-owned persistent flag registered by the production root, so
// an in-process test cmd would not even parse it.

import (
	"strings"
	"testing"
)

// TestTaskCreateDryRunWritesNothing is the headline regression: the
// store must be untouched afterwards.
//
// The assertion is on what `task list` reports, not on the create's own
// exit code — the bug exited 0 while writing, so a code check alone
// would have passed against the defect.
func TestTaskCreateDryRunWritesNothing(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "seed one")

	out := runTLCOK(t, bin, home, env, "task", "create", "dry run probe", "--dry-run")

	// Say what would happen, in the house phrasing the recipe path and
	// `track create` already use. "Created task" here is the defect.
	if !strings.Contains(out, "Dry run") {
		t.Errorf("dry-run output does not announce itself:\n%s", out)
	}
	if strings.Contains(out, "Created task") {
		t.Errorf("dry-run reported a create:\n%s", out)
	}
	if !strings.Contains(out, "dry run probe") {
		t.Errorf("dry-run output does not name the task it would create:\n%s", out)
	}

	listed := runTLCOK(t, bin, home, env, "task", "list")
	if strings.Contains(listed, "dry run probe") {
		t.Errorf("dry-run created a task:\n%s", listed)
	}
	if !strings.Contains(listed, "seed one") {
		t.Errorf("the seeded task went missing:\n%s", listed)
	}
}

// TestTaskCreateDryRunDoesNotConsumeSeq pins the subtler half. Seq is a
// per-project monotonic counter allocated on insert, so a dry run that
// reached storage would burn T-0002 and leave the next real create at
// T-0003 — a gap in the alias sequence with no task behind it.
func TestTaskCreateDryRunDoesNotConsumeSeq(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "first real")
	runTLCOK(t, bin, home, env, "task", "create", "dry run probe", "--dry-run")
	out := runTLCOK(t, bin, home, env, "task", "create", "second real")

	if !strings.Contains(out, "T-0002") {
		t.Errorf("dry run consumed a sequence number; next create was not T-0002:\n%s", out)
	}
}

// TestTaskCreateDryRunStillValidates keeps the flag from becoming an
// escape hatch around the rules. A dry run is a preview of a real run,
// so an input the real path would refuse must be refused here too —
// otherwise --dry-run reports success for a create that cannot happen.
func TestTaskCreateDryRunStillValidates(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)

	out, code := runTLC(t, bin, home, env,
		"task", "create", "bogus probe", "--status", "NOPE", "--dry-run")
	if code == 0 {
		t.Errorf("dry-run accepted an invalid status (exit 0):\n%s", out)
	}
}
