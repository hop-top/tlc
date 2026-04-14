package cli

import (
	"bytes"
	"context"
	"os/exec"
	"syscall"
)

// execRunner implements core.CommandRunner using os/exec.
type execRunner struct{}

func (r *execRunner) Run(
	ctx context.Context, name string, args ...string,
) (stdout string, stderr string, exitCode int, err error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitCode = ws.ExitStatus()
			} else {
				exitCode = exitErr.ExitCode()
			}
			err = nil // non-zero exit is not an execution error
		}
	}
	return
}
