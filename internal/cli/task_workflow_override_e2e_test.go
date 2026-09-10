package cli

// Per-tag workflow overrides (`task.workflows`), driven through a real
// config FILE and the real binary.
//
// The struct-level tests in internal/core exercise GetWorkflowForTags
// directly, and so pass whether or not anything CALLS it — which is
// precisely how this feature sat fully constructed, normalised and
// validated, with zero production callers, for as long as it did. Every
// case here goes through a transition command, so a resolver nobody
// consults fails them.

import (
	"strings"
	"testing"
)

// tagOverrideConfig is the headline fixture. The base state machine is
// deliberately strict — TODO may only reach IN_PROGRESS — while the
// `hotfix` override lets a task jump straight to DONE and forbids the
// IN_PROGRESS step the base requires. Every assertion below is therefore
// two-sided: the override must both ALLOW something the base forbids and
// FORBID something the base allows. A wiring that quietly used the base
// everywhere passes only half of that, and a wiring that used the
// override everywhere passes only the other half.
const tagOverrideConfig = `storage:
  db_path: %s
task:
  default_status: TODO
  statuses:
    - name: TODO
      label: To Do
      role: initial
      tls_marker: " "
    - name: IN_PROGRESS
      label: In Progress
      role: active
      tls_marker: ">"
    - name: DONE
      label: Done
      is_terminal: true
      role: completed
      tls_marker: "x"
    - name: SKIPPED
      label: Skipped
      is_terminal: true
      role: skipped
      tls_marker: "-"
  state_machine:
    rules:
      TODO: [IN_PROGRESS]
      IN_PROGRESS: [DONE, TODO, SKIPPED]
  workflows:
    hotfix:
      state_machine:
        rules:
          TODO: [DONE]
`

// twoOverrideConfig declares two overrides so a task can match both. The
// two rule sets disagree about where TODO may go, so "whichever won" is
// observable, and the ambiguity refusal has something real to prevent.
const twoOverrideConfig = `storage:
  db_path: %s
task:
  default_status: TODO
  statuses:
    - name: TODO
      label: To Do
      role: initial
      tls_marker: " "
    - name: IN_PROGRESS
      label: In Progress
      role: active
      tls_marker: ">"
    - name: DONE
      label: Done
      is_terminal: true
      role: completed
      tls_marker: "x"
    - name: SKIPPED
      label: Skipped
      is_terminal: true
      role: skipped
      tls_marker: "-"
  state_machine:
    rules:
      TODO: [IN_PROGRESS]
      IN_PROGRESS: [DONE, TODO, SKIPPED]
  workflows:
    hotfix:
      state_machine:
        rules:
          TODO: [DONE]
    experimental:
      state_machine:
        rules:
          TODO: [SKIPPED]
`

// strandingConfig's override rules TODO and says nothing about
// IN_PROGRESS. A task tagged `narrow` that is already IN_PROGRESS would,
// under a literal replace, have no rules at all for its current status
// and be stuck there.
const strandingConfig = `storage:
  db_path: %s
task:
  default_status: TODO
  statuses:
    - name: TODO
      label: To Do
      role: initial
      tls_marker: " "
    - name: IN_PROGRESS
      label: In Progress
      role: active
      tls_marker: ">"
    - name: DONE
      label: Done
      is_terminal: true
      role: completed
      tls_marker: "x"
    - name: SKIPPED
      label: Skipped
      is_terminal: true
      role: skipped
      tls_marker: "-"
  state_machine:
    rules:
      TODO: [IN_PROGRESS]
      IN_PROGRESS: [DONE, TODO, SKIPPED]
  workflows:
    narrow:
      state_machine:
        rules:
          TODO: [IN_PROGRESS, DONE]
`

// badOverrideConfig names a status no vocabulary declares, inside an
// override rather than the base machine.
const badOverrideConfig = `storage:
  db_path: %s
task:
  default_status: TODO
  statuses:
    - name: TODO
      label: To Do
      role: initial
      tls_marker: " "
    - name: IN_PROGRESS
      label: In Progress
      role: active
      tls_marker: ">"
    - name: DONE
      label: Done
      is_terminal: true
      role: completed
      tls_marker: "x"
  state_machine:
    rules:
      TODO: [IN_PROGRESS]
      IN_PROGRESS: [DONE, TODO]
  workflows:
    hotfix:
      state_machine:
        rules:
          TODO: [SHIPPED]
`

