package cli

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/IdeaCraftersLabs/oss-tlc-cli/internal/core"
	"github.com/IdeaCraftersLabs/oss-tlc-cli/internal/tui"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the interactive TUI",
	Long:  "TLC TUI provides an interactive interface for task management and flow monitoring.",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStorage()
		if err != nil {
			return err
		}
		defer store.Close()

		service := core.NewTaskService(store, store)

		p := tea.NewProgram(tui.NewModel(service), tea.WithAltScreen(), tea.WithMouseCellMotion())
		if _, err := p.Run(); err != nil {
			fmt.Printf("Error running TUI: %v", err)
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
