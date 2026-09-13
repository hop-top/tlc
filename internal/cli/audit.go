package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/output"
	"hop.top/tlc/internal/core"
)

// tlc audit — ledger for runs executed by EXTERNAL tools that use tlc
// as their audit home. Second audit leg beside task logs (`tlc log`).
// External tools report
// a run after (or during) execution as one JSON record on stdin;
// operators query it with list/show.

var (
	auditRecordTool  string
	auditRecordStdin bool
	auditListTool    string
	auditListSubject string
	auditListLimit   int
	auditShowTool    string
)

var auditCmd = &cobra.Command{
	Use:   "audit [command]",
	Short: "Audit ledger for external tool runs",
	Long: `Audit ledger for runs executed by external tools.

tlc audits its own tasks (tlc log). The audit ledger is the second
leg: external tools record their runs here so operators get one
queryable history per project.

Runs are keyed by (tool, run id) within the current project. Recording
the same run again replaces it — tools may record once at the end, or
record early and re-record on completion.`,
	Annotations: map[string]string{
		"kit/top-level-verb": "true",
	},
}

var auditRecordCmd = &cobra.Command{
	Use:   "record --tool <name> --stdin",
	Short: "Record one external run from a JSON document on stdin",
	Long: `Record one external-tool run from a JSON document on stdin.

The document shape:

  {
    "run_id": "run-2038",                       (required)
    "subject": "skills/example/SKILL.md",
    "started_at": "2026-08-20T06:00:00Z",       (required, RFC3339)
    "finished_at": "2026-08-20T06:12:31Z",
    "outcome": "promoted",
    "metrics": {"baseline": 0.70, "best": 0.82},
    "steps": [
      {"seq": 1, "name": "generation 1", "status": "rejected"},
      {"seq": 2, "name": "generation 2", "status": "accepted"}
    ]
  }

A "tool" field inside the document is optional; when present it must
match --tool. Recording is an upsert on (tool, run id) in the current
project: steps are replaced wholesale.

Example:
  some-tool report --json | tlc audit record --tool some-tool --stdin`,
	Annotations: map[string]string{
		"kit/side-effect": "write",
		"kit/idempotent":  "yes",
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		if auditRecordTool == "" {
			return fmt.Errorf("--tool is required; re-run with: tlc audit record --tool <name> --stdin")
		}
		if !auditRecordStdin {
			return fmt.Errorf("audit record reads the run document from stdin; re-run with --stdin")
		}

		raw, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		run, err := parseAuditRecord(raw, auditRecordTool)
		if err != nil {
			return err
		}
		run.ProjectID = resolveProjectID()

		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		if err := s.UpsertAuditRun(context.Background(), run); err != nil {
			return err
		}

		format := viper.GetString("output.format")
		out := cmd.OutOrStdout()
		switch format {
		case formatJSON, formatYAML:
			_ = output.Render(out, format, run) //nolint:errcheck // best-effort output
		default:
			outcome := run.Outcome
			if outcome == "" {
				outcome = "recorded"
			}
			fmt.Fprintf(out, "Recorded audit run %s/%s (%s, %d step(s))\n",
				run.Tool, run.RunID, outcome, len(run.Steps))
		}
		return nil
	},
}

var auditListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recorded external runs (newest first)",
	Long: `List recorded external runs, newest first.

Inside a project directory the listing is scoped to that project;
outside one it spans all projects. Steps are omitted — use
'tlc audit show <run-id>' for the full record.`,
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		runs, err := s.ListAuditRuns(context.Background(), core.AuditRunQuery{
			ProjectID: resolveProjectID(),
			Tool:      auditListTool,
			Subject:   auditListSubject,
			Limit:     auditListLimit,
		})
		if err != nil {
			return err
		}

		format := viper.GetString("output.format")
		out := cmd.OutOrStdout()
		switch format {
		case formatJSON, formatYAML:
			_ = output.Render(out, format, runs) //nolint:errcheck // best-effort output
		default:
			renderAuditRunTable(out, runs)
		}
		return nil
	},
}

