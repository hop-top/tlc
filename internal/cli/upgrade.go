package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"hop.top/kit/xdg"
	"hop.top/upgrade"
	"hop.top/upgrade/skill"
)

const tlcGitHubRepo = "hop-top/tlc"

func newChecker() *upgrade.Checker {
	return upgrade.New(
		upgrade.WithBinary("tlc", tlcVersion),
		upgrade.WithGitHub(tlcGitHubRepo),
	)
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Check for and install updates",
	Long:  `Check for a newer version of tlc and optionally install it.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		auto, _ := cmd.Flags().GetBool("auto")
		quiet, _ := cmd.Flags().GetBool("quiet")
		return upgrade.RunCLI(cmd.Context(), newChecker(), upgrade.CLIOptions{
			AutoUpgrade: auto,
			Quiet:       quiet,
		})
	},
}

var upgradePreambleCmd = &cobra.Command{
	Use:   "preamble",
	Short: "Print the upgrade preamble fragment for skill files",
	Long: `Print a markdown preamble fragment for embedding in TLC skill files.
Agents read this to know how to self-upgrade tlc before executing tasks.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		auto, _ := cmd.Flags().GetBool("auto")
		never, _ := cmd.Flags().GetBool("never")
		install, _ := cmd.Flags().GetBool("install")

		level := skill.SnoozeOnce
		if auto {
			level = skill.SnoozeNever
		} else if never {
			level = skill.SnoozeAlways
		}

		preamble := skill.Generate(skill.PreambleOptions{
			BinaryName: "tlc",
			Snooze:     level,
		})

		if install {
			return installTLCPreamble(preamble)
		}

		fmt.Print(preamble)
		return nil
	},
}

func installTLCPreamble(preamble string) error {
	configDir, err := xdg.ConfigDir("tlc")
	if err != nil {
		return fmt.Errorf("upgrade preamble: %w", err)
	}
	dir := filepath.Join(configDir, "skills")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("upgrade preamble: mkdir: %w", err)
	}
	path := dir + "/upgrade-preamble.md"
	if err := os.WriteFile(path, []byte(preamble), 0o600); err != nil {
		return fmt.Errorf("upgrade preamble: write: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Installed upgrade preamble → %s\n", path)
	return nil
}

func init() {
	upgradePreambleCmd.Flags().Bool("auto", false, "Emit auto-upgrade (SnoozeNever) variant")
	upgradePreambleCmd.Flags().Bool("never", false, "Emit check-only (SnoozeAlways) variant")
	upgradePreambleCmd.Flags().Bool("install", false, "Write preamble to ~/.config/tlc/skills/")
	upgradeCmd.Flags().Bool("auto", false, "Install without prompting")
	upgradeCmd.Flags().BoolP("quiet", "q", false, "Suppress output when already up to date")
	upgradeCmd.AddCommand(upgradePreambleCmd)
	RootCmd.AddCommand(upgradeCmd)
}
