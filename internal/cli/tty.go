package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"
)

// ttyAvailable reports whether an interactive terminal device is actually
// openable.
//
// huh/bubbletea open /dev/tty directly rather than reading from os.Stdin.
// On the macos-latest GitHub runner, stdin/stdout can satisfy
// os.ModeCharDevice while /dev/tty is unopenable ("device not configured").
// A plain isatty / ModeCharDevice check therefore passes even though the
// dialog will crash the moment huh tries to grab the controlling terminal.
//
// Probing /dev/tty directly is the only reliable gate: if we cannot open it
// read-write, no huh dialog can, so callers must fall back to their
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

// charDevice reports whether f is a character device (interactive stream).
func charDevice(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// interactiveAvailable reports whether cmd is attached to a usable interactive
// terminal: stdin and stdout are both character devices AND /dev/tty is
// openable. It is the single gate every huh dialog should pass before running.
func interactiveAvailable(cmd *cobra.Command) bool {
	stdin, ok := cmd.InOrStdin().(*os.File)
	if !ok || !charDevice(stdin) {
		return false
	}
	stdout, ok := cmd.OutOrStdout().(*os.File)
	if !ok || !charDevice(stdout) {
		return false
	}
	return ttyAvailable()
}

// writerInteractive reports whether w is a character-device terminal AND
// /dev/tty is openable. Used by call sites that only hold an io.Writer rather
// than the full cobra command.
func writerInteractive(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok || !charDevice(f) {
		return false
	}
	return ttyAvailable()
}
