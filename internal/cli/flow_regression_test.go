package cli

import (
	"bytes"
	"testing"

	"hop.top/tlc/internal/core"
)

// --- Regression test 4: topo sort must use map key, not step.ID ---
//
// The original bug: flowTopoSort built in-degree using step.ID instead
// of the map key. When step.ID differs from (or is empty compared to)
// the map key, the sort either panics or silently produces wrong order.

func TestFlowTopoSort_UsesMapKey_NotStepID(t *testing.T) {
	// Build a flow where map keys are "alpha", "beta", "gamma"
	// but step.ID fields are deliberately different ("x", "y", "z").
	// Topo sort must still work using the map keys.
	flow := &core.Flow{
		ID:   "flow:regression-topo:1.0",
		Name: "Topo Regression",
		Steps: map[string]core.Step{
			"alpha": {
				ID:    "x",
				Type:  "task",
				Title: "First",
			},
			"beta": {
				ID:        "y",
				Type:      "task",
				Title:     "Second",
				DependsOn: []string{"alpha"},
			},
			"gamma": {
				ID:        "z",
				Type:      "task",
				Title:     "Third",
				DependsOn: []string{"beta"},
			},
		},
	}

	order, err := flowTopoSort(flow)
	if err != nil {
		t.Fatalf("flowTopoSort failed: %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("expected 3 steps; got %d: %v", len(order), order)
	}

	// alpha must come before beta, beta before gamma.
	idx := make(map[string]int, len(order))
	for i, id := range order {
		idx[id] = i
	}

	if idx["alpha"] >= idx["beta"] {
		t.Errorf("alpha (%d) should precede beta (%d)", idx["alpha"], idx["beta"])
	}
	if idx["beta"] >= idx["gamma"] {
		t.Errorf("beta (%d) should precede gamma (%d)", idx["beta"], idx["gamma"])
	}
}

func TestFlowTopoSort_EmptyStepID(t *testing.T) {
	// Variant: step.ID is empty — sort must still work using map keys.
	flow := &core.Flow{
		ID:   "flow:regression-topo-empty:1.0",
		Name: "Topo Empty ID Regression",
		Steps: map[string]core.Step{
			"a": {Type: "task", Title: "A"},
			"b": {Type: "task", Title: "B", DependsOn: []string{"a"}},
		},
	}

	order, err := flowTopoSort(flow)
	if err != nil {
		t.Fatalf("flowTopoSort failed: %v", err)
	}

	if len(order) != 2 {
		t.Fatalf("expected 2 steps; got %d: %v", len(order), order)
	}
	if order[0] != "a" || order[1] != "b" {
		t.Errorf("expected [a b]; got %v", order)
	}
}

// --- Regression test 5: flowDryRun flag must not leak between tests ---
//
// The original bug: flowDryRun (a package-level bool) was never reset
// by resetFlowFlags, so a --dry-run test would cause all subsequent
// flow tests to silently run in dry-run mode.

func TestFlowDryRun_FlagDoesNotLeak(t *testing.T) {
	// Simulate first test: run with --dry-run.
	flowPath := findFixture(t, "dry-run/happy-path/flow.yaml")

	withTestLock(func() {
		cmd1 := newTestCmd()
		cmd1.AddCommand(FlowCmd)
		buf1 := new(bytes.Buffer)
		cmd1.SetOut(buf1)
		cmd1.SetErr(buf1)
		cmd1.SetArgs([]string{"flow", "run", "--dry-run", flowPath})

		if err := cmd1.Execute(); err != nil {
			t.Fatalf("dry-run failed: %v\noutput: %s", err, buf1.String())
		}
		if !contains(buf1.String(), "No agents dispatched") {
			t.Fatalf("first run should be dry-run; output: %s", buf1.String())
		}

		// Now reset flags (simulating what happens between tests).
		resetFlowFlags()

		// flowDryRun must be false after reset.
		if flowDryRun {
			t.Error("flowDryRun still true after resetFlowFlags(); flag leaked")
		}
	})
}

// --- Regression test 6: dry-run with tlc:// URI ref ---
//
// The original bug: dry-run tried to os.Open the raw flowRef string
// BEFORE resolving URIs, so a tlc:// ref would fail with a confusing
// "open tlc://..." error instead of being resolved first.

func TestFlowDryRun_URIRef_NoOpenCrash(t *testing.T) {
	// Use a URI that will fail resolution (no registry configured) but
	// the error should come from the URI resolver, not from os.Open.
	uriRef := "tlc://hop-top/tlc/flow:fake:1.0"

	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		cmd := newTestCmd()
		cmd.AddCommand(FlowCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"flow", "run", "--dry-run", uriRef})

		err := cmd.Execute()
		// We expect an error (URI can't be resolved in test env), but it
		// must NOT be an os.Open error on the raw URI string.
		if err == nil {
			t.Fatal("expected error for unresolvable URI; got success")
		}

		errMsg := err.Error()
		// The old bug would produce: "cannot open tlc://..."
		// i.e. the raw URI string passed directly to os.Open.
		if contains(errMsg, "cannot open tlc://") {
			t.Errorf("got os.Open error on raw URI — resolver not invoked first: %v", err)
		}
		// The URI itself should not appear in a "no such file" error.
		if contains(errMsg, "open tlc://") {
			t.Errorf("URI treated as file path — resolver not invoked first: %v", err)
		}
	})
}
