package cli

// End-to-end coverage for the config-driven tag policy.
//
// Driven through a real config FILE and the real binary, for the reason
// spelled out in status_vocabulary_e2e_test.go: a struct-level test can
// agree with itself while the CLI's own gate behaves differently in the
// field, because it never enters through the door the user does. Every
// case below spawns the binary with a config whose db_path is pinned — an
// unpinned probe resolves the DB through the global project registry, not
// the cwd, and would read (and write) an unrelated real database.

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// closedTagConfig declares a closed policy with a small project
// vocabulary: two literals and one opened dimension. The dimension is
// what distinguishes a wildcard match from a literal one.
const closedTagConfig = `storage:
  db_path: %s
task:
  tags:
    policy: closed
    allowed:
      - storage
      - cli
      - domain:*
`

// openTagConfig declares nothing about tags, so the default open policy
// applies. Pins "unchanged config behaves as before" — the contract that
// keeps this key from being a breaking change.
const openTagConfig = `storage:
  db_path: %s
`

// literalOnlyTagConfig closes a dimension by listing its members instead
// of opening it, so the two guarantees can be asserted side by side.
const literalOnlyTagConfig = `storage:
  db_path: %s
task:
  tags:
    policy: closed
    allowed:
      - area:storage
`

// tagPolicyFixture builds the isolated world for one case. Shares the
// config writer and env builder with the status suite; only the templates
// differ.
func tagPolicyFixture(t *testing.T, template string) (bin, home string, env []string) {
	t.Helper()
	return statusVocabFixture(t, template)
}

