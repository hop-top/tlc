package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/labels"
)

var projectType string

// projectTypeList renders the accepted --type values for help text.
//
// Both the long help and the flag usage read from labels.AllProjectTypes
// so the documented set cannot drift from the set that actually
// produces distinct labels — the drift that let `--type python-mvc` be
// advertised while silently emitting the generic labels.
func projectTypeList() string {
	names := make([]string, 0, len(labels.AllProjectTypes()))
	for _, pt := range labels.AllProjectTypes() {
		names = append(names, string(pt))
	}
	return strings.Join(names, ", ")
}

var labelCmd = &cobra.Command{
	Use:   "label",
	Short: "Manage project labels",
}

var labelInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Auto-detect and initialize project labels",
	Long: `Detect the project type and seed a suggested label set.

Use --type to force a specific template (` + projectTypeList() + `).`,
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
		for _, pt := range labels.AllProjectTypes() {
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
	labelInitCmd.Flags().StringVar(&projectType, "type", "", "Force project type ("+projectTypeList()+")")

	labelCmd.AddCommand(labelInitCmd)
	labelCmd.AddCommand(labelListCmd)
	labelCmd.AddCommand(labelTemplatesCmd)
	RootCmd.AddCommand(labelCmd)
}
