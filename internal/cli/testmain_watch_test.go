package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/spf13/viper"
)

// The stray-config guard in TestMain runs after m.Run() and can only
// report that a file appeared, never who wrote it. By then the writing
// goroutine is gone and the running test has finished, so three
// successive fixes were aimed at candidates inferred from the path
// alone. Two of them missed.
//
// watchForStrayConfig closes that gap. It polls for the config
// directory while the suite runs and, the first time the path exists,
// dumps every goroutine stack. The writer names itself: the dump
// contains the live call chain from the test function down to the
// os.WriteFile, so the report identifies both the test and the code
// path instead of leaving it to be guessed.
//
// The watcher only reads, and only reports on a path whose presence
// already fails the run, so a clean suite never prints anything.

// strayWatchInterval is short enough to catch the window between the
// MkdirAll that creates the directory and the WriteFile that fills it,
// which is where the writing goroutine is still on the stack.
const strayWatchInterval = 500 * time.Microsecond

var strayWatchOnce sync.Once

// watchForStrayConfig starts a background poll for a stray config under
// dir and dumps all goroutine stacks the first time one appears.
//
// The goroutine runs for the lifetime of the test binary by design: it
// must observe a write that can happen in any test, and it holds no
// resource beyond a timer. Reporting is latched, so a leak that
// persists across many later tests still prints exactly one dump.
func watchForStrayConfig(dir string) {
	go func() {
		for {
			if name, ok := findStrayConfig(dir); ok {
				strayWatchOnce.Do(func() { reportStrayWriter(dir, name) })
				return
			}
			time.Sleep(strayWatchInterval)
		}
	}()
}

// findStrayConfig reports the first candidate path that exists under
// dir. It matches on the containing directory as well as the file so
// the dump is taken during the MkdirAll/WriteFile window, while the
// writer is still running.
func findStrayConfig(dir string) (string, bool) {
	for _, name := range strayConfigNames {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return name, true
		}
		if parent := filepath.Dir(name); parent != "." {
			if _, err := os.Stat(filepath.Join(dir, parent)); err == nil {
				return parent, true
			}
		}
	}
	return "", false
}

// reportStrayWriter prints the state a post-hoc check cannot recover:
// the working directory and viper's active config file at write time,
// and the stacks of every live goroutine. The writing goroutine appears
// in that dump with its full call chain.
func reportStrayWriter(dir, name string) {
	buf := make([]byte, 4<<20)
	n := runtime.Stack(buf, true)

	cwd, err := os.Getwd()
	if err != nil {
		cwd = fmt.Sprintf("<getwd failed: %v>", err)
	}

	fmt.Fprintf(os.Stderr,
		"\n=== stray config appeared: %s ===\n"+
			"Dumped while the writer was still running, because the guard\n"+
			"after m.Run() sees only the file and cannot name what wrote it.\n"+
			"The writing goroutine is in the dump below; read it from the\n"+
			"bottom (the test function) up to the config write.\n"+
			"os.Getwd:              %s\n"+
			"viper.ConfigFileUsed:  %s\n"+
			"--- all goroutine stacks ---\n%s\n"+
			"=== end stray config report ===\n\n",
		filepath.Join(dir, name), cwd, viper.ConfigFileUsed(), buf[:n])
}
