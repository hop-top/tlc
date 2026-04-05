// Package main provides the TLC CLI entry point.
package main

import (
	"hop.top/tlc/internal/cli"

	// Register LLM adapters for task prompt NL routing.
	_ "hop.top/kit/llm/anthropic"
	_ "hop.top/kit/llm/ollama"
	_ "hop.top/kit/llm/openai"
)

func main() {
	cli.Execute()
}
