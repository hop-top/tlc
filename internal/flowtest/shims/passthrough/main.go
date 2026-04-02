// Package main is the passthrough-always shim for the flowtest sandbox.
//
// It is symlinked to tool names that should always run for real (node, ls,
// cat, grep, etc.) regardless of TLC_FLOW_TEST_MODE. It finds the real
// binary by scanning PATH beyond the sandbox bin/ dir and execs it directly.
// It never reads or writes cassettes.
//go:build shimbin
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	self := filepath.Base(os.Args[0])
	real, err := findReal(self)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tlc-flow-test passthrough: %v\n", err)
		os.Exit(1)
	}

	cmd := exec.Command(real, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "tlc-flow-test passthrough: exec %s: %v\n", self, err)
		os.Exit(1)
	}
}

// findReal walks PATH skipping the sandbox bin/ dir and returns the first
// executable named name.
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
