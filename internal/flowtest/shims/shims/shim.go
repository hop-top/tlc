// Package shimlib provides the shared logic for LLM shim binaries used by the
// flowtest sandbox.  Each shim (claude, codex, gemini, …) calls Run with its
// own name; everything else is handled here.
package shims

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	xrr "hop.top/xrr"
	execadapter "hop.top/xrr/adapters/exec"
)

// Run is the entry-point for every LLM shim binary.
// name is the canonical binary name this shim is proxying (e.g. "claude").
func Run(name string) {
	if err := run(name); err != nil {
		fmt.Fprintf(os.Stderr, "tlc-flow-test: %s: %v\n", name, err)
		os.Exit(1)
	}
}

func run(name string) error {
	mode := xrr.Mode(os.Getenv("TLC_FLOW_TEST_MODE"))
	if mode == "" {
		mode = xrr.ModePassthrough
	}

	// Per-shim passthrough override.
	if isPassthrough(name, os.Getenv("TLC_FLOW_TEST_PASSTHROUGH")) {
		mode = xrr.ModePassthrough
	}

	cassetteDir := os.Getenv("TLC_FLOW_TEST_CASSETTE_DIR")

	cassette := xrr.NewFileCassette(cassetteDir)
	session := xrr.NewSession(mode, cassette)
	defer session.Close()

	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	req := &execadapter.Request{
		Argv:  os.Args[1:],
		Stdin: string(stdin),
	}

	adapter := execadapter.NewAdapter()

	resp, err := session.Record(context.Background(), adapter, req, func() (xrr.Response, error) {
		return runReal(name, os.Args[1:], stdin)
	})
	if err != nil {
		if errors.Is(err, xrr.ErrCassetteMiss) {
			fmt.Fprintf(os.Stderr,
				"tlc-flow-test: cassette miss for %q: re-run with --record\n", name)
			os.Exit(2)
		}
		return fmt.Errorf("cassette record %s: %w", name, err)
	}

	// Replay sessions return *xrr.RawResponse; record/passthrough return
	// the typed exec response. DecodeExecResponse normalizes both.
	r, err := DecodeExecResponse(resp)
	if err != nil {
		return err
	}
	fmt.Fprint(os.Stdout, r.Stdout)
	fmt.Fprint(os.Stderr, r.Stderr)
	os.Exit(r.ExitCode)

	return nil
}

// runReal executes the real binary (outside the shim sandbox directory) and
// returns the captured output as an execadapter.Response.
func runReal(name string, args []string, stdin []byte) (xrr.Response, error) {
	realPath, err := findReal(name)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	cmd := exec.Command(realPath, args...)
	cmd.Stdin = strings.NewReader(string(stdin))

	stdoutBytes, err := cmd.Output()
	elapsed := time.Since(start).Milliseconds()

	var stderrStr string
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderrStr = string(exitErr.Stderr)
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("exec %s: %w", name, err)
		}
	}

	return &execadapter.Response{
		Stdout:     string(stdoutBytes),
		Stderr:     stderrStr,
		ExitCode:   exitCode,
		DurationMs: elapsed,
	}, nil
}

// findReal returns the path of the real binary named `name` by scanning PATH
// and skipping the directory that contains this shim binary.
func findReal(name string) (string, error) {
	shimDir := filepath.Dir(os.Args[0])
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == shimDir {
			continue
		}
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && info.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s: real binary not found in PATH", name)
}

// isPassthrough reports whether name appears in the comma-separated list.
func isPassthrough(name, list string) bool {
	if list == "" {
		return false
	}
	for _, s := range strings.Split(list, ",") {
		if strings.TrimSpace(s) == name {
			return true
		}
	}
	return false
}
