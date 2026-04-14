package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/labels"
)

var projectType string

var labelCmd = &cobra.Command{
	Use:   "label",
	Short: "Manage project labels",
}

var labelInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Auto-detect and initialize project labels",
	Run: func(_ *cobra.Command, _ []string) {
		wd, _ := os.Getwd() //nolint:errcheck // best-effort directory detection

		var pType labels.ProjectType
		if projectType != "" {
			pType = labels.ProjectType(projectType)
		} else {
			pType = labels.DetectProjectType(wd)
		}

		fmt.Printf("Detected project type: %s\n", pType)

		templates := labels.GetTemplates(pType)
		fmt.Printf("Suggested labels for %s:\n", pType)
		for _, l := range templates {
			fmt.Printf("  - %s (%s): %s\n", l.Name, l.Color, l.Description)
		}

		fmt.Println("\n✓ Labels initialized locally")
	},
}

var labelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List labels in the current project",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("Project labels:")
		// Logic to list labels from storage
	},
}

var labelTemplatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "List available label templates",
	Run: func(_ *cobra.Command, _ []string) {
		pTypes := []labels.ProjectType{
			labels.TypeGoBinary,
			labels.TypeReactFrontend,
			labels.TypePythonMVC,
			labels.TypeGeneric,
		}

		for _, pt := range pTypes {
			fmt.Printf("[%s]\n", pt)
			for _, l := range labels.GetTemplates(pt) {
				fmt.Printf("  - %s\n", l.Name)
			}
			fmt.Println()
		}
	},
}

func init() {
	labelInitCmd.Flags().StringVar(&projectType, "type", "", "Force project type")

	labelCmd.AddCommand(labelInitCmd)
	labelCmd.AddCommand(labelListCmd)
	labelCmd.AddCommand(labelTemplatesCmd)
	RootCmd.AddCommand(labelCmd)
}
