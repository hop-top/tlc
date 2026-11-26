package flowtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// discoverManifest writes manifest as the only run's test.yaml and loads it.
func discoverManifest(t *testing.T, manifest string) (Run, error) {
	t.Helper()
	base := t.TempDir()
	runDir := filepath.Join(base, "fixtures", "r", "run")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "test.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	runs, err := DiscoverRuns("r", base)
	if err != nil {
		return Run{}, err
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %d; want 1", len(runs))
	}
	return runs[0], nil
}

func TestManifestGates_Decoded(t *testing.T) {
	run, err := discoverManifest(t, `name: run
expected_exit: 0
gates:
  review: approve
  sign-off:
    decision: approve
    by: lead
    note: looks good
  ship:
    decision: reject
    by: owner
    reason: not ready
`)
	if err != nil {
		t.Fatalf("DiscoverRuns: %v", err)
	}
	want := map[string]GateDecision{
		"review":   {Decision: GateApprove},
		"sign-off": {Decision: GateApprove, By: "lead", Note: "looks good"},
		"ship":     {Decision: GateReject, By: "owner", Reason: "not ready"},
	}
	if len(run.Gates) != len(want) {
		t.Fatalf("gates = %+v; want %+v", run.Gates, want)
	}
	for id, w := range want {
		if run.Gates[id] != w {
			t.Errorf("gate %s = %+v; want %+v", id, run.Gates[id], w)
		}
	}
}

func TestManifestGates_Invalid(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		wantErr  string
	}{
		{"typo for gates", "expected_exit: 0\ngate:\n  review: approve\n", "gate"},
		{"unknown top-level key", "expected_exit: 0\nbogus: 1\n", "bogus"},
		{"unknown key in a gate", "gates:\n  review:\n    decision: approve\n    decison: approve\n", "decison"},
		{"shorthand reject has no reason", "gates:\n  review: reject\n", "reason"},
		{"reject without reason", "gates:\n  review:\n    decision: reject\n", "reason"},
		{"reject with empty reason", "gates:\n  review:\n    decision: reject\n    reason: \"\"\n", "reason"},
		{"decision missing", "gates:\n  review:\n    by: lead\n", "decision"},
		{"decision other than approve or reject", "gates:\n  review:\n    decision: maybe\n", "maybe"},
		{"shorthand other than approve", "gates:\n  review: maybe\n", "maybe"},
		{"approve with a reason", "gates:\n  review:\n    decision: approve\n    reason: why\n", "reason"},
		{"reject with a note", "gates:\n  review:\n    decision: reject\n    reason: why\n    note: n\n", "note"},
		{"gate is a list", "gates:\n  review: [approve]\n", "review"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := discoverManifest(t, tc.manifest)
			if err == nil {
				t.Fatalf("manifest %q loaded; want an error naming %q", tc.manifest, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v; want it to name %q", err, tc.wantErr)
			}
		})
	}
}

// Strict decoding must not break manifests without gates, empty ones
// included.
func TestManifest_StaysLenientWhereItWas(t *testing.T) {
	for name, manifest := range map[string]string{
		"empty file":   "",
		"comment only": "# nothing here\n",
		"all old keys": "name: run\ndescription: d\nexpected_exit: 0\npassthrough: [git]\nvars:\n  a: b\n",
		"no gates run": "expected_exit: 0\n",
		"empty gates":  "gates: {}\n",
		"null gates":   "gates:\n",
	} {
		t.Run(name, func(t *testing.T) {
			run, err := discoverManifest(t, manifest)
			if err != nil {
				t.Fatalf("DiscoverRuns: %v", err)
			}
			if len(run.Gates) != 0 {
				t.Errorf("gates = %+v; want none", run.Gates)
			}
		})
	}
}
