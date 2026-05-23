//go:build shimbin

// Package main is the claude shim for the flowtest sandbox.
package main

import "hop.top/tlc/internal/flowtest/shims/shims"

func main() { shims.Run("claude") }
