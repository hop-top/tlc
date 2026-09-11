package cli

import (
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

// TestEnumMessagesReadTheCanon pins the dedup: every rejection message
// names exactly the domain's canonical set, in declaration order. A site
// that reverts to a hard-coded literal drifts from the canon the moment a
// value is added or removed, and this test is what catches it.
func TestEnumMessagesReadTheCanon(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want []string
	}{
		{"status", unknownStatusError("BOGUS"), core.TaskStatusStrings()},
		{"priority", unknownPriorityError("BOGUS"), core.PriorityStrings()},
		{"priority-write", invalidPriorityError("BOGUS"), core.PriorityStrings()},
		{"effort", unknownEffortError("BOGUS"), core.EffortStrings()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.err.Error()
			if !strings.Contains(got, `"BOGUS"`) {
				t.Errorf("message should quote the rejected input, got: %s", got)
			}
			// The joined canon must appear verbatim: that proves the
			// message is rendered FROM the canon rather than from a
			// literal that happens to agree with it today.
			if want := strings.Join(tc.want, ", "); !strings.Contains(got, want) {
				t.Errorf("message should name the canonical set %q, got: %s", want, got)
			}
		})
	}
}

// TestNormalizersReadTheCanon pins the fuzzy normalisers to the same
// canon, so a value added to the domain becomes resolvable without a
// second edit here.
func TestNormalizersReadTheCanon(t *testing.T) {
	for _, want := range core.TaskStatusStrings() {
		got, ok := NormalizeStatus(want)
		if !ok || got != want {
			t.Errorf("NormalizeStatus(%q) = (%q, %v), want (%q, true)", want, got, ok, want)
		}
	}
	for _, want := range core.PriorityStrings() {
		got, ok := NormalizePriority(want)
		if !ok || got != want {
			t.Errorf("NormalizePriority(%q) = (%q, %v), want (%q, true)", want, got, ok, want)
		}
	}
	for _, want := range core.EffortStrings() {
		got, ok := NormalizeEffort(want)
		if !ok || got != want {
			t.Errorf("NormalizeEffort(%q) = (%q, %v), want (%q, true)", want, got, ok, want)
		}
	}
}

// TestFlagEnumsRegisteredPerCommand pins the scoping decision. `--status`
// means different things on a task and on a track, so the registrations
// are command-scoped: a tree-wide declaration would stamp one set onto
// both flags and the last one registered would win everywhere. This test
// fails if someone "simplifies" it to WithFlagEnum.
func TestFlagEnumsRegisteredPerCommand(t *testing.T) {
	// kitRootInstance, not a fresh kitRoot(): the subcommands are attached
	// to the package-level globals by init(), so only the instance carries
	// the real tree. A fresh root has none of them, and every scoped
	// registration below would be vacuously absent.
	root := kitRootInstance

	taskStatuses := strings.Join(core.TaskStatusStrings(), ",")
	trackStatuses := strings.Join(core.TrackStatusStrings(), ",")
	if taskStatuses == trackStatuses {
		t.Fatal("task and track status sets are identical; the scoping rationale no longer holds")
	}

	for _, path := range []string{"task list", "task graph", "task create", "task update"} {
		if got := strings.Join(root.CommandFlagEnum(path, "status"), ","); got != taskStatuses {
			t.Errorf("%s --status enum = %q, want %q", path, got, taskStatuses)
		}
		if got := strings.Join(root.CommandFlagEnum(path, "priority"), ","); got != strings.Join(core.PriorityStrings(), ",") {
			t.Errorf("%s --priority enum = %q, want the priority canon", path, got)
		}
	}
	for _, path := range []string{"track list", "track update"} {
		if got := strings.Join(root.CommandFlagEnum(path, "status"), ","); got != trackStatuses {
			t.Errorf("%s --status enum = %q, want %q", path, got, trackStatuses)
		}
	}

	// No tree-wide registration of a name whose meaning is command-local:
	// that is precisely the drift the scoping avoids.
	if got := root.FlagEnum("status"); got != nil {
		t.Errorf("--status must not be registered tree-wide (got %v); task and track sets differ", got)
	}
}

// TestFlagEnumHelpNamesTheCanonOnce guards against the enum being spelled
// out in a flag's usage string as well as by kit's help suffix, which is
// how the values were duplicated into help text before the dedup.
func TestFlagEnumHelpNamesTheCanonOnce(t *testing.T) {
	root := kitRootInstance
	for _, tc := range [][2]string{
		{"task list", "priority"},
		{"task create", "priority"},
		{"task create", "effort"},
		{"track update", "status"},
	} {
		path, name := tc[0], tc[1]
		cmd, _, err := root.Cmd.Find(strings.Fields(path))
		if err != nil {
			t.Fatalf("find %s: %v", path, err)
		}
		f := cmd.Flags().Lookup(name)
		if f == nil {
			t.Fatalf("%s has no --%s flag", path, name)
		}
		// The declared usage must not hand-write the values: kit appends
		// them from the registration, so a literal here is the duplicate
		// this dedup removed. Checked on the DECLARED usage (kit stamps
		// its "(one of: ...)" suffix later, at Execute time).
		for _, lit := range []string{
			"P0, P1, P2, P3",
			"XS, S, M, L, XL",
			"pending, active, completed, abandoned, archived",
			"TODO, IN_PROGRESS, DONE, SKIPPED",
		} {
			if strings.Contains(f.Usage, lit) {
				t.Errorf("%s --%s usage hand-writes %q; kit renders it from the registration: %q",
					path, name, lit, f.Usage)
			}
		}
		// And the flag must actually have an enum registered, or there
		// would be nothing to render the values from.
		if len(root.CommandFlagEnum(path, name)) == 0 {
			t.Errorf("%s --%s has no enum registered", path, name)
		}
	}
}
