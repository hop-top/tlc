// Package core: human approval step primitive.
//
// Implements story 025 (flow human step). Provides the StepTypeHuman
// constant, HumanStepConfig, validation, URL-safe token generation,
// timeout/default-action handling, capability gate, and the
// HumanStepGate primitive that owns approve/reject/cancel state
// transitions and audit-log entries.
//
// Author: jadb
package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

func encodeJSON(v any) (io.Reader, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

// StepTypeHuman is the human-approval step type. Added to the StepType
// enum in flow.go via init() — kept here to keep story 025 self-contained.
const StepTypeHuman StepType = "human"

// HumanStepStatus extends StepStatus for the awaiting-approval state.
// "awaiting_approval" is added as an alias of StepStatus to avoid breaking
// callers that switch on the existing enum.
const (
	StepStatusAwaitingApproval StepStatus = "awaiting_approval"
	StepStatusRejected         StepStatus = "rejected"
)

// HumanTimeoutAction is the auto-action when timeout elapses.
type HumanTimeoutAction string

const (
	HumanTimeoutApprove     HumanTimeoutAction = "approve"
	HumanTimeoutReject      HumanTimeoutAction = "reject"
	HumanTimeoutKeepWaiting HumanTimeoutAction = "keep_waiting"
)

// ApprovalChannel records how the approval/rejection arrived.
type ApprovalChannel string

const (
	ApprovalChannelCLI     ApprovalChannel = "cli"
	ApprovalChannelWebhook ApprovalChannel = "webhook"
	ApprovalChannelAPI     ApprovalChannel = "api"
	ApprovalChannelTimeout ApprovalChannel = "timeout"
)

// HumanStepConfig holds yaml-level config for a type:human step.
// Carried on Step.Human (added in flow.go).
type HumanStepConfig struct {
	// Timeout is parseable by time.ParseDuration ("24h", "30m", etc).
	// Empty = no timeout.
	Timeout string `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	// OnTimeout is the auto-action when Timeout elapses. Default: keep_waiting.
	OnTimeout HumanTimeoutAction `json:"on_timeout,omitempty" yaml:"on_timeout,omitempty"`
	// ApproverCapability gates who may approve. Empty = anyone.
	ApproverCapability string `json:"approver_capability,omitempty" yaml:"approver_capability,omitempty"`
	// Webhook is fired once on entry to awaiting_approval.
	Webhook string `json:"webhook,omitempty" yaml:"webhook,omitempty"`
}

// ValidateHumanStep checks structural integrity of a human step's
// HumanStepConfig. Called by flow_parser ValidateFlow.
func ValidateHumanStep(s Step) error {
	if s.Type != StepTypeHuman {
		return nil
	}
	if s.Human == nil {
		return nil // empty config is valid (no timeout, anyone can approve)
	}
	if s.Human.Timeout != "" {
		if _, err := time.ParseDuration(s.Human.Timeout); err != nil {
			return fmt.Errorf("invalid timeout %q: %w", s.Human.Timeout, err)
		}
	}
	switch s.Human.OnTimeout {
	case "", HumanTimeoutApprove, HumanTimeoutReject, HumanTimeoutKeepWaiting:
	default:
		return fmt.Errorf("invalid on_timeout %q (want approve|reject|keep_waiting)", s.Human.OnTimeout)
	}
	return nil
}

// GenerateApprovalToken returns a URL-safe random token. 32 bytes of
// entropy → 43-char base64url string. Used as the one-shot approval
// token scoped to (run, step).
func GenerateApprovalToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("token entropy: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// ApprovalAudit records who/when/how a human step was decided.
type ApprovalAudit struct {
	RunID            string          `json:"run_id"`
	StepID           string          `json:"step_id"`
	Approved         bool            `json:"approved"`
	By               string          `json:"by"` // aps profile id
	At               time.Time       `json:"at"`
	Channel          ApprovalChannel `json:"channel"`
	Reason           string          `json:"reason,omitempty"`
	TimeoutTriggered bool            `json:"timeout_triggered,omitempty"`
}

// CapabilityChecker reports whether an aps profile carries a capability.
// Implementation lives in the aps adapter; the gate depends on the
// interface only.
type CapabilityChecker interface {
	HasCapability(ctx context.Context, profileID, capability string) (bool, error)
}

// AuditWriter persists ApprovalAudit entries.
type AuditWriter interface {
	WriteApprovalAudit(ctx context.Context, audit ApprovalAudit) error
}

// WebhookFirer fires the awaiting-approval webhook. The default
// implementation uses net/http; tests inject a fake.
type WebhookFirer interface {
	Fire(ctx context.Context, url string, payload map[string]any) error
}

// HTTPWebhookFirer is the default WebhookFirer.
type HTTPWebhookFirer struct {
	Client *http.Client
}

// Fire posts the JSON payload to the URL with a 5s timeout.
// Delivery failure is returned but does not block the pause (caller logs).
func (h *HTTPWebhookFirer) Fire(ctx context.Context, url string, payload map[string]any) error {
	c := h.Client
	if c == nil {
		c = &http.Client{Timeout: 5 * time.Second}
	}
	body, err := encodeJSON(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook %s returned %d", url, resp.StatusCode)
	}
	return nil
}

// HumanStepGate owns the approve/reject/cancel/timeout state machine
// for a single paused step. One gate per (run-id, step-id).
type HumanStepGate struct {
	RunID    string
	StepID   string
	Title    string
	Token    string
	Config   HumanStepConfig
	Audit    AuditWriter
	Caps     CapabilityChecker
	Hook     WebhookFirer
	OpenedAt time.Time

	// internal
	resolved chan ApprovalAudit
}

// NewHumanStepGate constructs a gate ready for use. Token is generated
// here. Caller must call Open() to fire the webhook and start the timer.
func NewHumanStepGate(runID, stepID, title string, cfg HumanStepConfig) (*HumanStepGate, error) {
	tok, err := GenerateApprovalToken()
	if err != nil {
		return nil, err
	}
	return &HumanStepGate{
		RunID:    runID,
		StepID:   stepID,
		Title:    title,
		Token:    tok,
		Config:   cfg,
		OpenedAt: time.Now().UTC(),
		resolved: make(chan ApprovalAudit, 1),
	}, nil
}

// Open transitions the step to awaiting_approval semantically. Fires the
// webhook (if configured) once. Returns the awaiting_approval status the
// caller persists on the step.
func (g *HumanStepGate) Open(ctx context.Context) StepStatus {
	if g.Config.Webhook != "" && g.Hook != nil {
		_ = g.Hook.Fire(ctx, g.Config.Webhook, map[string]any{
			"run_id":       g.RunID,
			"step_id":      g.StepID,
			"title":        g.Title,
			"approval_url": g.approvalURL(),
			"expires_at":   g.expiresAt(),
		})
		// Per AC #11: delivery failure is logged but does not block pause.
	}
	return StepStatusAwaitingApproval
}

// Wait blocks until the gate is resolved by Approve, Reject, Cancel,
// or timeout. Returns the audit entry that was written.
//
// Concurrency note: Wait is single-consumer; the Open/Wait pattern is
// (1) caller calls Open(), (2) caller persists awaiting_approval, (3)
// caller calls Wait() in a goroutine or blocks if synchronous semantics
// are desired. Resolve methods are safe to call from any goroutine.
func (g *HumanStepGate) Wait(ctx context.Context) (ApprovalAudit, error) {
	timeoutCh := g.timeoutChannel()
	for {
		select {
		case <-ctx.Done():
			return ApprovalAudit{}, ctx.Err()
		case a := <-g.resolved:
			return a, g.persist(ctx, a)
		case <-timeoutCh:
			audit, ok := g.handleTimeout()
			if !ok {
				// keep_waiting: log only, no state change; loop back without
				// resolution, but we must persist the audit-only record and
				// stop the timer (one-shot).
				_ = g.persist(ctx, audit)
				timeoutCh = nil // stop firing
				continue
			}
			return audit, g.persist(ctx, audit)
		}
	}
}

// Approve resolves the gate as success. Returns error if a capability
// gate blocks the caller.
func (g *HumanStepGate) Approve(ctx context.Context, by string, channel ApprovalChannel) error {
	if err := g.checkCapability(ctx, by); err != nil {
		return err
	}
	a := ApprovalAudit{
		RunID: g.RunID, StepID: g.StepID,
		Approved: true, By: by, At: time.Now().UTC(),
		Channel: channel,
	}
	return g.send(a)
}

// Reject resolves the gate as rejected. Capability gate applies.
func (g *HumanStepGate) Reject(ctx context.Context, by, reason string, channel ApprovalChannel) error {
	if err := g.checkCapability(ctx, by); err != nil {
		return err
	}
	a := ApprovalAudit{
		RunID: g.RunID, StepID: g.StepID,
		Approved: false, By: by, At: time.Now().UTC(),
		Channel: channel, Reason: reason,
	}
	return g.send(a)
}

// Cancel resolves the gate as canceled. No capability gate (caller is
// canceling their own run).
func (g *HumanStepGate) Cancel(_ context.Context, by string) error {
	a := ApprovalAudit{
		RunID: g.RunID, StepID: g.StepID,
		Approved: false, By: by, At: time.Now().UTC(),
		Channel: ApprovalChannelCLI, Reason: "canceled by user",
	}
	return g.send(a)
}

func (g *HumanStepGate) send(a ApprovalAudit) error {
	select {
	case g.resolved <- a:
		return nil
	default:
		return errors.New("gate already resolved")
	}
}

func (g *HumanStepGate) checkCapability(ctx context.Context, by string) error {
	if g.Config.ApproverCapability == "" || g.Caps == nil {
		return nil
	}
	ok, err := g.Caps.HasCapability(ctx, by, g.Config.ApproverCapability)
	if err != nil {
		return fmt.Errorf("capability check: %w", err)
	}
	if !ok {
		return fmt.Errorf("caller missing capability: %s", g.Config.ApproverCapability)
	}
	return nil
}

// handleTimeout produces the audit record for a timeout fire, plus a
// boolean reporting whether the gate should resolve (false for
// keep_waiting per AC #8).
func (g *HumanStepGate) handleTimeout() (ApprovalAudit, bool) {
	action := g.Config.OnTimeout
	if action == "" {
		action = HumanTimeoutKeepWaiting
	}
	switch action {
	case HumanTimeoutApprove:
		return ApprovalAudit{
			RunID: g.RunID, StepID: g.StepID,
			Approved: true, By: "timeout", At: time.Now().UTC(),
			Channel: ApprovalChannelTimeout, TimeoutTriggered: true,
		}, true
	case HumanTimeoutReject:
		return ApprovalAudit{
			RunID: g.RunID, StepID: g.StepID,
			Approved: false, By: "timeout", At: time.Now().UTC(),
			Channel: ApprovalChannelTimeout, TimeoutTriggered: true,
			Reason: "timeout elapsed",
		}, true
	default: // keep_waiting
		return ApprovalAudit{
			RunID: g.RunID, StepID: g.StepID,
			Approved: false, By: "timeout", At: time.Now().UTC(),
			Channel: ApprovalChannelTimeout, TimeoutTriggered: true,
			Reason: "timeout elapsed (keep_waiting)",
		}, false
	}
}

func (g *HumanStepGate) timeoutChannel() <-chan time.Time {
	if g.Config.Timeout == "" {
		return nil
	}
	d, err := time.ParseDuration(g.Config.Timeout)
	if err != nil {
		return nil
	}
	return time.After(d)
}

func (g *HumanStepGate) persist(ctx context.Context, a ApprovalAudit) error {
	if g.Audit == nil {
		return nil
	}
	return g.Audit.WriteApprovalAudit(ctx, a)
}

func (g *HumanStepGate) approvalURL() string {
	// Caller may override by inspecting Token + RunID + StepID; we surface
	// a stable default for webhook consumers.
	return fmt.Sprintf("tlc://flow/runs/%s/steps/%s?token=%s", g.RunID, g.StepID, g.Token)
}

func (g *HumanStepGate) expiresAt() string {
	if g.Config.Timeout == "" {
		return ""
	}
	d, err := time.ParseDuration(g.Config.Timeout)
	if err != nil {
		return ""
	}
	return g.OpenedAt.Add(d).Format(time.RFC3339)
}