// showTags returns the tags actually stored on a task, read back as JSON
// rather than scraped from the table. Asserting on exit code alone cannot
// distinguish "accepted the tag" from "exited 0 having silently dropped
// it".
func showTags(t *testing.T, bin, home string, env []string, id string) []string {
	t.Helper()
	out := runTLCOK(t, bin, home, env, "task", "show", id, "--format", "json")
	var payload struct {
		Task struct {
			Tags []string `json:"tags"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode task show json: %v\n%s", err, out)
	}
	return payload.Task.Tags
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

// rejectedTag is the tag every negative case in this suite offers. One
// constant rather than a literal per call, so assertNamesTagAndSet can
// assert against the same value the caller passed without taking a
// parameter that is never anything else.
const rejectedTag = "bogustag"

// assertNamesTagAndSet is the error-quality contract, asserted from one
// place because every write path owes the same message. A bare rejection
// is unusable when the vocabulary lives in a config file the user may not
// have written and cannot see from the error.
func assertNamesTagAndSet(t *testing.T, out string) {
	t.Helper()
	if !strings.Contains(out, rejectedTag) {
		t.Errorf("rejection should name the offending tag %q, got:\n%s", rejectedTag, out)
	}
	for _, want := range []string{"storage", "cli", "domain:*"} {
		if !strings.Contains(out, want) {
			t.Errorf("rejection should name allowed tag %q, got:\n%s", want, out)
		}
	}
}

// TestOpenTagPolicyAcceptsAnything is the no-regression contract, and the
// most important test here. Tags were free-form; a project that has
// configured nothing must not notice this key exists.
func TestOpenTagPolicyAcceptsAnything(t *testing.T) {
	bin, home, env := tagPolicyFixture(t, openTagConfig)

	out, code := runTLC(t, bin, home, env, "task", "create", "open probe", "--tag", "anything")
	if code != 0 {
		t.Fatalf("--tag anything rejected under the default policy (exit %d):\n%s", code, out)
	}
	if got := showTags(t, bin, home, env, "T-0001"); !hasTag(got, "anything") {
		t.Errorf("stored tags = %v, want to contain %q", got, "anything")
	}

	// The update path is a separate gate; an open policy must be a no-op
	// there too.
	runTLCOK(t, bin, home, env, "task", "update", "T-0001", "--add-tag", "whatever-else")
	if got := showTags(t, bin, home, env, "T-0001"); !hasTag(got, "whatever-else") {
		t.Errorf("after --add-tag, stored tags = %v, want to contain %q", got, "whatever-else")
	}
}

// TestClosedTagPolicyAcceptsListedTag is the headline positive case.
func TestClosedTagPolicyAcceptsListedTag(t *testing.T) {
	bin, home, env := tagPolicyFixture(t, closedTagConfig)

	out, code := runTLC(t, bin, home, env, "task", "create", "listed", "--tag", "storage")
	if code != 0 {
		t.Fatalf("a listed tag was rejected (exit %d):\n%s", code, out)
	}
	if got := showTags(t, bin, home, env, "T-0001"); !hasTag(got, "storage") {
		t.Errorf("stored tags = %v, want to contain %q", got, "storage")
	}
}

// TestClosedTagPolicyRejectsUnlistedTagOnCreate is the headline negative
// case, and it pins the message rather than only the exit code.
func TestClosedTagPolicyRejectsUnlistedTagOnCreate(t *testing.T) {
	bin, home, env := tagPolicyFixture(t, closedTagConfig)

	out, code := runTLC(t, bin, home, env, "task", "create", "unlisted", "--tag", rejectedTag)
	if code == 0 {
		t.Fatalf("an unlisted tag should be rejected under a closed policy:\n%s", out)
	}
	assertNamesTagAndSet(t, out)

	// The task must not exist: a gate that rejected the tag but created
	// the task anyway would leave the user a half-applied write.
	list := runTLCOK(t, bin, home, env, "task", "list")
	if strings.Contains(list, "unlisted") {
		t.Errorf("the rejected task was created anyway:\n%s", list)
	}
}

// TestClosedTagPolicyRejectsOnUpdate covers the second CLI write path.
// A policy enforced only on create is not a policy: `--add-tag` reaches
// the same column.
func TestClosedTagPolicyRejectsOnUpdate(t *testing.T) {
	bin, home, env := tagPolicyFixture(t, closedTagConfig)

	runTLCOK(t, bin, home, env, "task", "create", "target", "--tag", "cli")

	out, code := runTLC(t, bin, home, env, "task", "update", "T-0001", "--add-tag", rejectedTag)
	if code == 0 {
		t.Fatalf("--add-tag with an unlisted tag should be rejected:\n%s", out)
	}
	assertNamesTagAndSet(t, out)

	if got := showTags(t, bin, home, env, "T-0001"); hasTag(got, rejectedTag) {
		t.Errorf("the rejected tag landed anyway: %v", got)
	}

	// A listed tag must still go through on the same path.
	runTLCOK(t, bin, home, env, "task", "update", "T-0001", "--add-tag", "storage")
	if got := showTags(t, bin, home, env, "T-0001"); !hasTag(got, "storage") {
		t.Errorf("a listed tag was not applied by --add-tag: %v", got)
	}
}

// TestClosedTagPolicyRejectsOnTagCommand covers `tlc tag <id> <tag…>`,
// the third CLI write path and the most direct one — it writes tags
// without going through applyTaskFieldChanges at all.
//
// Only the rejection is asserted, not the accepted write. That is not an
// omission: this command resolves its task through uri.NewResolver, which
// does not do the project-scoped seq lookup `task show` does, so a
// T-NNNN-addressed invocation fails NOT_FOUND before any tag is written —
// under the default open policy too, so it is a pre-existing defect
// rather than one this change introduced. The gate therefore runs BEFORE
// resolution, which is what makes the rejection reachable at all, and is
// what this pins.
func TestClosedTagPolicyRejectsOnTagCommand(t *testing.T) {
	bin, home, env := tagPolicyFixture(t, closedTagConfig)

	runTLCOK(t, bin, home, env, "task", "create", "target", "--tag", "cli")

	out, code := runTLC(t, bin, home, env, "tag", "T-0001", rejectedTag)
	if code == 0 {
		t.Fatalf("`tlc tag` with an unlisted tag should be rejected:\n%s", out)
	}
	assertNamesTagAndSet(t, out)

	if got := showTags(t, bin, home, env, "T-0001"); hasTag(got, rejectedTag) {
		t.Errorf("the rejected tag landed anyway: %v", got)
	}

	// A listed tag must NOT be rejected by the policy. It still fails —
	// on the pre-existing resolution defect above — so what is asserted
	// is that the failure is no longer the policy's.
	allowed, code := runTLC(t, bin, home, env, "tag", "T-0001", "storage")
	if code != 0 && strings.Contains(allowed, "not allowed under tag policy") {
		t.Errorf("a listed tag was rejected by the policy:\n%s", allowed)
	}
}

// TestClosedTagPolicyAdmitsGeneratedAxes is the composition contract: the
// axes tlc already generates from this same config are members by
// construction, so a user never restates them in `allowed`.
//
// Without this the closed policy would be unusable in exactly the
// situation it exists for — a project that has decided on a vocabulary
// would find `tlc`'s own `label init` output rejected by `tlc`.
func TestClosedTagPolicyAdmitsGeneratedAxes(t *testing.T) {
	bin, home, env := tagPolicyFixture(t, closedTagConfig)

	// One per axis, plus the two spellings the built-in priorities carry
	// (their own names, and the rank aliases every sync plugin puts on
	// the wire) and `status:blocked`, which is not a status but is on the
	// axis because all four sync plugins push and pull it.
	axes := []string{
		"type:feat", "type:breaking",
		"priority:p0", "priority:critical", "priority:low",
		"effort:xs", "effort:xl",
		"status:in-progress", "status:blocked",
	}
	for i, tag := range axes {
		t.Run(tag, func(t *testing.T) {
			out, code := runTLC(t, bin, home, env, "task", "create", "axis probe", "--tag", tag)
			if code != 0 {
				t.Fatalf("generated axis tag %q was rejected (exit %d):\n%s", tag, code, out)
			}
			if got := showTags(t, bin, home, env, seqTaskID(i+1)); !hasTag(got, tag) {
				t.Errorf("stored tags = %v, want to contain %q", got, tag)
			}
		})
	}
}

// TestClosedTagPolicyAdmitsRenamedAxes pins that composition tracks the
// user's config rather than the built-ins.
//
// This is the case a hardcoded axis list would pass the test above and
// still get wrong: a user who renamed their priorities must be able to
// tag with the axis their OWN config implies, and must not be stuck with
// the built-in names they abandoned.
func TestClosedTagPolicyAdmitsRenamedAxes(t *testing.T) {
	const renamed = `storage:
  db_path: %s
task:
  tags:
    policy: closed
  priorities:
    - name: URGENT
    - name: LATER
`
	bin, home, env := tagPolicyFixture(t, renamed)

	out, code := runTLC(t, bin, home, env, "task", "create", "renamed axis", "--tag", "priority:urgent")
	if code != 0 {
		t.Fatalf("a renamed priority's axis tag was rejected (exit %d):\n%s", code, out)
	}

	// And the built-ins the user abandoned must NOT be admitted: a
	// vocabulary that quietly kept them would be advertising a set the
	// config no longer declares.
	rej, code := runTLC(t, bin, home, env, "task", "create", "stale axis", "--tag", "priority:p0")
	if code == 0 {
		t.Errorf("a built-in priority tag was admitted under a renamed vocabulary:\n%s", rej)
	}
}

// TestClosedTagPolicyWildcardOpensDimension is the wildcard contract.
func TestClosedTagPolicyWildcardOpensDimension(t *testing.T) {
	bin, home, env := tagPolicyFixture(t, closedTagConfig)

	for i, tag := range []string{"domain:storage", "domain:anything-at-all"} {
		t.Run(tag, func(t *testing.T) {
			out, code := runTLC(t, bin, home, env, "task", "create", "wild", "--tag", tag)
			if code != 0 {
				t.Fatalf("`domain:*` should admit %q (exit %d):\n%s", tag, code, out)
			}
			if got := showTags(t, bin, home, env, seqTaskID(i+1)); !hasTag(got, tag) {
				t.Errorf("stored tags = %v, want to contain %q", got, tag)
			}
		})
	}
}

// TestClosedTagPolicyWildcardIsBounded is the other half of the wildcard
// decision, and the reason the shape is `dimension:*` rather than a glob:
// opening one dimension must not open anything else, or `closed` would be
// a policy a single config line could silently turn back into `open`.
func TestClosedTagPolicyWildcardIsBounded(t *testing.T) {
	bin, home, env := tagPolicyFixture(t, closedTagConfig)

	cases := []struct {
		name string
		tag  string
	}{
		// A different dimension is not opened.
		{"other dimension", "team:core"},
		// The bare prefix is not a value.
		{"bare prefix", "domain:"},
		// A tag that merely CONTAINS the prefix is not anchored to it.
		{"unanchored", "sub-domain:storage"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runTLC(t, bin, home, env, "task", "create", "bounded", "--tag", tc.tag)
			if code == 0 {
				t.Fatalf("`domain:*` must not admit %q:\n%s", tc.tag, out)
			}
		})
	}
}

// TestClosedTagPolicyLiteralClosesDimension pins that a project that
// wants a dimension genuinely closed still gets that: listing members
// literally, rather than opening the prefix, keeps everything else out.
// Both guarantees are available and which applies is visible in config.
func TestClosedTagPolicyLiteralClosesDimension(t *testing.T) {
	bin, home, env := tagPolicyFixture(t, literalOnlyTagConfig)

	runTLCOK(t, bin, home, env, "task", "create", "listed member", "--tag", "area:storage")

	out, code := runTLC(t, bin, home, env, "task", "create", "unlisted member", "--tag", "area:cli")
	if code == 0 {
		t.Fatalf("a literal list must not admit an unlisted member of the same dimension:\n%s", out)
	}
	if !strings.Contains(out, "area:cli") || !strings.Contains(out, "area:storage") {
		t.Errorf("rejection should name both the offending tag and the allowed set, got:\n%s", out)
	}
}

// TestClosedTagPolicyIsCaseInsensitive pins that a user who types
// `Type:Feat` gets the tag that exists. The rest of the tool's
// normalisers do not draw that distinction and neither should this.
func TestClosedTagPolicyIsCaseInsensitive(t *testing.T) {
	bin, home, env := tagPolicyFixture(t, closedTagConfig)

	for _, tag := range []string{"STORAGE", "Type:Feat", "DOMAIN:Api"} {
		t.Run(tag, func(t *testing.T) {
			out, code := runTLC(t, bin, home, env, "task", "create", "case probe", "--tag", tag)
			if code != 0 {
				t.Fatalf("%q should be admitted case-insensitively (exit %d):\n%s", tag, code, out)
			}
		})
	}
}

// TestClosedTagPolicyNamesEveryRejectedTag pins that one run tells the
// user everything they have to fix, rather than making them discover the
// rejections one command at a time.
func TestClosedTagPolicyNamesEveryRejectedTag(t *testing.T) {
	bin, home, env := tagPolicyFixture(t, closedTagConfig)

	out, code := runTLC(t, bin, home, env,
		"task", "create", "many bad", "--tag", "badone", "--tag", "badtwo")
	if code == 0 {
		t.Fatalf("two unlisted tags should be rejected:\n%s", out)
	}
	for _, want := range []string{"badone", "badtwo"} {
		if !strings.Contains(out, want) {
			t.Errorf("rejection should name %q, got:\n%s", want, out)
		}
	}
}

// TestClosedTagPolicyAllowsRemovingDisallowedTag is the escape hatch. A
// task can carry a tag the vocabulary no longer admits — it predates the
// policy, or the policy tightened — and gating the MERGED tag set rather
// than only the additions would make such a task uneditable and leave the
// user no way to bring it back into compliance.
func TestClosedTagPolicyAllowsRemovingDisallowedTag(t *testing.T) {
	bin, home := buildTLCBinary(t), t.TempDir()
	dbPath := filepath.Join(home, "tasks.db")

	// Create the task while the policy is still open...
	openEnv := statusVocabEnv(t, home, dbPath,
		writeVocabConfig(t, home, dbPath, openTagConfig))
	runTLCOK(t, bin, home, openEnv, "task", "create", "legacy", "--tag", "legacytag")

	// ...then tighten the policy over the same database.
	closedHome := t.TempDir()
	closedEnv := statusVocabEnv(t, closedHome, dbPath,
		writeVocabConfig(t, closedHome, dbPath, closedTagConfig))

	// The task is still readable and still carries its legacy tag.
	if got := showTags(t, bin, closedHome, closedEnv, "T-0001"); !hasTag(got, "legacytag") {
		t.Fatalf("legacy tag disappeared under a tightened policy: %v", got)
	}

	// And removing it is possible, which it would not be if the merged
	// set were gated.
	out, code := runTLC(t, bin, closedHome, closedEnv,
		"task", "update", "T-0001", "--remove-tag", "legacytag")
	if code != 0 {
		t.Fatalf("removing a since-disallowed tag was refused (exit %d):\n%s", code, out)
	}
	if got := showTags(t, bin, closedHome, closedEnv, "T-0001"); hasTag(got, "legacytag") {
		t.Errorf("legacy tag survived removal: %v", got)
	}
}

// TestInvalidTagPolicyReported pins that a typo in the policy value is a
// diagnosis rather than a silent fallback to open.
//
// This is the failure mode that would make the whole feature worse than
// useless: a config declaring `policy: colsed` that quietly enforced
// nothing would report itself as configured while accepting every tag the
// user believed they had closed off.
func TestInvalidTagPolicyReported(t *testing.T) {
	const bogus = `storage:
  db_path: %s
task:
  tags:
    policy: nonsense
`
	bin, home, env := tagPolicyFixture(t, bogus)

	out, code := runTLC(t, bin, home, env, "task", "create", "probe", "--tag", "whatever")
	if code == 0 {
		t.Fatalf("an unknown tag policy should be reported, not ignored:\n%s", out)
	}
	if !strings.Contains(out, "nonsense") {
		t.Errorf("diagnosis should quote the offending value, got:\n%s", out)
	}
	for _, want := range []string{"open", "closed"} {
		if !strings.Contains(out, want) {
			t.Errorf("diagnosis should name the known policy %q, got:\n%s", want, out)
		}
	}
}

// TestEmptyAllowedTagReported pins the other config-shape mistake: a YAML
// list item that decoded to nothing.
func TestEmptyAllowedTagReported(t *testing.T) {
	const empty = `storage:
  db_path: %s
task:
  tags:
    policy: closed
    allowed:
      - storage
      - ""
`
	bin, home, env := tagPolicyFixture(t, empty)

	out, code := runTLC(t, bin, home, env, "task", "create", "probe")
	if code == 0 {
		t.Fatalf("an empty `allowed` entry should be reported:\n%s", out)
	}
	if !strings.Contains(out, "task.tags.allowed") {
		t.Errorf("diagnosis should name the offending key, got:\n%s", out)
	}
}

// TestTagPolicyInHelp covers discoverability. The annotation is applied
// from Execute rather than only from PersistentPreRunE, because `--help`
// never reaches PersistentPreRunE — cobra's OnInitialize(initConfig)
// fires from PersistentPreRun, which the help path skips entirely.
func TestTagPolicyInHelp(t *testing.T) {
	bin, home, env := tagPolicyFixture(t, closedTagConfig)

	create := runTLCOK(t, bin, home, env, "task", "create", "--help")
	if !strings.Contains(create, "policy closed") {
		t.Errorf("`task create --help` should advertise the closed policy, got:\n%s", create)
	}
	if !strings.Contains(create, "domain:*") {
		t.Errorf("`task create --help` should name the allowed set, got:\n%s", create)
	}

	update := runTLCOK(t, bin, home, env, "task", "update", "--help")
	if !strings.Contains(update, "policy closed") {
		t.Errorf("`task update --help` should advertise the closed policy, got:\n%s", update)
	}
}

// TestOpenTagPolicyHelpUnchanged is the other half: an open policy must
// not clutter the help with a vocabulary that governs nothing.
func TestOpenTagPolicyHelpUnchanged(t *testing.T) {
	bin, home, env := tagPolicyFixture(t, openTagConfig)

	create := runTLCOK(t, bin, home, env, "task", "create", "--help")
	if strings.Contains(create, "policy closed") {
		t.Errorf("an open policy must not annotate --tag, got:\n%s", create)
	}
}

// TestTagPolicyViaConfigFlag pins that the policy also arrives through
// `-c <file>`, not only through TLC_CONFIG.
//
// A separate case because it exercises a different code path: `-c` is
// parsed from argv, so anything reading config before argv is parsed — or
// pinning it into the memoising workflow singleton — would enforce an
// open policy here while passing every TLC_CONFIG-based test above.
func TestTagPolicyViaConfigFlag(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	dbPath := filepath.Join(home, "tasks.db")
	cfgPath := writeVocabConfig(t, home, dbPath, closedTagConfig)
	// e2eEnv strips TLC_CONFIG, so config can only arrive via -c here.
	env := e2eEnv(t, home, dbPath)

	out, code := runTLC(t, bin, home, env, "-c", cfgPath, "task", "create", "via flag", "--tag", rejectedTag)
	if code == 0 {
		t.Fatalf("-c should carry the closed policy into the write gate:\n%s", out)
	}
	assertNamesTagAndSet(t, out)

	ok, code := runTLC(t, bin, home, env, "-c", cfgPath, "task", "create", "via flag ok", "--tag", "storage")
	if code != 0 {
		t.Fatalf("-c + a listed tag rejected (exit %d):\n%s", code, ok)
	}

	help := runTLCOK(t, bin, home, env, "-c", cfgPath, "task", "create", "--help")
	if !strings.Contains(help, "policy closed") {
		t.Errorf("-c should carry the policy into --help, got:\n%s", help)
	}
}

// TestTagPolicyEnforcedOnServeRoutes covers the non-CLI write path. The
// HTTP surface writes to the same store, so a policy it does not enforce
// is a policy with a hole in it.
func TestTagPolicyEnforcedOnServeRoutes(t *testing.T) {
	bin, home, env := tagPolicyFixture(t, closedTagConfig)

	base, token := startServe(t, bin, home, env)

	t.Run("create rejects", func(t *testing.T) {
		status, body := postJSON(t, base+"/tasks", token,
			`{"title":"http reject","tags":["bogustag"]}`)
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("POST /tasks with an unlisted tag = %d, want 422\n%s", status, body)
		}
		assertNamesTagAndSet(t, body)
	})

	t.Run("create accepts", func(t *testing.T) {
		status, body := postJSON(t, base+"/tasks", token,
			`{"title":"http accept","tags":["storage","domain:api"]}`)
		if status != http.StatusCreated {
			t.Fatalf("POST /tasks with listed tags = %d, want 201\n%s", status, body)
		}
	})

	t.Run("patch rejects", func(t *testing.T) {
		status, body := patchJSON(t, base+"/tasks/T-0001", token,
			`{"add_tags":["bogustag"]}`)
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("PATCH add_tags with an unlisted tag = %d, want 422\n%s", status, body)
		}
		assertNamesTagAndSet(t, body)
	})

	t.Run("patch accepts", func(t *testing.T) {
		status, body := patchJSON(t, base+"/tasks/T-0001", token, `{"add_tags":["cli"]}`)
		if status != http.StatusOK {
			t.Fatalf("PATCH add_tags with a listed tag = %d, want 200\n%s", status, body)
		}
	})
}

// startServe launches `tlc serve` on a kernel-assigned port and returns
// its base URL and bearer token, tearing the process down on cleanup.
func startServe(t *testing.T, bin, home string, env []string) (base, token string) {
	t.Helper()

	// context.Background rather than t.Context: the cleanup below owns
	// the teardown, and a context canceled at subtest boundaries would
	// kill the server mid-suite.
	cmd := exec.CommandContext(context.Background(), bin, "serve", "--port", "0")
	cmd.Env = env
	cmd.Dir = home
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("serve stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start serve: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	var banner struct {
		Port  int    `json:"port"`
		Token string `json:"token"`
	}
	if err := json.NewDecoder(stdout).Decode(&banner); err != nil {
		t.Fatalf("decode serve banner: %v", err)
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(banner.Port))
	base = "http://" + addr

	// The banner is printed as the listener binds; poll until a
	// connection actually succeeds so the first request is not a race.
	dialer := &net.Dialer{Timeout: time.Second}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := dialer.DialContext(t.Context(), "tcp", addr)
		if dialErr == nil {
			_ = conn.Close()
			return base, banner.Token
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("serve did not accept connections on port %d", banner.Port)
	return "", ""
}

func postJSON(t *testing.T, url, token, body string) (int, string) {
	t.Helper()
	return doJSON(t, http.MethodPost, url, token, body)
}

func patchJSON(t *testing.T, url, token, body string) (int, string) {
	t.Helper()
	return doJSON(t, http.MethodPatch, url, token, body)
}

func doJSON(t *testing.T, method, url, token, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(
		t.Context(), method, url, strings.NewReader(body),
	)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, url, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s %s body: %v", method, url, err)
	}
	return resp.StatusCode, string(payload)
}

// TestTagPolicyDropsDisallowedTagsOnTodoIngest covers the todo.txt ingest
// path, which enforces by DROPPING rather than by failing.
//
// The asymmetry is deliberate and is the point of the test: ingest runs
// from ensureDBSynced on every storage open, on the happy path of
// read-only commands, so an error there would make one stale `#token` in
// todo.txt refuse every command in the tool. Skipping the line would be
// just as wrong — it would silently discard the task itself. So the task
// lands without the tags the vocabulary does not admit.
func TestTagPolicyDropsDisallowedTagsOnTodoIngest(t *testing.T) {
	const withTodo = `storage:
  db_path: %s
task:
  todo_file: TODOFILE
  tags:
    policy: closed
    allowed:
      - storage
      - cli
      - domain:*
`
	bin := buildTLCBinary(t)
	home := t.TempDir()
	dbPath := filepath.Join(home, "tasks.db")
	todoPath := filepath.Join(home, "todo.txt")

	tmpl := strings.Replace(withTodo, "TODOFILE", todoPath, 1)
	env := statusVocabEnv(t, home, dbPath, writeVocabConfig(t, home, dbPath, tmpl))

	// One TLS line carrying one allowed and one disallowed tag.
	// A raw typeid, not a T-NNNN alias: aliases are display-only and the
	// ingest deliberately refuses to mint a row from one (it would create
	// a phantom mirror keyed by the alias). `project_id=` must be present
	// and match, or the line is treated as belonging to another project.
	line := `[ ] task_01hzzzzzzzzzzzzzzzzzzzzzzz "ingested task" ` +
		`#storage #bogustag project_id=unknown` + "\n"
	if err := os.WriteFile(todoPath, []byte(line), 0o600); err != nil {
		t.Fatalf("write todo file: %v", err)
	}

	// The command must succeed: ingest must not turn a stale tag into a
	// tool that refuses to run.
	out, code := runTLC(t, bin, home, env, "task", "list")
	if code != 0 {
		t.Fatalf("`task list` failed with a disallowed tag in todo.txt (exit %d):\n%s", code, out)
	}
	if !strings.Contains(out, "ingested task") {
		t.Fatalf("the task itself was dropped rather than its tags:\n%s", out)
	}

	tags := showTags(t, bin, home, env, "T-0001")
	if !hasTag(tags, "storage") {
		t.Errorf("the allowed tag was dropped too: %v", tags)
	}
	if hasTag(tags, "bogustag") {
		t.Errorf("the disallowed tag was ingested: %v", tags)
	}
}

// TestTagPolicyDropsDisallowedTagsOnDoctorFix covers `doctor --fix`,
// which reads the same todo.txt as the startup ingest and so must not
// become the door the ingest closed.
//
// It needs a real PROJECT context, not just a config file: the check
// short-circuits to "no project context" unless DetectProject reports
// InProject with a ConfigPath, and it then resolves todo.txt relative to
// that config's directory — which is why `task.todo_file` must be
// RELATIVE here. The process-wide default is an absolute path, and
// joining a directory to an absolute path yields a file that does not
// exist, so an absolute value makes the whole check silently pass.
func TestTagPolicyDropsDisallowedTagsOnDoctorFix(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	dbPath := filepath.Join(home, "tasks.db")

	proj := filepath.Join(home, "proj")
	tlcDir := filepath.Join(proj, ".tlc")
	if err := os.MkdirAll(tlcDir, 0o750); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	cfg := "project:\n  id: docproj\nstorage:\n  db_path: " + dbPath +
		"\ntask:\n  todo_file: todo.txt\n  tags:\n    policy: closed\n" +
		"    allowed:\n      - storage\n"
	if err := os.WriteFile(filepath.Join(tlcDir, "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	line := `[ ] task_01hyyyyyyyyyyyyyyyyyyyyyyy "doctor task" ` +
		`#storage #bogustag project_id=docproj` + "\n"
	if err := os.WriteFile(filepath.Join(tlcDir, "todo.txt"), []byte(line), 0o600); err != nil {
		t.Fatalf("write project todo: %v", err)
	}

	// No TLC_CONFIG: the project's own .tlc/config.yaml must be what is
	// discovered, since that discovery is what puts DetectProject in
	// project context in the first place.
	env := e2eEnv(t, home, dbPath)

	// Exit code deliberately ignored: `doctor` exits non-zero when ANY of
	// its fourteen checks fails, and this fixture is a bare directory that
	// legitimately fails unrelated ones. What is under test is the state
	// the todo-sync check left behind, which the assertions below read
	// from the store directly.
	out, _ := runTLC(t, bin, proj, env, "doctor", "--fix")
	if !strings.Contains(out, "todo.txt synced") {
		t.Fatalf("doctor did not run the todo-sync check:\n%s", out)
	}
	if strings.Contains(out, "no project context") ||
		strings.Contains(out, "no project todo.txt or empty") {
		t.Fatalf("the fixture did not reach the sync path:\n%s", out)
	}

	tags := showTags(t, bin, proj, env, "T-0001")
	if !hasTag(tags, "storage") {
		t.Errorf("doctor --fix dropped the allowed tag too: %v", tags)
	}
	if hasTag(tags, "bogustag") {
		t.Errorf("doctor --fix wrote a disallowed tag: %v", tags)
	}
}

// TestOpenTagPolicyIngestUnchanged is the no-regression half of the
// ingest path: with no policy configured the filter must be a no-op.
func TestOpenTagPolicyIngestUnchanged(t *testing.T) {
	const withTodo = `storage:
  db_path: %s
task:
  todo_file: TODOFILE
`
	bin := buildTLCBinary(t)
	home := t.TempDir()
	dbPath := filepath.Join(home, "tasks.db")
	todoPath := filepath.Join(home, "todo.txt")

	tmpl := strings.Replace(withTodo, "TODOFILE", todoPath, 1)
	env := statusVocabEnv(t, home, dbPath, writeVocabConfig(t, home, dbPath, tmpl))

	// Raw typeid plus project_id, for the reason spelled out in the
	// closed-policy twin of this test.
	line := `[ ] task_01hzzzzzzzzzzzzzzzzzzzzzzz "ingested task" ` +
		`#storage #anything project_id=unknown` + "\n"
	if err := os.WriteFile(todoPath, []byte(line), 0o600); err != nil {
		t.Fatalf("write todo file: %v", err)
	}

	runTLCOK(t, bin, home, env, "task", "list")

	tags := showTags(t, bin, home, env, "T-0001")
	for _, want := range []string{"storage", "anything"} {
		if !hasTag(tags, want) {
			t.Errorf("open policy dropped %q from ingest: %v", want, tags)
		}
	}
}
