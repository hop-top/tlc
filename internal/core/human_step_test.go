package core

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeAudit is an in-memory AuditWriter.
type fakeAudit struct {
	mu      sync.Mutex
	entries []ApprovalAudit
}

func (f *fakeAudit) WriteApprovalAudit(_ context.Context, a ApprovalAudit) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, a)
	return nil
}

func (f *fakeAudit) Last() *ApprovalAudit {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.entries) == 0 {
		return nil
	}
	a := f.entries[len(f.entries)-1]
	return &a
}

// fakeCaps is an in-memory CapabilityChecker.
type fakeCaps struct {
	caps map[string][]string
	err  error
}

func (f *fakeCaps) HasCapability(_ context.Context, profileID, capability string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	for _, c := range f.caps[profileID] {
		if c == capability {
			return true, nil
		}
	}
	return false, nil
}

// fakeWebhook is a deterministic WebhookFirer.
type fakeWebhook struct {
	mu      sync.Mutex
	calls   []map[string]any
	failNow bool
}

func (f *fakeWebhook) Fire(_ context.Context, _ string, payload map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, payload)
	if f.failNow {
		return errors.New("delivery failed")
	}
	return nil
}

func TestGenerateApprovalToken_UrlSafe(t *testing.T) {
	tok, err := GenerateApprovalToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) < 40 {
		t.Errorf("token too short: %d chars", len(tok))
	}
	for _, ch := range tok {
		ok := (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_'
		if !ok {
			t.Errorf("token contains non-url-safe char %q", ch)
		}
	}
}

func TestGenerateApprovalToken_Unique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		tok, err := GenerateApprovalToken()
		if err != nil {
			t.Fatal(err)
		}
		if seen[tok] {
			t.Fatalf("token collision at iter %d", i)
		}
		seen[tok] = true
	}
}

func TestValidateHumanStep_GoodConfigs(t *testing.T) {
	cases := []HumanStepConfig{
		{},
		{Timeout: "24h"},
		{Timeout: "30m", OnTimeout: HumanTimeoutApprove},
		{Timeout: "1h", OnTimeout: HumanTimeoutReject, ApproverCapability: "founder"},
		{OnTimeout: HumanTimeoutKeepWaiting},
		{Webhook: "https://hooks.example.com/notify"},
	}
	for i, c := range cases {
		s := Step{Type: StepTypeHuman, Human: &c}
		if err := ValidateHumanStep(s); err != nil {
			t.Errorf("case %d: %v", i, err)
		}
	}
}

func TestValidateHumanStep_BadTimeoutDuration(t *testing.T) {
	s := Step{Type: StepTypeHuman, Human: &HumanStepConfig{Timeout: "wat"}}
	if err := ValidateHumanStep(s); err == nil {
		t.Fatal("expected error on bad duration")
	}
}

func TestValidateHumanStep_BadOnTimeout(t *testing.T) {
	s := Step{Type: StepTypeHuman, Human: &HumanStepConfig{OnTimeout: "later"}}
	if err := ValidateHumanStep(s); err == nil {
		t.Fatal("expected error on bad on_timeout")
	}
}

func TestHumanStepGate_ApproveResolves(t *testing.T) {
	audit := &fakeAudit{}
	g, err := NewHumanStepGate("run:1", "step:approve", "Approve email send", HumanStepConfig{})
	if err != nil {
		t.Fatal(err)
	}
	g.Audit = audit
	g.Open(context.Background())

	var gotErr error
	done := make(chan ApprovalAudit, 1)
	go func() {
		a, err := g.Wait(context.Background())
		gotErr = err
		done <- a
	}()
	time.Sleep(5 * time.Millisecond)
	if err := g.Approve(context.Background(), "noor", ApprovalChannelCLI); err != nil {
		t.Fatal(err)
	}
	a := <-done
	if gotErr != nil {
		t.Fatal(gotErr)
	}
	if !a.Approved {
		t.Fatal("expected approved")
	}
	if a.By != "noor" {
		t.Errorf("by = %q", a.By)
	}
	if a.Channel != ApprovalChannelCLI {
		t.Errorf("channel = %q", a.Channel)
	}
	if audit.Last() == nil {
		t.Fatal("audit not written")
	}
}

