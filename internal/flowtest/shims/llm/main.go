// Package main is the llm shim for the flowtest sandbox.
//go:build shimbin
package main

import "hop.top/tlc/internal/flowtest/shims/shims"

func main() { shims.Run("llm") }
