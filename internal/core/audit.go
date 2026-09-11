package core

import (
	"context"
	"time"
)

// AuditStep is one step inside an externally executed run.
type AuditStep struct {
	Seq    int    `json:"seq" yaml:"seq"`
	Name   string `json:"name" yaml:"name"`
	Status string `json:"status" yaml:"status"`
	Detail string `json:"detail,omitempty" yaml:"detail,omitempty"`
}

// AuditRun is an audit-ledger entry for a run executed by an external
// tool that uses tlc as its audit home. Distinct from AgentRunRecord
// (tlc-launched agents) and flow runs (tlc-executed flows): audit runs
// are reported after the fact by the external tool itself.
type AuditRun struct {
	ProjectID  string         `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	Tool       string         `json:"tool" yaml:"tool"`
	RunID      string         `json:"run_id" yaml:"run_id"`
	Subject    string         `json:"subject,omitempty" yaml:"subject,omitempty"`
	StartedAt  time.Time      `json:"started_at" yaml:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty" yaml:"finished_at,omitempty"`
	Outcome    string         `json:"outcome,omitempty" yaml:"outcome,omitempty"`
	Metrics    map[string]any `json:"metrics,omitempty" yaml:"metrics,omitempty"`
	Steps      []AuditStep    `json:"steps,omitempty" yaml:"steps,omitempty"`
	CreatedAt  time.Time      `json:"created_at" yaml:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at" yaml:"updated_at"`
}

// AuditRunQuery filters ListAuditRuns. Zero-value fields are ignored.
// ProjectID scopes to one project; empty means all projects (callers
// outside a project context list everything, mirroring task behavior
// where rows carry a possibly-empty project id).
type AuditRunQuery struct {
	ProjectID string
	Tool      string
	Subject   string
	Limit     int
}

// AuditRunStore persists external-tool audit runs.
type AuditRunStore interface {
	// UpsertAuditRun creates or replaces a run keyed by
	// (project_id, tool, run_id). Steps are replaced wholesale.
	UpsertAuditRun(ctx context.Context, r *AuditRun) error
	// ListAuditRuns returns runs newest-first WITHOUT steps.
	ListAuditRuns(ctx context.Context, q AuditRunQuery) ([]*AuditRun, error)
	// GetAuditRuns returns all runs matching run_id (optionally
	// narrowed by tool and project), steps included. Multiple rows
	// mean the id is ambiguous across tools/projects.
	GetAuditRuns(ctx context.Context, runID, tool, projectID string) ([]*AuditRun, error)
}
