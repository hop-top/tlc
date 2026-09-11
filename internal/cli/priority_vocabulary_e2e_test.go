package cli

// End-to-end coverage for the config-driven task-priority vocabulary.
//
// Driven through a real config FILE and the real binary, for the reason
// spelled out in status_vocabulary_e2e_test.go: struct-level tests pass
// while the CLI's own gate rejects the value in the field, because they
// never enter through the door the user does. Every case below spawns the
// binary with a config file whose db_path is pinned — an unpinned probe
// resolves the DB through the global project registry, not the cwd, and
// would read an unrelated real database.

import (
	"encoding/json"
	"strings"
	"testing"
)

// customPriorityConfig renames the whole vocabulary. The names are chosen
// so that alphabetical order (LATER, NORMAL, URGENT) is exactly the
// REVERSE of rank order — that is what makes the sort assertions able to
// tell a rank sort from a text sort. With the built-in P0..P3 the two
// coincide, which is why lexicographic sorting survived unnoticed.
const customPriorityConfig = `storage:
  db_path: %s
task:
  priorities:
    - name: URGENT
      label: Urgent
    - name: NORMAL
      label: Normal
    - name: LATER
      label: Later
`

// defaultPriorityConfig declares no priorities, so the built-in
// vocabulary applies. Pins "unchanged config behaves as before".
const defaultPriorityConfig = `storage:
  db_path: %s
`

// badSchedulingConfig names a priority outside the vocabulary in
// by_priority. Before validation this key was silently unreachable.
const badSchedulingConfig = `storage:
  db_path: %s
task:
  priorities:
    - name: URGENT
      label: Urgent
    - name: LATER
      label: Later
  scheduling:
    by_priority:
      NOSUCH:
        due: 24h
`

// goodSchedulingConfig names an in-vocabulary priority, so the rule must
// both validate and actually fire.
const goodSchedulingConfig = `storage:
  db_path: %s
task:
  scheduling:
    by_priority:
      P0:
        due: 24h
`

// priorityVocabFixture builds the isolated world for one case. Shares the
// config writer and env builder with the status suite; only the templates
// differ.
func priorityVocabFixture(t *testing.T, template string) (bin, home string, env []string) {
	t.Helper()
	return statusVocabFixture(t, template)
}

