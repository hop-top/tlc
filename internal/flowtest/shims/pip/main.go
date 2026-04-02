// Package main is the pip shim for the flowtest sandbox.
//go:build shimbin
package main

import "hop.top/tlc/internal/flowtest/shims/shims"

func main() { shims.Run("pip") }
