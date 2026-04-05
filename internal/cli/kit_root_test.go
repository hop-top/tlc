package cli

import (
	"bytes"
	"testing"
)

func TestKitRootHelp(t *testing.T) {
	buf := new(bytes.Buffer)
	root := kitRoot()
	root.Cmd.SetOut(buf)
	root.Cmd.SetErr(buf)
	root.Cmd.SetArgs([]string{"--help"})
	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("help failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("expected help output")
	}
}
