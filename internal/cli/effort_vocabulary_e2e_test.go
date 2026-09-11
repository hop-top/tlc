package cli

// End-to-end coverage for the config-driven task-effort vocabulary.
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

// customEffortConfig renames the whole vocabulary. The names are chosen
// so that alphabetical order (BIG, SMALL, TINY) is exactly the REVERSE of
// the declared rank order, which is what lets the sort assertions tell a
// rank sort from a text sort.
const customEffortConfig = `storage:
  db_path: %s
task:
  efforts:
    - name: TINY
      label: Tiny
    - name: SMALL
      label: Small
    - name: BIG
      label: Big
`

// defaultEffortConfig declares no efforts, so the built-in vocabulary
// applies. Pins "unchanged config behaves as before".
const defaultEffortConfig = `storage:
  db_path: %s
`

// effortVocabFixture builds the isolated world for one case. Shares the
// config writer and env builder with the status suite; only the templates
// differ.
func effortVocabFixture(t *testing.T, template string) (bin, home string, env []string) {
	t.Helper()
	return statusVocabFixture(t, template)
}

// showEffort returns the effort actually stored on a task, read back as
// JSON rather than scraped from the table. Asserting on exit code alone
// cannot distinguish "accepted TINY" from "silently fuzzy-matched TINY
// onto some other value and accepted that".
func showEffort(t *testing.T, bin, home string, env []string, id string) string {
	t.Helper()
	out := runTLCOK(t, bin, home, env, "task", "show", id, "--format", "json")
	var payload struct {
		Task struct {
			Effort string `json:"effort"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode task show json: %v\n%s", err, out)
	}
	return payload.Task.Effort
}

// TestConfiguredEffortAccepted is the headline regression: an effort that
// exists only in the user's config must be accepted on write.
func TestConfiguredEffortAccepted(t *testing.T) {
	bin, home, env := effortVocabFixture(t, customEffortConfig)

	out, code := runTLC(t, bin, home, env, "task", "create", "size it", "-e", "TINY")
	if code != 0 {
		t.Fatalf("-e TINY rejected (exit %d):\n%s", code, out)
	}
	if got := showEffort(t, bin, home, env, "T-0001"); got != "TINY" {
		t.Errorf("stored effort = %q, want TINY", got)
	}

	// The update path is a separate gate with its own call into the
	// normaliser; both read the same canon, so both must accept.
	runTLCOK(t, bin, home, env, "task", "update", "T-0001", "--effort", "BIG")
	if got := showEffort(t, bin, home, env, "T-0001"); got != "BIG" {
		t.Errorf("after update, stored effort = %q, want BIG", got)
	}
}

// TestUnknownEffortNamesConfiguredSet pins the rejection message to the
// CONFIGURED vocabulary. A message still naming XS-XL would tell a user
// with TINY declared that the built-ins they no longer have are the only
// legal values.
func TestUnknownEffortNamesConfiguredSet(t *testing.T) {
	bin, home, env := effortVocabFixture(t, customEffortConfig)

	out, code := runTLC(t, bin, home, env, "task", "create", "bogus", "-e", "NOSUCHEFFORT")
	if code == 0 {
		t.Fatalf("-e NOSUCHEFFORT should be rejected under a renamed vocabulary:\n%s", out)
	}
	for _, want := range []string{"TINY", "SMALL", "BIG"} {
		if !strings.Contains(out, want) {
			t.Errorf("rejection should name %q, got:\n%s", want, out)
		}
	}
	// The built-in sizes must not appear as though they were still legal.
	// "S", "M" and "L" are single letters that occur inside ordinary
	// words, so only the unambiguous multi-character ones are asserted.
	for _, leak := range []string{"XS", "XL"} {
		if strings.Contains(out, leak) {
			t.Errorf("rejection leaked built-in %q under a renamed vocabulary:\n%s", leak, out)
		}
	}
}

// TestConfiguredEffortInHelpAndCompletion covers discoverability. kit
// stamps flag enums in prepareTree, before argv is parsed and long before
// any config file is read, so the configured vocabulary reaches --help and
// the shell only via the post-config restamp.
func TestConfiguredEffortInHelpAndCompletion(t *testing.T) {
	bin, home, env := effortVocabFixture(t, customEffortConfig)

	help := runTLCOK(t, bin, home, env, "task", "create", "--help")
	if !strings.Contains(help, "TINY") {
		t.Errorf("--help should advertise the configured vocabulary, got:\n%s", help)
	}
	if strings.Contains(help, "one of: XS, S, M, L, XL") {
		t.Errorf("--help still shows the pre-config built-in set:\n%s", help)
	}

	comp := runTLCOK(t, bin, home, env, "__complete", "task", "create", "--effort", "")
	if !strings.Contains(comp, "TINY") {
		t.Errorf("completion should offer TINY, got:\n%s", comp)
	}
	if strings.Contains(comp, "XS") {
		t.Errorf("completion leaked the built-in set:\n%s", comp)
	}
}

// TestConfiguredEffortViaConfigFlag pins that the vocabulary also reaches
// --help through `-c <file>`, not only through TLC_CONFIG.
//
// A separate case because it exercises a different code path: `-c` is
// parsed from argv, so anything that reads config before argv is parsed —
// or that pins config into the memoising workflow singleton — would serve
// the built-ins here while passing every TLC_CONFIG-based test above.
func TestConfiguredEffortViaConfigFlag(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	dbPath := home + "/tasks.db"
	cfgPath := writeVocabConfig(t, home, dbPath, customEffortConfig)
	// e2eEnv strips TLC_CONFIG, so config can only arrive via -c here.
	env := e2eEnv(t, home, dbPath)

	help := runTLCOK(t, bin, home, env, "-c", cfgPath, "task", "create", "--help")
	if !strings.Contains(help, "TINY") {
		t.Errorf("-c should carry the configured vocabulary into --help, got:\n%s", help)
	}
	if strings.Contains(help, "one of: XS, S, M, L, XL") {
		t.Errorf("-c --help still shows the built-in set:\n%s", help)
	}

	out, code := runTLC(t, bin, home, env, "-c", cfgPath, "task", "create", "via flag", "-e", "TINY")
	if code != 0 {
		t.Fatalf("-c + -e TINY rejected (exit %d):\n%s", code, out)
	}
}

// TestDefaultEffortVocabularyUnchanged is the no-regression contract. A
// config declaring no efforts must behave exactly as before — descriptive
// aliases and fuzzy matching included. This is what stops the
// config-driven canon from quietly costing existing users their
// shorthands.
func TestDefaultEffortVocabularyUnchanged(t *testing.T) {
	bin, home, env := effortVocabFixture(t, defaultEffortConfig)

	cases := []struct {
		input string
		want  string
	}{
		{"XS", "XS"}, // canonical
		{"xs", "XS"}, // derived lowercase variant
		{"S", "S"},
		{"M", "M"},
		{"L", "L"},
		{"XL", "XL"},
		{"tiny", "XS"}, // hand-written semantic aliases
		{"extra-small", "XS"},
		{"extrasmall", "XS"},
		{"xsmall", "XS"},
		{"small", "S"},
		{"medium", "M"},
		{"med", "M"},
		{"large", "L"},
		{"extra-large", "XL"},
		{"extralarge", "XL"},
		{"xlarge", "XL"},
		{"huge", "XL"},
	}
	for i, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			out, code := runTLC(t, bin, home, env, "task", "create", "alias probe", "-e", tc.input)
			if code != 0 {
				t.Fatalf("-e %s rejected (exit %d):\n%s", tc.input, code, out)
			}
			id := seqTaskID(i + 1)
			if got := showEffort(t, bin, home, env, id); got != tc.want {
				t.Errorf("-e %s stored %q, want %q", tc.input, got, tc.want)
			}
		})
	}

	// The rejection message must still name exactly the built-in five,
	// with nothing from another config leaking in.
	rej, code := runTLC(t, bin, home, env, "task", "create", "bogus", "-e", "NOSUCHEFFORT")
	if code == 0 {
		t.Fatalf("-e NOSUCHEFFORT should have been rejected:\n%s", rej)
	}
	if !strings.Contains(rej, "XS, S, M, L, XL") {
		t.Errorf("default rejection should name the built-in five verbatim, got:\n%s", rej)
	}
	for _, leak := range []string{"TINY", "SMALL", "BIG"} {
		if strings.Contains(rej, leak) {
			t.Errorf("default vocabulary leaked %q from another config:\n%s", leak, rej)
		}
	}

	// And --help must still advertise the built-ins.
	help := runTLCOK(t, bin, home, env, "task", "create", "--help")
	if !strings.Contains(help, "one of: XS, S, M, L, XL") {
		t.Errorf("default --help should name the built-in five, got:\n%s", help)
	}
}

// TestDefaultEffortFuzzyMatchingUnchanged pins that inputs which resolve
// only through the fuzzy fallback still resolve, and to the same value.
//
// Separate from the alias table above because fuzzy runs against the
// canonical set and the alias KEYS, both of which this change made
// config-derived. A canon that resolved through a different path would
// still pass the exact-alias cases while quietly moving these.
func TestDefaultEffortFuzzyMatchingUnchanged(t *testing.T) {
	bin, home, env := effortVocabFixture(t, defaultEffortConfig)

	cases := []struct {
		input string
		want  string
	}{
		{"smal", "S"},
		{"larg", "L"},
		{"mediu", "M"},
	}
	for i, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			out, code := runTLC(t, bin, home, env, "task", "create", "fuzzy probe", "-e", tc.input)
			if code != 0 {
				t.Fatalf("-e %s rejected (exit %d):\n%s", tc.input, code, out)
			}
			id := seqTaskID(i + 1)
			if got := showEffort(t, bin, home, env, id); got != tc.want {
				t.Errorf("-e %s stored %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestEffortOptional pins that effort stays optional: a task created
// without -e must be storable, showable and listable, exactly as today.
func TestEffortOptional(t *testing.T) {
	bin, home, env := effortVocabFixture(t, customEffortConfig)

	runTLCOK(t, bin, home, env, "task", "create", "no effort here")
	if got := showEffort(t, bin, home, env, "T-0001"); got != "" {
		t.Errorf("task created without -e has effort %q, want empty", got)
	}

	out, code := runTLC(t, bin, home, env, "task", "list")
	if code != 0 {
		t.Fatalf("task list failed with an unestimated task (exit %d):\n%s", code, out)
	}
	if !strings.Contains(out, "T-0001") {
		t.Errorf("unestimated task missing from listing:\n%s", out)
	}
}

// TestConfiguredEffortSortUsesRank is the ordering contract for a renamed
// vocabulary. BIG, SMALL, TINY is the alphabetical order and the exact
// reverse of the declared rank order, so a lexicographic sort and a rank
// sort produce opposite answers and this test can tell them apart.
//
// Unset sorts last in BOTH directions: "not estimated" is not a size, and
// it must not sort ahead of a task the user deliberately marked smallest.
func TestConfiguredEffortSortUsesRank(t *testing.T) {
	bin, home, env := effortVocabFixture(t, customEffortConfig)

	runTLCOK(t, bin, home, env, "task", "create", "big task", "-e", "BIG")
	runTLCOK(t, bin, home, env, "task", "create", "tiny task", "-e", "TINY")
	runTLCOK(t, bin, home, env, "task", "create", "unset task")
	runTLCOK(t, bin, home, env, "task", "create", "small task", "-e", "SMALL")

	asc := effortSortedIDs(t, bin, home, env, "asc")
	wantAsc := []string{"T-0002", "T-0004", "T-0001", "T-0003"}
	if !equalIDs(asc, wantAsc) {
		t.Errorf("asc order = %v, want %v (TINY, SMALL, BIG, unset)", asc, wantAsc)
	}

	desc := effortSortedIDs(t, bin, home, env, "desc")
	wantDesc := []string{"T-0001", "T-0004", "T-0002", "T-0003"}
	if !equalIDs(desc, wantDesc) {
		t.Errorf("desc order = %v, want %v (BIG, SMALL, TINY, unset last)", desc, wantDesc)
	}
}

// TestDefaultEffortSortUsesRank is the same contract for the BUILT-IN
// vocabulary, and it is not redundant with the renamed case above.
//
// Priority needed a renamed vocabulary to expose its text sort because
// P0..P3 sorts correctly as text by accident. Effort has no such
// accident: XS, S, M, L, XL sorts to L, M, S, XL, XS as text, so
// --sort-by effort was wrong for every user with no config at all.
func TestDefaultEffortSortUsesRank(t *testing.T) {
	bin, home, env := effortVocabFixture(t, defaultEffortConfig)

	runTLCOK(t, bin, home, env, "task", "create", "xl task", "-e", "XL")
	runTLCOK(t, bin, home, env, "task", "create", "xs task", "-e", "XS")
	runTLCOK(t, bin, home, env, "task", "create", "unset task")
	runTLCOK(t, bin, home, env, "task", "create", "m task", "-e", "M")

	asc := effortSortedIDs(t, bin, home, env, "asc")
	wantAsc := []string{"T-0002", "T-0004", "T-0001", "T-0003"}
	if !equalIDs(asc, wantAsc) {
		t.Errorf("asc order = %v, want %v (XS, M, XL, unset)", asc, wantAsc)
	}

	desc := effortSortedIDs(t, bin, home, env, "desc")
	wantDesc := []string{"T-0001", "T-0004", "T-0002", "T-0003"}
	if !equalIDs(desc, wantDesc) {
		t.Errorf("desc order = %v, want %v (XL, M, XS, unset last)", desc, wantDesc)
	}
}

// TestConfiguredEffortLabelAxis pins that `label init` emits one
// `effort:*` label per DECLARED effort rather than per built-in constant.
func TestConfiguredEffortLabelAxis(t *testing.T) {
	bin, home, env := effortVocabFixture(t, customEffortConfig)

	out := runTLCOK(t, bin, home, env, "label", "templates")
	for _, want := range []string{"effort:tiny", "effort:small", "effort:big"} {
		if !strings.Contains(out, want) {
			t.Errorf("label templates should emit %q, got:\n%s", want, out)
		}
	}
	for _, leak := range []string{"effort:xs", "effort:xl"} {
		if strings.Contains(out, leak) {
			t.Errorf("label templates leaked built-in %q:\n%s", leak, out)
		}
	}
}

// TestDefaultEffortLabelAxisUnchanged is the no-regression half: with no
// declared efforts the axis must still emit the built-in five, with the
// swatches and prose they have always carried.
func TestDefaultEffortLabelAxisUnchanged(t *testing.T) {
	bin, home, env := effortVocabFixture(t, defaultEffortConfig)

	out := runTLCOK(t, bin, home, env, "label", "templates")
	for _, want := range []string{
		"effort:xs", "effort:s", "effort:m", "effort:l", "effort:xl",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("label templates should emit %q, got:\n%s", want, out)
		}
	}
	// `label templates` prints names only, so the swatches are pinned
	// where they are visible: TestEffortAxisColoursUnchanged in
	// internal/labels.
}

// TestDuplicateEffortRejected pins the vocabulary's own validation: two
// definitions with the same name have no defensible rank reading.
func TestDuplicateEffortRejected(t *testing.T) {
	const dupe = `storage:
  db_path: %s
task:
  efforts:
    - name: TINY
    - name: TINY
`
	bin, home, env := effortVocabFixture(t, dupe)

	out, code := runTLC(t, bin, home, env, "task", "create", "probe")
	if code == 0 {
		t.Fatalf("a duplicate effort name should be rejected:\n%s", out)
	}
	if !strings.Contains(out, "duplicate effort name") {
		t.Errorf("expected a duplicate-effort diagnosis, got:\n%s", out)
	}
}

// effortSortedIDs returns the task IDs from an effort-sorted listing, in
// the order the CLI printed them.
func effortSortedIDs(t *testing.T, bin, home string, env []string, direction string) []string {
	t.Helper()
	out := runTLCOK(t, bin, home, env,
		"task", "list", "--sort-by", "effort", "--sort-direction", direction)
	var ids []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "T-") {
			continue
		}
		ids = append(ids, fields[0])
	}
	return ids
}
