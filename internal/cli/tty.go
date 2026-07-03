package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"
)

// ttyAvailable reports whether an interactive terminal device is actually
// usable. Its implementation is OS-specific (see tty_unix.go / tty_windows.go):
// on Unix it probes /dev/tty directly, on Windows it checks the console via
// isatty. It is the terminal-openability half of the interactive gate.

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

// stdioInteractive applies the same gate as interactiveAvailable against the
// process stdio (os.Stdin/os.Stdout). Used by call sites that don't hold the
// cobra command — both stdin and stdout must be character devices AND the
// terminal must be openable, so a huh dialog can't run when stdout is piped
// to a file (which would corrupt the redirected output).
func stdioInteractive() bool {
	return charDevice(os.Stdin) && charDevice(os.Stdout) && ttyAvailable()
}