// showPriority returns the priority actually stored on a task, read back
// as JSON rather than scraped from the table. Asserting on exit code
// alone cannot distinguish "accepted URGENT" from "silently fuzzy-matched
// URGENT onto some other value and accepted that".
func showPriority(t *testing.T, bin, home string, env []string, id string) string {
	t.Helper()
	out := runTLCOK(t, bin, home, env, "task", "show", id, "--format", "json")
	var payload struct {
		Task struct {
			Priority string `json:"priority"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode task show json: %v\n%s", err, out)
	}
	return payload.Task.Priority
}

// TestConfiguredPriorityAccepted is the headline regression: a priority
// that exists only in the user's config must be accepted on write.
func TestConfiguredPriorityAccepted(t *testing.T) {
	bin, home, env := priorityVocabFixture(t, customPriorityConfig)

	out, code := runTLC(t, bin, home, env, "task", "create", "fix it", "-p", "URGENT")
	if code != 0 {
		t.Fatalf("-p URGENT rejected (exit %d):\n%s", code, out)
	}
	if got := showPriority(t, bin, home, env, "T-0001"); got != "URGENT" {
		t.Errorf("stored priority = %q, want URGENT", got)
	}

	// The update path is a separate gate with its own error wording;
	// both read the same canon, so both must accept.
	runTLCOK(t, bin, home, env, "task", "update", "T-0001", "--priority", "LATER")
	if got := showPriority(t, bin, home, env, "T-0001"); got != "LATER" {
		t.Errorf("after update, stored priority = %q, want LATER", got)
	}
}

// TestUnknownPriorityNamesConfiguredSet pins the rejection message to the
// CONFIGURED vocabulary. A message still naming P0-P3 would tell a user
// with URGENT declared that the built-ins they no longer have are the
// only legal values.
func TestUnknownPriorityNamesConfiguredSet(t *testing.T) {
	bin, home, env := priorityVocabFixture(t, customPriorityConfig)

	out, code := runTLC(t, bin, home, env, "task", "create", "bogus", "-p", "P0")
	if code == 0 {
		t.Fatalf("-p P0 should be rejected under a renamed vocabulary:\n%s", out)
	}
	for _, want := range []string{"URGENT", "NORMAL", "LATER"} {
		if !strings.Contains(out, want) {
			t.Errorf("rejection should name %q, got:\n%s", want, out)
		}
	}
	// P0 appears in the message as the rejected INPUT, so its presence
	// proves nothing; the leak worth catching is the rest of the
	// built-in set showing up as though it were still legal.
	for _, leak := range []string{"P1", "P2", "P3"} {
		if strings.Contains(out, leak) {
			t.Errorf("rejection leaked built-in %q under a renamed vocabulary:\n%s", leak, out)
		}
	}
}

// TestConfiguredPriorityInHelpAndCompletion covers discoverability. kit
// stamps flag enums in prepareTree, before argv is parsed and long before
// any config file is read, so the configured vocabulary reaches --help
// and the shell only via the post-config restamp.
func TestConfiguredPriorityInHelpAndCompletion(t *testing.T) {
	bin, home, env := priorityVocabFixture(t, customPriorityConfig)

	help := runTLCOK(t, bin, home, env, "task", "create", "--help")
	if !strings.Contains(help, "URGENT") {
		t.Errorf("--help should advertise the configured vocabulary, got:\n%s", help)
	}
	if strings.Contains(help, "one of: P0, P1, P2, P3") {
		t.Errorf("--help still shows the pre-config built-in set:\n%s", help)
	}

	comp := runTLCOK(t, bin, home, env, "__complete", "task", "create", "--priority", "")
	if !strings.Contains(comp, "URGENT") {
		t.Errorf("completion should offer URGENT, got:\n%s", comp)
	}
	if strings.Contains(comp, "P0") {
		t.Errorf("completion leaked the built-in set:\n%s", comp)
	}

	// --status is a different vocabulary on a different flag; assert the
	// priority restamp did not reach it.
	if strings.Contains(help, "one of: URGENT, NORMAL, LATER) ") &&
		strings.Contains(help, "--status") {
		t.Errorf("priority vocabulary may have leaked onto --status:\n%s", help)
	}
}

// TestDefaultPriorityVocabularyUnchanged is the no-regression contract. A
// config declaring no priorities must behave exactly as before —
// semantic aliases, numeric shorthands and fuzzy matching included. This
// is what stops the config-driven canon from quietly costing existing
// users their shorthands.
func TestDefaultPriorityVocabularyUnchanged(t *testing.T) {
	bin, home, env := priorityVocabFixture(t, defaultPriorityConfig)

	cases := []struct {
		input string
		want  string
	}{
		{"P0", "P0"},       // canonical
		{"p0", "P0"},       // derived lowercase variant
		{"critical", "P0"}, // hand-written semantic alias
		{"high", "P1"},
		{"medium", "P2"},
		{"med", "P2"},
		{"low", "P3"},
		{"0", "P0"}, // numeric shorthand
		{"3", "P3"},
		{"crit", "P0"}, // fuzzy
		{"lo", "P3"},
	}
	for i, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			out, code := runTLC(t, bin, home, env, "task", "create", "alias probe", "-p", tc.input)
			if code != 0 {
				t.Fatalf("-p %s rejected (exit %d):\n%s", tc.input, code, out)
			}
			id := seqTaskID(i + 1)
			if got := showPriority(t, bin, home, env, id); got != tc.want {
				t.Errorf("-p %s stored %q, want %q", tc.input, got, tc.want)
			}
		})
	}

	// The rejection message must still name exactly the built-in four,
	// with nothing from another config leaking in.
	rej, code := runTLC(t, bin, home, env, "task", "create", "bogus", "-p", "NOSUCHPRIORITY")
	if code == 0 {
		t.Fatalf("-p NOSUCHPRIORITY should have been rejected:\n%s", rej)
	}
	if !strings.Contains(rej, "P0, P1, P2, P3") {
		t.Errorf("default rejection should name the built-in four verbatim, got:\n%s", rej)
	}
	for _, leak := range []string{"URGENT", "NORMAL", "LATER"} {
		if strings.Contains(rej, leak) {
			t.Errorf("default vocabulary leaked %q from another config:\n%s", leak, rej)
		}
	}

	// And --help must still advertise the built-ins.
	help := runTLCOK(t, bin, home, env, "task", "create", "--help")
	if !strings.Contains(help, "one of: P0, P1, P2, P3") {
		t.Errorf("default --help should name the built-in four, got:\n%s", help)
	}
}

// TestPriorityOptional pins the one way priority differs from status: it
// is optional, and "unset" must stay a distinct third state rather than
// collapsing into the least urgent value.
func TestPriorityOptional(t *testing.T) {
	bin, home, env := priorityVocabFixture(t, customPriorityConfig)

	runTLCOK(t, bin, home, env, "task", "create", "no priority here")
	if got := showPriority(t, bin, home, env, "T-0001"); got != "" {
		t.Errorf("task created without -p has priority %q, want empty", got)
	}

	// It must remain listable and showable, not filtered out or errored.
	out, code := runTLC(t, bin, home, env, "task", "list")
	if code != 0 {
		t.Fatalf("task list failed with an unprioritised task (exit %d):\n%s", code, out)
	}
	if !strings.Contains(out, "T-0001") {
		t.Errorf("unprioritised task missing from listing:\n%s", out)
	}
}

// TestPrioritySortUsesConfiguredRank is the ordering contract. LATER,
// NORMAL, URGENT is the alphabetical order and the exact reverse of the
// declared rank order, so a lexicographic sort and a rank sort produce
// opposite answers and this test can tell them apart.
//
// Unset sorts last in BOTH directions: "never triaged" is not a rank, and
// it must not outrank a task the user deliberately marked least urgent.
func TestPrioritySortUsesConfiguredRank(t *testing.T) {
	bin, home, env := priorityVocabFixture(t, customPriorityConfig)

	runTLCOK(t, bin, home, env, "task", "create", "later task", "-p", "LATER")
	runTLCOK(t, bin, home, env, "task", "create", "urgent task", "-p", "URGENT")
	runTLCOK(t, bin, home, env, "task", "create", "unset task")
	runTLCOK(t, bin, home, env, "task", "create", "normal task", "-p", "NORMAL")

	asc := listedIDs(t, bin, home, env, "asc")
	wantAsc := []string{"T-0002", "T-0004", "T-0001", "T-0003"}
	if !equalIDs(asc, wantAsc) {
		t.Errorf("asc order = %v, want %v (URGENT, NORMAL, LATER, unset)", asc, wantAsc)
	}

	desc := listedIDs(t, bin, home, env, "desc")
	wantDesc := []string{"T-0001", "T-0004", "T-0002", "T-0003"}
	if !equalIDs(desc, wantDesc) {
		t.Errorf("desc order = %v, want %v (LATER, NORMAL, URGENT, unset last)", desc, wantDesc)
	}
}

// TestOutOfVocabularySchedulingKeyRejected covers the silent no-op. An
// out-of-vocabulary by_priority key was previously unreachable and
// produced no diagnostic at all.
//
// It must warn, not abort: auto-scheduling defaults are an optional
// convenience, so a stale key is a reason to tell the user, not a reason
// to refuse to run every command in the tool.
func TestOutOfVocabularySchedulingKeyRejected(t *testing.T) {
	bin, home, env := priorityVocabFixture(t, badSchedulingConfig)

	out, code := runTLC(t, bin, home, env, "task", "create", "probe")
	if code != 0 {
		t.Fatalf("a bad by_priority key must warn, not abort (exit %d):\n%s", code, out)
	}
	if !strings.Contains(out, "by_priority") {
		t.Errorf("expected a by_priority validation warning, got:\n%s", out)
	}
	if !strings.Contains(out, "URGENT") || !strings.Contains(out, "LATER") {
		t.Errorf("warning should name the configured vocabulary, got:\n%s", out)
	}
}

// TestInVocabularySchedulingKeyApplies is the other half: a valid key
// must validate silently AND actually fire. Asserting only on the absence
// of a warning would pass just as well if the rule were still unreachable
// — which it was, because viper lower-cases map keys and the lookup
// interpolated the canonical spelling.
func TestInVocabularySchedulingKeyApplies(t *testing.T) {
	bin, home, env := priorityVocabFixture(t, goodSchedulingConfig)

	out, code := runTLC(t, bin, home, env, "task", "create", "urgent thing", "-p", "P0")
	if code != 0 {
		t.Fatalf("create failed (exit %d):\n%s", code, out)
	}
	if strings.Contains(out, "by_priority") {
		t.Errorf("a valid by_priority key must not warn, got:\n%s", out)
	}

	shown := runTLCOK(t, bin, home, env, "task", "show", "T-0001", "--format", "json")
	var payload struct {
		Task struct {
			DueAt *string `json:"due_at"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(shown), &payload); err != nil {
		t.Fatalf("decode task show json: %v\n%s", err, shown)
	}
	if payload.Task.DueAt == nil || *payload.Task.DueAt == "" {
		t.Errorf("by_priority due rule did not fire; task has no due date:\n%s", shown)
	}
}

// listedIDs returns the task IDs from a priority-sorted listing, in the
// order the CLI printed them.
func listedIDs(t *testing.T, bin, home string, env []string, direction string) []string {
	t.Helper()
	out := runTLCOK(t, bin, home, env,
		"task", "list", "--sort-by", "priority", "--sort-direction", direction)
	var ids []string
	for _, line := range strings.Split(out, "\n") {
		for _, field := range strings.Fields(line) {
			if strings.HasPrefix(field, "T-") {
				ids = append(ids, field)
				break
			}
		}
	}
	return ids
}

// equalIDs compares two ID slices element-wise.
func equalIDs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// seqTaskID renders the Nth task's default ID.
func seqTaskID(n int) string {
	digits := []byte{
		byte('0' + (n/1000)%10),
		byte('0' + (n/100)%10),
		byte('0' + (n/10)%10),
		byte('0' + n%10),
	}
	return "T-" + string(digits)
}
