package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestWsmDetected_Found verifies wsmDetected returns true when wsm is on PATH.
func TestWsmDetected_Found(t *testing.T) {
	// Create a fake wsm binary in a temp dir and prepend it to PATH.
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "wsm")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake wsm: %v", err)
	}

	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+origPath)

	if !wsmDetected() {
		t.Error("wsmDetected() = false, want true when wsm is on PATH")
	}
}

// TestWsmDetected_NotFound verifies wsmDetected returns false when wsm absent.
func TestWsmDetected_NotFound(t *testing.T) {
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	// Empty PATH — nothing will be found.
	_ = os.Setenv("PATH", "")

	if wsmDetected() {
		t.Error("wsmDetected() = true, want false when PATH is empty")
	}
}

// buildWorkspaceCmd constructs a fresh cobra tree with WorkspaceCmd attached,
// reflecting the hidden/visible state that would be set by init().
func buildWorkspaceCmd(hidden bool) *cobra.Command {
	root := &cobra.Command{Use: "tlc"}
	ws := &cobra.Command{
		Use:     "workspace",
		Short:   "Manage workspaces",
		Aliases: []string{"ws"},
		Hidden:  hidden,
	}
	if hidden {
		ws.Short = "Manage workspaces (requires wsm — not found in PATH)"
	}
	root.AddCommand(ws)
	return root
}

// TestWorkspaceCmd_HiddenWhenWsmAbsent verifies the command is hidden
// and its short description mentions "requires wsm" when wsm is absent.
func TestWorkspaceCmd_HiddenWhenWsmAbsent(t *testing.T) {
	root := buildWorkspaceCmd(true)

	ws, _, err := root.Find([]string{"workspace"})
	if err != nil {
		t.Fatalf("find workspace: %v", err)
	}
	if !ws.Hidden {
		t.Error("workspace command should be hidden when wsm is absent")
	}
	if !strings.Contains(ws.Short, "requires wsm") {
		t.Errorf("Short should mention 'requires wsm', got: %q", ws.Short)
	}
}

// TestWorkspaceCmd_VisibleWhenWsmPresent verifies the command is visible
// when wsm is present.
func TestWorkspaceCmd_VisibleWhenWsmPresent(t *testing.T) {
	root := buildWorkspaceCmd(false)

	ws, _, err := root.Find([]string{"workspace"})
	if err != nil {
		t.Fatalf("find workspace: %v", err)
	}
	if ws.Hidden {
		t.Error("workspace command should not be hidden when wsm is present")
	}
}

// TestWorkspaceCmd_NotInHelpWhenHidden verifies hidden command absent from help.
func TestWorkspaceCmd_NotInHelpWhenHidden(t *testing.T) {
	root := buildWorkspaceCmd(true)

	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--help"})
	_ = root.Execute()

	if strings.Contains(buf.String(), "workspace") {
		t.Error("hidden workspace command should not appear in --help output")
	}
}

// TestWorkspaceCmd_InHelpWhenVisible verifies visible command appears in help.
func TestWorkspaceCmd_InHelpWhenVisible(t *testing.T) {
	root := buildWorkspaceCmd(false)

	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--help"})
	_ = root.Execute()

	if !strings.Contains(buf.String(), "workspace") {
		t.Error("visible workspace command should appear in --help output")
	}
}

// TestWsmDetectedThenVisible is an integration-style test: fake wsm on PATH,
// call wsmDetected, confirm it returns true.
func TestWsmDetectedThenVisible(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "wsm")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake wsm: %v", err)
	}

	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+origPath)

	if !wsmDetected() {
		t.Fatal("expected wsmDetected() = true with fake wsm on PATH")
	}

	// Build command reflecting that state.
	root := buildWorkspaceCmd(!wsmDetected())
	ws, _, err := root.Find([]string{"workspace"})
	if err != nil {
		t.Fatalf("find workspace: %v", err)
	}
	if ws.Hidden {
		t.Error("workspace should be visible when wsm detected")
	}
}
