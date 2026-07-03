//go:build windows

package cli

import (
	"os"

	"github.com/mattn/go-isatty"
)

// ttyAvailable reports whether the console is usable for interactive dialogs.
//
// Windows has no /dev/tty. The Unix probe (opening /dev/tty read-write) would
// always fail here and wrongly disable every huh dialog even in a real
// console. Instead, check whether stdout is attached to a Windows console
// (conhost) or a Cygwin/MSYS pty via isatty, which handles the ConPTY and
// MSYS cases go's os.ModeCharDevice does not.
func ttyAvailable() bool {
	fd := os.Stdout.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
