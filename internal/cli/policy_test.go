package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"hop.top/kit/go/runtime/bus"
	"hop.top/kit/go/runtime/domain"
	"hop.top/kit/go/runtime/policy"
)

// TestInitPolicyEngine_BundledDefaultLoads is a wiring smoke test. It
// resets the once-guarded engine, points TLC_POLICY_FILE at a fresh
// temp YAML containing the bundled default, and confirms the engine
// builds + the bus subscriptions are live (a delete-without-note
// publish surfaces a PolicyDeniedError that wraps domain.ErrConflict
// and exit-maps to 4).
func TestInitPolicyEngine_BundledDefaultLoads(t *testing.T) {
	resetPolicyOnce(t)

	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "policies.yaml")
	if err := os.WriteFile(yamlPath, []byte(`policies:
  - name: delete-requires-note
    on: kit.runtime.entity.pre_persisted
    when: 'payload.Op != "delete" || context.note != ""'
    effect: allow
    otherwise: deny
    message: "deleting a task requires --note explaining why"
`), 0o600); err != nil {
		t.Fatalf("write policy yaml: %v", err)
	}
	t.Setenv("TLC_POLICY_FILE", yamlPath)

	b := bus.New()
	t.Cleanup(func() { _ = b.Close(context.Background()) })

	eng, err := initPolicyEngine(b)
	if err != nil {
		t.Fatalf("initPolicyEngine: %v", err)
	}
	if eng == nil {
		t.Fatal("engine nil after init")
	}

	// Publish a delete pre_persisted with no note in ctx — must veto.
	ev := bus.NewEvent("kit.runtime.entity.pre_persisted", "test", domain.PreEntityPayload{
		Op:       domain.OpDelete,
		Phase:    domain.PhasePrePersisted,
		EntityID: "task_test",
	})
	err = b.Publish(context.Background(), ev)
	if err == nil {
		t.Fatal("expected veto from delete-requires-note; got nil")
	}
	var pde *policy.PolicyDeniedError
	if !errors.As(err, &pde) {
		t.Fatalf("err = %v; want PolicyDeniedError", err)
	}
	if pde.PolicyName != "delete-requires-note" {
		t.Errorf("policy name = %q; want %q", pde.PolicyName, "delete-requires-note")
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Error("PolicyDeniedError must unwrap to domain.ErrConflict")
	}
	if got := exitCodeFor(err); got != 4 {
		t.Errorf("exitCodeFor = %d; want 4", got)
	}

	// Same publish, this time with a note in ctx — must allow.
	ctx := context.WithValue(context.Background(), policy.ContextAttrsKey, map[string]any{
		"note": "fixture cleanup",
	})
	if err := b.Publish(ctx, ev); err != nil {
		t.Fatalf("publish with note vetoed: %v", err)
	}
}

// resetPolicyOnce clears the package-level sync.Once + cached engine so
// the bootstrap can be re-run from a single test process. tests that
// touch the global engine MUST t.Cleanup back to a clean slate.
func resetPolicyOnce(t *testing.T) {
	t.Helper()
	prevEng := policyEng
	prevUnwire := policyUnwire
	prevErr := policyErr

	policyOnceReset()
	t.Cleanup(func() {
		if policyUnwire != nil {
			policyUnwire()
		}
		policyEng = prevEng
		policyUnwire = prevUnwire
		policyErr = prevErr
		policyOnceReset()
	})
}
