// Package main is the npx shim for the flowtest sandbox.
// Cassette-backed: records/replays npx interactions without network access.
package main

import "hop.top/tlc/internal/flowtest/shims/shimlib"

func main() { shimlib.Run("npx") }
