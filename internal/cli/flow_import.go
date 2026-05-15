package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"hop.top/tlc/internal/core"
)

var flowImportCmd = &cobra.Command{
	Use:   "import <url>",
	Short: "Import a flow from a URI (e.g., GitHub)",
	Long: `Import a flow definition from a URI, such as a GitHub URL pointing to
a Fabric pattern or similar markdown-based workflow specification.

The command fetches the content from the URI and uses an LLM to intelligently
convert it into a TLC flow definition that can be saved to a file or printed to stdout.

LLM Provider Configuration:
  Set LLM_API_KEY for your LLM provider (works with OpenRouter, OpenAI, Anthropic, etc.)
  Or set provider-specific env vars: OPENROUTER_API_KEY, OPENAI_API_KEY, ANTHROPIC_API_KEY
  Optionally set LLM_API_URL for custom endpoint and LLM_MODEL for specific model

Supported formats:
- GitHub blob URLs (automatically converted to raw URLs)
- Direct raw content URLs

Examples:
  tlc flow import https://github.com/danielmiessler/Fabric/blob/main/data/patterns/prepare_7s_strategy/system.md
  tlc flow import https://raw.githubusercontent.com/user/repo/main/pattern.md
  # With custom provider
  LLM_API_URL=https://openrouter.ai/api/v1/chat/completions LLM_API_KEY=sk-xxx tlc flow import <url>`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "no",
	},
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		url := args[0]
		outputFile, _ := cmd.Flags().GetString("output") //nolint:errcheck // registered flag

		importer := core.NewFlowImporter()

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Importing flow from: %s\n\n", url)

		flow, err := importer.ImportFromURL(context.Background(), url)
		if err != nil {
			return fmt.Errorf("failed to import flow: %w", err)
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✓ Successfully imported flow: %s\n", flow.ID)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Name: %s\n", flow.Name)
		if flow.Description != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Description: %s\n", flow.Description)
		}
		if flow.Config != nil && flow.Config.Category != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Category: %s\n", flow.Config.Category)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Steps: %d\n\n", len(flow.Steps))

		data, err := yaml.Marshal(flow)
		if err != nil {
			return fmt.Errorf("failed to marshal flow to YAML: %w", err)
		}

		if outputFile != "" {
			if err := os.WriteFile(outputFile, data, 0o600); err != nil {
				return fmt.Errorf("failed to write flow file: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✓ Flow saved to: %s\n", outputFile)
		} else {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "---")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		}

		return nil
	},
}

func init() {
	flowImportCmd.Flags().StringP("output", "o", "", "Output file path (default: print to stdout)")
	FlowCmd.AddCommand(flowImportCmd)
}