// showStatus reads a task's landing status back off `task show`, so every
// assertion below checks where the task ACTUALLY went rather than only
// that a command exited 0. A transition that silently no-opped exits 0.
func showStatus(t *testing.T, bin, home string, env []string, id string) string {
	t.Helper()
	return runTLCOK(t, bin, home, env, "task", "show", id)
}

// TestOverrideTagAllowsTransitionBaseForbids is acceptance criterion 1's
// first half. The base machine has no TODO -> DONE edge; the `hotfix`
// override does. The tagged task must be judged by the override.
func TestOverrideTagAllowsTransitionBaseForbids(t *testing.T) {
	bin, home, env := statusVocabFixture(t, tagOverrideConfig)

	runTLCOK(t, bin, home, env, "task", "create", "urgent fix", "--tag", "hotfix")

	out, code := runTLC(t, bin, home, env, "task", "update", "T-0001", "--status", "DONE")
	if code != 0 {
		t.Fatalf("TODO -> DONE should be allowed by the hotfix override (exit %d):\n%s", code, out)
	}
	if shown := showStatus(t, bin, home, env, "T-0001"); !strings.Contains(shown, "DONE") &&
		!strings.Contains(shown, "Done") {
		t.Errorf("task did not land in DONE:\n%s", shown)
	}
}

// TestOverrideTagForbidsTransitionBaseAllows is the other half, and the
// one a half-wiring fails. The base allows TODO -> IN_PROGRESS; the
// hotfix override replaces that rule set and does not. If the override
// were merged with the base, or ignored, this transition would succeed.
func TestOverrideTagForbidsTransitionBaseAllows(t *testing.T) {
	bin, home, env := statusVocabFixture(t, tagOverrideConfig)

	runTLCOK(t, bin, home, env, "task", "create", "urgent fix", "--tag", "hotfix")

	out, code := runTLC(t, bin, home, env, "task", "update", "T-0001", "--status", "IN_PROGRESS")
	if code == 0 {
		t.Fatalf("TODO -> IN_PROGRESS is not in the hotfix override; it must be rejected:\n%s", out)
	}
	if !strings.Contains(out, "hotfix") {
		t.Errorf("rejection should name the override it came from, got:\n%s", out)
	}
	if shown := showStatus(t, bin, home, env, "T-0001"); strings.Contains(shown, "In Progress") {
		t.Errorf("rejected transition must leave the task alone:\n%s", shown)
	}
}

// TestOverrideAppliesToLifecycleCommands pins that the override reaches
// the lifecycle verbs, not only `task update --status`. `task claim`
// transitions to the active status, which the hotfix override forbids
// from TODO — so a claim that succeeds here is a claim consulting the
// base machine.
func TestOverrideAppliesToLifecycleCommands(t *testing.T) {
	bin, home, env := statusVocabFixture(t, tagOverrideConfig)

	runTLCOK(t, bin, home, env, "task", "create", "urgent fix", "--tag", "hotfix")

	out, code := runTLC(t, bin, home, env, "task", "claim", "T-0001")
	if code == 0 {
		t.Fatalf("claim moves TODO -> IN_PROGRESS, which the hotfix override forbids:\n%s", out)
	}

	// And the verb the override DOES permit works: complete goes to the
	// completed-role status, DONE, which the override allows from TODO.
	if out, code := runTLC(t, bin, home, env, "task", "complete", "T-0001"); code != 0 {
		t.Fatalf("complete should be allowed by the hotfix override (exit %d):\n%s", code, out)
	}
	if shown := showStatus(t, bin, home, env, "T-0001"); !strings.Contains(shown, "DONE") &&
		!strings.Contains(shown, "Done") {
		t.Errorf("complete did not land in DONE:\n%s", shown)
	}
}

