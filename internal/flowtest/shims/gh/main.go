//go:build shimbin

// Package main is the gh shim for tlc flow test.
//
// When tlc flow test runs, it prepends a sandbox bin/ to PATH. The real gh
// binary is replaced by this shim. Depending on env vars injected by the
// sandbox, the shim records, replays, or passes through every gh invocation.
package main

import (
	"bytes"
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

func main() {
	mode, cassetteDir, passthroughList := readEnv()

	// Force passthrough if "gh" is in TLC_FLOW_TEST_PASSTHROUGH.
	for _, name := range passthroughList {
		if name == "gh" {
			mode = xrr.ModePassthrough
			break
		}
	}

	ctx := context.Background()

	if mode == xrr.ModePassthrough || mode == xrr.ModeRecord {
		realPath, err := findReal("gh")
		if err != nil {
			fmt.Fprintf(os.Stderr, "tlc-flow-test: %v\n", err)
			os.Exit(1)
		}

		if mode == xrr.ModePassthrough {
			resp, err := runReal(realPath, os.Args[1:])
			if err != nil {
				fmt.Fprintf(os.Stderr, "tlc-flow-test: runReal: %v\n", err)
				os.Exit(1)
			}
			emit(resp)
			return
		}

		// record mode: run real, write cassette, emit.
		c := xrr.NewFileCassette(cassetteDir)
		s := xrr.NewSession(xrr.ModeRecord, c)
		req := &execadapter.Request{Argv: os.Args[1:], Stdin: readStdin()}
		resp, err := s.Record(ctx, execadapter.NewAdapter(), req, func() (xrr.Response, error) {
			return runReal(realPath, os.Args[1:])
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "tlc-flow-test: record: %v\n", err)
			os.Exit(1)
		}
		emit(resp)
		return
	}

	// replay mode.
	c := xrr.NewFileCassette(cassetteDir)
	s := xrr.NewSession(xrr.ModeReplay, c)
	req := &execadapter.Request{Argv: os.Args[1:], Stdin: readStdin()}
	resp, err := s.Record(ctx, execadapter.NewAdapter(), req, nil)
	if err != nil {
		if errors.Is(err, xrr.ErrCassetteMiss) {
			fmt.Fprintf(os.Stderr, "tlc-flow-test: cassette miss for \"gh\": re-run with --record\n")
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "tlc-flow-test: replay: %v\n", err)
		os.Exit(1)
	}
	emit(resp)
}

// readEnv reads the three env vars injected by the sandbox.
// Defaults: mode=replay, cassetteDir="", passthroughList=[].
func readEnv() (xrr.Mode, string, []string) {
	mode := xrr.Mode(os.Getenv("TLC_FLOW_TEST_MODE"))
	if mode == "" {
		mode = xrr.ModeReplay
	}
	cassetteDir := os.Getenv("TLC_FLOW_TEST_CASSETTE_DIR")
	var passthroughList []string
	if pt := os.Getenv("TLC_FLOW_TEST_PASSTHROUGH"); pt != "" {
		for _, name := range strings.Split(pt, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				passthroughList = append(passthroughList, name)
			}
		}
	}
	return mode, cassetteDir, passthroughList
}

// readStdin reads all of os.Stdin when data is available, else returns "".
func readStdin() string {
	info, err := os.Stdin.Stat()
	if err != nil {
		return ""
	}
	// Data available if named pipe/regular file or size > 0.
	if (info.Mode() & os.ModeCharDevice) != 0 {
		return ""
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return ""
	}
	return string(data)
}

// runReal executes the real binary with args, captures stdout/stderr/exit.
func runReal(bin string, args []string) (*execadapter.Response, error) {
	start := time.Now()
	cmd := exec.Command(bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("exec %s: %w", bin, err)
		}
	}
	return &execadapter.Response{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		ExitCode:   exitCode,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

// findReal walks PATH, skipping the sandbox bin/ dir, returning the first
// executable named name.
func findReal(name string) (string, error) {
	shimDir := filepath.Dir(os.Args[0])
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == shimDir {
			continue
		}
		p := filepath.Join(dir, name)
		info, err := os.Stat(p)
		if err == nil && info.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("gh: real binary not found in PATH")
}

// emit writes stored output to os.Stdout/os.Stderr and exits with exit_code.
func emit(resp xrr.Response) {
	switch r := resp.(type) {
	case *execadapter.Response:
		fmt.Fprint(os.Stdout, r.Stdout)
		fmt.Fprint(os.Stderr, r.Stderr)
		os.Exit(r.ExitCode)
	case *xrr.RawResponse:
		if v, ok := r.Payload["stdout"]; ok {
			fmt.Fprint(os.Stdout, v)
		}
		if v, ok := r.Payload["stderr"]; ok {
			fmt.Fprint(os.Stderr, v)
		}
		exitCode := 0
		if v, ok := r.Payload["exit_code"]; ok {
			switch code := v.(type) {
			case int:
				exitCode = code
			case float64:
				exitCode = int(code)
			}
		}
		os.Exit(exitCode)
	default:
		fmt.Fprintf(os.Stderr, "tlc-flow-test: unexpected response type %T\n", resp)
		os.Exit(1)
	}
}
