// Package cli provides a cobra+fang+viper root command factory that
// implements the hop-top CLI contract.
//
// Every tool built with this package gets the following automatically:
//
//   - -v / --version  prints "<name> version <version>" (handled by fang)
//   - --quiet         persistent flag, suppresses non-essential output
//   - --no-color      persistent flag, disables ANSI colour
//   - help subcommand hidden; -h / --help flag retained
//   - completion subcommand disabled
//   - styled help, errors, and man pages via fang
//   - a Theme built from CharmTone palette + optional accent
//
// The persistent flags are bound to viper under the keys "quiet" and
// "no-color". Subcommands should read these values from Root.Viper
// rather than inspecting the flags directly.
//
// Typical usage:
//
//	root := cli.New(cli.Config{Name: "mytool", Version: "1.2.3", Short: "..."})
//	root.Cmd.AddCommand(doSomethingCmd())
//	if err := root.Execute(context.Background()); err != nil {
//		os.Exit(1)
//	}
package cli

import (
	"context"

	"charm.land/fang/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/output"
)

// Config holds the tool identity for root command construction.
type Config struct {
	// Name is the binary name as invoked by the user (e.g. "mytool").
	Name string
	// Version is the semver string printed by --version (e.g. "1.2.3").
	Version string
	// Short is the one-line description shown in help output.
	Short string
	// Accent is an optional hex color string (e.g. "#FF0000") used as the
	// theme accent. Zero value falls back to CharmTone Charple.
	Accent string
}

// Root wraps the cobra root command, viper instance, theme, and hint
// registry.
type Root struct {
	// Cmd is the configured cobra root command. Add subcommands to it,
	// then call Execute(ctx) to run the CLI.
	Cmd *cobra.Command
	// Viper is the viper instance with --quiet and --no-color already
	// bound. Subcommands should read output preferences from here.
	Viper *viper.Viper
	// Config is the identity provided to New; retained for subcommands
	// that need the tool name or version at runtime.
	Config Config
	// Theme holds semantic colors and styles built from CharmTone +
	// the optional accent.
	Theme Theme
	// Hints is the per-command hint registry. Commands register
	// next-step hints here; the output pipeline renders them after
	// primary output when enabled.
	Hints *output.HintSet
}

// New returns a Root pre-configured to the hop-top CLI contract:
//   - no help or completion subcommands (only -h/--help flag)
//   - version handled by fang (-v/--version)
//   - persistent global flags: --quiet, --no-color
//   - styled help/errors via fang
func New(cfg Config) *Root {
	v := viper.New()

	cmd := &cobra.Command{
		Use:          cfg.Name,
		Short:        cfg.Short,
		SilenceUsage: true,
		Args:         cobra.NoArgs,
	}

	// Hide the default help command; -h/--help flag remains.
	cmd.SetHelpCommand(&cobra.Command{Hidden: true})

	// Eagerly register -h/--help so it is available before Execute().
	cmd.InitDefaultHelpFlag()

	// Disable completion subcommand entirely.
	cmd.CompletionOptions.DisableDefaultCmd = true

	// Global persistent flags bound to viper.
	pf := cmd.PersistentFlags()
	pf.Bool("quiet", false, "Suppress non-essential output")
	pf.Bool("no-color", false, "Disable ANSI colour")
	_ = v.BindPFlag("quiet", pf.Lookup("quiet"))
	_ = v.BindPFlag("no-color", pf.Lookup("no-color"))

	output.RegisterFlags(cmd, v)
	output.RegisterHintFlags(cmd, v)

	theme := buildTheme(cfg.Accent)

	return &Root{
		Cmd:    cmd,
		Viper:  v,
		Config: cfg,
		Theme:  theme,
		Hints:  output.NewHintSet(),
	}
}

// Execute runs the root command through fang, which provides styled help,
// version output, error rendering, and man page generation.
func (r *Root) Execute(ctx context.Context) error {
	return fang.Execute(ctx, r.Cmd,
		fang.WithVersion(r.Config.Version),
		fang.WithoutCompletions(),
		fang.WithColorSchemeFunc(brandColorScheme),
	)
}

// brandColorScheme returns a fang ColorScheme with hop.top brand accents.
func brandColorScheme(c lipgloss.LightDarkFunc) fang.ColorScheme {
	cs := fang.DefaultColorScheme(c)
	cs.Title = lipgloss.Color("#FFFFFF")
	cs.Command = Neon.Command
	cs.Flag = Neon.Flag
	cs.Program = Neon.Command
	return cs
}
