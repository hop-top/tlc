package flowtest_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hop.top/tlc/internal/flowtest"
)

func TestHookNoContract(t *testing.T) {
	h := flowtest.NewHook(t.TempDir()) // empty contracts dir
	err := h.Run("step-foo", map[string]any{"result": "ok"})
	assert.NoError(t, err, "no contract → silent pass")
}

func TestHookMissingContractsDir(t *testing.T) {
	h := flowtest.NewHook("/nonexistent/contracts")
	err := h.Run("step-foo", map[string]any{"result": "ok"})
	assert.NoError(t, err, "missing dir → silent pass (no contract file)")
}

func TestHookNoEVAURL(t *testing.T) {
	// Contract exists but EVA_URL is not set → skip evaluation, return nil.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "step-foo.yaml"),
		[]byte("contract: my-contract\n"), 0o644))

	t.Setenv("EVA_URL", "") // ensure not set
	h := flowtest.NewHook(dir)
	err := h.Run("step-foo", map[string]any{"result": "ok"})
	assert.NoError(t, err, "no EVA_URL → silent pass")
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