var auditShowCmd = &cobra.Command{
	Use:   "show <run-id>",
	Short: "Show one external run, steps included",
	Long: `Show one recorded external run in full, steps included.

When the same run id exists for more than one tool, disambiguate with
--tool <name>.`,
	Args: cobra.ExactArgs(1),
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		runID := args[0]

		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		runs, err := s.GetAuditRuns(
			context.Background(), runID, auditShowTool, resolveProjectID(),
		)
		if err != nil {
			return err
		}
		switch {
		case len(runs) == 0:
			return &auditRunNotFoundError{runID: runID, tool: auditShowTool}
		case len(runs) > 1:
			tools := make([]string, 0, len(runs))
			for _, r := range runs {
				tools = append(tools, r.Tool)
			}
			return output.ConflictError(fmt.Sprintf(
				"run id %q is ambiguous across tools (%s); re-run with --tool <name>",
				runID, strings.Join(tools, ", "),
			))
		}
		run := runs[0]

		format := viper.GetString("output.format")
		out := cmd.OutOrStdout()
		switch format {
		case formatJSON, formatYAML:
			_ = output.Render(out, format, run) //nolint:errcheck // best-effort output
		default:
			renderAuditRunDetail(out, run)
		}
		return nil
	},
}

// auditRunNotFoundError maps to ExitNotFound via Unwrap. Follows the
// trackNotFoundError pattern: actionable message, sentinel underneath.
type auditRunNotFoundError struct {
	runID string
	tool  string
}

func (e *auditRunNotFoundError) Error() string {
	if e.tool != "" {
		return fmt.Sprintf(
			"audit run %q for tool %q not found; run 'tlc audit list --tool %s' to see recorded runs",
			e.runID, e.tool, e.tool,
		)
	}
	return fmt.Sprintf(
		"audit run %q not found; run 'tlc audit list' to see recorded runs", e.runID,
	)
}

func (e *auditRunNotFoundError) Unwrap() error { return ErrNotFound }

// AsCLIError returns the kit output envelope so the cli middleware
// classifies this error as NOT_FOUND (exit 3).
func (e *auditRunNotFoundError) AsCLIError() *output.Error {
	return output.NotFoundError(e.Error())
}

// auditRecordDoc is the wire shape accepted on stdin.
type auditRecordDoc struct {
	Tool       string           `json:"tool"`
	RunID      string           `json:"run_id"`
	Subject    string           `json:"subject"`
	StartedAt  string           `json:"started_at"`
	FinishedAt string           `json:"finished_at"`
	Outcome    string           `json:"outcome"`
	Metrics    map[string]any   `json:"metrics"`
	Steps      []core.AuditStep `json:"steps"`
}

// parseAuditRecord validates the stdin document against the wire
// contract and returns the storable run.
func parseAuditRecord(raw []byte, tool string) (*core.AuditRun, error) {
	var doc auditRecordDoc
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("invalid audit record JSON: %w", err)
	}
	if doc.Tool != "" && doc.Tool != tool {
		return nil, fmt.Errorf(
			"document tool %q does not match --tool %q", doc.Tool, tool,
		)
	}
	if doc.RunID == "" {
		return nil, fmt.Errorf("audit record is missing required field \"run_id\"")
	}
	if doc.StartedAt == "" {
		return nil, fmt.Errorf("audit record is missing required field \"started_at\"")
	}
	startedAt, err := time.Parse(time.RFC3339, doc.StartedAt)
	if err != nil {
		return nil, fmt.Errorf("invalid started_at %q: %w; use RFC3339", doc.StartedAt, err)
	}
	var finishedAt *time.Time
	if doc.FinishedAt != "" {
		t, err := time.Parse(time.RFC3339, doc.FinishedAt)
		if err != nil {
			return nil, fmt.Errorf("invalid finished_at %q: %w; use RFC3339", doc.FinishedAt, err)
		}
		finishedAt = &t
	}

	steps := doc.Steps
	// Assign 1-based sequence when the document omits seq entirely,
	// preserving document order.
	allZero := len(steps) > 0
	for _, st := range steps {
		if st.Seq != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		for i := range steps {
			steps[i].Seq = i + 1
		}
	}
	for _, st := range steps {
		if st.Name == "" {
			return nil, fmt.Errorf("audit step %d is missing required field \"name\"", st.Seq)
		}
	}

	return &core.AuditRun{
		Tool:       tool,
		RunID:      doc.RunID,
		Subject:    doc.Subject,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Outcome:    doc.Outcome,
		Metrics:    doc.Metrics,
		Steps:      steps,
	}, nil
}

