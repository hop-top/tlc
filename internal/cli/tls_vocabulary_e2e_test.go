package cli

// End-to-end coverage for the TLS metadata vocabulary, driven through a
// real todo.txt file and the real binary.
//
// The unit tests in sync_local_vocabulary_test.go call parseTLS directly,
// which is the right level for the classification rules themselves. What
// they cannot show is that the ingest actually RUNS on the path a user
// takes: importFromProjection is called from ensureDBSynced on every
// storage open, reading the file `task.todo_file` names, and a token that
// parses correctly in a unit test is still broken if the tasks it lands
// on never reach the DB. So these spawn the binary and read the task
// back out of storage as JSON.
//
// Every case pins storage.db_path in the config file, for the reason the
// tag-policy suite spells out: an unpinned probe resolves the DB through
// the global project registry rather than the cwd, and would read — and
// write — an unrelated real database.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tlsVocabProjectID is declared in config AND carried on every seeded
// line, because importFromProjection requires the two to agree.
//
// In project context the ingest accepts only lines whose `project_id=`
// names the current project: a shared global todo.txt must not let one
// project's lines update another's same-ID tasks. A line with no
// project_id is skipped outright, so a fixture that omits the token
// ingests nothing at all and every assertion below would fail on an
// empty list rather than on the behaviour under test.
const tlsVocabProjectID = "tls-vocab-e2e"

// tlsSeedSuffix is the tail every seeded line carries: the project_id the
// ingest matches on, plus fixed timestamps so a task's identity does not
// vary run to run.
const tlsSeedSuffix = " project_id=" + tlsVocabProjectID +
	" created_at=2026-01-01T00:00:00Z updated_at=2026-01-01T00:00:00Z"

// tlsVocabConfig pins the DB and points task.todo_file at a file the test
// writes. Two %s, filled in order: db_path, then todo_file.
const tlsVocabConfig = `project:
  id: ` + tlsVocabProjectID + `
storage:
  db_path: %s
task:
  todo_file: %s
`

// tlsVocabClosedConfig is the same with a CLOSED tag policy, to pin that
// the vocabulary work composes with the policy rather than bypassing it.
//
// `domain:*` is the only project entry, so the four generated axes are
// admitted by construction while `bogustag` is not — which is what makes
// the drop observable.
const tlsVocabClosedConfig = `project:
  id: ` + tlsVocabProjectID + `
storage:
  db_path: %s
task:
  todo_file: %s
  tags:
    policy: closed
    allowed:
      - domain:*
`

// tlsIngestFixture builds an isolated world whose todo.txt contains the
// given TLS lines, and returns everything needed to run the binary
// against it.
//
// The task IDs in `lines` must NOT be T-NNNN display aliases. parseTLS
// resolves an alias through the seq counter and DROPS the line when it
// does not resolve, which is the anti-phantom guard that keeps a stale
// projection from minting mirror rows — correct, but it means an alias in
// a fixture silently ingests nothing at all.
func tlsIngestFixture(t *testing.T, cfgTemplate string, lines ...string) (bin, home string, env []string) {
	t.Helper()
	bin = buildTLCBinary(t)
	home = t.TempDir()
	dbPath := filepath.Join(home, "tasks.db")
	todoPath := filepath.Join(home, "todo.txt")

	body := strings.Replace(cfgTemplate, "%s", dbPath, 1)
	body = strings.Replace(body, "%s", todoPath, 1)
	cfgPath := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(todoPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write todo.txt: %v", err)
	}

	return bin, home, append(e2eEnv(t, home, dbPath), "TLC_CONFIG="+cfgPath)
}

// ingestedTask is the subset of a task these tests assert on.
type ingestedTask struct {
	Title    string   `json:"title"`
	Status   string   `json:"status"`
	Priority string   `json:"priority"`
	Effort   string   `json:"effort"`
	Tags     []string `json:"tags"`
}

// listIngested runs `task list --format json` and returns the tasks the
// ingest produced.
func listIngested(t *testing.T, bin, home string, env []string) []ingestedTask {
	t.Helper()
	out := runTLCOK(t, bin, home, env, "task", "list", "--format", "json")
	// A DB migration can print a banner ahead of the payload, so decode
	// from the first `[` rather than from byte zero.
	idx := strings.Index(out, "[")
	if idx < 0 {
		t.Fatalf("no JSON array in task list output:\n%s", out)
	}
	var tasks []ingestedTask
	if err := json.Unmarshal([]byte(out[idx:]), &tasks); err != nil {
		t.Fatalf("decode task list: %v\n%s", err, out)
	}
	return tasks
}

