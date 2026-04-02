// Package main is the npx shim for the flowtest sandbox.
package main

import "hop.top/tlc/internal/flowtest/shims/shims"

func main() { shims.Run("npx") }
