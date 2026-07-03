//go:build !windows

package cli

import "os"

// ttyAvailable reports whether the controlling terminal is openable.
//
// huh/bubbletea open /dev/tty directly rather than reading from os.Stdin.
// On the macos-latest GitHub runner, stdin/stdout can satisfy
// os.ModeCharDevice while /dev/tty is unopenable ("device not configured").
// A plain isatty / ModeCharDevice check therefore passes even though the
// dialog will crash the moment huh tries to grab the controlling terminal.
//
// Probing /dev/tty directly is the only reliable gate on Unix: if we cannot
// open it read-write, no huh dialog can, so callers must fall back to their
// non-interactive path (e.g. require --yes / --no-prompt) instead of letting
// bubbletea spew a confusing trace to stderr.
func ttyAvailable() bool {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	_ = tty.Close()
	return true
}