func onlyIngested(t *testing.T, bin, home string, env []string) ingestedTask {
	t.Helper()
	tasks := listIngested(t, bin, home, env)
	if len(tasks) != 1 {
		t.Fatalf("expected exactly 1 ingested task, got %d: %+v", len(tasks), tasks)
	}
	return tasks[0]
}

// TestTLSIngestClassifiesFullVocabulary is the headline regression. Every
// axis token on the line must reach its field or its tag, and NONE of
// them may reach the title — a token the parser does not recognise is not
// merely ignored, it becomes part of the task's name, which is the harm
// that survives into every later read.
func TestTLSIngestClassifiesFullVocabulary(t *testing.T) {
	bin, home, env := tlsIngestFixture(t, tlsVocabConfig,
		"[ ] task_v1aaaaaaaaaaaaaaaaaaaaaaaa Rewrite the parser "+
			"type:feat status:in-progress priority:high effort:S "+
			"domain:core ref:R-9 @alice #plain"+tlsSeedSuffix,
	)

	got := onlyIngested(t, bin, home, env)

	if got.Title != "Rewrite the parser" {
		t.Errorf("title = %q, want %q; an unrecognised token leaked into it",
			got.Title, "Rewrite the parser")
	}
	if got.Priority != "P1" {
		t.Errorf("priority = %q, want P1 (from priority:high)", got.Priority)
	}
	if got.Effort != "S" {
		t.Errorf("effort = %q, want S", got.Effort)
	}
	// The bracket marker is the only status source; the status: token is
	// a tag, not a second way to set the field.
	if got.Status != "TODO" {
		t.Errorf("status = %q, want TODO (from the bracket, not status:in-progress)", got.Status)
	}
	for _, want := range []string{"type:feat", "status:in-progress", "plain"} {
		if !hasTag(got.Tags, want) {
			t.Errorf("tags = %v, want to contain %q", got.Tags, want)
		}
	}
}

// TestTLSIngestPrioAliasStillWorks pins backward compatibility on the
// real binary: `prio:` is the spelling formatTLS emits, so every todo.txt
// tlc has ever written depends on it.
func TestTLSIngestPrioAliasStillWorks(t *testing.T) {
	bin, home, env := tlsIngestFixture(t, tlsVocabConfig,
		"[ ] task_v1bbbbbbbbbbbbbbbbbbbbbbbb Legacy prio spelling prio:P2"+tlsSeedSuffix,
	)

	got := onlyIngested(t, bin, home, env)
	if got.Title != "Legacy prio spelling" {
		t.Errorf("title = %q, want %q", got.Title, "Legacy prio spelling")
	}
	if got.Priority != "P2" {
		t.Errorf("priority = %q, want P2", got.Priority)
	}
}

// TestTLSIngestComposesWithClosedTagPolicy pins the interaction with the
// tag policy.
//
// Two assertions, and the first is the important one: the command must
// still SUCCEED. importFromProjection runs from ensureDBSynced on every
// storage open, so an error on this path would make one stale token in
// todo.txt refuse every command in the tool. The disallowed tag is
// dropped; the axis tags, which the vocabulary admits by construction,
// are kept.
func TestTLSIngestComposesWithClosedTagPolicy(t *testing.T) {
	bin, home, env := tlsIngestFixture(t, tlsVocabClosedConfig,
		"[ ] task_v1cccccccccccccccccccccccc Closed policy probe "+
			"type:feat status:in-progress priority:high effort:S #"+rejectedTag+tlsSeedSuffix,
	)

	out, code := runTLC(t, bin, home, env, "task", "list", "--format", "json")
	if code != 0 {
		t.Fatalf("ingest under a closed policy must not fail the command (exit %d):\n%s", code, out)
	}

	got := onlyIngested(t, bin, home, env)
	if got.Title != "Closed policy probe" {
		t.Errorf("title = %q, want %q", got.Title, "Closed policy probe")
	}
	if hasTag(got.Tags, rejectedTag) {
		t.Errorf("tags = %v, want %q dropped by the closed policy", got.Tags, rejectedTag)
	}
	// The generated axes are vocabulary members by construction, so the
	// policy must not take them with it.
	for _, want := range []string{"type:feat", "status:in-progress"} {
		if !hasTag(got.Tags, want) {
			t.Errorf("tags = %v, want to keep generated axis tag %q", got.Tags, want)
		}
	}
}