// auditRunRow is the table schema for `tlc audit list`.
type auditRunRow struct {
	Tool     string `table:"Tool"`
	RunID    string `table:"Run ID"`
	Subject  string `table:"Subject"`
	Outcome  string `table:"Outcome"`
	Started  string `table:"Started"`
	Finished string `table:"Finished"`
}

func renderAuditRunTable(w io.Writer, runs []*core.AuditRun) {
	if len(runs) == 0 {
		fmt.Fprintln(w, "No audit runs recorded.")
		return
	}
	rows := make([]auditRunRow, 0, len(runs))
	for _, r := range runs {
		finished := "-"
		if r.FinishedAt != nil {
			finished = r.FinishedAt.Local().Format("2006-01-02 15:04")
		}
		outcome := r.Outcome
		if outcome == "" {
			outcome = "-"
		}
		subject := r.Subject
		if subject == "" {
			subject = "-"
		}
		rows = append(rows, auditRunRow{
			Tool:     r.Tool,
			RunID:    r.RunID,
			Subject:  subject,
			Outcome:  outcome,
			Started:  r.StartedAt.Local().Format("2006-01-02 15:04"),
			Finished: finished,
		})
	}
	_ = renderStyledList(w, formatTable, rows, nil) //nolint:errcheck // best-effort output
}

// auditStepRow is the steps table schema for `tlc audit show`.
type auditStepRow struct {
	Seq    int    `table:"Seq"`
	Name   string `table:"Name"`
	Status string `table:"Status"`
	Detail string `table:"Detail"`
}

func renderAuditRunDetail(w io.Writer, r *core.AuditRun) {
	fmt.Fprintf(w, "Run:      %s/%s\n", r.Tool, r.RunID)
	if r.Subject != "" {
		fmt.Fprintf(w, "Subject:  %s\n", r.Subject)
	}
	if r.ProjectID != "" {
		fmt.Fprintf(w, "Project:  %s\n", r.ProjectID)
	}
	if r.Outcome != "" {
		fmt.Fprintf(w, "Outcome:  %s\n", r.Outcome)
	}
	fmt.Fprintf(w, "Started:  %s\n", r.StartedAt.Local().Format(time.RFC3339))
	if r.FinishedAt != nil {
		fmt.Fprintf(w, "Finished: %s\n", r.FinishedAt.Local().Format(time.RFC3339))
	}
	if len(r.Metrics) > 0 {
		b, err := json.Marshal(r.Metrics)
		if err == nil {
			fmt.Fprintf(w, "Metrics:  %s\n", string(b))
		}
	}
	if len(r.Steps) == 0 {
		return
	}
	fmt.Fprintln(w)
	rows := make([]auditStepRow, 0, len(r.Steps))
	for _, st := range r.Steps {
		detail := st.Detail
		if detail == "" {
			detail = "-"
		}
		rows = append(rows, auditStepRow{
			Seq: st.Seq, Name: st.Name, Status: st.Status, Detail: detail,
		})
	}
	_ = renderStyledList(w, formatTable, rows, nil) //nolint:errcheck // best-effort output
}

func init() {
	auditRecordCmd.Flags().StringVar(&auditRecordTool, "tool", "", "External tool name (required)")
	auditRecordCmd.Flags().BoolVar(&auditRecordStdin, "stdin", false, "Read the run document from stdin")

	auditListCmd.Flags().StringVar(&auditListTool, "tool", "", "Filter by tool name")
	auditListCmd.Flags().StringVar(&auditListSubject, "subject", "", "Filter by subject")
	auditListCmd.Flags().IntVar(&auditListLimit, "limit", 50, "Maximum runs to return")

	auditShowCmd.Flags().StringVar(&auditShowTool, "tool", "", "Disambiguate by tool name")

	auditCmd.AddCommand(auditRecordCmd)
	auditCmd.AddCommand(auditListCmd)
	auditCmd.AddCommand(auditShowCmd)
	RootCmd.AddCommand(auditCmd)
}
