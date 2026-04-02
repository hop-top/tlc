// Package main is the docker-compose shim for the flowtest sandbox.
//
// It intercepts standalone docker-compose (v1) invocations and records/replays
// them via xrr cassettes. The binary name is derived from os.Args[0] so it
// works regardless of whether the binary is called "docker-compose" or any
// alias.
//
// Note: this shim covers the standalone docker-compose binary only. The docker
// compose v2 plugin (subcommand of docker) is intercepted by the docker shim.
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
	name := filepath.Base(os.Args[0])
	mode, cassetteDir := resolveMode(name)

	switch mode {
	case xrr.ModeReplay:
		if err := runReplay(name, cassetteDir); err != nil {
			if errors.Is(err, xrr.ErrCassetteMiss) {
				fmt.Fprintf(os.Stderr,
					"tlc-flow-test: cassette miss for %q: re-run with --record\n", name)
				os.Exit(2)
			}
			fmt.Fprintf(os.Stderr, "tlc-flow-test: replay error: %v\n", err)
			os.Exit(2)
		}
	default:
		// record or passthrough
		if err := runReal(name, cassetteDir, mode); err != nil {
			fmt.Fprintf(os.Stderr, "tlc-flow-test: %v\n", err)
			os.Exit(1)
		}
	}
}

// resolveMode returns the effective mode and cassette dir.
// TLC_FLOW_TEST_PASSTHROUGH can override any mode to passthrough for this name.
func resolveMode(name string) (xrr.Mode, string) {
	mode := xrr.Mode(os.Getenv("TLC_FLOW_TEST_MODE"))
	if mode == "" {
		mode = xrr.ModePassthrough
	}

	passthrough := os.Getenv("TLC_FLOW_TEST_PASSTHROUGH")
	if passthrough != "" {
		for _, entry := range strings.Split(passthrough, ",") {
			if strings.TrimSpace(entry) == name {
				mode = xrr.ModePassthrough
				break
			}
		}
	}

	cassetteDir := os.Getenv("TLC_FLOW_TEST_CASSETTE_DIR")
	return mode, cassetteDir
}

// runReal finds and executes the real binary, optionally recording the result.
func runReal(name, cassetteDir string, mode xrr.Mode) error {
	realPath, err := findReal(name)
	if err != nil {
		return err
	}

	stdinData, err := readStdin()
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	do := func() (xrr.Response, error) {
		return execBinary(realPath, os.Args[1:], stdinData)
	}

	if mode == xrr.ModePassthrough || cassetteDir == "" {
		resp, err := do()
		if err != nil {
			return err
		}
		return emitResponse(resp.(*execadapter.Response))
	}

	// record mode
	req := &execadapter.Request{
		Argv:  os.Args[1:],
		Stdin: string(stdinData),
	}
	c := xrr.NewFileCassette(cassetteDir)
	s := xrr.NewSession(mode, c)
	resp, err := s.Record(context.Background(), execadapter.NewAdapter(), req, do)
	if err != nil {
		return err
	}
	return emitResponse(resp.(*execadapter.Response))
}

// runReplay loads the cassette and emits stored output.
func runReplay(name, cassetteDir string) error {
	stdinData, err := readStdin()
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	req := &execadapter.Request{
		Argv:  os.Args[1:],
		Stdin: string(stdinData),
	}
	c := xrr.NewFileCassette(cassetteDir)
	s := xrr.NewSession(xrr.ModeReplay, c)

	raw, err := s.Record(context.Background(), execadapter.NewAdapter(), req,
		func() (xrr.Response, error) {
			return nil, xrr.ErrCassetteMiss
		},
	)
	if err != nil {
		return err
	}

	// raw is *xrr.RawResponse; decode payload into exec.Response
	rawResp, ok := raw.(*xrr.RawResponse)
	if !ok {
		return fmt.Errorf("unexpected replay response type %T", raw)
	}
	resp, err := decodeRawResponse(rawResp)
	if err != nil {
		return err
	}
	return emitResponse(resp)
}

// execBinary runs the real binary and captures stdout/stderr/exit code.
func execBinary(path string, args []string, stdinData []byte) (xrr.Response, error) {
	start := time.Now()
	cmd := exec.Command(path, args...)
	cmd.Stdin = bytes.NewReader(stdinData)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &outBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)

	exitCode := 0
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, err
		}
	}

	return &execadapter.Response{
		Stdout:     outBuf.String(),
		Stderr:     errBuf.String(),
		ExitCode:   exitCode,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

// emitResponse writes stored output to stdout/stderr and exits with stored code.
func emitResponse(resp *execadapter.Response) error {
	if resp.Stdout != "" {
		fmt.Fprint(os.Stdout, resp.Stdout)
	}
	if resp.Stderr != "" {
		fmt.Fprint(os.Stderr, resp.Stderr)
	}
	if resp.ExitCode != 0 {
		os.Exit(resp.ExitCode)
	}
	return nil
}

// decodeRawResponse converts a RawResponse payload map into exec.Response.
func decodeRawResponse(raw *xrr.RawResponse) (*execadapter.Response, error) {
	resp := &execadapter.Response{}
	p := raw.Payload

	if v, ok := p["stdout"]; ok {
		resp.Stdout, _ = v.(string)
	}
	if v, ok := p["stderr"]; ok {
		resp.Stderr, _ = v.(string)
	}
	if v, ok := p["exit_code"]; ok {
		switch n := v.(type) {
		case int:
			resp.ExitCode = n
		case float64:
			resp.ExitCode = int(n)
		}
	}
	if v, ok := p["duration_ms"]; ok {
		switch n := v.(type) {
		case int64:
			resp.DurationMs = n
		case float64:
			resp.DurationMs = int64(n)
		}
	}
	return resp, nil
}

// readStdin reads all of stdin without blocking if there is nothing to read.
func readStdin() ([]byte, error) {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return nil, err
	}
	if fi.Mode()&os.ModeCharDevice != 0 {
		// terminal — no piped input
		return nil, nil
	}
	return io.ReadAll(os.Stdin)
}

// findReal finds the real binary by scanning PATH and skipping the shim directory.
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
