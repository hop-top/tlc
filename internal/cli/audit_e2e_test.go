package cli

// CLI-level tests for `tlc audit` — the external-tool run ledger.
// record (stdin JSON, upsert, validation), list (filters, order, JSON
// shape), show (found / missing / ambiguous-across-tools).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"hop.top/kit/go/console/output"
	"hop.top/tlc/internal/core"
)

func runAudit(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	cmd := newTestCmd()
	cmd.AddCommand(auditCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if stdin != "" {
		cmd.SetIn(strings.NewReader(stdin))
	}
	cmd.SetArgs(append([]string{"audit"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// resetAuditFlags clears package-level flag state between executions
// (cobra flag vars are process-global).
func resetAuditFlags(t *testing.T) {
	t.Helper()
	auditRecordTool, auditRecordStdin = "", false
	auditListTool, auditListSubject, auditListLimit = "", "", 50
	auditShowTool = ""
	t.Cleanup(func() {
		auditRecordTool, auditRecordStdin = "", false
		auditListTool, auditListSubject, auditListLimit = "", "", 50
		auditShowTool = ""
	})
}

const auditDocTemplate = `{
	"run_id": %q,
	"subject": "skills/example/SKILL.md",
	"started_at": "2026-08-20T06:00:00Z",
	"finished_at": "2026-08-20T06:12:31Z",
	"outcome": %q,
	"metrics": {"baseline": 0.7, "best": 0.82},
	"steps": [
		{"seq": 1, "name": "generation 1", "status": "rejected"},
		{"seq": 2, "name": "generation 2", "status": "accepted"}
	]
}`

func recordAuditRun(t *testing.T, tool, runID, outcome string) {
	t.Helper()
	resetAuditFlags(t)
	doc := fmt.Sprintf(auditDocTemplate, runID, outcome)
	out, err := runAudit(t, doc, "record", "--tool", tool, "--stdin")
	if err != nil {
		t.Fatalf("audit record: %v; output:\n%s", err, out)
	}
}

func TestAuditRecordListShowRoundTrip(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	recordAuditRun(t, "looptool", "run-001", "rejected")

	// List, JSON shape.
	setOutputFormatJSON(t)
	resetAuditFlags(t)
	out, err := runAudit(t, "", "list", "--format", "json")
	if err != nil {
		t.Fatalf("audit list: %v; output:\n%s", err, out)
	}
	var runs []*core.AuditRun
	if err := json.Unmarshal([]byte(out), &runs); err != nil {
		t.Fatalf("decode list json: %v; raw:\n%s", err, out)
	}
	if len(runs) != 1 || runs[0].RunID != "run-001" || runs[0].Tool != "looptool" {
		t.Fatalf("unexpected list contents: %+v", runs)
	}
	if len(runs[0].Steps) != 0 {
		t.Fatalf("list must omit steps, got %d", len(runs[0].Steps))
	}

	// Show, JSON shape with steps.
	resetAuditFlags(t)
	out, err = runAudit(t, "", "show", "run-001", "--format", "json")
	if err != nil {
		t.Fatalf("audit show: %v; output:\n%s", err, out)
	}
	var run core.AuditRun
	if err := json.Unmarshal([]byte(out), &run); err != nil {
		t.Fatalf("decode show json: %v; raw:\n%s", err, out)
	}
	if run.Outcome != "rejected" || len(run.Steps) != 2 {
		t.Fatalf("unexpected show contents: %+v", run)
	}
	if run.Metrics["best"].(float64) != 0.82 {
		t.Fatalf("metrics lost: %+v", run.Metrics)
	}
}

func TestAuditRecordUpsertReplaces(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	recordAuditRun(t, "looptool", "run-001", "rejected")
	recordAuditRun(t, "looptool", "run-001", "promoted")

	setOutputFormatJSON(t)
	resetAuditFlags(t)
	out, err := runAudit(t, "", "list", "--format", "json")
	if err != nil {
		t.Fatalf("audit list: %v", err)
	}
	var runs []*core.AuditRun
	if err := json.Unmarshal([]byte(out), &runs); err != nil {
		t.Fatalf("decode: %v; raw:\n%s", err, out)
	}
	if len(runs) != 1 {
		t.Fatalf("upsert must not duplicate: got %d rows", len(runs))
	}
	if runs[0].Outcome != "promoted" {
		t.Fatalf("upsert did not replace outcome: %+v", runs[0])
	}
}

func TestAuditRecordValidation(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	cases := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{
			name:  "malformed json",
			stdin: `{not json`,
			args:  []string{"record", "--tool", "looptool", "--stdin"},
			want:  "invalid audit record JSON",
		},
		{
			name:  "missing run_id",
			stdin: `{"started_at": "2026-08-20T06:00:00Z"}`,
			args:  []string{"record", "--tool", "looptool", "--stdin"},
			want:  "run_id",
		},
		{
			name:  "missing started_at",
			stdin: `{"run_id": "r1"}`,
			args:  []string{"record", "--tool", "looptool", "--stdin"},
			want:  "started_at",
		},
		{
			name:  "bad timestamp",
			stdin: `{"run_id": "r1", "started_at": "yesterday"}`,
			args:  []string{"record", "--tool", "looptool", "--stdin"},
			want:  "RFC3339",
		},
		{
			name:  "tool mismatch",
			stdin: `{"tool": "other", "run_id": "r1", "started_at": "2026-08-20T06:00:00Z"}`,
			args:  []string{"record", "--tool", "looptool", "--stdin"},
			want:  "does not match --tool",
		},
		{
			name:  "missing --stdin",
			stdin: "",
			args:  []string{"record", "--tool", "looptool"},
			want:  "--stdin",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetAuditFlags(t)
			out, err := runAudit(t, tc.stdin, tc.args...)
			if err == nil {
				t.Fatalf("expected error, got success; output:\n%s", out)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err.Error(), tc.want)
			}
		})
	}
}

func TestAuditShowMissingAndAmbiguous(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	// Missing → ErrNotFound sentinel (exit 3 mapping).
	resetAuditFlags(t)
	_, err := runAudit(t, "", "show", "run-missing")
	if err == nil {
		t.Fatal("expected not-found error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound in chain, got: %v", err)
	}

	// Ambiguous across tools → conflict with actionable hint.
	recordAuditRun(t, "looptool", "run-001", "rejected")
	recordAuditRun(t, "othertool", "run-001", "promoted")

	resetAuditFlags(t)
	_, err = runAudit(t, "", "show", "run-001")
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
	var oe *output.Error
	if !errors.As(err, &oe) || oe.Code != output.CodeConflict {
		t.Fatalf("want output CONFLICT error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--tool") {
		t.Fatalf("ambiguity error must hint --tool: %v", err)
	}

	// Disambiguated by --tool succeeds.
	setOutputFormatJSON(t)
	resetAuditFlags(t)
	out, err := runAudit(t, "", "show", "run-001", "--tool", "othertool", "--format", "json")
	if err != nil {
		t.Fatalf("disambiguated show: %v; output:\n%s", err, out)
	}
	var run core.AuditRun
	if err := json.Unmarshal([]byte(out), &run); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if run.Tool != "othertool" || run.Outcome != "promoted" {
		t.Fatalf("wrong run returned: %+v", run)
	}
}

// Guard against cobra flag-state leakage across Execute calls in this
// package's own tests (pattern from run/targets test harness lessons).
var _ = func() *cobra.Command { return auditCmd }()
