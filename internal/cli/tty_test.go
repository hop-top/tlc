package cli

import (
	"bytes"
	"os"
	"testing"

	"github.com/spf13/cobra"
)

// TestInteractiveAvailableNonTTY verifies that non-terminal streams (the case
// in every test and in CI) gate interactive dialogs off, so callers take their
// non-interactive fallback instead of crashing in huh/bubbletea.
func TestInteractiveAvailableNonTTY(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(&bytes.Buffer{})
	if interactiveAvailable(cmd) {
		t.Fatal("interactiveAvailable must be false when stdin/stdout are buffers")
	}
}

// TestInteractiveAvailablePipeStdin verifies that a pipe (non-char-device
// *os.File) also gates off, even though it is a real *os.File.
func TestInteractiveAvailablePipeStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	cmd := &cobra.Command{}
	cmd.SetIn(r)
	cmd.SetOut(w)
	if interactiveAvailable(cmd) {
		t.Fatal("interactiveAvailable must be false for pipe streams")
	}
}

// TestWriterInteractiveNonTTY verifies the io.Writer variant gates off for
// buffers.
func TestWriterInteractiveNonTTY(t *testing.T) {
	if writerInteractive(&bytes.Buffer{}) {
		t.Fatal("writerInteractive must be false for a bytes.Buffer")
	}
}

// TestStdioInteractiveNonTTY verifies the process-stdio variant gates off
// under `go test`, where os.Stdin/os.Stdout are not character devices. This
// guards the sync conflict-resolution fallback path.
func TestStdioInteractiveNonTTY(t *testing.T) {
	if stdioInteractive() {
		t.Fatal("stdioInteractive must be false when process stdio is not a terminal")
	}
}

// TestCharDevicePipe verifies charDevice rejects a pipe.
func TestCharDevicePipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	if charDevice(r) {
		t.Fatal("charDevice must be false for a pipe read end")
	}
}
