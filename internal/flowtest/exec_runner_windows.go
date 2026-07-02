//go:build windows

package flowtest

import "os/exec"

// setProcessGroup is a no-op on Windows: SysProcAttr.Setpgid does not exist
// (process groups are a POSIX concept). Windows job objects would be the
// equivalent if grandchild-reaping is ever needed here; today flowtest only
// runs on linux + macos per .github/workflows/ci.yml, so direct-child SIGKILL
// via cmd.Process.Kill is sufficient.
func setProcessGroup(_ *exec.Cmd) {}

// killProcessGroup falls back to killing only the direct child on Windows.
// See setProcessGroup for the rationale.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
