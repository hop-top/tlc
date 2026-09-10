package cli

import (
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

// The TLS parser's metadata-prefix list had gone deaf to three of the
// four label axes. `type:`, `status:` and `priority:` were not
// recognized, so a token on any of them was not classified as metadata
// at all and fell through into the task TITLE — including `priority:`,
// the spelling `label init` seeds and every sync plugin emits, while the
// original `prio:` kept working.
//
// These tests pin the vocabulary the parser accepts. The title assertion
// is the load-bearing half of each case: a token that stops setting its
// field is a bug, but a token that lands in the title is the bug this
// file exists to prevent, and it is the one that survives into every
// later read of the task.

// tokenNotInTitle is the assertion the old behavior failed. Kept
// separate from the field assertions so a mutation that re-breaks
// classification fails on the title, which is the user-visible harm,
// rather than only on a field nobody looks at.
func tokenNotInTitle(t *testing.T, title, token, wantTitle string) {
	t.Helper()
	if strings.Contains(title, token) {
		t.Errorf("token %q leaked into title %q; it must be classified as metadata", token, title)
	}
	if title != wantTitle {
		t.Errorf("title = %q, want %q", title, wantTitle)
	}
}

func TestParseTLS_TypeTokenBecomesTagNotTitle(t *testing.T) {
	task, err := parseTLS(`[ ] T-0001 Rewrite the parser type:feat`)
	if err != nil {
		t.Fatalf("parseTLS failed: %v", err)
	}
	tokenNotInTitle(t, task.Title, "type:feat", "Rewrite the parser")

	// Stored WHOLE, prefix included: the tag policy validates
	// `type:feat`, and a bare `feat` would not be admitted by it.
	if !hasTag(task.Tags, "type:feat") {
		t.Errorf("tags = %v, want to contain %q", task.Tags, "type:feat")
	}
}

func TestParseTLS_StatusTokenBecomesTagAndDoesNotSetStatus(t *testing.T) {
	// The bracket says TODO. The token says in-progress. The bracket
	// wins — status has exactly one source on a TLS line, the same way
	// an issue's open/closed state is the one source on a forge.
	task, err := parseTLS(`[ ] T-0001 Rewrite the parser status:in-progress`)
	if err != nil {
		t.Fatalf("parseTLS failed: %v", err)
	}
	tokenNotInTitle(t, task.Title, "status:in-progress", "Rewrite the parser")

	if task.Status != core.StatusTodo {
		t.Errorf("status = %q, want %q; the bracket marker is the only status source",
			task.Status, core.StatusTodo)
	}
	if !hasTag(task.Tags, "status:in-progress") {
		t.Errorf("tags = %v, want to contain %q", task.Tags, "status:in-progress")
	}
}

func TestParseTLS_PriorityAxisResolvesToCanonicalName(t *testing.T) {
	// `priority:high` is the forge spelling; P1 is the config name. The
	// rank alias is what every sync plugin's priorityToLabel emits, so
	// the parser has to resolve it or `label init`'s own vocabulary
	// cannot round trip.
	for _, tc := range []struct {
		token string
		want  core.Priority
	}{
		{"priority:critical", core.PriorityP0},
		{"priority:high", core.PriorityP1},
		{"priority:medium", core.PriorityP2},
		{"priority:low", core.PriorityP3},
		{"priority:P1", core.PriorityP1},
		{"priority:p1", core.PriorityP1},
	} {
		t.Run(tc.token, func(t *testing.T) {
			task, err := parseTLS(`[ ] T-0001 Rewrite the parser ` + tc.token)
			if err != nil {
				t.Fatalf("parseTLS failed: %v", err)
			}
			tokenNotInTitle(t, task.Title, tc.token, "Rewrite the parser")
			if task.Priority != tc.want {
				t.Errorf("priority = %q, want %q", task.Priority, tc.want)
			}
		})
	}
}

// TestParseTLS_PrioAliasStillParses is the backward-compatibility pin.
// formatTLS emits `prio:`, so every todo.txt tlc has ever written uses
// it; widening acceptance to `priority:` must not cost the original
// spelling.
func TestParseTLS_PrioAliasStillParses(t *testing.T) {
	task, err := parseTLS(`[ ] T-0001 Rewrite the parser prio:P1`)
	if err != nil {
		t.Fatalf("parseTLS failed: %v", err)
	}
	tokenNotInTitle(t, task.Title, "prio:P1", "Rewrite the parser")
	if task.Priority != core.PriorityP1 {
		t.Errorf("priority = %q, want %q", task.Priority, core.PriorityP1)
	}
	if len(task.Tags) != 0 {
		t.Errorf("tags = %v, want none; prio: sets the field, not a tag", task.Tags)
	}
}

// TestParseTLS_UnchangedTokenBehaviour pins the five token shapes that
// worked before, so widening the vocabulary cannot be paid for by any of
// them.
func TestParseTLS_UnchangedTokenBehaviour(t *testing.T) {
	task, err := parseTLS(
		`[ ] T-0001 Rewrite the parser @alice #plain effort:S domain:core ref:R-9 k=v`)
	if err != nil {
		t.Fatalf("parseTLS failed: %v", err)
	}
	if task.Title != "Rewrite the parser" {
		t.Errorf("title = %q, want %q", task.Title, "Rewrite the parser")
	}
	if task.AssignedTo == nil || *task.AssignedTo != "alice" {
		t.Errorf("assignee = %v, want alice", task.AssignedTo)
	}
	if !hasTag(task.Tags, "plain") {
		t.Errorf("tags = %v, want to contain %q", task.Tags, "plain")
	}
	if task.Effort != core.EffortS {
		t.Errorf("effort = %q, want %q", task.Effort, core.EffortS)
	}
	if task.Meta["domain"] != "core" {
		t.Errorf("meta[domain] = %v, want core", task.Meta["domain"])
	}
	if task.Reference != "R-9" {
		t.Errorf("reference = %q, want R-9", task.Reference)
	}
	if task.Meta["k"] != "v" {
		t.Errorf("meta[k] = %v, want v", task.Meta["k"])
	}
}

// TestTLSRoundTripPreservesDimensions is the acceptance criterion that
// matters most: a task written out and read back must carry the same
// dimensions. It runs through the real formatTLS, so it also pins that
// the tag spelling formatTLS emits (`#type:feat`) is one parseTLS
// accepts.
func TestTLSRoundTripPreservesDimensions(t *testing.T) {
	task := &core.Task{
		ID:       "T-0001",
		Title:    "Rewrite the parser",
		Status:   core.StatusTodo,
		Priority: core.PriorityP1,
		Effort:   core.EffortS,
		Tags:     []string{"type:feat", "status:in-progress"},
		Meta:     map[string]interface{}{"domain": "core"},
	}

	line := formatTLS(task)
	back, err := parseTLS(line)
	if err != nil {
		t.Fatalf("parseTLS roundtrip failed on %q: %v", line, err)
	}

	if back.Title != task.Title {
		t.Errorf("roundtrip title = %q, want %q (line: %s)", back.Title, task.Title, line)
	}
	if back.Priority != task.Priority {
		t.Errorf("roundtrip priority = %q, want %q", back.Priority, task.Priority)
	}
	if back.Effort != task.Effort {
		t.Errorf("roundtrip effort = %q, want %q", back.Effort, task.Effort)
	}
	for _, want := range task.Tags {
		if !hasTag(back.Tags, want) {
			t.Errorf("roundtrip tags = %v, want to contain %q", back.Tags, want)
		}
	}
	if back.Meta["domain"] != "core" {
		t.Errorf("roundtrip meta[domain] = %v, want core", back.Meta["domain"])
	}
}

// TestParseTLS_UnresolvableAxisValueBecomesTag pins the fallback. A value
// outside the vocabulary must not be forced into a typed field, and must
// not vanish either — it lands as a tag, where it is visible and where
// the tag policy gets the final say.
func TestParseTLS_UnresolvableAxisValueBecomesTag(t *testing.T) {
	task, err := parseTLS(`[ ] T-0001 Rewrite the parser priority:nonsensical`)
	if err != nil {
		t.Fatalf("parseTLS failed: %v", err)
	}
	tokenNotInTitle(t, task.Title, "priority:nonsensical", "Rewrite the parser")
	if task.Priority != "" {
		t.Errorf("priority = %q, want empty; an unresolvable value must not reach the field",
			task.Priority)
	}
	if !hasTag(task.Tags, "priority:nonsensical") {
		t.Errorf("tags = %v, want to contain %q", task.Tags, "priority:nonsensical")
	}
}
