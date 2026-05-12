package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

var FlowValidateCmd = &cobra.Command{
	Use:   "validate <flow-file>",
	Short: "Validate a flow definition without running it",
	Long: `Parse and validate a flow YAML/JSON file for structural integrity.

Checks:
  - YAML/JSON syntax
  - Required fields (flow_id, entry_step, steps)
  - Entry step exists in step map
  - All depends_on references point to existing steps
  - No dependency cycles

No storage or database is opened. Exit 0 on valid, exit 1 on error.

Example:
  tlc flow validate examples/flows/brainstorming.yaml`,
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		out := cmd.OutOrStdout()

		f, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("cannot open %s: %w", filePath, err)
		}
		defer func() { _ = f.Close() }()

		_, err = core.ParseFlow(f, filePath)
		if err != nil {
			_, _ = fmt.Fprintf(out, "Validation failed: %s\n", err)
			return err
		}

		_, _ = fmt.Fprintf(out, "Flow %s is valid\n", filePath)
		return nil
	},
}

func init() {
	FlowCmd.AddCommand(FlowValidateCmd)
}
