package cli

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestURIRoot() *cobra.Command {
	root := &cobra.Command{Use: "tlc", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(newURICmd())
	return root
}

func TestURIRegisterHelp(t *testing.T) {
	root := newTestURIRoot()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"uri", "register", "--help"})
	err := root.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "tlc://")
}

func TestURIRegisterOnDarwinFails(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only test")
	}
	root := newTestURIRoot()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"uri", "register"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".app bundle")
}

func TestURISnippetMacOS(t *testing.T) {
	root := newTestURIRoot()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"uri", "snippet", "--platform", "macos"})
	err := root.Execute()
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "CFBundleURLTypes")
	assert.Contains(t, out, "<string>tlc</string>")
}

func TestURISnippetLinux(t *testing.T) {
	root := newTestURIRoot()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"uri", "snippet", "--platform", "linux"})
	err := root.Execute()
	require.NoError(t, err)
	out := buf.String()
	assert.True(t, strings.HasPrefix(out, "[Desktop Entry]"), "should be a .desktop file")
	assert.Contains(t, out, "x-scheme-handler/tlc")
}

func TestURISnippetWindows(t *testing.T) {
	root := newTestURIRoot()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"uri", "snippet", "--platform", "windows"})
	err := root.Execute()
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "tlc")
}

func TestURISnippetUnknownPlatform(t *testing.T) {
	root := newTestURIRoot()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"uri", "snippet", "--platform", "amiga"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown platform")
}

func TestURISnippetDefaultsToCurrentOS(t *testing.T) {
	supportedOS := map[string]bool{"darwin": true, "linux": true, "windows": true}
	if !supportedOS[runtime.GOOS] {
		t.Skipf("unsupported OS: %s", runtime.GOOS)
	}
	root := newTestURIRoot()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"uri", "snippet"})
	err := root.Execute()
	require.NoError(t, err)
	assert.NotEmpty(t, buf.String())
}
