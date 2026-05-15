package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestConfirmBridgeSetsConfirmYes verifies that when a registered
// local "skip" flag is true, the bridge PreRunE flips the inherited
// persistent --confirm to "yes" before the kit gate runs.
func TestConfirmBridgeSetsConfirmYes(t *testing.T) {
	root := newSyntheticConfirmRoot()
	leaf := &cobra.Command{Use: "delete", RunE: func(*cobra.Command, []string) error { return nil }}
	leaf.Flags().Bool("yes", false, "")
	leaf.Flags().Bool("no-prompt", false, "")
	root.AddCommand(leaf)

	installConfirmBridge(leaf, "yes", "no-prompt")

	cases := []struct {
		name     string
		args     []string
		wantYes  bool
	}{
		{"baseline no flag", []string{"delete"}, false},
		{"local --yes flips", []string{"delete", "--yes"}, true},
		{"local --no-prompt flips", []string{"delete", "--no-prompt"}, true},
		{"explicit --confirm=yes already set", []string{"delete", "--confirm=yes"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetConfirmFlag(root)
			root.SetArgs(tc.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			got := root.PersistentFlags().Lookup("confirm").Value.String()
			if (got == "yes") != tc.wantYes {
				t.Errorf("confirm = %q, wantYes=%v", got, tc.wantYes)
			}
		})
	}
}

// TestConfirmBridgePreservesOriginalPreRunE verifies that an existing
// PreRunE on the command is preserved (called after the bridge).
func TestConfirmBridgePreservesOriginalPreRunE(t *testing.T) {
	root := newSyntheticConfirmRoot()
	called := false
	leaf := &cobra.Command{
		Use: "delete",
		PreRunE: func(*cobra.Command, []string) error {
			called = true
			return nil
		},
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	leaf.Flags().Bool("yes", false, "")
	root.AddCommand(leaf)
	installConfirmBridge(leaf, "yes")

	root.SetArgs([]string{"delete", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !called {
		t.Error("original PreRunE was not called")
	}
	if root.PersistentFlags().Lookup("confirm").Value.String() != "yes" {
		t.Error("bridge did not set --confirm=yes")
	}
}

// TestConfirmBridgeNoLocalFlagsLeavesConfirmAlone verifies the bridge
// is inert when no local flag is true.
func TestConfirmBridgeNoLocalFlagsLeavesConfirmAlone(t *testing.T) {
	root := newSyntheticConfirmRoot()
	leaf := &cobra.Command{Use: "delete", RunE: func(*cobra.Command, []string) error { return nil }}
	leaf.Flags().Bool("yes", false, "")
	root.AddCommand(leaf)
	installConfirmBridge(leaf, "yes")

	root.SetArgs([]string{"delete"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if v := root.PersistentFlags().Lookup("confirm").Value.String(); v != "" {
		t.Errorf("confirm = %q, want empty (default)", v)
	}
}

// newSyntheticConfirmRoot mints a minimal cobra root that mirrors
// kit's --confirm persistent-flag shape. Tests use this rather than
// the live RootCmd so they remain hermetic.
func newSyntheticConfirmRoot() *cobra.Command {
	root := &cobra.Command{Use: "tlc"}
	root.PersistentFlags().String("confirm", "", "")
	return root
}

// resetConfirmFlag clears the persistent --confirm value between
// sub-tests so prior --yes runs don't leak into baseline cases.
func resetConfirmFlag(root *cobra.Command) {
	if f := root.PersistentFlags().Lookup("confirm"); f != nil {
		_ = f.Value.Set("")
	}
}
