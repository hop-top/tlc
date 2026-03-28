package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// evaInvokeRequest is the body for POST /v1/contract/invoke.
type evaInvokeRequest struct {
	Contract string         `json:"contract"`
	Body     map[string]any `json:"body"`
}

// evaInvokeResponse covers both pass and violation responses.
type evaInvokeResponse struct {
	EvaStatus  string         `json:"eva_status"`
	Attempts   int            `json:"attempts"`
	Violations []evaViolation `json:"violations,omitempty"`
}

// evaViolation describes a single evaluator failure from the EVA gateway.
type evaViolation struct {
	Evaluator string  `json:"evaluator"`
	Score     float64 `json:"score"`
	Reason    *string `json:"reason,omitempty"`
}

// RunEvaGate calls the EVA gateway to validate stepOutput against gate.Contract.
//
// Returns nil on pass.
// Returns a descriptive, actionable error on violation or server error.
// A nil gate is a no-op (returns nil immediately).
//
// EVA API contract:
//   - Endpoint: POST {gate.EvaURL}/v1/contract/invoke
//   - Request:  {"contract": "<name>", "body": <stepOutput>}
//   - Auth:     apiKey sent as X-Eva-Key header (omitted when empty)
//   - Pass:     HTTP 200, {"eva_status": "pass", "attempts": N}
//   - Violation: HTTP 422, {"eva_status": "contract_violation", "violations": [...]}
//   - Error:    any other non-200 status
func RunEvaGate(ctx context.Context, gate *StepGate, stepOutput map[string]any, apiKey string) error {
	if gate == nil {
		return nil
	}

	payload := evaInvokeRequest{
		Contract: gate.Contract,
		Body:     stepOutput,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("eva gate: marshal request: %w", err)
	}

	url := strings.TrimRight(gate.EvaURL, "/") + "/v1/contract/invoke"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("eva gate: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("X-Eva-Key", apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("eva gate: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	var evaResp evaInvokeResponse
	if jerr := json.Unmarshal(body, &evaResp); jerr != nil {
		return fmt.Errorf("eva gate: unexpected status %d from %s", resp.StatusCode, gate.EvaURL)
	}

	switch evaResp.EvaStatus {
	case "contract_violation":
		return fmt.Errorf(
			"eva gate: contract violation on %q (%d attempt(s)): %s; "+
				"fix the step output so it satisfies contract %q before retrying",
			gate.Contract, evaResp.Attempts,
			formatEvaViolations(evaResp.Violations),
			gate.Contract,
		)
	case "request_invalid":
		return fmt.Errorf(
			"eva gate: request invalid for contract %q; "+
				"check that the contract name is correct and EVA gateway is reachable at %s",
			gate.Contract, gate.EvaURL,
		)
	default:
		return fmt.Errorf(
			"eva gate: unexpected eva_status %q (http %d) from %s",
			evaResp.EvaStatus, resp.StatusCode, gate.EvaURL,
		)
	}
}

// formatEvaViolations formats a list of violations into a human-readable string.
func formatEvaViolations(vs []evaViolation) string {
	if len(vs) == 0 {
		return "no detail"
	}
	parts := make([]string, 0, len(vs))
	for _, v := range vs {
		s := v.Evaluator
		if v.Reason != nil && *v.Reason != "" {
			s += ": " + *v.Reason
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}
