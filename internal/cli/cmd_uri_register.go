package cli

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
	"hop.top/cite/handle"
	"hop.top/cite/handle/generate"
)

func newURICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uri",
		Short: "URI scheme management",
	}
	cmd.AddCommand(newURIRegisterCmd())
	cmd.AddCommand(newURISnippetCmd())
	return cmd
}

func newURIRegisterCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "register",
		Short: "Register tlc:// URI scheme with the OS",
		Long: `Register the tlc:// URI scheme so the OS opens tlc when a tlc:// link is clicked.

On Linux and Windows, registration is immediate.
On macOS, tlc must be wrapped in a .app bundle for Launch Services to accept
the handler. Use 'tlc uri snippet --platform macos' to get the Info.plist
fragment instead.`,
		Annotations: map[string]string{
			"kit/side-effect": "write-local",
			"kit/idempotent":  "yes",
		},
		RunE: runURIRegister,
	}
}

func runURIRegister(cmd *cobra.Command, _ []string) error {
	if handle.ErrUnsupported != nil {
		return fmt.Errorf("URI registration not supported on this platform: %w", handle.ErrUnsupported)
	}

	// On macOS the runtime Register call needs a bundle ID. For an unbundled
	// CLI that identifier is meaningless to Launch Services, so guide the user
	// to the snippet command instead.
	if runtime.GOOS == "darwin" {
		return fmt.Errorf(
			"macOS requires a .app bundle to register URL schemes.\n" +
				"Use 'tlc uri snippet --platform macos' to get the Info.plist fragment.",
		)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine binary path: %w", err)
	}

	if err := handle.Register("tlc", exe); err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "tlc:// URI scheme registered.")
	return nil
}

func newURISnippetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snippet",
		Short: "Print OS-specific config snippet for tlc:// registration",
		Long: `Print a platform-specific configuration snippet for registering the tlc://
URI scheme when runtime registration is not possible (e.g. macOS .app bundles,
installer scripts).`,
		Annotations: map[string]string{
			"kit/side-effect": "read",
			"kit/idempotent":  "yes",
		},
		RunE: runURISnippet,
	}
	cmd.Flags().String("platform", "", "target platform: macos, linux, windows (default: current OS)")
	return cmd
}

func runURISnippet(cmd *cobra.Command, _ []string) error {
	platform, _ := cmd.Flags().GetString("platform") //nolint:errcheck // registered flag
	if platform == "" {
		switch runtime.GOOS {
		case "darwin":
			platform = "macos"
		case "linux":
			platform = "linux"
		case "windows":
			platform = "windows"
		default:
			return fmt.Errorf("unknown platform %q; use --platform to specify", runtime.GOOS)
		}
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine binary path: %w", err)
	}

	spec := generate.HandlerSpec{
		Vendor:   "hop-top",
		App:      "tlc",
		Language: generate.LanguageGo,
		Scheme:   "tlc",
		AppPath:  exe,
	}
	snippet, err := generate.Snippet(platform, spec)
	if err != nil {
		return err
	}
	fmt.Fprint(cmd.OutOrStdout(), snippet)
	return nil
}

func init() {
	RootCmd.AddCommand(newURICmd())
}
