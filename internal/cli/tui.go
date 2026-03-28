package cli

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/tui"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the interactive TUI",
	Long:  "TLC TUI provides an interactive interface for task management and flow monitoring.",
	RunE: func(_ *cobra.Command, _ []string) error {
		store, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()

		service := core.NewTaskService(store, store)

		p := tea.NewProgram(tui.NewModel(service), tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Printf("Error running TUI: %v", err)
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	RootCmd.AddCommand(tuiCmd)
}
