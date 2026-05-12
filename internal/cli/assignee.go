package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

func init() {
	RootCmd.AddCommand(newAssigneeCmd())
}

func newAssigneeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "assignee",
		Short: "Manage assignees (specialized task executors)",
		Long: `Assignees are specialized executors that handle tasks based on their capabilities.
They are the TLC equivalent of superpowers agents.`,
	}

	cmd.AddCommand(newAssigneeListCmd())
	cmd.AddCommand(newAssigneeShowCmd())

	return cmd
}

func newAssigneeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all available assignees",
		Long:  "Display all assignees with their capabilities",
		Annotations: map[string]string{
			"kit/side-effect": "read",
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			// Get assignees directory from config or default
			assigneesDir := assigneesDirFromConfig()

			loader := core.NewAssigneeLoader(assigneesDir)
			assignees, err := loader.LoadAll()
			if err != nil {
				return fmt.Errorf("failed to load assignees: %w", err)
			}

			if len(assignees) == 0 {
				fmt.Println("No assignees found.")
				fmt.Printf("Create assignee YAML files in: %s\n", assigneesDir)
				return nil
			}

			fmt.Printf("Available Assignees (%d):\n\n", len(assignees))

			for _, a := range assignees {
				fmt.Printf("  %s\n", a.Name)
				fmt.Printf("    ID: %s\n", a.ID)
				fmt.Printf("    Description: %s\n", a.Description)
				fmt.Printf("    Capabilities:\n")

				if len(a.Capabilities.TaskTypes) > 0 {
					fmt.Printf("      Task Types: %v\n", a.Capabilities.TaskTypes)
				}
				if len(a.Capabilities.Domains) > 0 {
					fmt.Printf("      Domains: %v\n", a.Capabilities.Domains)
				}
				if len(a.Capabilities.Tools) > 0 {
					fmt.Printf("      Tools: %v\n", a.Capabilities.Tools)
				}

				if a.Delegation != nil && len(a.Delegation.Unblocks) > 0 {
					fmt.Printf("    Unblocks: %v\n", a.Delegation.Unblocks)
				}

				fmt.Println()
			}

			return nil
		},
	}
}

func newAssigneeShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <assignee-id>",
		Short: "Show detailed information about an assignee",
		Long:  "Display the full assignee definition including capabilities, delegation rules, and instructions.",
		Args:  cobra.ExactArgs(1),
		Annotations: map[string]string{
			"kit/side-effect": "read",
		},
		RunE: func(_ *cobra.Command, args []string) error {
			assigneeID := args[0]
			assigneesDir := assigneesDirFromConfig()

			loader := core.NewAssigneeLoader(assigneesDir)
			assignee, err := loader.GetAssignee(assigneeID)
			if err != nil {
				return fmt.Errorf("failed to load assignee: %w", err)
			}

			fmt.Printf("Assignee: %s\n", assignee.Name)
			fmt.Printf("ID: %s\n", assignee.ID)
			fmt.Printf("Version: %s\n", assignee.Version)
			fmt.Printf("Description: %s\n\n", assignee.Description)

			fmt.Println("Capabilities:")
			if len(assignee.Capabilities.TaskTypes) > 0 {
				fmt.Printf("  Task Types: %v\n", assignee.Capabilities.TaskTypes)
			}
			if len(assignee.Capabilities.Domains) > 0 {
				fmt.Printf("  Domains: %v\n", assignee.Capabilities.Domains)
			}
			if len(assignee.Capabilities.Tools) > 0 {
				fmt.Printf("  Tools: %v\n", assignee.Capabilities.Tools)
			}

			if assignee.Instructions != "" {
				fmt.Printf("\nInstructions:\n%s\n", assignee.Instructions)
			}

			if assignee.Delegation != nil {
				fmt.Println("\nDelegation Rules:")
				if len(assignee.Delegation.HandoffConditions) > 0 {
					fmt.Println("  Handoff Conditions:")
					for _, cond := range assignee.Delegation.HandoffConditions {
						fmt.Printf("    - When: %s -> Delegate to: %s\n", cond.When, cond.DelegateTo)
					}
				}
				if len(assignee.Delegation.Unblocks) > 0 {
					fmt.Printf("  Unblocks: %v\n", assignee.Delegation.Unblocks)
				}
			}

			return nil
		},
	}
}
