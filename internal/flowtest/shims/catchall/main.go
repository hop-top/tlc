// Package main is the catch-all shim for the flowtest sandbox.
//
// It is installed as tlc-shim-catchall and symlinked to any binary name not
// covered by a named shim. The tool name is derived from os.Args[0] via
// filepath.Base — the symlink name IS the tool being intercepted.
//
// Routing (xrr):
//   - record / passthrough: find real binary (skipping sandbox bin/), exec,
//     capture, write cassette, emit output.
//   - replay: load cassette, emit stored output.
//   - miss → stderr message, exit 2.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	execadapter "hop.top/xrr/adapters/exec"

	xrr "hop.top/xrr"
)

func main() {
	self := filepath.Base(os.Args[0])

	mode := xrr.Mode(os.Getenv("TLC_FLOW_TEST_MODE"))

	// passthrough-list override
	passthroughList := strings.Split(os.Getenv("TLC_FLOW_TEST_PASSTHROUGH"), ",")
	for _, t := range passthroughList {
		if strings.TrimSpace(t) == self {
			mode = xrr.ModePassthrough
			break
		}
	}

	cassetteDir := os.Getenv("TLC_FLOW_TEST_CASSETTE_DIR")
	c := xrr.NewFileCassette(cassetteDir)
	s := xrr.NewSession(mode, c)
	defer s.Close() //nolint:errcheck

	stdin := readStdin()

	req := &execadapter.Request{
		Argv:  os.Args[1:],
		Stdin: stdin,
	}

	ctx := context.Background()
	resp, err := s.Record(ctx, execadapter.NewAdapter(), req, func() (xrr.Response, error) {
		return runReal(self, os.Args[1:], stdin)
	})
	if errors.Is(err, xrr.ErrCassetteMiss) {
		fmt.Fprintf(os.Stderr,
			"tlc-flow-test: cassette miss for %q: re-run with --record or add to --passthrough\n", self)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "tlc-flow-test: %s: %v\n", self, err)
		os.Exit(1)
	}

	execResp, ok := resp.(*execadapter.Response)
	if !ok {
		fmt.Fprintf(os.Stderr, "tlc-flow-test: %s: unexpected response type %T\n", self, resp)
		os.Exit(1)
	}

	if execResp.Stdout != "" {
		fmt.Fprint(os.Stdout, execResp.Stdout)
	}
	if execResp.Stderr != "" {
		fmt.Fprint(os.Stderr, execResp.Stderr)
	}
	if execResp.ExitCode != 0 {
		os.Exit(execResp.ExitCode)
	}
}

// readStdin drains os.Stdin when it is not a terminal.
func readStdin() string {
	info, err := os.Stdin.Stat()
	if err != nil {
		return ""
	}
	// only read when piped / redirected
	if info.Mode()&os.ModeCharDevice != 0 {
		return ""
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return ""
	}
	return string(b)
}

// runReal finds the real binary for name (skipping sandbox bin/), executes it,
// and returns the captured output as an xrr.Response.
func runReal(name string, args []string, stdin string) (xrr.Response, error) {
	realPath, err := findReal(name)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(realPath, args...)
	cmd.Env = os.Environ()
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	stdoutBytes, runErr := cmd.Output()
	var exitCode int
	var stderrStr string

	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
			stderrStr = string(exitErr.Stderr)
		} else {
			return nil, runErr
		}
	}

	return &execadapter.Response{
		Stdout:   string(stdoutBytes),
		Stderr:   stderrStr,
		ExitCode: exitCode,
	}, nil
}

// findReal returns the path to the real binary named `name` by scanning PATH
// and skipping the directory containing this shim.
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