func TestHumanStepGate_RejectCancelsRun(t *testing.T) {
	audit := &fakeAudit{}
	g, _ := NewHumanStepGate("run:1", "step:reject", "Reject this", HumanStepConfig{})
	g.Audit = audit
	g.Open(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		_ = g.Reject(context.Background(), "noor", "policy violation", ApprovalChannelCLI)
	}()
	a, err := g.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a.Approved {
		t.Fatal("expected rejected")
	}
	if a.Reason != "policy violation" {
		t.Errorf("reason = %q", a.Reason)
	}
}

func TestHumanStepGate_CancelResolves(t *testing.T) {
	audit := &fakeAudit{}
	g, _ := NewHumanStepGate("run:1", "step:cancel", "Cancel", HumanStepConfig{})
	g.Audit = audit
	g.Open(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		_ = g.Cancel(context.Background(), "noor")
	}()
	a, err := g.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a.Approved {
		t.Fatal("expected not approved on cancel")
	}
}

func TestHumanStepGate_TimeoutDefaultApprove(t *testing.T) {
	audit := &fakeAudit{}
	g, _ := NewHumanStepGate("run:1", "step:to1", "Auto-approve", HumanStepConfig{
		Timeout:   "20ms",
		OnTimeout: HumanTimeoutApprove,
	})
	g.Audit = audit
	g.Open(context.Background())
	a, err := g.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !a.Approved {
		t.Fatal("expected timeout-approve")
	}
	if !a.TimeoutTriggered {
		t.Fatal("expected timeout flag")
	}
	if a.By != "timeout" {
		t.Errorf("by = %q", a.By)
	}
	if a.Channel != ApprovalChannelTimeout {
		t.Errorf("channel = %q", a.Channel)
	}
}

func TestHumanStepGate_TimeoutDefaultReject(t *testing.T) {
	audit := &fakeAudit{}
	g, _ := NewHumanStepGate("run:1", "step:to2", "Auto-reject", HumanStepConfig{
		Timeout:   "20ms",
		OnTimeout: HumanTimeoutReject,
	})
	g.Audit = audit
	g.Open(context.Background())
	a, err := g.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a.Approved {
		t.Fatal("expected timeout-reject")
	}
}

func TestHumanStepGate_TimeoutKeepWaiting_ResolvedByLaterApprove(t *testing.T) {
	audit := &fakeAudit{}
	g, _ := NewHumanStepGate("run:1", "step:to3", "Keep waiting", HumanStepConfig{
		Timeout:   "20ms",
		OnTimeout: HumanTimeoutKeepWaiting,
	})
	g.Audit = audit
	g.Open(context.Background())
	go func() {
		time.Sleep(60 * time.Millisecond)
		_ = g.Approve(context.Background(), "noor", ApprovalChannelCLI)
	}()
	a, err := g.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !a.Approved {
		t.Fatal("expected later approve to resolve")
	}
	// Audit should have AT LEAST 2 entries: timeout (no resolution) + final approve.
	audit.mu.Lock()
	defer audit.mu.Unlock()
	if len(audit.entries) < 2 {
		t.Fatalf("expected timeout+approve audit entries, got %d", len(audit.entries))
	}
}

func TestHumanStepGate_CapabilityGate_Blocked(t *testing.T) {
	caps := &fakeCaps{caps: map[string][]string{"intern": nil}}
	g, _ := NewHumanStepGate("run:1", "step:cap", "Founder approval", HumanStepConfig{
		ApproverCapability: "founder-approval",
	})
	g.Caps = caps
	g.Open(context.Background())
	err := g.Approve(context.Background(), "intern", ApprovalChannelCLI)
	if err == nil {
		t.Fatal("expected capability rejection")
	}
	if !strings.Contains(err.Error(), "missing capability") {
		t.Errorf("error %q missing 'missing capability'", err)
	}
}

func TestHumanStepGate_CapabilityGate_Allowed(t *testing.T) {
	caps := &fakeCaps{caps: map[string][]string{"jad": {"founder-approval"}}}
	g, _ := NewHumanStepGate("run:1", "step:cap2", "Founder approval", HumanStepConfig{
		ApproverCapability: "founder-approval",
	})
	g.Caps = caps
	g.Open(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		_ = g.Approve(context.Background(), "jad", ApprovalChannelCLI)
	}()
	a, err := g.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !a.Approved {
		t.Fatal("expected approved")
	}
}

