package cli

import (
	"fmt"
	"os"

	"github.com/IdeaCraftersLabs/oss-tlc-cli/internal/labels"
	"github.com/spf13/cobra"
)

var (
	projectType string
)

var labelsCmd = &cobra.Command{
	Use:   "labels",
	Short: "Manage project labels",
}

var labelsInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Auto-detect and initialize project labels",
	Run: func(cmd *cobra.Command, args []string) {
		wd, _ := os.Getwd()
		
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

var labelsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List labels in the current project",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Project labels:")
		// Logic to list labels from storage
	},
}

var labelsTemplatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "List available label templates",
	Run: func(cmd *cobra.Command, args []string) {
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
	labelsInitCmd.Flags().StringVar(&projectType, "type", "", "Force project type")
	
	labelsCmd.AddCommand(labelsInitCmd)
	labelsCmd.AddCommand(labelsListCmd)
	labelsCmd.AddCommand(labelsTemplatesCmd)
	rootCmd.AddCommand(labelsCmd)
}
