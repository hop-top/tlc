//go:build !windows

package flowtest

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts cmd into its own process group so that killProcessGroup
// can reap the whole subtree (child + grandchildren) on timeout. Without this,
// exec.CommandContext-style SIGKILL only hits the direct child; a shell
// child's `sleep` grandchild survives and keeps the stdout/stderr pipes open,
// blocking cmd.Wait until the grandchild exits naturally.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcessGroup sends SIGKILL to the entire process group identified by
// cmd.Process.Pid (which equals the pgid because Setpgid=true with Pgid=0).
// Errors are ignored: the process may have already exited between the timeout
// firing and our signal landing, which is the happy path, not a failure.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	// Negative pid targets the process group, per kill(2).
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) //nolint:errcheck // process may have already exited, which is the happy path
}
