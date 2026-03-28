package core_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

func TestEvaGate_Pass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/contract/invoke" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"eva_status": "pass",
			"attempts":   1,
		})
	}))
	defer srv.Close()

	gate := &core.StepGate{Contract: "test-contract", EvaURL: srv.URL}
	err := core.RunEvaGate(context.Background(), gate, map[string]any{"output": "ok"}, "")
	if err != nil {
		t.Fatalf("expected pass, got error: %v", err)
	}
}

func TestEvaGate_Violation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"eva_status": "contract_violation",
			"attempts":   2,
			"violations": []map[string]any{
				{"evaluator": "contains", "score": 0.0, "reason": "missing required section"},
			},
		})
	}))
	defer srv.Close()

	gate := &core.StepGate{Contract: "test-contract", EvaURL: srv.URL}
	err := core.RunEvaGate(context.Background(), gate, map[string]any{"output": "bad"}, "")
	if err == nil {
		t.Fatal("expected error on violation, got nil")
	}
	if !strings.Contains(err.Error(), "contract_violation") && !strings.Contains(err.Error(), "contract violation") {
		t.Errorf("error message should mention violation, got: %v", err)
	}
	if !strings.Contains(err.Error(), "missing required section") {
		t.Errorf("error message should include violation reason, got: %v", err)
	}
}

func TestEvaGate_NilGate(t *testing.T) {
	// nil gate must be a no-op
	err := core.RunEvaGate(context.Background(), nil, nil, "")
	if err != nil {
		t.Fatalf("nil gate should be no-op, got: %v", err)
	}
}

func TestEvaGate_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	gate := &core.StepGate{Contract: "test-contract", EvaURL: srv.URL}
	err := core.RunEvaGate(context.Background(), gate, nil, "")
	if err == nil {
		t.Fatal("expected error on 502, got nil")
	}
}

// TestEvaGate_AuthHeader verifies EVA_KEY is sent as X-Eva-Key header.
func TestEvaGate_AuthHeader(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Eva-Key")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"eva_status": "pass", "attempts": 1})
	}))
	defer srv.Close()

	gate := &core.StepGate{Contract: "c", EvaURL: srv.URL}
	_ = core.RunEvaGate(context.Background(), gate, nil, "my-test-key")
	if gotKey != "my-test-key" {
		t.Errorf("expected X-Eva-Key my-test-key, got %q", gotKey)
	}
}

// TestEvaGate_Integration tests the full request/response flow via mock HTTP server.
func TestEvaGate_Integration(t *testing.T) {
	var capturedReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify method + path
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/contract/invoke" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// Verify content-type
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		// Verify auth header
		if key := r.Header.Get("X-Eva-Key"); key != "integration-key" {
			t.Errorf("expected X-Eva-Key integration-key, got %q", key)
		}
		// Decode and capture request body
		if err := json.NewDecoder(r.Body).Decode(&capturedReq); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"eva_status": "pass",
			"attempts":   1,
		})
	}))
	defer srv.Close()

	gate := &core.StepGate{Contract: "integration-contract", EvaURL: srv.URL}
	output := map[string]any{"plan": "implement feature X", "quality": "high"}
	err := core.RunEvaGate(context.Background(), gate, output, "integration-key")
	if err != nil {
		t.Fatalf("expected pass, got error: %v", err)
	}

	// Verify request body structure
	if capturedReq["contract"] != "integration-contract" {
		t.Errorf("expected contract integration-contract in body, got %v", capturedReq["contract"])
	}
	body, ok := capturedReq["body"].(map[string]any)
	if !ok {
		t.Fatalf("expected body field in request, got %T", capturedReq["body"])
	}
	if body["plan"] != "implement feature X" {
		t.Errorf("expected body.plan to be passed through, got %v", body["plan"])
	}
}