// TestUntaggedTaskUsesBaseWorkflow is acceptance criterion 2. Same
// config, no matching tag: the base rules apply, unchanged. A wiring that
// applied the sole declared override to every task fails here.
func TestUntaggedTaskUsesBaseWorkflow(t *testing.T) {
	bin, home, env := statusVocabFixture(t, tagOverrideConfig)

	runTLCOK(t, bin, home, env, "task", "create", "ordinary work")
	runTLCOK(t, bin, home, env, "task", "create", "labelled work", "--tag", "backend")

	for _, id := range []string{"T-0001", "T-0002"} {
		// Base forbids TODO -> DONE.
		out, code := runTLC(t, bin, home, env, "task", "update", id, "--status", "DONE")
		if code == 0 {
			t.Fatalf("%s: base machine has no TODO -> DONE edge:\n%s", id, out)
		}
		if strings.Contains(out, "hotfix") {
			t.Errorf("%s: an untagged task must not be judged by an override:\n%s", id, out)
		}
		// Base allows TODO -> IN_PROGRESS.
		if out, code := runTLC(t, bin, home, env, "task", "update", id, "--status", "IN_PROGRESS"); code != 0 {
			t.Fatalf("%s: base machine allows TODO -> IN_PROGRESS (exit %d):\n%s", id, code, out)
		}
	}
}

// TestAmbiguousOverrideTagsRefuse is acceptance criterion 3: a task
// matching two overrides is refused, by name, rather than silently judged
// by whichever the map handed over first.
func TestAmbiguousOverrideTagsRefuse(t *testing.T) {
	bin, home, env := statusVocabFixture(t, twoOverrideConfig)

	runTLCOK(t, bin, home, env, "task", "create", "double tagged",
		"--tag", "hotfix", "--tag", "experimental")

	out, code := runTLC(t, bin, home, env, "task", "update", "T-0001", "--status", "DONE")
	if code == 0 {
		t.Fatalf("a task matching two overrides must be refused, not resolved:\n%s", out)
	}
	for _, want := range []string{"hotfix", "experimental"} {
		if !strings.Contains(out, want) {
			t.Errorf("refusal must name %q so the user can disambiguate, got:\n%s", want, out)
		}
	}
	if shown := showStatus(t, bin, home, env, "T-0001"); !strings.Contains(shown, "TODO") &&
		!strings.Contains(shown, "To Do") {
		t.Errorf("refused transition must leave the task in TODO:\n%s", shown)
	}
}

// TestAmbiguousOverrideRefusalIsDeterministic is the criterion the task
// brief singles out. A task's tags are rebuilt by ranging a Go map, so
// the order the resolver sees varies between runs and a single
// invocation proves nothing. This runs the same command many times over
// the same task and demands one answer, with a stable message.
//
// Under the old first-match rule this is the test that fails: some runs
// would resolve `hotfix` (TODO -> DONE, exit 0) and some `experimental`
// (TODO -> SKIPPED, rejected).
func TestAmbiguousOverrideRefusalIsDeterministic(t *testing.T) {
	bin, home, env := statusVocabFixture(t, twoOverrideConfig)

	runTLCOK(t, bin, home, env, "task", "create", "double tagged",
		"--tag", "hotfix", "--tag", "experimental")

	var first string
	for i := range 25 {
		out, code := runTLC(t, bin, home, env, "task", "update", "T-0001", "--status", "DONE")
		if code == 0 {
			t.Fatalf("run %d resolved an ambiguous task instead of refusing:\n%s", i, out)
		}
		// Compare the tag list the message names, which is the part
		// tag order could perturb.
		names := overrideNamesIn(out)
		if i == 0 {
			first = names
			continue
		}
		if names != first {
			t.Fatalf("run %d named %q, run 0 named %q — tag order is still deciding",
				i, names, first)
		}
	}
	if !strings.Contains(first, "experimental") || !strings.Contains(first, "hotfix") {
		t.Errorf("expected both tags named, got %q", first)
	}
}

// overrideNamesIn extracts the comma-joined tag list from an ambiguity
// refusal, so the determinism check compares the ORDER the message names
// them in rather than incidental output.
func overrideNamesIn(out string) string {
	open := strings.Index(out, "(")
	closeIdx := strings.Index(out, ")")
	if open < 0 || closeIdx < open {
		return ""
	}
	return out[open+1 : closeIdx]
}

