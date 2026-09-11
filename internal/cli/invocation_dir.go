package cli

import (
	"os"
	"sync"
)

// invocationDir is the working directory as of command entry, recorded
// by Execute once -C/--chdir has been applied.
//
// Commands that WRITE a project-local config must resolve their
// destination against this rather than calling os.Getwd() at write
// time. The two differ whenever something moves the process mid-command
// — a goroutine outliving the sandbox it was told to work in, a
// deferred cleanup restoring a saved directory — and a write that
// resolves late lands wherever the process ended up. That failure is
// silent: .tlc/ is gitignored repo-wide, so a config written into the
// wrong directory never appears in git status, and any later run there
// discovers it as a project config and inherits its state.
//
// Guarded by a mutex because tests set it from their own goroutines.
var (
	invocationDirMu sync.RWMutex
	invocationDir   string
)

// SetInvocationDir records the current working directory as the
// invocation directory. Execute calls this once, after -C/--chdir is
// applied and before any command body runs.
func SetInvocationDir() {
	cwd, err := os.Getwd()
	if err != nil {
		// Leave it unset: InvocationDir falls back to os.Getwd(), which
		// is no worse than what a caller would have done anyway.
		return
	}
	invocationDirMu.Lock()
	defer invocationDirMu.Unlock()
	invocationDir = cwd
}

// InvocationDir returns the directory the command was invoked from.
//
// When no invocation directory was recorded — a library caller, or a
// test driving a command directly rather than through Execute — it
// falls back to the current working directory, preserving the previous
// behavior rather than returning an empty path that callers would have
// to special-case.
func InvocationDir() string {
	invocationDirMu.RLock()
	dir := invocationDir
	invocationDirMu.RUnlock()
	if dir != "" {
		return dir
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// resetInvocationDirForTest clears the recorded invocation directory so
// one test's entry point does not leak into the next.
func resetInvocationDirForTest() {
	invocationDirMu.Lock()
	defer invocationDirMu.Unlock()
	invocationDir = ""
}