func TestHumanStepGate_WebhookFiredOnPause(t *testing.T) {
	hook := &fakeWebhook{}
	g, _ := NewHumanStepGate("run:1", "step:hook", "Webhook fire", HumanStepConfig{
		Webhook: "https://hooks.example.com/notify",
		Timeout: "1h",
	})
	g.Hook = hook
	if got := g.Open(context.Background()); got != StepStatusAwaitingApproval {
		t.Fatalf("expected awaiting_approval, got %q", got)
	}
	if len(hook.calls) != 1 {
		t.Fatalf("expected 1 webhook call, got %d", len(hook.calls))
	}
	payload := hook.calls[0]
	for _, k := range []string{"run_id", "step_id", "title", "approval_url", "expires_at"} {
		if _, ok := payload[k]; !ok {
			t.Errorf("payload missing %q", k)
		}
	}
}

func TestHumanStepGate_WebhookFailureDoesNotBlockPause(t *testing.T) {
	hook := &fakeWebhook{failNow: true}
	g, _ := NewHumanStepGate("run:1", "step:hookfail", "Should still pause", HumanStepConfig{
		Webhook: "https://hooks.example.com/notify",
	})
	g.Hook = hook
	got := g.Open(context.Background())
	if got != StepStatusAwaitingApproval {
		t.Fatalf("expected awaiting_approval despite webhook failure, got %q", got)
	}
}

func TestHumanStepGate_HTTPWebhookFirer_Live(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	firer := &HTTPWebhookFirer{}
	if err := firer.Fire(context.Background(), srv.URL, map[string]any{"x": 1}); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 hit, got %d", hits.Load())
	}
}

func TestHumanStepGate_DoubleResolveErrors(t *testing.T) {
	g, _ := NewHumanStepGate("run:1", "step:dbl", "x", HumanStepConfig{})
	g.Open(context.Background())
	if err := g.Approve(context.Background(), "noor", ApprovalChannelCLI); err != nil {
		t.Fatal(err)
	}
	if err := g.Approve(context.Background(), "noor", ApprovalChannelCLI); err == nil {
		t.Fatal("expected error on double-resolve")
	}
}

func TestHumanStepGate_ParseFlow_YAML_HumanStep(t *testing.T) {
	yaml := `
flow_id: "flow:human:1"
name: "human gate"
version: "1.0"
entry_step: "approve"
steps:
  approve:
    step_id: "approve"
    type: "human"
    title: "Approve email send"
    human:
      timeout: "24h"
      on_timeout: "approve"
      approver_capability: "founder-approval"
      webhook: "https://hooks.example.com/notify"
`
	f, err := ParseFlow(strings.NewReader(yaml), "f.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s, ok := f.Steps["approve"]
	if !ok {
		t.Fatal("step not found")
	}
	if s.Type != StepTypeHuman {
		t.Errorf("type = %q", s.Type)
	}
	if s.Human == nil {
		t.Fatal("human config missing")
	}
	if s.Human.Timeout != "24h" {
		t.Errorf("timeout = %q", s.Human.Timeout)
	}
	if s.Human.OnTimeout != HumanTimeoutApprove {
		t.Errorf("on_timeout = %q", s.Human.OnTimeout)
	}
	if s.Human.ApproverCapability != "founder-approval" {
		t.Errorf("approver_capability = %q", s.Human.ApproverCapability)
	}
	if s.Human.Webhook != "https://hooks.example.com/notify" {
		t.Errorf("webhook = %q", s.Human.Webhook)
	}
}

func TestHumanStepGate_ParseFlow_RejectsBadHumanConfig(t *testing.T) {
	yaml := `
flow_id: "flow:badhuman"
name: "bad"
version: "1.0"
entry_step: "s"
steps:
  s:
    step_id: "s"
    type: "human"
    title: "x"
    human:
      timeout: "garbage"
`
	_, err := ParseFlow(strings.NewReader(yaml), "f.yaml")
	if err == nil {
		t.Fatal("expected error on bad timeout")
	}
}
