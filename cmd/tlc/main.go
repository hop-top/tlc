// Package main provides the TLC CLI entry point.
package main

import (
	"hop.top/tlc/internal/cli"

	// Register LLM adapters for task prompt NL routing.
	_ "hop.top/kit/go/ai/llm/anthropic"
	_ "hop.top/kit/go/ai/llm/ollama"
	_ "hop.top/kit/go/ai/llm/openai"
)

func main() {
	cli.Execute()
}
