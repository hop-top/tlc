//go:build !shimbin

package flowtest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestCatchallReplaysRecordedCassette records an exec interaction through
// the real catchall shim binary, then replays it. Replay hands the shim an
// *xrr.RawResponse (xrr replays payloads untyped), which the shim must
// decode back into the exec result — a regression here surfaces as
// "unexpected response type *xrr.RawResponse" and a hard exit.
func TestCatchallReplaysRecordedCassette(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shim symlink/exec semantics differ on windows")
	}
	if testing.Short() {
		t.Skip("builds a shim binary; skipped in -short mode")
	}

	binDir := t.TempDir()
	shimPath := filepath.Join(binDir, "tlc-shim-catchall")

	build := exec.Command("go", "build", "-tags", "shimbin", "-buildvcs=false",
		"-o", shimPath, "hop.top/tlc/internal/flowtest/shims/catchall")
	build.Env = os.Environ()
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build catchall shim: %v\n%s", err, out)
	}

	// Symlink as a tool name so the shim intercepts "echo".
	toolPath := filepath.Join(binDir, "echo")
	if err := os.Symlink(shimPath, toolPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	cassetteDir := t.TempDir()
	env := append(os.Environ(),
		"TLC_FLOW_TEST_CASSETTE_DIR="+cassetteDir,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)

	// Record: catchall execs the real echo and writes the cassette.
	rec := exec.Command(toolPath, "hello-replay")
	rec.Env = append(env, "TLC_FLOW_TEST_MODE=record")
	recOut, err := rec.CombinedOutput()
	if err != nil {
		t.Fatalf("record run: %v\n%s", err, recOut)
	}
	if got := string(recOut); !strings.Contains(got, "hello-replay") {
		t.Fatalf("record output = %q, want to contain %q", got, "hello-replay")
	}

	// Replay: must emit the recorded stdout, not crash on RawResponse.
	rep := exec.Command(toolPath, "hello-replay")
	rep.Env = append(env, "TLC_FLOW_TEST_MODE=replay")
	repOut, err := rep.CombinedOutput()
	if err != nil {
		t.Fatalf("replay run: %v\n%s", err, repOut)
	}
	if got := string(repOut); !strings.Contains(got, "hello-replay") {
		t.Errorf("replay output = %q, want to contain %q", got, "hello-replay")
	}
	if got := string(repOut); strings.Contains(got, "unexpected response type") {
		t.Errorf("replay crashed on response type: %q", got)
	}
}
