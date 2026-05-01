// Package core: file-based approval store for human-step gates.
//
// The store persists approval intent across processes so that
// `tlc flow approve` (CLI) can resolve a gate held open by
// `tlc flow run` (executor). Story 025 / T-0199.
//
// Persistence layout:
//
//	<root>/<run-id>/<step-id>.json
//
// One file per (run, step). Atomic writes via tmp + rename.
// Awaiting → resolved is a one-shot transition; second resolve
// returns "already resolved".
//
// Author: jadb
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ApprovalState is the persisted lifecycle state of a human-step gate.
type ApprovalState string

const (
	ApprovalStateAwaiting ApprovalState = "awaiting"
	ApprovalStateApproved ApprovalState = "approved"
	ApprovalStateRejected ApprovalState = "rejected"
	ApprovalStateCanceled ApprovalState = "canceled"
)

// ApprovalRecord is the on-disk shape per (run, step). All fields are
// JSON-encoded; ResolvedAt is nil until a terminal transition.
type ApprovalRecord struct {
	RunID      string        `json:"run_id"`
	StepID     string        `json:"step_id"`
	Title      string        `json:"title"`
	Status     ApprovalState `json:"status"`
	OpenedAt   time.Time     `json:"opened_at"`
	ResolvedAt *time.Time    `json:"resolved_at,omitempty"`
	By         string        `json:"by,omitempty"`
	Reason     string        `json:"reason,omitempty"`
}

// ApprovalStore is the contract a human-step gate's persistence layer
// must satisfy. The CLI uses this to write intent; the executor uses
// it to read intent and resume the gate.
type ApprovalStore interface {
	Open(ctx context.Context, runID, stepID, title string) error
	Read(ctx context.Context, runID, stepID string) (*ApprovalRecord, error)
	Approve(ctx context.Context, runID, stepID, by, _ string) error
	Reject(ctx context.Context, runID, stepID, by, reason string) error
	CancelRun(ctx context.Context, runID, by string) error
	WaitFor(ctx context.Context, runID, stepID string, poll time.Duration, deadline time.Time) (*ApprovalRecord, error)
}

// FileApprovalStore persists ApprovalRecord rows as JSON files under root.
type FileApprovalStore struct {
	root string
}

// NewFileApprovalStore returns a store rooted at the given directory.
// Directories are created on demand. Use DefaultApprovalsDir for the
// canonical install path.
func NewFileApprovalStore(root string) *FileApprovalStore {
	return &FileApprovalStore{root: root}
}

// DefaultApprovalsDir returns the canonical store root —
// $TLC_DATA_PATH/approvals or $HOME/.tlc/approvals fallback.
func DefaultApprovalsDir() string {
	if v := os.Getenv("TLC_DATA_PATH"); v != "" {
		return filepath.Join(v, "approvals")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".tlc/approvals"
	}
	return filepath.Join(home, ".tlc", "approvals")
}

func (s *FileApprovalStore) pathFor(runID, stepID string) string {
	return filepath.Join(s.root, sanitize(runID), sanitize(stepID)+".json")
}

func (s *FileApprovalStore) runDir(runID string) string {
	return filepath.Join(s.root, sanitize(runID))
}

// sanitize strips path separators so a run-id like "run:abc/def" is
// safe to use as a path segment. Only allows [A-Za-z0-9._-].
func sanitize(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '.', c == '-', c == '_':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}

// Open creates the awaiting record. If a record already exists for the
// (runID, stepID), it is left untouched (idempotent on Open).
func (s *FileApprovalStore) Open(_ context.Context, runID, stepID, title string) error {
	path := s.pathFor(runID, stepID)
	if _, err := os.Stat(path); err == nil {
		return nil // idempotent
	}
	rec := &ApprovalRecord{
		RunID:    runID,
		StepID:   stepID,
		Title:    title,
		Status:   ApprovalStateAwaiting,
		OpenedAt: time.Now().UTC(),
	}
	return s.write(rec)
}

// Read returns the record for (runID, stepID). Returns os.ErrNotExist
// (wrapped) when no record exists.
func (s *FileApprovalStore) Read(_ context.Context, runID, stepID string) (*ApprovalRecord, error) {
	path := s.pathFor(runID, stepID)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("approval record not found: run=%s step=%s", runID, stepID)
		}
		return nil, fmt.Errorf("read approval: %w", err)
	}
	var rec ApprovalRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("parse approval %s: %w", path, err)
	}
	return &rec, nil
}

// Approve resolves the gate as approved. The third unused string arg
// preserves channel symmetry with the gate primitive.
func (s *FileApprovalStore) Approve(ctx context.Context, runID, stepID, by, _ string) error {
	return s.resolve(ctx, runID, stepID, ApprovalStateApproved, by, "")
}

// Reject resolves the gate as rejected with a reason.
func (s *FileApprovalStore) Reject(ctx context.Context, runID, stepID, by, reason string) error {
	return s.resolve(ctx, runID, stepID, ApprovalStateRejected, by, reason)
}

// CancelRun marks every step under runID as canceled (or leaves
// already-resolved steps alone). Used by `tlc flow cancel`.
func (s *FileApprovalStore) CancelRun(ctx context.Context, runID, by string) error {
	dir := s.runDir(runID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("run not found: %s", runID)
		}
		return err
	}
	var firstErr error
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		stepID := strings.TrimSuffix(e.Name(), ".json")
		if err := s.resolve(ctx, runID, stepID, ApprovalStateCanceled, by, "canceled by user"); err != nil {
			// Skip already-resolved entries — cancel-run is best-effort.
			if !strings.Contains(err.Error(), "already resolved") && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// WaitFor blocks until the record reaches a terminal state or deadline
// elapses. Polls at the given interval. Useful for executor wiring.
func (s *FileApprovalStore) WaitFor(
	ctx context.Context, runID, stepID string,
	poll time.Duration, deadline time.Time,
) (*ApprovalRecord, error) {
	if poll <= 0 {
		poll = 100 * time.Millisecond
	}
	for {
		rec, err := s.Read(ctx, runID, stepID)
		if err == nil && rec.Status != ApprovalStateAwaiting {
			return rec, nil
		}
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("approval wait deadline exceeded: run=%s step=%s", runID, stepID)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(poll):
		}
	}
}

func (s *FileApprovalStore) resolve(
	_ context.Context, runID, stepID string,
	status ApprovalState, by, reason string,
) error {
	path := s.pathFor(runID, stepID)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("approval record not found: run=%s step=%s", runID, stepID)
		}
		return fmt.Errorf("read approval: %w", err)
	}
	var rec ApprovalRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return fmt.Errorf("parse approval %s: %w", path, err)
	}
	if rec.Status != ApprovalStateAwaiting {
		return fmt.Errorf("gate already resolved (status=%s)", rec.Status)
	}
	now := time.Now().UTC()
	rec.Status = status
	rec.ResolvedAt = &now
	rec.By = by
	rec.Reason = reason
	return s.write(&rec)
}

func (s *FileApprovalStore) write(rec *ApprovalRecord) error {
	if err := os.MkdirAll(s.runDir(rec.RunID), 0o750); err != nil {
		return fmt.Errorf("mkdir approval dir: %w", err)
	}
	path := s.pathFor(rec.RunID, rec.StepID)
	tmp, err := os.CreateTemp(filepath.Dir(path), "approval-*.tmp")
	if err != nil {
		return fmt.Errorf("create tmp: %w", err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	if err := json.NewEncoder(tmp).Encode(rec); err != nil {
		return fmt.Errorf("encode approval: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
