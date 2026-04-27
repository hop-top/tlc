package flowtest_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"charm.land/log/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hop.top/tlc/internal/flowtest"
)

// captureLogs swaps the default logger output for a buffer the test can
// inspect, then restores the original output on cleanup. Returns the buffer.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := log.Default()
	captured := log.New(buf)
	captured.SetLevel(log.WarnLevel)
	log.SetDefault(captured)
	t.Cleanup(func() { log.SetDefault(prev) })
	return buf
}

func TestHookNoContract(t *testing.T) {
	buf := captureLogs(t)
	h := flowtest.NewHook(t.TempDir()) // empty contracts dir
	err := h.Run("step-foo", map[string]any{"result": "ok"})
	assert.NoError(t, err, "no contract → silent pass")
	assert.NotContains(t, buf.String(), "EVA_URL not set",
		"no contract file → no warning")
}

func TestHookMissingContractsDir(t *testing.T) {
	buf := captureLogs(t)
	h := flowtest.NewHook("/nonexistent/contracts")
	err := h.Run("step-foo", map[string]any{"result": "ok"})
	assert.NoError(t, err, "missing dir → silent pass (no contract file)")
	assert.NotContains(t, buf.String(), "EVA_URL not set",
		"missing contracts dir → no warning")
}

func TestHookNoEVAURL(t *testing.T) {
	// Contract exists but EVA_URL is not set → skip eval, return nil + WARN.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "step-foo.yaml"),
		[]byte("contract: my-contract\n"), 0o644))

	t.Setenv("EVA_URL", "") // ensure not set
	buf := captureLogs(t)

	h := flowtest.NewHook(dir)
	err := h.Run("step-foo", map[string]any{"result": "ok"})
	assert.NoError(t, err, "no EVA_URL → silent pass (no error)")

	out := buf.String()
	assert.Contains(t, out, "EVA_URL not set", "expected WARN about EVA_URL")
	assert.Contains(t, out, "my-contract", "warning should name the contract")
	assert.Contains(t, out, "step-foo", "warning should name the step")
	assert.Contains(t, out, "eva run --contract", "warning should suggest eva CLI")
}

func TestHookContractPresentEVAURLSetNoWarn(t *testing.T) {
	// Contract present + EVA_URL set → evaluation proceeds, no warning.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"eva_status":"pass","attempts":1}`))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "step-foo.yaml"),
		[]byte("contract: my-contract\n"), 0o644))

	buf := captureLogs(t)
	h := flowtest.NewHook(dir).WithEvaURL(srv.URL)
	err := h.Run("step-foo", map[string]any{"result": "ok"})
	assert.NoError(t, err)
	assert.NotContains(t, buf.String(), "EVA_URL not set",
		"EVA_URL set → no warning")
}

func TestHookContractPass(t *testing.T) {
	// Start a mock EVA gateway that returns 200 pass.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/contract/invoke", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"eva_status":"pass","attempts":1}`))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "step-foo.yaml"),
		[]byte("contract: step-foo\n"), 0o644))

	h := flowtest.NewHook(dir).WithEvaURL(srv.URL)
	err := h.Run("step-foo", map[string]any{"result": "ok"})
	assert.NoError(t, err)
}

func TestHookContractFail(t *testing.T) {
	reason := "output.missing_field must not be null"
	// Start a mock EVA gateway that returns 422 violation.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		resp := map[string]any{
			"eva_status": "contract_violation",
			"violations": []map[string]any{
				{"evaluator": "must_have_field", "reason": reason},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "step-foo.yaml"),
		[]byte("contract: step-foo\n"), 0o644))

	h := flowtest.NewHook(dir).WithEvaURL(srv.URL)
	err := h.Run("step-foo", map[string]any{"result": "ok"})

	var ce *flowtest.ContractError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, "step-foo", ce.StepID)
	assert.Equal(t, "must_have_field", ce.Rule)
}
