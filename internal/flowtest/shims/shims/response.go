package shims

import (
	"fmt"

	xrr "hop.top/xrr"
	execadapter "hop.top/xrr/adapters/exec"
)

// DecodeExecResponse normalizes an xrr session response into the typed exec
// response. Record and passthrough sessions return *execadapter.Response
// directly; replay sessions always return *xrr.RawResponse (xrr replays the
// stored payload without knowing the adapter's concrete type), so shims must
// accept both to keep record/replay symmetric.
func DecodeExecResponse(resp xrr.Response) (*execadapter.Response, error) {
	switch r := resp.(type) {
	case *execadapter.Response:
		return r, nil
	case *xrr.RawResponse:
		return decodeRawExec(r), nil
	default:
		return nil, fmt.Errorf("unexpected response type %T", resp)
	}
}

// decodeRawExec converts a replayed RawResponse payload map into the exec
// response shape. Payload values come from YAML (int) or JSON (float64)
// decoding, so both numeric forms are accepted.
func decodeRawExec(raw *xrr.RawResponse) *execadapter.Response {
	resp := &execadapter.Response{}
	p := raw.Payload

	if v, ok := p["stdout"]; ok {
		resp.Stdout, _ = v.(string)
	}
	if v, ok := p["stderr"]; ok {
		resp.Stderr, _ = v.(string)
	}
	if v, ok := p["exit_code"]; ok {
		resp.ExitCode = toInt(v)
	}
	if v, ok := p["duration_ms"]; ok {
		resp.DurationMs = int64(toInt(v))
	}
	return resp
}

// toInt coerces YAML/JSON numeric decodings into int.
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}