// TestUnruledStatusFallsBackToBase is acceptance criterion 5: the
// stranded-status case, demonstrated rather than left to chance. The
// `narrow` override rules TODO and never mentions IN_PROGRESS, so a task
// that reaches IN_PROGRESS has no override rules governing it. It must
// fall back to the base machine, not dead-end.
func TestUnruledStatusFallsBackToBase(t *testing.T) {
	bin, home, env := statusVocabFixture(t, strandingConfig)

	runTLCOK(t, bin, home, env, "task", "create", "narrow flow", "--tag", "narrow")

	// The override rules TODO and allows the step to IN_PROGRESS.
	runTLCOK(t, bin, home, env, "task", "update", "T-0001", "--status", "IN_PROGRESS")

	// Now the task sits in a status the override never mentions. The
	// base allows IN_PROGRESS -> TODO; without the fallback this is
	// rejected with "no transition rules defined for current status".
	out, code := runTLC(t, bin, home, env, "task", "update", "T-0001", "--status", "TODO")
	if code != 0 {
		t.Fatalf("a status the override leaves unruled must fall back to the base (exit %d):\n%s", code, out)
	}
	if shown := showStatus(t, bin, home, env, "T-0001"); !strings.Contains(shown, "TODO") &&
		!strings.Contains(shown, "To Do") {
		t.Errorf("fallback transition did not land in TODO:\n%s", shown)
	}
}

// TestOverrideRuledStatusDoesNotMerge is the boundary of that fallback,
// and the replace-vs-merge decision stated as a test. The `narrow`
// override DOES rule TODO, listing IN_PROGRESS and DONE. The base also
// allows TODO -> SKIPPED. If the fallback were a merge rather than a
// per-status carve-out, SKIPPED would leak in.
func TestOverrideRuledStatusDoesNotMerge(t *testing.T) {
	bin, home, env := statusVocabFixture(t, strandingConfig)

	runTLCOK(t, bin, home, env, "task", "create", "narrow flow", "--tag", "narrow")

	out, code := runTLC(t, bin, home, env, "task", "update", "T-0001", "--status", "SKIPPED")
	if code == 0 {
		t.Fatalf("the override rules TODO, so base rules for TODO must not merge in:\n%s", out)
	}
	if shown := showStatus(t, bin, home, env, "T-0001"); strings.Contains(shown, "Skipped") {
		t.Errorf("rejected transition must leave the task alone:\n%s", shown)
	}
}

// TestNoWorkflowsConfiguredIsUnchanged is acceptance criterion 4. With no
// `task.workflows` at all, every transition path behaves exactly as it
// did before overrides were wired: the built-in rules, the built-in
// message, and no mention of tags anywhere.
func TestNoWorkflowsConfiguredIsUnchanged(t *testing.T) {
	bin, home, env := statusVocabFixture(t, defaultStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "plain", "--tag", "hotfix")

	// Built-in rules: TODO -> IN_PROGRESS yes, TODO -> DONE no. The tag
	// is present but declares no override, so it must change nothing.
	out, code := runTLC(t, bin, home, env, "task", "update", "T-0001", "--status", "DONE")
	if code == 0 {
		t.Fatalf("built-in rules have no TODO -> DONE edge:\n%s", out)
	}
	if strings.Contains(out, "for tag") {
		t.Errorf("with no workflows configured nothing may mention a tag override:\n%s", out)
	}

	runTLCOK(t, bin, home, env, "task", "update", "T-0001", "--status", "IN_PROGRESS")
	runTLCOK(t, bin, home, env, "task", "update", "T-0001", "--status", "DONE")
	if shown := showStatus(t, bin, home, env, "T-0001"); !strings.Contains(shown, "DONE") &&
		!strings.Contains(shown, "Done") {
		t.Errorf("task did not land in DONE:\n%s", shown)
	}
}

// TestOverrideRuleUnknownStatusRejectedAtConfig pins that an override's
// rules get the same validation the base machine's do. Left unchecked,
// a typo'd target builds a workflow that accepts the config and then
// refuses every transition for that tag with a message pointing at the
// transition rather than the config.
func TestOverrideRuleUnknownStatusRejectedAtConfig(t *testing.T) {
	bin, home, env := statusVocabFixture(t, badOverrideConfig)

	out, code := runTLC(t, bin, home, env, "task", "create", "probe")
	if code == 0 {
		t.Fatalf("an override naming an undeclared status must be rejected:\n%s", out)
	}
	if !strings.Contains(out, "SHIPPED") {
		t.Errorf("error should name the offending status, got:\n%s", out)
	}
	if !strings.Contains(out, "hotfix") {
		t.Errorf("error should name the override it came from, got:\n%s", out)
	}
}
