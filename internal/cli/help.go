package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	helpLLMFormat string
)

var helpCmd = &cobra.Command{
	Use:   "help",
	Short: "Help about any command",
	Long:  `Help provides help for any command in the application.`,
}

var helpLLMCmd = &cobra.Command{
	Use:   "llm",
	Short: "Output LLM/agent tool definition for TLC",
	Long: `Output the tool definition that AI agents and LLMs can use to interact with TLC.
This command outputs a JSON schema that describes how to call TLC commands programmatically.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var output interface{}

		switch helpLLMFormat {
		case "json", "":
			output = getToolDefinition()
		case "mcp":
			output = getMCPToolDefinition()
		case "openai":
			output = getOpenAIToolDefinition()
		case "anthropic":
			output = getAnthropicToolDefinition()
		default:
			return fmt.Errorf("unknown format: %s (supported: json, mcp, openai, anthropic)", helpLLMFormat)
		}

		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	},
}

func init() {
	rootCmd.SetHelpCommand(helpCmd)
	helpCmd.AddCommand(helpLLMCmd)

	helpLLMCmd.Flags().StringVar(&helpLLMFormat, "format", "json", "Output format (json, mcp, openai, anthropic)")
}

// getMCPToolDefinition returns the MCP-formatted tool definition
func getMCPToolDefinition() map[string]interface{} {
	return map[string]interface{}{
		"name":        "manage_tlc_task",
		"description": "Create, list, or update tasks in the TLC (Task Line CLI) system for internal planning and tracking.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"create", "list", "update", "show", "claim", "unclaim"},
					"description": "The action to perform on tasks.",
				},
				"task_id": map[string]interface{}{
					"type":        "string",
					"description": "The unique ID of the task (e.g., 'T-0042'). Required for 'update', 'show', 'claim', and 'unclaim'.",
				},
				"title": map[string]interface{}{
					"type":        "string",
					"description": "The title of the task. Required for 'create'.",
				},
				"status": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"TODO", "IN_PROGRESS", "DONE", "SKIPPED"},
					"description": "The status of the task.",
				},
				"assigned_to": map[string]interface{}{
					"type":        "string",
					"description": "The username of the assignee (e.g., 'engineer-1').",
				},
				"tags": map[string]interface{}{
					"type": "array",
					"items": map[string]string{
						"type": "string",
					},
					"description": "A list of tags for categorization (e.g., ['infra', 'bug']).",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "A detailed description of the task.",
				},
				"reference": map[string]interface{}{
					"type":        "string",
					"description": "A reference pointer (URL, file path, or documentation reference).",
				},
			},
			"required": []string{"action"},
		},
	}
}

// getAnthropicToolDefinition returns the Anthropic Messages API tool format
func getAnthropicToolDefinition() map[string]interface{} {
	return map[string]interface{}{
		"name":        "manage_tlc_task",
		"description": "Create, list, or update tasks in the TLC (Task Line CLI) system for internal planning and tracking.",
		"input_schema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"create", "list", "update", "show", "claim", "unclaim"},
					"description": "The action to perform on tasks.",
				},
				"task_id": map[string]interface{}{
					"type":        "string",
					"description": "The unique ID of the task (e.g., 'T-0042'). Required for 'update', 'show', 'claim', and 'unclaim'.",
				},
				"title": map[string]interface{}{
					"type":        "string",
					"description": "The title of the task. Required for 'create'.",
				},
				"status": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"TODO", "IN_PROGRESS", "DONE", "SKIPPED"},
					"description": "The status of the task.",
				},
				"assigned_to": map[string]interface{}{
					"type":        "string",
					"description": "The username of the assignee (e.g., 'engineer-1').",
				},
				"tags": map[string]interface{}{
					"type": "array",
					"items": map[string]string{
						"type": "string",
					},
					"description": "A list of tags for categorization (e.g., ['infra', 'bug']).",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "A detailed description of the task.",
				},
				"reference": map[string]interface{}{
					"type":        "string",
					"description": "A reference pointer (URL, file path, or documentation reference).",
				},
			},
			"required": []string{"action"},
		},
	}
}

// getOpenAIToolDefinition returns the OpenAI function calling format
func getOpenAIToolDefinition() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "manage_tlc_task",
			"description": "Create, list, or update tasks in the TLC (Task Line CLI) system for internal planning and tracking.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"create", "list", "update", "show", "claim", "unclaim"},
						"description": "The action to perform on tasks.",
					},
					"task_id": map[string]interface{}{
						"type":        "string",
						"description": "The unique ID of the task (e.g., 'T-0042'). Required for 'update', 'show', 'claim', and 'unclaim'.",
					},
					"title": map[string]interface{}{
						"type":        "string",
						"description": "The title of the task. Required for 'create'.",
					},
					"status": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"TODO", "IN_PROGRESS", "DONE", "SKIPPED"},
						"description": "The status of the task.",
					},
					"assigned_to": map[string]interface{}{
						"type":        "string",
						"description": "The username of the assignee (e.g., 'engineer-1').",
					},
					"tags": map[string]interface{}{
						"type": "array",
						"items": map[string]string{
							"type": "string",
						},
						"description": "A list of tags for categorization (e.g., ['infra', 'bug']).",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "A detailed description of the task.",
					},
					"reference": map[string]interface{}{
						"type":        "string",
						"description": "A reference pointer (URL, file path, or documentation reference).",
					},
				},
				"required": []string{"action"},
			},
		},
	}
}

// getToolDefinition returns the standard tool definition for AI agents
func getToolDefinition() map[string]interface{} {
	return map[string]interface{}{
		"name":        "manage_tlc_task",
		"description": "Create, list, or update tasks in the TLC (Task Line CLI) system for internal planning and tracking.",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"create", "list", "update", "show", "claim", "unclaim"},
					"description": "The action to perform on tasks.",
				},
				"task_id": map[string]interface{}{
					"type":        "string",
					"description": "The unique ID of the task (e.g., 'T-0042'). Required for 'update', 'show', 'claim', and 'unclaim'.",
				},
				"title": map[string]interface{}{
					"type":        "string",
					"description": "The title of the task. Required for 'create'.",
				},
				"status": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"TODO", "IN_PROGRESS", "DONE", "SKIPPED"},
					"description": "The status of the task.",
				},
				"assigned_to": map[string]interface{}{
					"type":        "string",
					"description": "The username of the assignee (e.g., 'engineer-1').",
				},
				"tags": map[string]interface{}{
					"type": "array",
					"items": map[string]string{
						"type": "string",
					},
					"description": "A list of tags for categorization (e.g., ['infra', 'bug']).",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "A detailed description of the task.",
				},
				"reference": map[string]interface{}{
					"type":        "string",
					"description": "A reference pointer (URL, file path, or documentation reference).",
				},
			},
			"required": []string{"action"},
		},
		"examples": []map[string]interface{}{
			{
				"description": "Creating a task",
				"tool_call": map[string]interface{}{
					"action":      "create",
					"title":       "Implement authentication middleware",
					"assigned_to": "engineer-1",
					"tags":        []string{"auth", "security"},
				},
				"command": "tlc task create \"Implement authentication middleware\" --assigned-to engineer-1 --tag auth --tag security",
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
				"command": "tlc task list --status TODO --assigned-to engineer-1",
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
			"Plan First: Before starting a complex task, use the 'create' action to break it down into smaller sub-tasks.",
			"Stay Updated: Always move a task to 'IN_PROGRESS' when you start working on it, and to 'DONE' when finished.",
			"Use Tags: Apply domain tags (e.g., 'storage', 'cli', 'sync') to help teammates filter and understand your work.",
			"Reference IDs: When committing code or sending messages, refer to the Task IDs (e.g., 'Refs: T-0042') to maintain a clear link between planning and execution.",
			"Self-Update: When TLC is updated, refresh your tool knowledge by running 'tlc help llm'.",
		},
	}
}
