package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tlc",
		Short: "Task Line CLI - Multi-agent task orchestration",
	}

	cmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file")
	cmd.PersistentFlags().StringP("format", "f", "", "output format (table, json, yaml, tls)")
	cmd.PersistentFlags().Bool("no-color", false, "disable colored output")
	cmd.PersistentFlags().BoolP("verbose", "v", false, "verbose logging")
	cmd.PersistentFlags().BoolP("quiet", "q", false, "suppress non-essential output")

	viper.BindPFlag("output.format", cmd.PersistentFlags().Lookup("format"))
	viper.BindPFlag("output.color", cmd.PersistentFlags().Lookup("no-color"))
	viper.BindPFlag("output.verbose", cmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("output.quiet", cmd.PersistentFlags().Lookup("quiet"))

	return cmd
}

func newTestInitCmd() *cobra.Command {
	storageBackend := "sqlite"
	dbPath := ""
	force := false
	fallbackMode := ""
	duplicateIDStrategy := ""

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize TLC in current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, &storageBackend, &dbPath, &force, &fallbackMode, &duplicateIDStrategy)
		},
	}

	cmd.Flags().StringVar(&storageBackend, "storage", "sqlite", "Storage backend: local, sqlite")
	cmd.Flags().StringVar(&dbPath, "db-path", "", "Database file path (default: global)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config")
	cmd.Flags().Bool("track", false, "Add .tlc/ to .gitignore")
	cmd.Flags().Bool("no-track", false, "Do not add .tlc/ to .gitignore")
	cmd.Flags().StringVar(&fallbackMode, "fallback-mode", "", "Project fallback mode: auto, detected, prompt (default: auto)")
	cmd.Flags().StringVar(&duplicateIDStrategy, "duplicate-id-strategy", "", "Duplicate ID strategy: share, unique, prompt (default: share)")

	return cmd
}
