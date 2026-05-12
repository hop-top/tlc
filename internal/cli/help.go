package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"hop.top/kit/go/ai/toolspec"
)

var helpLLMFormat string

var helpLLMCmd = &cobra.Command{
	Use:   "schema",
	Short: "Output LLM/agent tool definition for TLC",
	Long: `Output the tool definition that AI agents and LLMs can use to interact with TLC.
This command outputs a JSON schema that describes how to call TLC commands programmatically.

Replaces the former "help llm" subcommand.`,
	Annotations: map[string]string{
		"kit/side-effect":    "read",
		"kit/idempotent":     "yes",
		"kit/top-level-verb": "true",
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		spec := buildToolSpec()
		output := renderToolSpec(spec, helpLLMFormat)
		if output == nil {
			return fmt.Errorf(
				"unknown format: %s (supported: json, mcp, openai, anthropic)",
				helpLLMFormat,
			)
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	},
}

func init() {
	helpLLMCmd.Flags().StringVar(
		&helpLLMFormat, "format", "json",
		"Output format (json, mcp, openai, anthropic)",
	)
	RootCmd.AddCommand(helpLLMCmd)
}

const (
	toolName        = "manage_tlc_task"
	toolDescription = "Create, list, or update tasks in the TLC " +
		"(Task Line CLI) system for internal planning and tracking."
)

// buildToolSpec constructs the canonical ToolSpec for the TLC task tool.
func buildToolSpec() *toolspec.ToolSpec {
	return &toolspec.ToolSpec{
		Name: toolName,
		Commands: []toolspec.Command{
			{Name: "create"},
			{Name: "list"},
			{Name: "update"},
			{Name: "show"},
			{Name: "claim"},
			{Name: "unclaim"},
		},
		Flags: []toolspec.Flag{
			{
				Name: "action", Type: "string",
				Description: "The action to perform on tasks.",
			},
			{
				Name: "task_id", Type: "string",
				Description: "The unique ID of the task (e.g., 'T-0042'). " +
					"Required for 'update', 'show', 'claim', and 'unclaim'.",
			},
			{
				Name: "title", Type: "string",
				Description: "The title of the task. Required for 'create'.",
			},
			{
				Name: "status", Type: "string",
				Description: "The status of the task.",
			},
			{
				Name: "assigned_to", Type: "string",
				Description: "The username of the assignee (e.g., 'engineer-1').",
			},
			{
				Name: "tags", Type: "array",
				Description: "A list of tags for categorization " +
					"(e.g., ['infra', 'bug']).",
			},
			{
				Name: "description", Type: "string",
				Description: "A detailed description of the task.",
			},
			{
				Name: "reference", Type: "string",
				Description: "A reference pointer " +
					"(URL, file path, or documentation reference).",
			},
		},
	}
}

// specProperties builds the JSON Schema properties map from a ToolSpec's
// top-level flags. This replaces the former hand-coded getToolProperties.
func specProperties(spec *toolspec.ToolSpec) map[string]interface{} {
	props := make(map[string]interface{}, len(spec.Flags))
	for _, f := range spec.Flags {
		p := map[string]interface{}{
			"type":        f.Type,
			"description": f.Description,
		}
		// Add enum for known constrained fields.
		switch f.Name {
		case "action":
			names := make([]string, len(spec.Commands))
			for i, c := range spec.Commands {
				names[i] = c.Name
			}
			p["enum"] = names
		case "status":
			p["enum"] = []string{
				"TODO", "IN_PROGRESS", "DONE", "SKIPPED",
			}
		case "tags":
			p["items"] = map[string]string{"type": "string"}
		}
		props[f.Name] = p
	}
	return props
}

// specInputSchema returns the JSON Schema object envelope.
func specInputSchema(spec *toolspec.ToolSpec) map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": specProperties(spec),
		"required":   []string{"action"},
	}
}

// renderToolSpec returns the tool definition in the requested format.
// Returns nil for unknown formats.
func renderToolSpec(
	spec *toolspec.ToolSpec, format string,
) interface{} {
	switch format {
	case "mcp":
		return map[string]interface{}{
			"name":        spec.Name,
			"description": toolDescription,
			"inputSchema": specInputSchema(spec),
		}
	case "anthropic":
		return map[string]interface{}{
			"name":         spec.Name,
			"description":  toolDescription,
			"input_schema": specInputSchema(spec),
		}
	case "openai":
		return map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        spec.Name,
				"description": toolDescription,
				"parameters":  specInputSchema(spec),
			},
		}
	case formatJSON, "":
		return renderJSONFormat(spec)
	default:
		return nil
	}
}

// renderJSONFormat produces the rich JSON format with examples and
// best practices derived from the ToolSpec.
func renderJSONFormat(spec *toolspec.ToolSpec) map[string]interface{} {
	return map[string]interface{}{
		"name":        spec.Name,
		"description": toolDescription,
		"parameters":  specInputSchema(spec),
		"examples": []map[string]interface{}{
			{
				"description": "Creating a task",
				"tool_call": map[string]interface{}{
					"action":      "create",
					"title":       "Implement authentication middleware",
					"assigned_to": "engineer-1",
					"tags":        []string{"auth", "security"},
				},
				"command": "tlc task create " +
					"\"Implement authentication middleware\" " +
					"--assigned-to engineer-1 --tag auth --tag security",
			},
			{
				"description": "Marking a task as in-progress",
				"tool_call": map[string]interface{}{
					"action":  "update",
					"task_id": "T-0042",
					"status":  "IN_PROGRESS",
				},
				"command": "tlc task update T-0042 --status IN_PROGRESS",
			},
			{
				"description": "Listing pending tasks",
				"tool_call": map[string]interface{}{
					"action":      "list",
					"status":      "TODO",
					"assigned_to": "engineer-1",
				},
				"command": "tlc task list " +
					"--status TODO --assigned-to engineer-1",
			},
			{
				"description": "Claiming a task",
				"tool_call": map[string]interface{}{
					"action":  "claim",
					"task_id": "T-0042",
				},
				"command": "tlc task claim T-0042",
			},
			{
				"description": "Releasing a task",
				"tool_call": map[string]interface{}{
					"action":  "unclaim",
					"task_id": "T-0042",
				},
				"command": "tlc task unclaim T-0042",
			},
		},
		"best_practices": []string{
			"Plan First: Before starting a complex task, " +
				"use the 'create' action to break it down " +
				"into smaller sub-tasks.",
			"Stay Updated: Always move a task to 'IN_PROGRESS' " +
				"when you start working on it, " +
				"and to 'DONE' when finished.",
			"Use Tags: Apply domain tags (e.g., 'storage', 'cli', " +
				"'sync') to help teammates filter and " +
				"understand your work.",
			"Reference IDs: When committing code or sending messages, " +
				"refer to the Task IDs (e.g., 'Refs: T-0042') " +
				"to maintain a clear link between " +
				"planning and execution.",
			"Self-Update: When TLC is updated, refresh your tool " +
				"knowledge by running 'tlc schema'.",
		},
	}
}
