package flowtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"charm.land/log/v2"
	"gopkg.in/yaml.v3"
)

// ContractError is returned when an eva contract rejects a step's output.
type ContractError struct {
	StepID  string
	Rule    string
	Message string
}

func (e *ContractError) Error() string {
	if e.Rule != "" {
		return fmt.Sprintf("flowtest: step %q: contract rule %q failed: %s", e.StepID, e.Rule, e.Message)
	}
	return fmt.Sprintf("flowtest: step %q: contract failed: %s", e.StepID, e.Message)
}

// contractFile is the parsed representation of a <stepID>.yaml contract file.
// The contract field is the named contract registered at the EVA gateway.
type contractFile struct {
	Contract string `yaml:"contract"` // EVA contract name
}

// Hook runs an optional eva contract after a step completes.
// Contract evaluation requires the EVA gateway — configured via EVA_URL env var.
// When EVA_URL is not set, the hook passes silently (contracts are not evaluated).
type Hook struct {
	contractsDir string
	evaURL       string // override for tests; defaults to os.Getenv("EVA_URL")
	evaKey       string // override for tests; defaults to os.Getenv("EVA_KEY")
}

// NewHook creates a Hook that looks for contracts in contractsDir.
func NewHook(contractsDir string) *Hook {
	return &Hook{
		contractsDir: contractsDir,
		evaURL:       os.Getenv("EVA_URL"),
		evaKey:       os.Getenv("EVA_KEY"),
	}
}

// WithEvaURL sets the EVA gateway URL (overrides EVA_URL env var). Used in tests.
func (h *Hook) WithEvaURL(url string) *Hook {
	h.evaURL = url
	return h
}

// Run looks for <contractsDir>/<stepID>.yaml. If present and EVA_URL is set,
// it invokes POST {EVA_URL}/v1/contract/invoke with the step output.
//
//   - No contract file → nil (silent pass).
//   - EVA_URL not set → nil + WARN log (contract validation skipped, user notified).
//   - EVA 200 → nil.
//   - EVA 422 (contract_violation) → ContractError.
//   - Other EVA errors → wrapped error.
func (h *Hook) Run(stepID string, output map[string]any) error {
	contractPath := filepath.Join(h.contractsDir, stepID+".yaml")
	if _, err := os.Stat(contractPath); os.IsNotExist(err) {
		return nil // no contract → silent pass
	} else if err != nil {
		return fmt.Errorf("flowtest: hook: stat contract %s: %w", contractPath, err)
	}

	// Read contract file to get the contract name. Done before the EVA_URL
	// gate so the warning can name the contract the user thought was checked.
	data, err := os.ReadFile(contractPath)
	if err != nil {
		return fmt.Errorf("flowtest: hook: read contract %s: %w", contractPath, err)
	}
	var cf contractFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return fmt.Errorf("flowtest: hook: parse contract %s: %w", contractPath, err)
	}
	contractName := cf.Contract
	if contractName == "" {
		contractName = stepID // fall back to step ID as contract name
	}

	if h.evaURL == "" {
		log.Warn(
			"contract present but EVA_URL not set; skipping evaluation. "+
				"Use 'eva run --contract <path> --input <path>' for standalone CI invocation.",
			"contract", contractName,
			"step", stepID,
			"path", contractPath,
		)
		return nil
	}

	return h.invokeEvaGate(stepID, contractName, output)
}

// invokeEvaGate calls POST {evaURL}/v1/contract/invoke.
func (h *Hook) invokeEvaGate(stepID, contractName string, output map[string]any) error {
	payload := map[string]any{
		"contract": contractName,
		"body":     output,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("flowtest: hook: marshal payload: %w", err)
	}

	url := strings.TrimRight(h.evaURL, "/") + "/v1/contract/invoke"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("flowtest: hook: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if h.evaKey != "" {
		req.Header.Set("X-Eva-Key", h.evaKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("flowtest: hook: eva request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for error message

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	// Parse violation response.
	var evaResp struct {
		EvaStatus  string `json:"eva_status"`
		Violations []struct {
			Evaluator string  `json:"evaluator"`
			Reason    *string `json:"reason,omitempty"`
		} `json:"violations,omitempty"`
	}
	if jerr := json.Unmarshal(body, &evaResp); jerr != nil {
		return fmt.Errorf("flowtest: hook: unexpected eva status %d", resp.StatusCode)
	}

	if evaResp.EvaStatus == "contract_violation" && len(evaResp.Violations) > 0 {
		v := evaResp.Violations[0]
		msg := v.Evaluator
		if v.Reason != nil && *v.Reason != "" {
			msg += ": " + *v.Reason
		}
		return &ContractError{StepID: stepID, Rule: v.Evaluator, Message: msg}
	}

	return &ContractError{
		StepID:  stepID,
		Message: fmt.Sprintf("eva status %q (http %d)", evaResp.EvaStatus, resp.StatusCode),
	}
}
