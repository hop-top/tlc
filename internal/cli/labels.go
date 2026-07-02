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
	Long: `Detect the project type and seed a suggested label set.

Use --type to force a specific template (go-binary, react-frontend,
python-mvc, generic).`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "yes",
	},
	RunE: func(_ *cobra.Command, _ []string) error {
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
		return nil
	},
}

var labelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List labels in the current project",
	Long:  "Show every label defined for the current project's storage.",
	Annotations: map[string]string{
		"kit/side-effect": "read",
	},
	RunE: func(_ *cobra.Command, _ []string) error {
		fmt.Println("Project labels:")
		// Logic to list labels from storage
		return nil
	},
}

var labelTemplatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "List available label templates",
	Long:  "List every built-in label template grouped by project type.",
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
	RunE: func(_ *cobra.Command, _ []string) error {
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
		return nil
	},
}

func init() {
	labelInitCmd.Flags().StringVar(&projectType, "type", "", "Force project type")

	labelCmd.AddCommand(labelInitCmd)
	labelCmd.AddCommand(labelListCmd)
	labelCmd.AddCommand(labelTemplatesCmd)
	RootCmd.AddCommand(labelCmd)
}
