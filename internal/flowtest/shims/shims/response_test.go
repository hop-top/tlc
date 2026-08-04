package shims

import (
	"context"
	"testing"

	xrr "hop.top/xrr"
	execadapter "hop.top/xrr/adapters/exec"
)

// TestDecodeExecResponseReplayRoundTrip records an exec interaction to a
// cassette, replays it (xrr always hands back *xrr.RawResponse on replay),
// and asserts the decoded response matches what was recorded. This is the
// record/replay symmetry the catchall shim relies on.
func TestDecodeExecResponseReplayRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	req := &execadapter.Request{Argv: []string{"echo", "hi"}}
	recorded := &execadapter.Response{
		Stdout:   "hi\n",
		Stderr:   "warn\n",
		ExitCode: 3,
	}

	rec := xrr.NewSession(xrr.ModeRecord, xrr.NewFileCassette(dir))
	if _, err := rec.Record(context.Background(), execadapter.NewAdapter(), req,
		func() (xrr.Response, error) { return recorded, nil },
	); err != nil {
		t.Fatalf("record: %v", err)
	}

	rep := xrr.NewSession(xrr.ModeReplay, xrr.NewFileCassette(dir))
	raw, err := rep.Record(context.Background(), execadapter.NewAdapter(), req,
		func() (xrr.Response, error) {
			t.Fatal("do() must not run in replay mode")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if _, ok := raw.(*xrr.RawResponse); !ok {
		t.Fatalf("replay returned %T, expected *xrr.RawResponse", raw)
	}

	resp, err := DecodeExecResponse(raw)
	if err != nil {
		t.Fatalf("DecodeExecResponse: %v", err)
	}
	if resp.Stdout != recorded.Stdout {
		t.Errorf("Stdout = %q, want %q", resp.Stdout, recorded.Stdout)
	}
	if resp.Stderr != recorded.Stderr {
		t.Errorf("Stderr = %q, want %q", resp.Stderr, recorded.Stderr)
	}
	if resp.ExitCode != recorded.ExitCode {
		t.Errorf("ExitCode = %d, want %d", resp.ExitCode, recorded.ExitCode)
	}
}

func TestDecodeExecResponseTyped(t *testing.T) {
	t.Parallel()

	typed := &execadapter.Response{Stdout: "out", ExitCode: 1}
	resp, err := DecodeExecResponse(typed)
	if err != nil {
		t.Fatalf("DecodeExecResponse: %v", err)
	}
	if resp != typed {
		t.Error("typed response must pass through unchanged")
	}
}

func TestDecodeExecResponseNumericForms(t *testing.T) {
	t.Parallel()

	for name, v := range map[string]any{"int": 7, "int64": int64(7), "float64": float64(7)} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resp, err := DecodeExecResponse(&xrr.RawResponse{
				Payload: map[string]any{"exit_code": v},
			})
			if err != nil {
				t.Fatalf("DecodeExecResponse: %v", err)
			}
			if resp.ExitCode != 7 {
				t.Errorf("ExitCode = %d, want 7", resp.ExitCode)
			}
		})
	}
}

func TestDecodeExecResponseUnknownType(t *testing.T) {
	t.Parallel()

	if _, err := DecodeExecResponse(nil); err == nil {
		t.Error("expected error for unknown response type")
	}
}
